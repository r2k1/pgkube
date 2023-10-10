package scraper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/listers/core/v1"

	"github.com/r2k1/pgkube/app/queries"
)

// TODO: make tread-safe
// TODO: make testable
// TODO: delete node data when node is deleted
type Scraper struct {
	k8sClients          *kubernetes.Clientset
	psql                *pgxpool.Pool
	queries             *queries.Queries
	nodeLister          v1.NodeLister
	prevCPUSecondsTotal map[string]map[PodKey]Value
	prevCores           map[string]map[PodKey]Value
	interval            time.Duration
}

type PodKey struct {
	Name      string
	Namespace string
	NodeName  string
}

type PodMetrics struct {
	PodCPUUsageSecondsTotal  Value
	PodMemoryWorkingSetBytes Value
}

type Value struct {
	Value       float64
	TimestampMs int64
}

type NodeMetrics struct {
	PodCPUUsageSecondsTotal  map[PodKey]Value
	PodMemoryWorkingSetBytes map[PodKey]Value
}

const resyncInterval = time.Hour

func NewScraper(ctx context.Context, psql *pgxpool.Pool, clientSet *kubernetes.Clientset, interval time.Duration) (*Scraper, error) {
	scraper := &Scraper{
		k8sClients:          clientSet,
		psql:                psql,
		queries:             queries.New(psql),
		prevCPUSecondsTotal: make(map[string]map[PodKey]Value),
		prevCores:           make(map[string]map[PodKey]Value),
		interval:            interval,
	}
	factory := informers.NewSharedInformerFactory(scraper.k8sClients, resyncInterval)
	scraper.nodeLister = factory.Core().V1().Nodes().Lister()

	rsHandler := NewReplicaSetEventHandler(scraper.queries)
	if _, err := factory.Apps().V1().ReplicaSets().Informer().AddEventHandlerWithResyncPeriod(rsHandler, resyncInterval); err != nil {
		return nil, fmt.Errorf("adding replica set event handler: %w", err)
	}

	podHandler := NewPodEventHandler(scraper.queries)
	if _, err := factory.Core().V1().Pods().Informer().AddEventHandlerWithResyncPeriod(podHandler, resyncInterval); err != nil {
		return nil, fmt.Errorf("adding pod event handler: %w", err)
	}

	jobHandler := NewJobEventHandler(scraper.queries)
	if _, err := factory.Batch().V1().Jobs().Informer().AddEventHandlerWithResyncPeriod(jobHandler, resyncInterval); err != nil {
		return nil, fmt.Errorf("adding job event handler: %w", err)
	}

	factory.Start(ctx.Done())
	factory.WaitForCacheSync(ctx.Done())
	return scraper, nil
}

func (s *Scraper) Start(ctx context.Context) {
	err := s.Scrape(ctx)
	if err != nil {
		slog.Error("scraping nodes", "error", err)
	}
	t := time.NewTicker(s.interval)
	defer t.Stop()
	slog.Info("starting scraper")
	for range t.C {
		err := s.Scrape(ctx)
		if err != nil {
			slog.Error("exporting", "error", err)
		}
	}
}

func (s *Scraper) Scrape(ctx context.Context) error {
	nodes, err := s.nodeLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("listing nodes: %w", err)
	}
	for _, node := range nodes {
		err := s.ScrapeNode(ctx, node.Name)
		if err != nil {
			slog.Error("scraping node", "node", node.Name, "error", err)
		}
	}
	return nil
}

func (s *Scraper) ScrapeNode(ctx context.Context, nodeName string) error {
	metrics, err := s.nodeMetrics(ctx, nodeName)
	if err != nil {
		return err
	}
	cpuData := s.cpuData(nodeName, metrics.PodCPUUsageSecondsTotal)
	if len(cpuData) > 0 {
		if err := s.queries.UpsertPodUsedCPU(ctx, cpuData).Close(); err != nil {
			return fmt.Errorf("upserting pod used cpu: %w", err)
		}
		slog.Info("updated pod CPU usage", "node", nodeName, "count", len(cpuData))
	}

	memoryData := s.memoryData(metrics.PodMemoryWorkingSetBytes)
	if len(memoryData) > 0 {
		if err := s.queries.UpsertPodUsedMemory(ctx, memoryData).Close(); err != nil {
			return fmt.Errorf("upserting pod used memory: %w", err)
		}
		slog.Info("updated pod memory usage", "node", nodeName, "count", len(memoryData))
	}

	return nil
}

