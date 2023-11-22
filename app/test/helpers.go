package test

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	postgresUser     = "user"
	postgresPassword = "password"
	postgresDBName   = "testdb"
	container        testcontainers.Container
	migrateOnce      = sync.Once{}
	mainConn         *pgxpool.Pool
	mainDBLock       = sync.Mutex{}
)

func MustStartPostgresContainer(t *testing.T, ctx context.Context) testcontainers.Container {
	t.Helper()
	cont, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:15-alpine",
			ExposedPorts: []string{"5432/tcp"},
			WaitingFor:   wait.ForListeningPort("5432/tcp"),
			Env: map[string]string{
				"POSTGRES_USER":     postgresUser,
				"POSTGRES_PASSWORD": postgresPassword,
				"POSTGRES_DB":       postgresDBName,
			},
		},
		Started: true,
	})
	require.NoError(t, err)
	return cont
}

// nolint:contextcheck
// CreateTestDB spawns a new postgres container (if not spawned yet)
// Runs migrations on the main database (if not run yet)
// Creates a new database for the test and returns a connection to it
// The test database is dropped after the test
func CreateTestDB(t *testing.T, migrationsPath string) *pgx.Conn {
	t.Helper()

	migrateOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		container = MustStartPostgresContainer(t, ctx)
		mappedPort, err := container.MappedPort(ctx, "5432")
		require.NoError(t, err)

		hostIP, err := container.Host(ctx)
		require.NoError(t, err)

		mainConnString := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", postgresUser, postgresPassword, hostIP, mappedPort.Port(), postgresDBName)
		mainConn, err = pgxpool.New(ctx, mainConnString)
		require.NoError(t, err)
		Migrate(t, mainConnString, migrationsPath)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// nolint:gosec
	testDBName := "db_" + strconv.Itoa(rand.Intn(10000000))
	createDBQuery := fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s", testDBName, postgresDBName)
	mainDBLock.Lock()
	_, err := mainConn.Exec(ctx, createDBQuery)
	mainDBLock.Unlock()

	require.NoError(t, err)

	hostIP, err := container.Host(ctx)
	require.NoError(t, err)
	mappedPort, err := container.MappedPort(ctx, "5432")
	require.NoError(t, err)
	connString := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", postgresUser, postgresPassword, hostIP, mappedPort.Port(), testDBName)

	var testConn *pgx.Conn

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		_ = testConn.Close(ctx)
		mainDBLock.Lock()
		_, err := mainConn.Exec(ctx, "DROP DATABASE "+testDBName)
		mainDBLock.Unlock()
		require.NoError(t, err)
	})

	testConn, err = pgx.Connect(ctx, connString)
	require.NoError(t, err)

	return testConn
}

func Migrate(t *testing.T, databaseURL string, migrationsPath string) {
	if strings.HasPrefix(databaseURL, "postgres://") {
		databaseURL = strings.TrimPrefix(databaseURL, "postgres")
		databaseURL = "pgx5" + databaseURL
	} else {
		t.Fatal("unsupported database url")
	}
	m, err := migrate.New(
		"file://"+migrationsPath,
		databaseURL)
	require.NoError(t, err)

	err = m.Up()
	defer m.Close()

	if errors.Is(err, migrate.ErrNoChange) {
		return
	}
	require.NoError(t, err)
}
