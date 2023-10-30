package k8s

import (
	"bytes"
	"context"
	"fmt"

	"github.com/prometheus/common/expfmt"
	"k8s.io/client-go/kubernetes"

	dto "github.com/prometheus/client_model/go"
)

type Client struct {
	internal *kubernetes.Clientset
}

type NodeMetrics struct {
	PodCPUUsageSecondsTotal  PodMetric
	PodMemoryWorkingSetBytes PodMetric
}

type PodMetric map[PodKey]MetricValue

type PodKey struct {
	Name      string
	Namespace string
}

type MetricValue struct {
	Value       float64
	TimestampMs int64
}

var _ ClientInterface = &Client{}

//go:generate moq -out client_mock.go . ClientInterface:ClientMock
type ClientInterface interface {
	NodeMetrics(ctx context.Context, nodeName string) (NodeMetrics, error)
}

func NewClient(clientset *kubernetes.Clientset) *Client {
	return &Client{internal: clientset}

}

func (c *Client) NodeMetrics(ctx context.Context, nodeName string) (NodeMetrics, error) {
	body, err := c.internal.CoreV1().RESTClient().Get().
		Resource("nodes").Name(nodeName).SubResource("proxy").
		Suffix("metrics/resource").DoRaw(ctx)
	if err != nil {
		return NodeMetrics{}, fmt.Errorf("getting node metrics: %w", err)
	}
	parser := &expfmt.TextParser{}
	metrics, err := parser.TextToMetricFamilies(bytes.NewReader(body))
	if err != nil {
		return NodeMetrics{}, fmt.Errorf("parsing node metrics: %w", err)
	}
	result := NodeMetrics{
		PodCPUUsageSecondsTotal:  make(PodMetric),
		PodMemoryWorkingSetBytes: make(PodMetric),
	}
	if metric, ok := metrics["pod_cpu_usage_seconds_total"]; ok {
		for _, m := range metric.GetMetric() {
			result.PodCPUUsageSecondsTotal[PodKey{
				Name:      getLabel(m.GetLabel(), "pod"),
				Namespace: getLabel(m.GetLabel(), "namespace"),
			}] = MetricValue{
				Value:       m.GetCounter().GetValue(),
				TimestampMs: m.GetTimestampMs(),
			}
		}
	}
	if metric, ok := metrics["pod_memory_working_set_bytes"]; ok {
		for _, m := range metric.GetMetric() {
			result.PodMemoryWorkingSetBytes[PodKey{
				Name:      getLabel(m.GetLabel(), "pod"),
				Namespace: getLabel(m.GetLabel(), "namespace"),
			}] = MetricValue{
				Value:       m.GetGauge().GetValue(),
				TimestampMs: m.GetTimestampMs(),
			}
		}
	}
	return result, nil
}

func getLabel(labels []*dto.LabelPair, name string) string {
	for _, label := range labels {
		if label.GetName() == name {
			return label.GetValue()
		}
	}
	return ""
}
