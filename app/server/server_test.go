package server

import (
	"context"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/r2k1/pgkube/app/queries"
	"github.com/r2k1/pgkube/app/test"
)

func NewTestQueries(t *testing.T) *queries.Queries {
	db := test.CreateTestDB(t, "../migrations")
	q, err := queries.New(context.TODO(), db, "test-cluster")
	require.NoError(t, err)
	return q
}

func TestServer_HandleWorkload(t *testing.T) {
	handler := NewSrv(NewTestQueries(t), "../templates", "../assets", false).Handler()
	tests := []struct {
		path       string
		statusCode int
	}{
		{path: "/", statusCode: 302},
		{path: "/assets/htmx.js", statusCode: 200},
		{path: "/assets/style.css", statusCode: 200},
		{path: "/workload", statusCode: 302},
		{path: "/workload.csv", statusCode: 302},
		{path: "/workload?col=namespace", statusCode: 200},
		{path: "/workload?col=namespace&range=invalid", statusCode: 500},
		{path: "/workload.csv?col=namespace", statusCode: 200},
		{path: "/workload?col=namespace&order_by=namespace", statusCode: 200},
		{path: "/workload.csv?col=namespace&start=2021-01-01T00:00:00Z&end=2021-01-02T00:00:00Z", statusCode: 200},
		{path: "/workload?col=namespace&range=168h", statusCode: 200},
		{path: "/workload?col=namespace&col=controller_kind&col=controller_name&col=pod_name&col=node_name&col=total_cost&order_by=namespace&range=168h", statusCode: 200},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, test.path, nil)
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)
			require.Equal(t, test.statusCode, resp.Code)
		})
	}
}

func TestHandleWorkloadCSV_ReturnsCSVWhenValidQuery(t *testing.T) {
	q := NewTestQueries(t)
	ctx := context.TODO()
	podTime := time.Date(2021, 1, 15, 0, 0, 0, 0, time.UTC)
	err := q.UpsertObject(ctx, "Pod", &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID:       "11111111-1111-1111-1111-111111111111",
			Namespace: "test-ns",
		},
		Status: v1.PodStatus{
			StartTime: &metav1.Time{Time: podTime},
		},
	})
	require.NoError(t, err)
	err = q.UpsertPodUsedCPU(ctx, []queries.UpsertPodUsedCPUParams{
		{
			ClusterID: 1,
			PodUid:    test.MustParsePGUUID("11111111-1111-1111-1111-111111111111"),
			Timestamp: pgtype.Timestamptz{
				Time:  podTime,
				Valid: true,
			},
			CpuCores: 1,
		},
	})
	require.NoError(t, err)
	srv := NewSrv(q, "../templates", "../assets", false)
	req := httptest.NewRequest(http.MethodGet, "/workload.csv?start=2021-01-15T00%3A00%3A00Z&end=2021-01-16T00%3A00%3A00Z&col=namespace", nil)
	resp := httptest.NewRecorder()

	srv.HandleWorkloadCSV(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	assert.Equal(t, "text/csv", resp.Header().Get("Content-Type"))
	assert.Equal(t, "attachment; filename=pgkube.csv", resp.Header().Get("Content-Disposition"))
	assert.Equal(t, "namespace\ntest-ns\n", resp.Body.String())
}