func (s *Scraper) cpuData(nodeName string, currentCPUSecondsTotal map[PodKey]Value) []queries.UpsertPodUsedCPUParams {
	podCores := make(map[PodKey]Value, len(currentCPUSecondsTotal))
	for key, value := range currentCPUSecondsTotal {
		prevValue, ok := s.prevCPUSecondsTotal[nodeName][key]
		if !ok {
			continue
		}
		var cores float64
		if prevValue.TimestampMs != value.TimestampMs {
			cores = (value.Value - prevValue.Value) / float64((value.TimestampMs-prevValue.TimestampMs)/1000)
			podCores[key] = Value{
				Value:       cores,
				TimestampMs: value.TimestampMs,
			}
		} else {
			// if previous timestamp is the same as current it means metrics server hasn't updated the usage yet
			// so we don't know the current usage yet, use previous value
			value, ok = s.prevCores[nodeName][key]
			if !ok {
				continue
			}
			podCores[key] = value
		}
	}

	result := make([]queries.UpsertPodUsedCPUParams, 0, len(podCores))
	for key, value := range podCores {
		result = append(result, queries.UpsertPodUsedCPUParams{
			Timestamp: pgtype.Timestamptz{
				Time:  truncateToHour(time.UnixMilli(value.TimestampMs)).UTC(),
				Valid: true,
			},
			Namespace:   key.Namespace,
			Name:        key.Name,
			NodeName:    key.NodeName,
			CpuCoresMax: value.Value,
		})
	}
	s.prevCPUSecondsTotal[nodeName] = currentCPUSecondsTotal
	s.prevCores[nodeName] = podCores
	return result
}

func (s *Scraper) memoryData(currentPodMemoryUsed map[PodKey]Value) []queries.UpsertPodUsedMemoryParams {
	result := make([]queries.UpsertPodUsedMemoryParams, 0, len(currentPodMemoryUsed))
	for key, value := range currentPodMemoryUsed {
		result = append(result, queries.UpsertPodUsedMemoryParams{
			Timestamp: pgtype.Timestamptz{
				Time:  truncateToHour(time.UnixMilli(value.TimestampMs)).UTC(),
				Valid: true,
			},
			Namespace:      key.Namespace,
			Name:           key.Name,
			NodeName:       key.NodeName,
			MemoryBytesMax: value.Value,
		})
	}
	return result
}

func (s *Scraper) nodeMetrics(ctx context.Context, nodeName string) (NodeMetrics, error) {
	body, err := s.k8sClients.CoreV1().RESTClient().Get().
		Resource("nodes").Name(nodeName).SubResource("proxy").
		Suffix("metrics/resource").DoRaw(ctx)
	if err != nil {
		return NodeMetrics{}, fmt.Errorf("getting node metrics: %w", err)
	}
	parser := &expfmt.TextParser{}
	metrics, err := parser.TextToMetricFamilies(bytes.NewReader(body))
	if err != nil {
		return NodeMetrics{}, fmt.Errorf("parsing node metrics: %w", err)
	}
	result := NodeMetrics{
		PodCPUUsageSecondsTotal:  make(map[PodKey]Value),
		PodMemoryWorkingSetBytes: make(map[PodKey]Value),
	}
	if metric, ok := metrics["pod_cpu_usage_seconds_total"]; ok {
		for _, m := range metric.Metric {
			result.PodCPUUsageSecondsTotal[PodKey{
				Name:      getLabel(m.Label, "pod"),
				Namespace: getLabel(m.Label, "namespace"),
				NodeName:  nodeName,
			}] = Value{
				Value:       *m.Counter.Value,
				TimestampMs: *m.TimestampMs,
			}
		}
	}
	if metric, ok := metrics["pod_memory_working_set_bytes"]; ok {
		for _, m := range metric.Metric {
			result.PodMemoryWorkingSetBytes[PodKey{
				Name:      getLabel(m.Label, "pod"),
				Namespace: getLabel(m.Label, "namespace"),
				NodeName:  nodeName,
			}] = Value{
				Value:       *m.Gauge.Value,
				TimestampMs: *m.TimestampMs,
			}
		}
	}
	return result, nil
}

func truncateToHour(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location())
}

func getLabel(labels []*dto.LabelPair, name string) string {
	for _, label := range labels {
		if label.GetName() == name {
			return label.GetValue()
		}
	}
	return ""
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
