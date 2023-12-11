package queries

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/lmittmann/tint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/term"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"

	"github.com/r2k1/pgkube/app/test"
)

func init() {
	w := os.Stderr
	logger := slog.New(
		tint.NewHandler(w, &tint.Options{
			NoColor: !term.IsTerminal(int(w.Fd())),
			Level:   slog.LevelDebug,
		}),
	)
	slog.SetDefault(logger)
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

func TestUpsertPod(t *testing.T) {
	queries := NewTestQueries(t)
	params := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID: NewKUUID(),
		},
	}
	err := queries.UpsertObject(context.TODO(), "Pod", params)
	require.NoError(t, err)
}

func TestUpsertPodUsedCPU(t *testing.T) {
	queries := NewTestQueries(t)

	params := []UpsertPodUsedCPUParams{
		{
			PodUid:    NewUUID(),
			Timestamp: pgtype.Timestamptz{Time: time.Now(), Valid: true},
			CpuCores:  1.0,
		},
		{
			PodUid:    NewUUID(),
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
			PodUid:      NewUUID(),
			Timestamp:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
			MemoryBytes: 1024.0,
		},
		{
			PodUid:      NewUUID(),
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

func TestDeleteObjects(t *testing.T) {
	queries := NewTestQueries(t)
	createPod := func() pgtype.UUID {
		uid := NewKUUID()
		err := queries.UpsertObject(context.TODO(), "Pod", &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				UID: uid,
			},
		})
		require.NoError(t, err)
		puid, err := parsePGUUID(uid)
		require.NoError(t, err)
		return puid
	}

	requireObjectCount := func(expectedCount int) {
		count, err := queries.ActiveObjectCount(context.Background())
		require.NoError(t, err)
		require.Equal(t, expectedCount, count)
	}

	requireObjectCount(0)
	pod1 := createPod()
	pod2 := createPod()
	createPod()
	requireObjectCount(3)
	deletedCount, err := queries.DeleteObjects(context.TODO(), "Pod", []pgtype.UUID{pod1, pod2})
	require.NoError(t, err)
	require.Equal(t, 1, deletedCount)
	requireObjectCount(2)
}

func NewTestQueries(t *testing.T) *Queries {
	db := test.CreateTestDB(t, "../migrations")
	q, err := New(context.TODO(), db, "test-cluster")
	require.NoError(t, err)
	return q
}

func NewUUID() pgtype.UUID {
	pguuid, err := parsePGUUID(NewKUUID())
	if err != nil {
		panic(err)
	}
	return pguuid
}

func NewKUUID() types.UID {
	return uuid.NewUUID()
}
