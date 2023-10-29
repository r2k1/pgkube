package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"

	"github.com/r2k1/pgkube/app/k8s"
	"github.com/r2k1/pgkube/app/queries"
)

const resyncInterval = time.Hour

func StartScraper(ctx context.Context, psql *pgxpool.Pool, clientSet *kubernetes.Clientset, interval time.Duration) error {
	factory := informers.NewSharedInformerFactory(clientSet, resyncInterval)
	queries := queries.New(psql)

	rsHandler := NewReplicaSetEventHandler(queries)
	if _, err := factory.Apps().V1().ReplicaSets().Informer().AddEventHandlerWithResyncPeriod(rsHandler, resyncInterval); err != nil {
		return fmt.Errorf("adding replica set event handler: %w", err)
	}

	podHandler := NewPodEventHandler(queries)
	if _, err := factory.Core().V1().Pods().Informer().AddEventHandlerWithResyncPeriod(podHandler, resyncInterval); err != nil {
		return fmt.Errorf("adding pod event handler: %w", err)
	}

	jobHandler := NewJobEventHandler(queries)
	if _, err := factory.Batch().V1().Jobs().Informer().AddEventHandlerWithResyncPeriod(jobHandler, resyncInterval); err != nil {
		return fmt.Errorf("adding job event handler: %w", err)
	}

	manager := NewManager(ctx)

	nodeHandler := NewNodeEventHandler(manager, k8s.NewClient(clientSet), queries, interval)
	if _, err := factory.Core().V1().Nodes().Informer().AddEventHandlerWithResyncPeriod(nodeHandler, resyncInterval); err != nil {
		return fmt.Errorf("adding node event handler: %w", err)
	}

	factory.Start(ctx.Done())
	factory.WaitForCacheSync(ctx.Done())
	return nil
}

func truncateToHour(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location())
}

func marshalLabels(label map[string]string) ([]byte, error) {
	if len(label) == 0 {
		return []byte("{}"), nil
	}
	data, err := json.Marshal(label)
	if err != nil {
		return nil, fmt.Errorf("marshalling labels: %w", err)
	}
	return data, nil
}

func WithTransaction(ctx context.Context, conn *pgx.Conn, f func(tx pgx.Tx) error) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	err = f(tx)
	if err != nil {
		rollbackErr := tx.Rollback(ctx)
		if rollbackErr != nil {
			slog.ErrorContext(ctx, "rolling back transaction", "error", rollbackErr)
		}
		return fmt.Errorf("executing transaction: %w", err)
	}
	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}
