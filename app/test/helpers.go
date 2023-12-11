package test

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgtype"
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
	migrateErr       error
	mainConn         *pgxpool.Pool
	mainDBLock       = sync.Mutex{}
)

func MustStartPostgresContainer(t *testing.T, ctx context.Context) testcontainers.Container {
	t.Helper()
	cont, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:15-alpine",
			ExposedPorts: []string{"5432/tcp"},
			WaitingFor:   wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(5 * time.Second),
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

func connectionString(t *testing.T, ctx context.Context, container testcontainers.Container, dbName string) string {
	mappedPort, err := container.MappedPort(ctx, "5432")
	require.NoError(t, err)
	hostIP, err := container.Host(ctx)
	require.NoError(t, err)
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", postgresUser, postgresPassword, hostIP, mappedPort.Port(), dbName)
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
		mainConnString := connectionString(t, ctx, container, postgresDBName)
		mainConn, migrateErr = pgxpool.New(ctx, mainConnString)
		require.NoError(t, migrateErr)
		Migrate(t, mainConnString, migrationsPath)
	})

	require.NoError(t, migrateErr)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// nolint:gosec
	testDBName := "db_" + strconv.Itoa(rand.Intn(10000000))
	createDBQuery := fmt.Sprintf("create database %s template %s", testDBName, postgresDBName)
	mainDBLock.Lock()
	_, err := mainConn.Exec(ctx, createDBQuery)
	mainDBLock.Unlock()
	require.NoError(t, err)

	connString := connectionString(t, ctx, container, testDBName)

	var testConn *pgx.Conn

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		_ = testConn.Close(ctx)
		mainDBLock.Lock()
		_, err := mainConn.Exec(ctx, "drop database "+testDBName)
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

func MustParsePGUUID(src string) pgtype.UUID {
	uid, err := ParsePGUUID(src)
	if err != nil {
		panic(err)
	}
	return uid
}

func ParsePGUUID(src string) (pgtype.UUID, error) {
	uid, err := ParseUUID(src)
	return pgtype.UUID{Bytes: uid, Valid: err == nil}, err
}

func ParseUUID(src string) (dst [16]byte, err error) {
	switch len(src) {
	case 36:
		src = src[0:8] + src[9:13] + src[14:18] + src[19:23] + src[24:]
	case 32:
		// dashes already stripped, assume valid
	default:
		// assume invalid.
		return dst, fmt.Errorf("cannot parse UUID %v", src)
	}

	buf, err := hex.DecodeString(src)
	if err != nil {
		return dst, fmt.Errorf("cannot parse UUID %v: %w", src, err)
	}

	copy(dst[:], buf)
	return dst, nil
}
