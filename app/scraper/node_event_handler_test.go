package scraper

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"

	"github.com/r2k1/pgkube/app/k8s"
)

//go:generate moq -out pod_cache_moq_test.go . PodCache

func TestNodeScraper_Scrape(t *testing.T) {
	ctx := Context(t)

	t.Run("returns error when NodeMetrics fails", func(t *testing.T) {
		t.Parallel()
		client := &k8s.ClientMock{
			NodeMetricsFunc: func(ctx context.Context, nodeName string) (k8s.NodeMetrics, error) {
				return k8s.NodeMetrics{}, errors.New("node metrics error")
			},
		}
		queries := Queries(t)
		scraper := NewNodeScrapper("test-node", client, queries, &PodCacheMock{})

		err := scraper.Scrape(ctx)
		require.Error(t, err)
	})

	t.Run("update memory usage", func(t *testing.T) {
		t.Parallel()
		client := &k8s.ClientMock{
			NodeMetricsFunc: func(ctx context.Context, nodeName string) (k8s.NodeMetrics, error) {
				return k8s.NodeMetrics{
					PodMemoryWorkingSetBytes: k8s.PodMetric{
						{
							Name:      "test-pod",
							Namespace: "test-namespace",
						}: {
							Value:       100,
							TimestampMs: 1,
						},
					},
				}, nil
			},
		}
		queries := Queries(t)
		k8suid, pguuid := RandomUUID(t)
		cache := &PodCacheMock{
			GetFunc: func(namespace string, name string) (*v1.Pod, error) {
				return &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						UID:       k8suid,
						Namespace: "test-namespace",
						Name:      "test-pod",
					},
				}, nil
			},
		}
		scraper := NewNodeScrapper("test-node", client, queries, cache)

		err := scraper.Scrape(ctx)
		require.NoError(t, err)

		query, err := queries.ListPodUsageHourly(ctx)
		require.NoError(t, err)

		require.Len(t, query, 1)

		assert.Equal(t, pguuid, query[0].PodUid)
		assert.InDelta(t, 100.0, query[0].MemoryBytesMax, 0.0001)
		assert.InDelta(t, 100.0, query[0].MemoryBytesMin, 0.0001)
		assert.InDelta(t, 100.0, query[0].MemoryBytesAvg, 0.0001)

		client.NodeMetricsFunc = func(ctx context.Context, nodeName string) (k8s.NodeMetrics, error) {
			return k8s.NodeMetrics{
				PodMemoryWorkingSetBytes: k8s.PodMetric{
					{
						Name:      "test-pod",
						Namespace: "test-namespace",
					}: {
						Value:       200,
						TimestampMs: 2,
					},
				},
			}, nil
		}
		err = scraper.Scrape(ctx)
		require.NoError(t, err)

		query, err = queries.ListPodUsageHourly(ctx)
		require.NoError(t, err)

		require.Len(t, query, 1)
		assert.Equal(t, pguuid, query[0].PodUid)
		assert.InDelta(t, 200.0, query[0].MemoryBytesMax, 0.0001)
		assert.InDelta(t, 100.0, query[0].MemoryBytesMin, 0.0001)
		assert.InDelta(t, 150.0, query[0].MemoryBytesAvg, 0.0001)
	})

	t.Run("update memory usage, unknown pod", func(t *testing.T) {
		t.Parallel()
		client := &k8s.ClientMock{
			NodeMetricsFunc: func(ctx context.Context, nodeName string) (k8s.NodeMetrics, error) {
				return k8s.NodeMetrics{
					PodMemoryWorkingSetBytes: k8s.PodMetric{
						{
							Name:      "test-pod",
							Namespace: "test-namespace",
						}: {
							Value:       100,
							TimestampMs: 1,
						},
					},
				}, nil
			},
		}
		queries := Queries(t)
		cache := &PodCacheMock{
			GetFunc: func(namespace string, name string) (*v1.Pod, error) {
				return nil, errors.New("pod not found")
			},
		}
		scraper := NewNodeScrapper("test-node", client, queries, cache)

		err := scraper.Scrape(ctx)
		require.NoError(t, err)

		query, err := queries.ListPodUsageHourly(ctx)
		require.NoError(t, err)

		require.Len(t, query, 0)
	})

	t.Run("update cpu usage", func(t *testing.T) {
		t.Parallel()
		key := k8s.PodKey{Name: "test-pod", Namespace: "test-namespace"}
		data := []k8s.PodMetric{
			{key: k8s.MetricValue{Value: 10, TimestampMs: 0}},    // not enough information
			{key: k8s.MetricValue{Value: 30, TimestampMs: 1000}}, // 20 cores/sec
			{key: k8s.MetricValue{Value: 30, TimestampMs: 1000}}, // no change, use previous value
			{key: k8s.MetricValue{Value: 40, TimestampMs: 2000}}, // 10 cores/sec
			{key: k8s.MetricValue{Value: 40, TimestampMs: 2000}}, // no change, use previous value
		}
		callindex := -1
		client := &k8s.ClientMock{
			NodeMetricsFunc: func(ctx context.Context, nodeName string) (k8s.NodeMetrics, error) {
				callindex++
				return k8s.NodeMetrics{PodCPUUsageSecondsTotal: data[callindex]}, nil
			},
		}
		queries := Queries(t)
		k8suid, pguuid := RandomUUID(t)
		cache := &PodCacheMock{
			GetFunc: func(namespace string, name string) (*v1.Pod, error) {
				return &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						UID:       k8suid,
						Namespace: "test-namespace",
						Name:      "test-pod",
					},
				}, nil
			},
		}

		scraper := NewNodeScrapper("test-node", client, queries, cache)

		err := scraper.Scrape(ctx)
		require.NoError(t, err)

		query, err := queries.ListPodUsageHourly(ctx)
		require.NoError(t, err)
		require.Empty(t, query)

		scrapeAndAssertCPU := func(min, max, avg float64) {
			err := scraper.Scrape(ctx)
			require.NoError(t, err)

			query, err := queries.ListPodUsageHourly(ctx)
			require.NoError(t, err)

			require.Len(t, query, 1)
			assert.Equal(t, pguuid, query[0].PodUid)
			assert.InDelta(t, min, query[0].CpuCoresMin, 0.0001)
			assert.InDelta(t, max, query[0].CpuCoresMax, 0.0001)
			assert.InDelta(t, avg, query[0].CpuCoresAvg, 0.0001)
		}
		scrapeAndAssertCPU(20.0, 20.0, 20.0)
		scrapeAndAssertCPU(20.0, 20.0, 20.0)
		scrapeAndAssertCPU(10.0, 20.0, float64(20+20+10)/3)
		scrapeAndAssertCPU(10.0, 20.0, float64(20+20+10+10)/4)
	})
}

func RandomUUID(t *testing.T) (types.UID, pgtype.UUID) {
	uuid := uuid.NewUUID()
	pguuid, err := parsePGUUID(uuid)
	require.NoError(t, err)
	return uuid, pguuid
}
