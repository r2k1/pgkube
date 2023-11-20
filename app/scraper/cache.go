package scraper

import (
	"sync"

	"k8s.io/apimachinery/pkg/types"
)

// Cache is a thread-safe cache for pod UUIDs.
// TODO: is it actually saving memory?
type Cache struct {
	podNamespaceNameToUUID *sync.Map
}

func NewCache() *Cache {
	return &Cache{
		podNamespaceNameToUUID: &sync.Map{},
	}
}

type podKey struct {
	namespace string
	name      string
}

func (c *Cache) LoadPodUID(namespace string, name string) (uuid types.UID, ok bool) {
	val, ok := c.podNamespaceNameToUUID.Load(podKey{namespace: namespace, name: name})
	if !ok {
		return "", false
	}
	return val.(types.UID), ok
}

func (c *Cache) StorePodUUID(namespace string, name string, uuid types.UID) {
	c.podNamespaceNameToUUID.Store(podKey{namespace: namespace, name: name}, uuid)
}

func (c *Cache) CompareAndDeletePodUUID(namespace string, name string, uuid types.UID) {
	c.podNamespaceNameToUUID.CompareAndDelete(podKey{namespace: namespace, name: name}, uuid)
}
