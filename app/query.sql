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
                  controller_uid            = $7,
                  controller_kind           = $8,
                  controller_name           = $9,
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
    do update set name        = $2,
                  namespace   = $3,
                  labels      = $4,
                  annotations = $5,
                  controller_uid   = $6,
                  controller_kind  = $7,
                  controller_name  = $8,
                  created_at  = $9,
                  deleted_at  = $10
;

-- name: UpsertJob :exec
insert into job(job_uid, name, namespace, labels, annotations, controller_uid, controller_kind, controller_name, created_at,
                deleted_at)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
on conflict (job_uid)
    do update set name        = $2,
                  namespace   = $3,
                  labels      = $4,
                  annotations = $5,
                  controller_uid   = $6,
                  controller_kind  = $7,
                  controller_name  = $8,
                  created_at  = $9,
                  deleted_at  = $10
;


-- name: UpsertPodUsedMemory :batchexec
insert into pod_usage_hourly (timestamp, namespace, name, node_name, memory_bytes_max, memory_bytes_min,
                              memory_bytes_total,
                              memory_bytes_total_readings)
values ($1, $2, $3, $4, $5, $5, $5, 1)
on conflict (timestamp, namespace, name, node_name)
    do update set memory_bytes_total_readings = pod_usage_hourly.memory_bytes_total_readings + 1,
                  memory_bytes_max            = case
                                                    when pod_usage_hourly.memory_bytes_max > $5
                                                        then pod_usage_hourly.memory_bytes_max
                                                    else $5 end,
                  memory_bytes_min            = case
                                                    when pod_usage_hourly.memory_bytes_min < $5 and
                                                         pod_usage_hourly.memory_bytes_min != 0
                                                        then pod_usage_hourly.memory_bytes_min
                                                    else $5 end,
                  memory_bytes_total          = pod_usage_hourly.memory_bytes_total + $5
;

-- name: UpsertPodUsedCPU :batchexec
insert into pod_usage_hourly (timestamp, namespace, name, node_name, cpu_cores_max, cpu_cores_min, cpu_cores_total,
                              cpu_cores_total_readings)
values ($1, $2, $3, $4, $5, $5, $5, 1)
on conflict (timestamp, namespace, name, node_name)
    do update set cpu_cores_total_readings = pod_usage_hourly.cpu_cores_total_readings + 1,
                  cpu_cores_max            = case
                                                 when pod_usage_hourly.cpu_cores_max > $5
                                                     then pod_usage_hourly.cpu_cores_max
                                                 else $5 end,
                  cpu_cores_min            = case
                                                 when pod_usage_hourly.cpu_cores_min < $5 and
                                                      pod_usage_hourly.cpu_cores_min != 0
                                                     then pod_usage_hourly.cpu_cores_min
                                                 else $5 end,
                  cpu_cores_total          = pod_usage_hourly.cpu_cores_total + $5
;

-- name: ListPodUsageHourly :many
select * from pod_usage_hourly order by timestamp desc limit 100;
