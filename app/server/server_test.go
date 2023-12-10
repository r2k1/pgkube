package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/r2k1/pgkube/app/queries"
	"github.com/r2k1/pgkube/app/test"
)

func TestServer_HandleWorkload(t *testing.T) {
	db := test.CreateTestDB(t, "../migrations")
	handler := NewSrv(queries.New(db), "../templates", "../assets", false).Handler()
	tests := []struct {
		path       string
		statusCode int
	}{
		{path: "/", statusCode: 302},
		{path: "/assets/htmx.js", statusCode: 200},
		{path: "/assets/style.css", statusCode: 200},
		{path: "/workload", statusCode: 302},
		{path: "/workload?col=namespace", statusCode: 200},
		{path: "/workload?col=namespace&order_by=namespace", statusCode: 200},
		{path: "/workload?col=namespace&start=2021-01-01T00:00:00Z&end=2021-01-02T00:00:00Z", statusCode: 200},
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
