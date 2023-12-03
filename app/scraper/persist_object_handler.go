package scraper

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"k8s.io/apimachinery/pkg/types"

	"github.com/r2k1/pgkube/app/queries"
)

type PersistObjectHandler struct {
	queries *queries.Queries
	kind    string
}

func NewPersistObjectHandler(queries *queries.Queries, kind string) *PersistObjectHandler {
	return &PersistObjectHandler{
		queries: queries,
		kind:    kind,
	}
}

func (h *PersistObjectHandler) OnAdd(obj interface{}, isInInitialList bool) {
	h.Upsert(obj)
}

func (h *PersistObjectHandler) OnUpdate(oldObj, newObj interface{}) {
	h.Upsert(newObj)
}

func (h *PersistObjectHandler) OnDelete(obj interface{}) {
	type UidGetter interface {
		GetUID() types.UID
	}
	uidGetter, ok := obj.(UidGetter)
	if !ok {
		slog.Error("deleting object", "error", fmt.Errorf("unexpected object type: %T", obj))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.queries.DeleteObject(ctx, string(uidGetter.GetUID())); err != nil {
		slog.Error("deleting object", "error", err)
	}
}

func (h *PersistObjectHandler) Upsert(obj interface{}) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.queries.UpsertObject(ctx, h.kind, obj); err != nil {
		slog.Error("upserting object", "error", err)
		return
	}
}
