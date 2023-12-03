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
	job, ok := obj.(*v1.Job)
	if !ok {
		slog.Error("deleting job", "error", fmt.Errorf("expected *v1.Job, got %T", obj))
		return
	}
	if err := h.queries.DeleteObject(context.Background(), string(job.GetUID())); err != nil {
		slog.Error("deleting job", "error", err)
	}
}

func (h *JobEventHandler) tryUpsertJob(obj interface{}) {
	job, ok := obj.(*v1.Job)
	if !ok {
		slog.Error("upserting job", "error", fmt.Errorf("expected *v1.Job, got %T", obj))
		return
	}
	if err := h.queries.UpsertObject(context.Background(), jobToObject(job)); err != nil {
		slog.Error("upserting job", "error", err)
	}
}

func jobToObject(job *v1.Job) queries.Object {
	return queries.Object{
		Kind:     "Job",
		Metadata: job.ObjectMeta,
		Spec:     job.Spec,
		Status:   job.Status,
	}
}
