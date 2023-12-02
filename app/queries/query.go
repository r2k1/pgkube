package queries

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Object struct {
	Kind     string             `db:"kind"`
	Metadata any                `db:"metadata"`
	Spec     any                `db:"spec"`
	Status   any                `db:"status"`
	LastSeen pgtype.Timestamptz `db:"last_seen"`
}

func (q *Queries) UpsertObject(ctx context.Context, arg Object) error {
	const upsertObject = `
insert into object (kind, metadata, spec, status)
values (@kind, @metadata, @spec, @status)
on conflict (((metadata ->> 'uid')::uuid))
    do update set kind        = @kind,
                  metadata    = @metadata,
                  spec        = @spec,
                  status      = @status
`
	_, err := q.execStruct(ctx, upsertObject, arg)
	return err
}

func (q *Queries) DeleteObject(ctx context.Context, uid string) error {
	const deleteObject = `
update object set deleted_at = now() where metadata ->> 'uid' = $1 and deleted_at is null 
`
	_, err := q.db.Exec(ctx, deleteObject, uid)
	return err
}

type PodUsageHourly struct {
	PodUid                   pgtype.UUID        `db:"pod_uid"`
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
	const listPodUsageHourly = `select pod_uid, timestamp, memory_bytes_max, memory_bytes_min, memory_bytes_total, memory_bytes_total_readings, memory_bytes_avg, cpu_cores_max, cpu_cores_min, cpu_cores_total, cpu_cores_total_readings, cpu_cores_avg
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
	PodUid    pgtype.UUID        `db:"pod_uid"`
	Timestamp pgtype.Timestamptz `db:"timestamp"`
	CpuCores  float64            `db:"cpu_cores"`
}

func (q *Queries) UpsertPodUsedCPU(ctx context.Context, arg []UpsertPodUsedCPUParams) error {
	const upsertPodUsedCPU = `
insert into pod_usage_hourly (pod_uid, timestamp, cpu_cores_max, cpu_cores_min, cpu_cores_total,
                              cpu_cores_total_readings)
values (@pod_uid, @timestamp, @cpu_cores, @cpu_cores, @cpu_cores, 1)
on conflict (pod_uid, timestamp)
    do update set cpu_cores_total_readings = pod_usage_hourly.cpu_cores_total_readings + 1,
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
	PodUid      pgtype.UUID        `db:"pod_uid"`
	Timestamp   pgtype.Timestamptz `db:"timestamp"`
	MemoryBytes float64            `db:"memory_bytes"`
}

func (q *Queries) UpsertPodUsedMemory(ctx context.Context, arg []UpsertPodUsedMemoryParams) error {
	const upsertPodUsedMemory = `
insert into pod_usage_hourly (pod_uid, timestamp, memory_bytes_max, memory_bytes_min,
                              memory_bytes_total,
                              memory_bytes_total_readings)
values (@pod_uid, @timestamp, @memory_bytes, @memory_bytes, @memory_bytes, 1)
on conflict (pod_uid, timestamp)
    do update set memory_bytes_total_readings = pod_usage_hourly.memory_bytes_total_readings + 1,
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
