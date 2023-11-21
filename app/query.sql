-- name: StopOtherPods :exec
update pod
set deleted_at = $1
where deleted_at is null
  and pod_uid != all (sqlc.arg(running_pod_uids)::uuid[]);

-- name: UpsertPod :exec
insert into pod (pod_uid, name, namespace, node_name, labels, annotations, controller_uid, controller_kind,
                 controller_name, request_cpu_cores, request_memory_bytes, created_at, deleted_at, started_at)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
on conflict (pod_uid)
    do update set name                 = $2,
                  namespace            = $3,
                  node_name            = $4,
                  labels               = $5,
                  annotations          = $6,
                  controller_uid       = $7,
                  controller_kind      = $8,
                  controller_name      = $9,
                  request_cpu_cores    = $10,
                  request_memory_bytes = $11,
                  created_at           = $12,
                  deleted_at           = $13,
                  started_at           = $14
;

-- name: UpsertReplicaSet :exec
insert into replica_set (replica_set_uid, name, namespace, labels, annotations, controller_uid, controller_kind,
                         controller_name, created_at, deleted_at)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
on conflict (replica_set_uid)
    do update set name            = $2,
                  namespace       = $3,
                  labels          = $4,
                  annotations     = $5,
                  controller_uid  = $6,
                  controller_kind = $7,
                  controller_name = $8,
                  created_at      = $9,
                  deleted_at      = $10
;

-- name: UpsertJob :exec
insert into job(job_uid, name, namespace, labels, annotations, controller_uid, controller_kind, controller_name,
                created_at,
                deleted_at)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
on conflict (job_uid)
    do update set name            = $2,
                  namespace       = $3,
                  labels          = $4,
                  annotations     = $5,
                  controller_uid  = $6,
                  controller_kind = $7,
                  controller_name = $8,
                  created_at      = $9,
                  deleted_at      = $10
;


-- name: UpsertPodUsedMemory :batchexec
insert into pod_usage_hourly (pod_uid, timestamp, memory_bytes_max, memory_bytes_min,
                              memory_bytes_total,
                              memory_bytes_total_readings)
values ($1, $2, $3, $3, $3, 1)
on conflict (pod_uid, timestamp)
    do update set memory_bytes_total_readings = pod_usage_hourly.memory_bytes_total_readings + 1,
                  memory_bytes_max            = case
                                                    when pod_usage_hourly.memory_bytes_max > $3
                                                        then pod_usage_hourly.memory_bytes_max
                                                    else $3 end,
                  memory_bytes_min            = case
                                                    when pod_usage_hourly.memory_bytes_min < $3 and
                                                         pod_usage_hourly.memory_bytes_min != 0
                                                        then pod_usage_hourly.memory_bytes_min
                                                    else $3 end,
                  memory_bytes_total          = pod_usage_hourly.memory_bytes_total + $3
;

-- name: UpsertPodUsedCPU :batchexec
insert into pod_usage_hourly (pod_uid, timestamp, cpu_cores_max, cpu_cores_min, cpu_cores_total,
                              cpu_cores_total_readings)
values ($1, $2, $3, $3, $3, 1)
on conflict (pod_uid, timestamp)
    do update set cpu_cores_total_readings = pod_usage_hourly.cpu_cores_total_readings + 1,
                  cpu_cores_max            = case
                                                 when pod_usage_hourly.cpu_cores_max > $3
                                                     then pod_usage_hourly.cpu_cores_max
                                                 else $3 end,
                  cpu_cores_min            = case
                                                 when pod_usage_hourly.cpu_cores_min < $3 and
                                                      pod_usage_hourly.cpu_cores_min != 0
                                                     then pod_usage_hourly.cpu_cores_min
                                                 else $3 end,
                  cpu_cores_total          = pod_usage_hourly.cpu_cores_total + $3
;

-- name: ListPodUsageHourly :many
select *
from pod_usage_hourly
order by timestamp desc
limit 100;


-- name: WorkloadData :many
select *
from pod_usage_hourly
order by case
             when sqlc.arg(orderBy)::text = 'name_desc' then name
             end desc,
            case
                when sqlc.arg(orderBy)::text = 'name_asc' then name
                end asc,
            case
                when sqlc.arg(orderBy)::text = 'namespace_desc' then namespace
                end desc,
            case
                when sqlc.arg(orderBy)::text = 'namespace_asc' then namespace
                end asc
;
