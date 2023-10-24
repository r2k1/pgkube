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
	slog.Info("upserting replica set", "replica_set", obj.Name)
	uid, err := parsePGUUID(obj.UID)
	if err != nil {
		return err
	}
	controllerUid, controllerKind, controllerName := controller(obj.OwnerReferences)

	labels, err := marshalLabels(obj.Labels)
	if err != nil {
		return fmt.Errorf("marshaling labels: %w", err)
	}

	annotations, err := marshalLabels(obj.Annotations)
	if err != nil {
		return fmt.Errorf("marshaling annotations: %w", err)
	}

	queryParams := queries.UpsertReplicaSetParams{
		ReplicaSetUid:  uid,
		Namespace:      obj.Namespace,
		Name:           obj.Name,
		ControllerKind: controllerKind,
		ControllerName: controllerName,
		ControllerUid:  controllerUid,
		CreatedAt:      toPGTime(obj.CreationTimestamp),
		DeletedAt:      ptrToPGTime(obj.DeletionTimestamp),
		Labels:         labels,
		Annotations:    annotations,
	}

	if err := h.queries.UpsertReplicaSet(context.Background(), queryParams); err != nil {
		return fmt.Errorf("upserting replica set: %w", err)
	}
	return nil
}
