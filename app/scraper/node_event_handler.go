package scraper

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	v1 "k8s.io/api/core/v1"

	listerv1 "k8s.io/client-go/listers/core/v1"

	"github.com/r2k1/pgkube/app/k8s"
	"github.com/r2k1/pgkube/app/queries"
)

// type alias for mock generation
type PodLister listerv1.PodLister
type PodNamespaceLister listerv1.PodNamespaceLister

type PodCache interface {
	Get(namespace, name string) (*v1.Pod, error)
}

// PodCacheK8s is a wrapper around listerv1.PodLister
// original client is hard to mock
// wrapper provides an easier to use interface
type PodCacheK8s struct {
	lister listerv1.PodLister
}

func NewPodCacheK8s(lister listerv1.PodLister) *PodCacheK8s {
	return &PodCacheK8s{
		lister: lister,
	}
}

func (p *PodCacheK8s) Get(namespace, name string) (*v1.Pod, error) {
	pod, err := p.lister.Pods(namespace).Get(name)
	if err != nil {
		return nil, fmt.Errorf("getting pod from cache: %w", err)
	}
	return pod, nil
}

type NodeScraper struct {
	nodeName            string
	k8sClients          k8s.ClientInterface
	queries             *queries.Queries
	prevCPUSecondsTotal k8s.PodMetric
	prevCores           k8s.PodMetric
	mutex               sync.Mutex
	cache               PodCache
}

func NewNodeScrapper(name string, k8sClients k8s.ClientInterface, queries *queries.Queries, cache PodCache) *NodeScraper {
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
		if err := s.queries.UpsertPodUsedCPU(ctx, cpuData); err != nil {
			return fmt.Errorf("upserting pod used cpu: %w", err)
		}
		slog.Debug("updated pod CPU usage", "node", s.nodeName, "count", len(cpuData))
	}

	memoryData := s.memoryData(metrics.PodMemoryWorkingSetBytes)
	if len(memoryData) > 0 {
		if err := s.queries.UpsertPodUsedMemory(ctx, memoryData); err != nil {
			return fmt.Errorf("upserting pod used memory: %w", err)
		}
		slog.Debug("updated pod memory usage", "node", s.nodeName, "count", len(memoryData))
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
		pod, err := s.cache.Get(key.Namespace, key.Name)
		if err != nil {
			slog.Error("could not find pod in cache", "namespace", key.Namespace, "name", key.Name)
			continue
		}

		pgUUID, err := parsePGUUID(pod.UID)
		if err != nil {
			slog.Error("parsing uuid", "error", err)
			continue
		}

		result = append(result, queries.UpsertPodUsedCPUParams{
			Timestamp: pgtype.Timestamptz{
				Time:  truncateToHour(time.UnixMilli(value.TimestampMs)).UTC(),
				Valid: true,
			},
			PodUid:   pgUUID,
			CpuCores: value.Value,
		})
	}
	s.prevCPUSecondsTotal = currentCPUSecondsTotal
	s.prevCores = podCores
	return result
}

func (s *NodeScraper) memoryData(currentPodMemoryUsed k8s.PodMetric) []queries.UpsertPodUsedMemoryParams {
	result := make([]queries.UpsertPodUsedMemoryParams, 0, len(currentPodMemoryUsed))
	for key, value := range currentPodMemoryUsed {
		pod, err := s.cache.Get(key.Namespace, key.Name)
		if err != nil {
			slog.Error("could not find pod in cache", "namespace", key.Namespace, "name", key.Name)
			continue
		}
		pgUUID, err := parsePGUUID(pod.UID)
		if err != nil {
			slog.Error("parsing uuid", "error", err)
			continue
		}
		result = append(result, queries.UpsertPodUsedMemoryParams{
			Timestamp: pgtype.Timestamptz{
				Time:  truncateToHour(time.UnixMilli(value.TimestampMs)).UTC(),
				Valid: true,
			},
			PodUid:      pgUUID,
			MemoryBytes: value.Value,
		})
	}
	return result
}

type NodeEventHandler struct {
	manager   *Manager
	k8sClient k8s.ClientInterface
	queries   *queries.Queries
	interval  time.Duration
	cache     PodCache
}

func NewNodeEventHandler(
	manager *Manager,
	k8sClient k8s.ClientInterface,
	queries *queries.Queries,
	interval time.Duration,
	cache PodCache,
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
