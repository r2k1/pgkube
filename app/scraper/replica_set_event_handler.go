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
	h.tryUpsertReplicaSet(obj)
}

func (h *ReplicaSetEventHandler) tryUpsertReplicaSet(obj interface{}) {
	rs, ok := obj.(*v1.ReplicaSet)
	if !ok {
		slog.Error("upserting replica set", "error", fmt.Errorf("expected *v1.ReplicaSet, got %T", obj))
		return
	}
	if err := h.upsertReplicaSet(rs); err != nil {
		slog.Error("upserting replica set", "error", err)
	}
}

func (h *ReplicaSetEventHandler) upsertReplicaSet(obj *v1.ReplicaSet) error {
	slog.Debug("upserting replica set", "namespace", obj.Namespace, "replica_set", obj.Name)
	controllerUid, controllerKind, controllerName := controller(obj.OwnerReferences)

	queryParams := queries.UpsertReplicaSetParams{
		Object:         objectToQuery(obj.ObjectMeta),
		ControllerKind: controllerKind,
		ControllerName: controllerName,
		ControllerUid:  controllerUid,
	}

	if err := h.queries.UpsertReplicaSet(context.Background(), queryParams); err != nil {
		return fmt.Errorf("upserting replica set: %w", err)
	}
	return nil
}
