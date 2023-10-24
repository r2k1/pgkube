package scraper

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/prometheus/common/expfmt"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/r2k1/pgkube/app/queries"
)

type podKey struct {
	Name      string
	Namespace string
}

type metricValue struct {
	Value       float64
	TimestampMs int64
}

type nodeMetrics struct {
	PodCPUUsageSecondsTotal  map[podKey]metricValue
	PodMemoryWorkingSetBytes map[podKey]metricValue
}

type NodeScraper struct {
	nodeName            string
	k8sClients          kubernetes.Interface
	queries             *queries.Queries
	prevCPUSecondsTotal map[podKey]metricValue
	prevCores           map[podKey]metricValue
	mutex               sync.Mutex
}

func NewNodeScrapper(name string, k8sClients kubernetes.Interface, queries *queries.Queries) *NodeScraper {
	return &NodeScraper{
		nodeName:            name,
		k8sClients:          k8sClients,
		queries:             queries,
		prevCPUSecondsTotal: make(map[podKey]metricValue),
		prevCores:           make(map[podKey]metricValue),
		mutex:               sync.Mutex{},
	}
}

func (s *NodeScraper) Scrape(ctx context.Context) error {
	metrics, err := s.nodeMetrics(ctx)
	if err != nil {
		return err
	}
	cpuData := s.cpuData(metrics.PodCPUUsageSecondsTotal)
	if len(cpuData) > 0 {
		if err := s.queries.UpsertPodUsedCPU(ctx, cpuData).Close(); err != nil {
			return fmt.Errorf("upserting pod used cpu: %w", err)
		}
		slog.Info("updated pod CPU usage", "node", s.nodeName, "count", len(cpuData))
	}

	memoryData := s.memoryData(metrics.PodMemoryWorkingSetBytes)
	if len(memoryData) > 0 {
		if err := s.queries.UpsertPodUsedMemory(ctx, memoryData).Close(); err != nil {
			return fmt.Errorf("upserting pod used memory: %w", err)
		}
		slog.Info("updated pod memory usage", "node", s.nodeName, "count", len(memoryData))
	}

	return nil
}

func (s *NodeScraper) nodeMetrics(ctx context.Context) (nodeMetrics, error) {
	body, err := s.k8sClients.CoreV1().RESTClient().Get().
		Resource("nodes").Name(s.nodeName).SubResource("proxy").
		Suffix("metrics/resource").DoRaw(ctx)
	if err != nil {
		return nodeMetrics{}, fmt.Errorf("getting node metrics: %w", err)
	}
	parser := &expfmt.TextParser{}
	metrics, err := parser.TextToMetricFamilies(bytes.NewReader(body))
	if err != nil {
		return nodeMetrics{}, fmt.Errorf("parsing node metrics: %w", err)
	}
	result := nodeMetrics{
		PodCPUUsageSecondsTotal:  make(map[podKey]metricValue),
		PodMemoryWorkingSetBytes: make(map[podKey]metricValue),
	}
	if metric, ok := metrics["pod_cpu_usage_seconds_total"]; ok {
		for _, m := range metric.GetMetric() {
			result.PodCPUUsageSecondsTotal[podKey{
				Name:      getLabel(m.GetLabel(), "pod"),
				Namespace: getLabel(m.GetLabel(), "namespace"),
			}] = metricValue{
				Value:       m.GetCounter().GetValue(),
				TimestampMs: m.GetTimestampMs(),
			}
		}
	}
	if metric, ok := metrics["pod_memory_working_set_bytes"]; ok {
		for _, m := range metric.GetMetric() {
			result.PodMemoryWorkingSetBytes[podKey{
				Name:      getLabel(m.GetLabel(), "pod"),
				Namespace: getLabel(m.GetLabel(), "namespace"),
			}] = metricValue{
				Value:       m.GetGauge().GetValue(),
				TimestampMs: m.GetTimestampMs(),
			}
		}
	}
	return result, nil
}

func (s *NodeScraper) cpuData(currentCPUSecondsTotal map[podKey]metricValue) []queries.UpsertPodUsedCPUParams {
	// cpu usage is reported in total seconds consumed by the pod
	// in order to calculate avg core/sec we need to calculate the difference between current and previous value
	// cpu usage is calculated as (current - previous) / (current timestamp - previous timestamp)
	s.mutex.Lock()
	defer s.mutex.Unlock()
	podCores := make(map[podKey]metricValue, len(currentCPUSecondsTotal))
	for key, value := range currentCPUSecondsTotal {
		prevValue, ok := s.prevCPUSecondsTotal[key]
		if !ok {
			continue
		}
		var cores float64
		if prevValue.TimestampMs != value.TimestampMs {
			cores = (value.Value - prevValue.Value) / float64((value.TimestampMs-prevValue.TimestampMs)/1000)
			podCores[key] = metricValue{
				Value:       cores,
				TimestampMs: value.TimestampMs,
			}
		} else {
			// if previous timestamp is the same as current it means metrics server hasn't updated the usage yet
			// so we don't know the current usage yet, use previous value
			value, ok = s.prevCores[key]
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
			NodeName:    s.nodeName,
			CpuCoresMax: value.Value,
		})
	}
	s.prevCPUSecondsTotal = currentCPUSecondsTotal
	s.prevCores = podCores
	return result
}

func (s *NodeScraper) memoryData(currentPodMemoryUsed map[podKey]metricValue) []queries.UpsertPodUsedMemoryParams {
	result := make([]queries.UpsertPodUsedMemoryParams, 0, len(currentPodMemoryUsed))
	for key, value := range currentPodMemoryUsed {
		result = append(result, queries.UpsertPodUsedMemoryParams{
			Timestamp: pgtype.Timestamptz{
				Time:  truncateToHour(time.UnixMilli(value.TimestampMs)).UTC(),
				Valid: true,
			},
			Namespace:      key.Namespace,
			Name:           key.Name,
			NodeName:       s.nodeName,
			MemoryBytesMax: value.Value,
		})
	}
	return result
}

type NodeEventHandler struct {
	manager    *Manager
	k8sClients *kubernetes.Clientset
	queries    *queries.Queries
	interval   time.Duration
}

func NewNodeEventHandler(manager *Manager, k8sClients *kubernetes.Clientset, queries *queries.Queries, interval time.Duration) *NodeEventHandler {
	return &NodeEventHandler{
		manager:    manager,
		k8sClients: k8sClients,
		queries:    queries,
		interval:   interval,
	}
}

func (h *NodeEventHandler) OnAdd(obj interface{}, isInInitialList bool) {
	node, ok := obj.(*v1.Node)
	if !ok {
		slog.Error("adding node", "error", fmt.Errorf("expected *v1.Node, got %T", obj))
		return
	}
	if node.Name == "" {
		slog.Error("node name is empty")
		return
	}
	nodeScraper := NewNodeScrapper(node.Name, h.k8sClients, h.queries)
	targetID := "node/" + node.Name
	h.manager.AddTarget(targetID, nodeScraper.Scrape, h.interval)
}

func (h *NodeEventHandler) OnUpdate(oldObj, obj interface{}) {}

func (h *NodeEventHandler) OnDelete(obj interface{}) {
	node, ok := obj.(*v1.Node)
	if !ok {
		slog.Error("removing node", "error", fmt.Errorf("expected *v1.Node, got %T", obj))
		return
	}
	if node.Name == "" {
		slog.Error("node name is empty")
		return
	}
	targetID := "node/" + node.Name
	h.manager.RemoveTarget(targetID)
}
