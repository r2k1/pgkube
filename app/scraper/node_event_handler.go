package scraper

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	v1 "k8s.io/api/core/v1"

	"github.com/r2k1/pgkube/app/k8s"
	"github.com/r2k1/pgkube/app/queries"
)

type NodeScraper struct {
	nodeName            string
	k8sClients          k8s.ClientInterface
	queries             *queries.Queries
	prevCPUSecondsTotal k8s.PodMetric
	prevCores           k8s.PodMetric
	mutex               sync.Mutex
	cache               *Cache
}

func NewNodeScrapper(name string, k8sClients k8s.ClientInterface, queries *queries.Queries, cache *Cache) *NodeScraper {
	return &NodeScraper{
		nodeName:            name,
		k8sClients:          k8sClients,
		queries:             queries,
		prevCPUSecondsTotal: make(k8s.PodMetric),
		prevCores:           make(k8s.PodMetric),
		mutex:               sync.Mutex{},
		cache:               cache,
	}
}

func (s *NodeScraper) Scrape(ctx context.Context) error {
	metrics, err := s.k8sClients.NodeMetrics(ctx, s.nodeName)
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

func (s *NodeScraper) cpuData(currentCPUSecondsTotal k8s.PodMetric) []queries.UpsertPodUsedCPUParams {
	// cpu usage is reported in total seconds consumed by the pod
	// in order to calculate avg core/sec we need to calculate the difference between current and previous value
	// cpu usage is calculated as (current - previous) / (current timestamp - previous timestamp)
	s.mutex.Lock()
	defer s.mutex.Unlock()
	podCores := make(k8s.PodMetric, len(currentCPUSecondsTotal))
	for key, value := range currentCPUSecondsTotal {
		prevValue, ok := s.prevCPUSecondsTotal[key]
		if !ok {
			continue
		}
		var cores float64
		if prevValue.TimestampMs != value.TimestampMs {
			cores = (value.Value - prevValue.Value) / float64((value.TimestampMs-prevValue.TimestampMs)/1000)
			podCores[key] = k8s.MetricValue{
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
		uid, ok := s.cache.LoadPodUID(key.Namespace, key.Name)
		if !ok {
			slog.Error("pod uid not found", "namespace", key.Namespace, "name", key.Name)
			continue
		}
		pgUUID, err := parsePGUUID(uid)
		if err != nil {
			slog.Error("parsing uuid", "error", err)
			continue
		}

		result = append(result, queries.UpsertPodUsedCPUParams{
			Timestamp: pgtype.Timestamptz{
				Time:  truncateToHour(time.UnixMilli(value.TimestampMs)).UTC(),
				Valid: true,
			},
			PodUid:      pgUUID,
			CpuCoresMax: value.Value,
		})
	}
	s.prevCPUSecondsTotal = currentCPUSecondsTotal
	s.prevCores = podCores
	return result
}

func (s *NodeScraper) memoryData(currentPodMemoryUsed k8s.PodMetric) []queries.UpsertPodUsedMemoryParams {
	result := make([]queries.UpsertPodUsedMemoryParams, 0, len(currentPodMemoryUsed))
	for key, value := range currentPodMemoryUsed {
		uid, ok := s.cache.LoadPodUID(key.Namespace, key.Name)
		if !ok {
			slog.Error("pod uid not found", "namespace", key.Namespace, "name", key.Name)
			continue
		}
		pgUUID, err := parsePGUUID(uid)
		if err != nil {
			slog.Error("parsing uuid", "error", err)
			continue
		}
		result = append(result, queries.UpsertPodUsedMemoryParams{
			Timestamp: pgtype.Timestamptz{
				Time:  truncateToHour(time.UnixMilli(value.TimestampMs)).UTC(),
				Valid: true,
			},
			PodUid:         pgUUID,
			MemoryBytesMax: value.Value,
		})
	}
	return result
}

type NodeEventHandler struct {
	manager   *Manager
	k8sClient k8s.ClientInterface
	queries   *queries.Queries
	interval  time.Duration
	cache     *Cache
}

func NewNodeEventHandler(
	manager *Manager,
	k8sClient k8s.ClientInterface,
	queries *queries.Queries,
	interval time.Duration,
	cache *Cache,
) *NodeEventHandler {
	return &NodeEventHandler{
		manager:   manager,
		k8sClient: k8sClient,
		queries:   queries,
		interval:  interval,
		cache:     cache,
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
	nodeScraper := NewNodeScrapper(node.Name, h.k8sClient, h.queries, h.cache)
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
