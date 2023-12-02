package scraper

import (
	"context"
	"fmt"
	"log/slog"

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
	h.tryUpsertPod(pod)
	h.cache.StorePodUUID(pod.Namespace, pod.Name, pod.UID)
}

func (h *PodEventHandler) OnUpdate(oldObj, obj interface{}) {
	pod := obj.(*v1.Pod)
	h.tryUpsertPod(pod)
	h.cache.StorePodUUID(pod.Namespace, pod.Name, pod.UID)
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
	if err := h.queries.UpsertObject(context.Background(), podToObject(obj)); err != nil {
		return fmt.Errorf("upserting pod: %w", err)
	}
	return nil
}

func podToObject(pod *v1.Pod) queries.Object {
	return queries.Object{
		Kind:     "Pod",
		Metadata: pod.ObjectMeta,
		Spec:     pod.Spec,
		Status:   pod.Status,
	}
}
