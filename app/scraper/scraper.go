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
	"k8s.io/client-go/tools/cache"

	"github.com/r2k1/pgkube/app/k8s"
	"github.com/r2k1/pgkube/app/queries"
)

const resyncInterval = time.Hour
const gcInterval = time.Hour

func StartScraper(ctx context.Context, queries *queries.Queries, clientSet *kubernetes.Clientset, interval time.Duration, disableScrapingDelay bool) error {
	factory := informers.NewSharedInformerFactory(clientSet, resyncInterval)

	informers := map[string]cache.SharedInformer{
		"Pod":                   factory.Core().V1().Pods().Informer(),
		"Node":                  factory.Core().V1().Nodes().Informer(),
		"PersistentVolume":      factory.Core().V1().PersistentVolumes().Informer(),
		"PersistentVolumeClaim": factory.Core().V1().PersistentVolumeClaims().Informer(),
		"ReplicaSet":            factory.Apps().V1().ReplicaSets().Informer(),
		"Deployment":            factory.Apps().V1().Deployments().Informer(),
		"DaemonSet":             factory.Apps().V1().DaemonSets().Informer(),
		"StatefulSet":           factory.Apps().V1().StatefulSets().Informer(),
		"Job":                   factory.Batch().V1().Jobs().Informer(),
		"CronJob":               factory.Batch().V1().CronJobs().Informer(),
	}
	for kind, informer := range informers {
		eventHandler := NewPersistObjectHandler(queries, kind)
		if _, err := informer.AddEventHandlerWithResyncPeriod(eventHandler, resyncInterval); err != nil {
			return fmt.Errorf("adding %s persist event handler: %w", kind, err)
		}
	}
	manager := NewManager(ctx, disableScrapingDelay)
	cache := NewPodCacheK8s(factory.Core().V1().Pods().Lister())
	nodeScrapeHandler := NewNodeEventHandler(manager, k8s.NewClient(clientSet), queries, interval, cache)
	if _, err := factory.Core().V1().Nodes().Informer().AddEventHandlerWithResyncPeriod(nodeScrapeHandler, resyncInterval); err != nil {
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
	ticker := time.NewTicker(gcInterval)
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
	listAndDelete(ctx, "Pod", queries, factory.Core().V1().Pods().Lister().List)
	listAndDelete(ctx, "Nodes", queries, factory.Core().V1().Nodes().Lister().List)
	listAndDelete(ctx, "PersistentVolume", queries, factory.Core().V1().PersistentVolumes().Lister().List)
	listAndDelete(ctx, "PersistentVolumeClaim", queries, factory.Core().V1().PersistentVolumeClaims().Lister().List)
	listAndDelete(ctx, "ReplicaSet", queries, factory.Apps().V1().ReplicaSets().Lister().List)
	listAndDelete(ctx, "Deployment", queries, factory.Apps().V1().Deployments().Lister().List)
	listAndDelete(ctx, "Job", queries, factory.Batch().V1().Jobs().Lister().List)
	listAndDelete(ctx, "CronJob", queries, factory.Batch().V1().CronJobs().Lister().List)
	slog.Debug("garbage collection completed")
	return nil
}

func listAndDelete[T v1.Object](ctx context.Context, kind string, queries *queries.Queries, listFunc func(selector labels.Selector) (ret []T, err error)) {
	objects, err := listFunc(labels.Everything())
	if err != nil {
		slog.Error("listing objects", "error", err, "kind", kind)
		return
	}
	if err := deleteObjects(ctx, queries, kind, objects); err != nil {
		slog.Error("deleting objects", "error", err, "kind", kind)
		return
	}
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
