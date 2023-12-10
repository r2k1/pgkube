package queries

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkloadAgg_SQLInjection(t *testing.T) {
	queries := NewTestQueries(t)
	req := WorkloadAggRequest{
		Cols:    []string{"label_app'; DROP TABLE cost_hourly; --"},
		OrderBy: "label_app'; DROP TABLE cost_hourly; --",
		Start:   time.Now().Add(-24 * time.Hour),
		End:     time.Now(),
	}
	_, err := queries.WorkloadAgg(context.TODO(), req)
	require.ErrorContains(t, err, "invalid label")
}

func TestWorkloadAgg(t *testing.T) {
	queries := NewTestQueries(t)
	testCases := []struct {
		name string
		req  WorkloadAggRequest
		err  bool
	}{
		{
			name: "WithValidRequest",
			req: WorkloadAggRequest{
				Cols:    []string{"namespace", "name"},
				OrderBy: "namespace",
				Start:   time.Now().Add(-24 * time.Hour),
				End:     time.Now(),
			},
		},
		{
			name: "WithLabelColumns",
			req: WorkloadAggRequest{
				Cols:    []string{"namespace", "name", "label_app"},
				Start:   time.Now().Add(-24 * time.Hour),
				End:     time.Now(),
				OrderBy: "label_app",
			},
		},
		{
			name: "WithInvalidColumns",
			req: WorkloadAggRequest{
				Cols:    []string{"invalid_column"},
				OrderBy: "namespace",
				Start:   time.Now().Add(-24 * time.Hour),
				End:     time.Now(),
			},
			err: true,
		},
		{
			name: "WithInvalidOrderBy",
			req: WorkloadAggRequest{
				Cols:    []string{"namespace", "name"},
				OrderBy: "invalid_order_by",
				Start:   time.Now().Add(-24 * time.Hour),
				End:     time.Now(),
			},
			err: true,
		},
		{
			name: "WithEndTimeBeforeStartTime",
			req: WorkloadAggRequest{
				Cols:    []string{"namespace", "name"},
				OrderBy: "namespace",
				Start:   time.Now(),
				End:     time.Now().Add(-24 * time.Hour),
			},
			err: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := queries.WorkloadAgg(context.TODO(), tc.req)
			if tc.err {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
