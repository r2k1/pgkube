package scraper

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	postgresUser     = "user"
	postgresPassword = "password"
	postgresDb       = "testdb"
	container        = MustStartPostgresContainer(context.Background())
	migrateOnce      = sync.Once{}
)

func MustStartPostgresContainer(ctx context.Context) testcontainers.Container {
	cont, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:15-alpine",
			ExposedPorts: []string{"5432/tcp"},
			WaitingFor:   wait.ForListeningPort("5432/tcp"),
			Env: map[string]string{
				"POSTGRES_USER":     postgresUser,
				"POSTGRES_PASSWORD": postgresPassword,
				"POSTGRES_DB":       postgresDb,
			},
		},
		Started: true,
	})
	if err != nil {
		panic(err)
	}
	return cont
}

func CreateTestDB(t *testing.T, migrationsPath string) *pgx.Conn {
	t.Helper()
	ctx := context.Background()

	mappedPort, err := container.MappedPort(ctx, "5432")
	require.NoError(t, err)

	hostIP, err := container.Host(ctx)
	require.NoError(t, err)

	mainConnString := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", postgresUser, postgresPassword, hostIP, mappedPort.Port(), postgresDb)
	mainConn, err := pgx.Connect(context.Background(), mainConnString)
	require.NoError(t, err)

	migrateOnce.Do(func() {
		Migrate(t, mainConnString, migrationsPath)
	})

	// nolint:gosec
	dbName := "db_" + strconv.Itoa(rand.Intn(10000000))
	_, err = mainConn.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s", dbName, postgresDb))
	require.NoError(t, err)

	connString := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", postgresUser, postgresPassword, hostIP, mappedPort.Port(), dbName)

	var testConn *pgx.Conn

	t.Cleanup(func() {
		_ = testConn.Close(ctx)
		_, err := mainConn.Exec(ctx, "DROP DATABASE "+dbName)
		require.NoError(t, err)
		_ = mainConn.Close(ctx)
	})

	testConn, err = pgx.Connect(context.Background(), connString)
	require.NoError(t, err)

	return testConn
}

func CreateDB(t *testing.T) *pgx.Conn {
	return CreateTestDB(t, "../migrations")
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
