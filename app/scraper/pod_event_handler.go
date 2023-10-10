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
}

func NewPodEventHandler(queries *queries.Queries) *PodEventHandler {
	return &PodEventHandler{
		queries: queries,
	}

}

func (p *PodEventHandler) OnAdd(obj interface{}, isInInitialList bool) {
	p.tryUpsertPod(obj)
}

func (p *PodEventHandler) OnUpdate(oldObj, obj interface{}) {
	p.tryUpsertPod(obj)

}

func (p *PodEventHandler) OnDelete(obj interface{}) {
	p.tryUpsertPod(obj)

}

func (p *PodEventHandler) tryUpsertPod(obj interface{}) {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		slog.Error("upserting pod", "error", fmt.Errorf("expected *v1.Pod, got %T", obj))
		return
	}
	if err := p.upsertPod(pod); err != nil {
		slog.Error("upserting pod", "error", err)
	}
}

func (p *PodEventHandler) upsertPod(obj *v1.Pod) error {
	var cpuRequest, memoryRequest float64
	for _, container := range obj.Spec.Containers {
		cpuRequest += container.Resources.Requests.Cpu().AsApproximateFloat64()
		memoryRequest += container.Resources.Requests.Memory().AsApproximateFloat64()
	}
	for _, container := range obj.Spec.InitContainers {
		cpuRequest = math.Max(cpuRequest, container.Resources.Requests.Cpu().AsApproximateFloat64())
		memoryRequest = math.Max(memoryRequest, container.Resources.Requests.Memory().AsApproximateFloat64())
	}
	uid, err := parsePGUUID(obj.UID)
	if err != nil {
		return err
	}
	labels, err := marshalLabels(obj.Labels)
	if err != nil {
		return fmt.Errorf("marshalling labels: %w", err)
	}
	annotations, err := marshalLabels(obj.Annotations)
	if err != nil {
		return fmt.Errorf("marshalling labels: %w", err)
	}

	controllerUid, controllerKind, controllerName := controller(obj.OwnerReferences)

	queryParams := queries.UpsertPodParams{
		PodUid:             uid,
		Name:               obj.Name,
		Namespace:          obj.Namespace,
		Labels:             labels,
		Annotations:        annotations,
		NodeName:           obj.Spec.NodeName,
		ControllerUid:      controllerUid,
		ControllerKind:     controllerKind,
		ControllerName:     controllerName,
		RequestCpuCores:    cpuRequest,
		RequestMemoryBytes: memoryRequest,
		DeletedAt:          ptrToPGTime(obj.DeletionTimestamp),
		CreatedAt:          toPGTime(obj.CreationTimestamp),
		StartedAt:          ptrToPGTime(obj.Status.StartTime),
	}
	if err := p.queries.UpsertPod(context.Background(), queryParams); err != nil {
		return fmt.Errorf("upserting pod: %w", err)
	}
	return nil
}
