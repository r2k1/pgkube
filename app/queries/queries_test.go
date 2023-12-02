package queries

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/uuid"

	"github.com/r2k1/pgkube/app/test"
)

func TestWorkloadAgg(t *testing.T) {
	queries := NewTestQueries(t)
	_, err := queries.WorkloadAgg(context.TODO(), WorkloadAggRequest{})
	require.NoError(t, err)
}

func TestStructToMap(t *testing.T) {

	type TestStruct struct {
		Field1 string `db:"field1"`
		Field2 int    `db:"field2"`
		Field3 bool   `db:"field3"`
	}

	// Test with properly initialized TestStruct
	t.Run("with valid struct", func(t *testing.T) {
		testStruct := TestStruct{
			Field1: "value1",
			Field2: 42,
			Field3: true,
		}

		expectedResult := pgx.NamedArgs{
			"field1": "value1",
			"field2": 42,
			"field3": true,
		}

		result, err := structToNamedArgs(testStruct)
		require.NoError(t, err, "structToNamedArgs should not return an error for valid structs")
		assert.Equal(t, expectedResult, result, "The function did not return the expected map")
	})

	// Test with a non-struct type
	t.Run("with non-struct", func(t *testing.T) {
		_, err := structToNamedArgs(42)
		require.Error(t, err, "structToNamedArgs should return an error for non-struct types")
	})

	// Additional test cases can be added here
}

func TestUpsertObject(t *testing.T) {
	queries := NewTestQueries(t)
	params := Object{
		Metadata: map[string]interface{}{
			"uid": RandomUUID(),
		},
		Spec: map[string]interface{}{
			"test": "test",
		},
		Status: map[string]interface{}{
			"test": "test",
		},
	}
	err := queries.UpsertObject(context.TODO(), params)
	require.NoError(t, err)
}

func TestUpsertPodUsedCPU(t *testing.T) {
	queries := NewTestQueries(t)

	params := []UpsertPodUsedCPUParams{
		{
			PodUid:    RandomUUID(),
			Timestamp: pgtype.Timestamptz{Time: time.Now(), Valid: true},
			CpuCores:  1.0,
		},
		{
			PodUid:    RandomUUID(),
			Timestamp: pgtype.Timestamptz{Time: time.Now(), Valid: true},
			CpuCores:  2.0,
		},
	}

	// Happy path
	err := queries.UpsertPodUsedCPU(context.TODO(), params)
	require.NoError(t, err)

	// Test idempotency
	err = queries.UpsertPodUsedCPU(context.TODO(), params)
	require.NoError(t, err)
}

func TestUpsertPodUsedMemory(t *testing.T) {
	queries := NewTestQueries(t)

	params := []UpsertPodUsedMemoryParams{
		{
			PodUid:      RandomUUID(),
			Timestamp:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
			MemoryBytes: 1024.0,
		},
		{
			PodUid:      RandomUUID(),
			Timestamp:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
			MemoryBytes: 2048.0,
		},
	}

	// Happy path
	err := queries.UpsertPodUsedMemory(context.TODO(), params)
	require.NoError(t, err)

	// Test idempotency
	err = queries.UpsertPodUsedMemory(context.TODO(), params)
	require.NoError(t, err)
}

func TestListPodUsageHourly(t *testing.T) {
	_, err := NewTestQueries(t).ListPodUsageHourly(context.TODO())
	require.NoError(t, err)

}

func NewTestQueries(t *testing.T) *Queries {
	db := test.CreateTestDB(t, "../migrations")
	return New(db)
}

func RandomUUID() pgtype.UUID {
	uuid := uuid.NewUUID()
	pguuid, err := parsePGUUID(uuid)
	if err != nil {
		panic(err)
	}
	return pguuid
}
