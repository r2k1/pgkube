package scraper

import (
	v1 "k8s.io/api/core/v1"

	"github.com/r2k1/pgkube/app/queries"
)

type PodCache struct {
	queries *queries.Queries
	cache   *Cache
}

func NewPodEventHandler(queries *queries.Queries, cache *Cache) *PodCache {
	return &PodCache{
		queries: queries,
		cache:   cache,
	}
}

func (h *PodCache) OnAdd(obj interface{}, isInInitialList bool) {
	pod := obj.(*v1.Pod)
	h.cache.StorePodUUID(pod.Namespace, pod.Name, pod.UID)
}

func (h *PodCache) OnUpdate(oldObj, obj interface{}) {
	pod := obj.(*v1.Pod)
	h.cache.StorePodUUID(pod.Namespace, pod.Name, pod.UID)
}

func (h *PodCache) OnDelete(obj interface{}) {
	pod := obj.(*v1.Pod)
	h.cache.CompareAndDeletePodUUID(pod.Namespace, pod.Name, pod.UID)
}
