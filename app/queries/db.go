package queries

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"k8s.io/apimachinery/pkg/types"
)

type DBTX interface {
	Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error)
	Query(context.Context, string, ...interface{}) (pgx.Rows, error)
	QueryRow(context.Context, string, ...interface{}) pgx.Row
	SendBatch(context.Context, *pgx.Batch) pgx.BatchResults
}

func New(ctx context.Context, db DBTX, clusterName string) (*Queries, error) {
	q := &Queries{db: db}
	var err error
	q.clusterID, err = q.GetOrCreateCluster(ctx, clusterName)
	if err != nil {
		return nil, err
	}
	return q, nil
}

type Queries struct {
	db        DBTX
	clusterID int
}

func (q *Queries) WithTx(tx pgx.Tx) *Queries {
	return &Queries{
		db: tx,
	}
}

// nolint: unparam
func (q *Queries) execStruct(ctx context.Context, sql string, data interface{}) (pgconn.CommandTag, error) {
	dataMap, err := structToNamedArgs(data)
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	cmd, err := q.db.Exec(ctx, sql, dataMap)
	return cmd, WrapError(err)
}

func (q *Queries) query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	rows, err := q.db.Query(ctx, query, args...)
	return rows, WrapError(err)
}

func structToNamedArgs(obj interface{}) (pgx.NamedArgs, error) {
	val := reflect.ValueOf(obj)
	if val.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected a struct, received %T", obj)
	}

	result := make(map[string]interface{})
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		fieldValue := val.Field(i)

		// If the field is an embedded struct, recursively call structToNamedArgs
		if field.Anonymous && fieldValue.Kind() == reflect.Struct {
			embeddedResult, err := structToNamedArgs(fieldValue.Interface())
			if err != nil {
				return nil, err
			}
			for k, v := range embeddedResult {
				result[k] = v
			}
		} else {
			dbTag := field.Tag.Get("db")
			if dbTag != "" {
				result[dbTag] = fieldValue.Interface()
			}
		}
	}

	return result, nil
}

func parsePGUUID(src types.UID) (pgtype.UUID, error) {
	uid, err := parseUUID(string(src))
	return pgtype.UUID{Bytes: uid, Valid: err == nil}, err
}

// parseUUID converts a string UUID in standard form to a byte array.
func parseUUID(src string) (dst [16]byte, err error) {
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

// a helper wrapper to improve postgres error messages
type errWrapper struct {
	Err error
}

func WrapError(err error) error {
	if err == nil {
		return nil
	}
	return &errWrapper{Err: err}
}

func (e *errWrapper) Unwrap() error {
	return e.Err
}

func (e *errWrapper) Error() string {
	var pgErr *pgconn.PgError
	if errors.As(e.Err, &pgErr) {
		return e.Err.Error() + " " + pgErr.Hint
	}
	return e.Err.Error()
}
