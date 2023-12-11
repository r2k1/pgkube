package queries

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (q *Queries) UpsertObject(ctx context.Context, kind string, object any) error {
	objectGetter, ok := object.(metav1.Object)
	if !ok {
		return fmt.Errorf("unexpected object type: %T", object)
	}

	data, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("failed to marshal object: %w", err)
	}
	uo := struct {
		ClusterID int    `db:"cluster_id"`
		Kind      string `db:"kind"`
		Uid       string `db:"uid"`
		Namespace string `db:"namespace"`
		Name      string `db:"name"`
		Data      any    `db:"data"`
	}{
		ClusterID: q.clusterID,
		Kind:      kind,
		Uid:       string(objectGetter.GetUID()),
		Namespace: objectGetter.GetNamespace(),
		Name:      objectGetter.GetName(),
		Data:      data,
	}

	const upsertObject = `
insert into object (uid, cluster_id, kind, namespace, name, data)
values (@uid, @cluster_id, @kind, @namespace, @name, @data)
on conflict (uid)
    do update set cluster_id  = @cluster_id,
                  kind        = @kind,
                  namespace   = @namespace,
                  name        = @name,
                  data        = @data
`
	_, err = q.execStruct(ctx, upsertObject, uo)
	if err != nil {
		return err
	}
	slog.Debug("upserted object", "kind", kind, "uid", uo.Uid)
	return nil
}

func (q *Queries) DeleteObject(ctx context.Context, uid string) error {
	const deleteObject = `
update object set deleted_at = now() where uid = $1 and deleted_at is null and cluster_id = $2 
`
	_, err := q.db.Exec(ctx, deleteObject, uid, q.clusterID)
	return err
}

func (q *Queries) DeleteObjects(ctx context.Context, kind string, ignoreUids []pgtype.UUID) (int, error) {
	const deleteObject = `
with updated as ( 
	update object set deleted_at = now() where cluster_id = $1 and uid != all($2) and deleted_at is null and kind = $3 returning *
)
select count(*) from updated 
`
	var count int
	err := q.db.QueryRow(ctx, deleteObject, q.clusterID, ignoreUids, kind).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to scan count: %w", err)
	}
	return count, nil
}

func (q *Queries) ActiveObjectCount(ctx context.Context) (int, error) {
	const countObjects = `select count(*) from object where deleted_at is null`
	var count int
	err := q.db.QueryRow(ctx, countObjects).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to scan count: %w", err)
	}
	return count, nil
}

type PodUsageHourly struct {
	PodUid                   pgtype.UUID        `db:"pod_uid"`
	ClusterID                int                `db:"cluster_id"`
	Timestamp                pgtype.Timestamptz `db:"timestamp"`
	MemoryBytesMax           float64            `db:"memory_bytes_max"`
	MemoryBytesMin           float64            `db:"memory_bytes_min"`
	MemoryBytesTotal         float64            `db:"memory_bytes_total"`
	MemoryBytesTotalReadings int32              `db:"memory_bytes_total_readings"`
	MemoryBytesAvg           float64            `db:"memory_bytes_avg"`
	CpuCoresMax              float64            `db:"cpu_cores_max"`
	CpuCoresMin              float64            `db:"cpu_cores_min"`
	CpuCoresTotal            float64            `db:"cpu_cores_total"`
	CpuCoresTotalReadings    int32              `db:"cpu_cores_total_readings"`
	CpuCoresAvg              float64            `db:"cpu_cores_avg"`
}

func (q *Queries) ListPodUsageHourly(ctx context.Context) ([]PodUsageHourly, error) {
	const listPodUsageHourly = `select pod_uid, cluster_id, timestamp, memory_bytes_max, memory_bytes_min, memory_bytes_total, memory_bytes_total_readings, memory_bytes_avg, cpu_cores_max, cpu_cores_min, cpu_cores_total, cpu_cores_total_readings, cpu_cores_avg
from pod_usage_hourly
order by timestamp desc
limit 100
`
	rows, err := q.query(ctx, listPodUsageHourly)
	if err != nil {
		return nil, err
	}
	data, err := pgx.CollectRows(rows, pgx.RowToStructByName[PodUsageHourly])
	if err != nil {
		return nil, fmt.Errorf("failed to collect pod usage rows: %w", err)
	}
	return data, nil
}

type UpsertPodUsedCPUParams struct {
	ClusterID int                `db:"cluster_id"`
	PodUid    pgtype.UUID        `db:"pod_uid"`
	Timestamp pgtype.Timestamptz `db:"timestamp"`
	CpuCores  float64            `db:"cpu_cores"`
}

