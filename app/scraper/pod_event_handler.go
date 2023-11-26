package scraper

import (
	"context"
	"fmt"
	"log/slog"
	"math"

	v1 "k8s.io/api/core/v1"

	"github.com/r2k1/pgkube/app/queries"
)

type PodEventHandler struct {
	queries *queries.Queries
	cache   *Cache
}

func NewPodEventHandler(queries *queries.Queries, cache *Cache) *PodEventHandler {
	return &PodEventHandler{
		queries: queries,
		cache:   cache,
	}

}

func (h *PodEventHandler) OnAdd(obj interface{}, isInInitialList bool) {
	pod := obj.(*v1.Pod)
	h.cache.StorePodUUID(pod.Namespace, pod.Name, pod.UID)
	h.tryUpsertPod(pod)
}

func (h *PodEventHandler) OnUpdate(oldObj, obj interface{}) {
	pod := obj.(*v1.Pod)
	h.cache.StorePodUUID(pod.Namespace, pod.Name, pod.UID)
	h.tryUpsertPod(pod)
}

func (h *PodEventHandler) OnDelete(obj interface{}) {
	pod := obj.(*v1.Pod)
	h.tryUpsertPod(pod)
	h.cache.CompareAndDeletePodUUID(pod.Namespace, pod.Name, pod.UID)
}

func (h *PodEventHandler) tryUpsertPod(pod *v1.Pod) {
	if err := h.upsertPod(pod); err != nil {
		slog.Error("upserting pod", "error", err)
	}
}

func (h *PodEventHandler) upsertPod(obj *v1.Pod) error {
	slog.Debug("upserting pod", "namespace", obj.Namespace, "pod", obj.Name)
	var cpuRequest, memoryRequest float64
	for _, container := range obj.Spec.Containers {
		cpuRequest += container.Resources.Requests.Cpu().AsApproximateFloat64()
		memoryRequest += container.Resources.Requests.Memory().AsApproximateFloat64()
	}
	for _, container := range obj.Spec.InitContainers {
		cpuRequest = math.Max(cpuRequest, container.Resources.Requests.Cpu().AsApproximateFloat64())
		memoryRequest = math.Max(memoryRequest, container.Resources.Requests.Memory().AsApproximateFloat64())
	}

	controllerUid, controllerKind, controllerName := controller(obj.OwnerReferences)

	queryParams := queries.UpsertPodParams{
		Object:             objectToQuery(obj.ObjectMeta),
		NodeName:           obj.Spec.NodeName,
		ControllerUid:      controllerUid,
		ControllerKind:     controllerKind,
		ControllerName:     controllerName,
		RequestCpuCores:    cpuRequest,
		RequestMemoryBytes: memoryRequest,
		StartedAt:          ptrToPGTime(obj.Status.StartTime),
	}
	if err := h.queries.UpsertPod(context.Background(), queryParams); err != nil {
		return fmt.Errorf("upserting pod: %w", err)
	}
	return nil
}
