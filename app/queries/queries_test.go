package queries

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/r2k1/pgkube/app/test"
)

func TestWorkloadAgg(t *testing.T) {
	db := test.CreateTestDB(t, "../migrations")
	_, err := New(db).WorkloadAgg(context.TODO(), WorkloadAggRequest{})
	require.NoError(t, err)
}
