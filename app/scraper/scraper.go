package scraper

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"

	"github.com/r2k1/pgkube/app/k8s"
	"github.com/r2k1/pgkube/app/queries"
)

const resyncInterval = time.Hour

func StartScraper(ctx context.Context, queries *queries.Queries, clientSet *kubernetes.Clientset, interval time.Duration, cache *Cache) error {
	factory := informers.NewSharedInformerFactory(clientSet, resyncInterval)

	rsHandler := NewPersistObjectHandler(queries, "ReplicaSet")
	if _, err := factory.Apps().V1().ReplicaSets().Informer().AddEventHandlerWithResyncPeriod(rsHandler, resyncInterval); err != nil {
		return fmt.Errorf("adding replica set event handler: %w", err)
	}

	podHandler := NewPersistObjectHandler(queries, "Pod")
	if _, err := factory.Core().V1().Pods().Informer().AddEventHandlerWithResyncPeriod(podHandler, resyncInterval); err != nil {
		return fmt.Errorf("adding pod event handler: %w", err)
	}

	podCacheHandler := NewPodEventHandler(queries, cache)
	if _, err := factory.Core().V1().Pods().Informer().AddEventHandlerWithResyncPeriod(podCacheHandler, resyncInterval); err != nil {
		return fmt.Errorf("adding pod event handler: %w", err)
	}

	jobHandler := NewPersistObjectHandler(queries, "Job")
	if _, err := factory.Batch().V1().Jobs().Informer().AddEventHandlerWithResyncPeriod(jobHandler, resyncInterval); err != nil {
		return fmt.Errorf("adding job event handler: %w", err)
	}

	manager := NewManager(ctx)

	nodeHandler := NewNodeEventHandler(manager, k8s.NewClient(clientSet), queries, interval, cache)
	if _, err := factory.Core().V1().Nodes().Informer().AddEventHandlerWithResyncPeriod(nodeHandler, resyncInterval); err != nil {
		return fmt.Errorf("adding node event handler: %w", err)
	}

	slog.Info("starting scraper")
	factory.Start(ctx.Done())
	factory.WaitForCacheSync(ctx.Done())
	go StartGarbageCollector(ctx, queries, factory)
	return nil
}

func truncateToHour(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location())
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

func StartGarbageCollector(ctx context.Context, queries *queries.Queries, factory informers.SharedInformerFactory) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	factory.WaitForCacheSync(ctx.Done())
	// Trigger the first tick immediately
	if err := CollectGarbage(ctx, queries, factory); err != nil {
		slog.Error("cleaning up objects", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := CollectGarbage(ctx, queries, factory); err != nil {
				slog.Error("cleaning up objects", "error", err)
			}
		}
	}
}

func CollectGarbage(ctx context.Context, queries *queries.Queries, factory informers.SharedInformerFactory) error {
	pods, err := factory.Core().V1().Pods().Lister().List(labels.Everything())
	if err != nil {
		return fmt.Errorf("listing pods: %w", err)
	}
	if err := deleteObjects(ctx, queries, "Pod", pods); err != nil {
		return err
	}
	rs, err := factory.Apps().V1().ReplicaSets().Lister().List(labels.Everything())
	if err != nil {
		return fmt.Errorf("listing replica sets: %w", err)
	}
	if err := deleteObjects(ctx, queries, "ReplicaSet", rs); err != nil {
		return err
	}
	jobs, err := factory.Batch().V1().Jobs().Lister().List(labels.Everything())
	if err != nil {
		return fmt.Errorf("listing jobs: %w", err)
	}
	if err := deleteObjects(ctx, queries, "Job", jobs); err != nil {
		return err
	}
	slog.Debug("garbage collection done")
	return nil
}

func deleteObjects[T v1.Object](ctx context.Context, queries *queries.Queries, kind string, objectsToIgnore []T) error {
	uids := make([]pgtype.UUID, 0, len(objectsToIgnore))
	for _, pod := range objectsToIgnore {
		uid, err := parsePGUUID(pod.GetUID())
		if err != nil {
			slog.Error("parsing pod uuid", "error", err)
			continue
		}
		uids = append(uids, uid)
	}
	count, err := queries.DeleteObjects(ctx, kind, uids)
	if err != nil {
		return fmt.Errorf("listing pods: %w", err)
	}
	slog.Info("cleaned objects", kind, count)
	return err
}
