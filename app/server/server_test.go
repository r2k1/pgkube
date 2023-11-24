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
	q := queries.New(db)
	s := NewSrv(q, "../templates", "../assets")
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	for _, urlPath := range []string{
		"/",
		"/assets/htmx.js",
		"/assets/style.css",
		"/workload",
		"/workload?group_by=namespace",
		"/workload?order_by=namespace",
		"/workload?start=2021-01-01T00:00:00Z&end=2021-01-02T00:00:00Z",
		"/workload?range=168h",
		"/workload?group_by=namespace&group_by=controller_kind&group_by=controller_name&group_by=pod_name&group_by=node_name&order_by=namespace&range=168h",
	} {
		t.Run(urlPath, func(t *testing.T) {
			resp, err := http.Get(srv.URL + urlPath)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}
