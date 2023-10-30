package scraper

import (
	"context"
	"fmt"
	"log/slog"

	v1 "k8s.io/api/batch/v1"

	"github.com/r2k1/pgkube/app/queries"
)

type JobEventHandler struct {
	queries *queries.Queries
}

func NewJobEventHandler(queries *queries.Queries) *JobEventHandler {
	return &JobEventHandler{
		queries: queries,
	}

}

func (h *JobEventHandler) OnAdd(obj interface{}, isInInitialList bool) {
	h.tryUpsertJob(obj)
}

func (h *JobEventHandler) OnUpdate(oldObj, newObj interface{}) {
	h.tryUpsertJob(newObj)
}

func (h *JobEventHandler) OnDelete(obj interface{}) {
	h.tryUpsertJob(obj)
}

func (h *JobEventHandler) tryUpsertJob(obj interface{}) {
	job, ok := obj.(*v1.Job)
	if !ok {
		slog.Error("upserting job", "error", fmt.Errorf("expected *v1.Job, got %T", obj))
		return
	}
	if err := h.upsertJob(job); err != nil {
		slog.Error("upserting job", "error", err)
	}
}

func (h *JobEventHandler) upsertJob(obj *v1.Job) error {
	slog.Info("upserting job", "job", obj.Name)
	uid, err := parsePGUUID(obj.UID)
	if err != nil {
		return err
	}
	controllerUid, controllerKind, controllerName := controller(obj.OwnerReferences)

	labels, err := marshalLabels(obj.Labels)
	if err != nil {
		return fmt.Errorf("majobhaling labels: %w", err)
	}

	annotations, err := marshalLabels(obj.Annotations)
	if err != nil {
		return fmt.Errorf("majobhaling annotations: %w", err)
	}

	queryParams := queries.UpsertJobParams{
		JobUid:         uid,
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

	if err := h.queries.UpsertJob(context.Background(), queryParams); err != nil {
		return fmt.Errorf("upserting job set: %w", err)
	}
	return nil
}
