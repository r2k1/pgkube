package scraper

import (
	"context"
	"fmt"
	"log/slog"

	v1 "k8s.io/api/apps/v1"

	"github.com/r2k1/pgkube/app/queries"
)

type ReplicaSetEventHandler struct {
	queries *queries.Queries
}

func NewReplicaSetEventHandler(queries *queries.Queries) *ReplicaSetEventHandler {
	return &ReplicaSetEventHandler{
		queries: queries,
	}

}

func (h *ReplicaSetEventHandler) OnAdd(obj interface{}, isInInitialList bool) {
	h.tryUpsertReplicaSet(obj)
}

func (h *ReplicaSetEventHandler) OnUpdate(oldObj, newObj interface{}) {
	h.tryUpsertReplicaSet(newObj)
}

func (h *ReplicaSetEventHandler) OnDelete(obj interface{}) {
	rs, ok := obj.(*v1.ReplicaSet)
	if !ok {
		slog.Error("deleting ReplicaSet", "error", fmt.Errorf("expected *v1.ReplicaSet, got %T", obj))
		return
	}
	if err := h.queries.DeleteObject(context.Background(), string(rs.GetUID())); err != nil {
		slog.Error("deleting ReplicaSet", "error", err)
	}
	h.tryUpsertReplicaSet(obj)
}

func (h *ReplicaSetEventHandler) tryUpsertReplicaSet(obj interface{}) {
	rs, ok := obj.(*v1.ReplicaSet)
	if !ok {
		slog.Error("upserting replica set", "error", fmt.Errorf("expected *v1.ReplicaSet, got %T", obj))
		return
	}
	if err := h.queries.UpsertObject(context.Background(), replicaSetToObject(rs)); err != nil {
		slog.Error("upserting replica set", "error", err)
	}
}

func replicaSetToObject(rs *v1.ReplicaSet) queries.Object {
	return queries.Object{
		Kind:     "ReplicaSet",
		Metadata: rs.ObjectMeta,
		Spec:     rs.Spec,
		Status:   rs.Status,
	}
}