func (q *Queries) UpsertPodUsedCPU(ctx context.Context, arg []UpsertPodUsedCPUParams) error {
	for i := range arg {
		arg[i].ClusterID = q.clusterID
	}
	const upsertPodUsedCPU = `
insert into pod_usage_hourly (pod_uid, cluster_id, timestamp, cpu_cores_max, cpu_cores_min, cpu_cores_total,
                              cpu_cores_total_readings)
values (@pod_uid, @cluster_id, @timestamp, @cpu_cores, @cpu_cores, @cpu_cores, 1)
on conflict (pod_uid, timestamp)
    do update set cluster_id 			   = @cluster_id,
                  cpu_cores_total_readings = pod_usage_hourly.cpu_cores_total_readings + 1,
                  cpu_cores_max            = case
                                                 when pod_usage_hourly.cpu_cores_max > @cpu_cores
                                                     then pod_usage_hourly.cpu_cores_max
                                                 else @cpu_cores end,
                  cpu_cores_min            = case
                                                 when pod_usage_hourly.cpu_cores_min < @cpu_cores and
                                                      pod_usage_hourly.cpu_cores_min != 0
                                                     then pod_usage_hourly.cpu_cores_min
                                                 else @cpu_cores end,
                  cpu_cores_total          = pod_usage_hourly.cpu_cores_total + @cpu_cores
`
	return execBatch(ctx, q, upsertPodUsedCPU, arg)
}

type UpsertPodUsedMemoryParams struct {
	ClusterID   int                `db:"cluster_id"`
	PodUid      pgtype.UUID        `db:"pod_uid"`
	Timestamp   pgtype.Timestamptz `db:"timestamp"`
	MemoryBytes float64            `db:"memory_bytes"`
}

func (q *Queries) UpsertPodUsedMemory(ctx context.Context, arg []UpsertPodUsedMemoryParams) error {
	for i := range arg {
		arg[i].ClusterID = q.clusterID
	}
	const upsertPodUsedMemory = `
insert into pod_usage_hourly (pod_uid, cluster_id, timestamp, memory_bytes_max, memory_bytes_min,
                              memory_bytes_total,
                              memory_bytes_total_readings)
values (@pod_uid, @cluster_id, @timestamp, @memory_bytes, @memory_bytes, @memory_bytes, 1)
on conflict (pod_uid, timestamp)
    do update set cluster_id 			      = @cluster_id,
                  memory_bytes_total_readings = pod_usage_hourly.memory_bytes_total_readings + 1,
                  memory_bytes_max            = case
                                                    when pod_usage_hourly.memory_bytes_max > @memory_bytes
                                                        then pod_usage_hourly.memory_bytes_max
                                                    else @memory_bytes end,
                  memory_bytes_min            = case
                                                    when pod_usage_hourly.memory_bytes_min < @memory_bytes and
                                                         pod_usage_hourly.memory_bytes_min != 0
                                                        then pod_usage_hourly.memory_bytes_min
                                                    else @memory_bytes end,
                  memory_bytes_total          = pod_usage_hourly.memory_bytes_total + @memory_bytes
`
	return execBatch(ctx, q, upsertPodUsedMemory, arg)
}

func (q *Queries) GetClusterID(ctx context.Context, name string) (int, error) {
	const getClusterID = `select id from cluster where name = $1`
	var id int
	err := q.db.QueryRow(ctx, getClusterID, name).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to scan cluster id: %w", err)
	}
	return id, nil
}

func (q *Queries) CreateCluster(ctx context.Context, name string) (int, error) {
	const insertCluster = `insert into cluster (name) values ($1) returning id`
	var id int
	err := q.db.QueryRow(ctx, insertCluster, name).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to scan cluster id: %w", err)
	}
	return id, nil
}

func (q *Queries) GetOrCreateCluster(ctx context.Context, name string) (int, error) {
	id, err := q.GetClusterID(ctx, name)
	if errors.Is(err, pgx.ErrNoRows) {
		return q.CreateCluster(ctx, name)
	}
	return id, err
}

type NamedArgConverter interface {
	ToNamedArgs() (map[string]interface{}, error)
}

func execBatch[T any](ctx context.Context, q *Queries, query string, data []T) error {
	batch := &pgx.Batch{}
	for _, record := range data {
		namedArgs, err := structToNamedArgs(record)
		if err != nil {
			return err
		}
		batch.Queue(query, namedArgs)
	}
	batchResult := q.db.SendBatch(ctx, batch)
	if err := batchResult.Close(); err != nil {
		return fmt.Errorf("failed to execute batch: %w", WrapError(err))
	}
	return nil
}
