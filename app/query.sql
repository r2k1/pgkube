-- name: StopOtherPods :exec
UPDATE pod
SET deleted_at = $1
WHERE deleted_at IS NULL
  AND pod_uid != ALL (sqlc.arg(running_pod_uids)::uuid[]);

-- name: UpsertPod :exec
INSERT INTO pod (pod_uid, name, namespace, node_name, labels, annotations, controller_uid, controller_kind,
                 controller_name, request_cpu_cores, request_memory_bytes, created_at, deleted_at, started_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT (pod_uid)
    DO UPDATE SET name                 = $2,
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
INSERT INTO replica_set (replica_set_uid, name, namespace, labels, annotations, controller_uid, controller_kind,
                         controller_name, created_at, deleted_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (replica_set_uid)
    DO UPDATE SET name        = $2,
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
INSERT INTO job(job_uid, name, namespace, labels, annotations, controller_uid, controller_kind, controller_name, created_at,
                deleted_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (job_uid)
    DO UPDATE SET name        = $2,
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
INSERT INTO pod_usage_hourly (timestamp, namespace, name, node_name, memory_bytes_max, memory_bytes_min,
                              memory_bytes_total,
                              memory_bytes_total_readings)
VALUES ($1, $2, $3, $4, $5, $5, $5, 1)
ON CONFLICT (timestamp, namespace, name, node_name)
    DO UPDATE SET memory_bytes_total_readings = pod_usage_hourly.memory_bytes_total_readings + 1,
                  memory_bytes_max            = CASE
                                                    WHEN pod_usage_hourly.memory_bytes_max > $5
                                                        THEN pod_usage_hourly.memory_bytes_max
                                                    ELSE $5 END,
                  memory_bytes_min            = CASE
                                                    WHEN pod_usage_hourly.memory_bytes_min < $5 AND
                                                         pod_usage_hourly.memory_bytes_min != 0
                                                        THEN pod_usage_hourly.memory_bytes_min
                                                    ELSE $5 END,
                  memory_bytes_total          = pod_usage_hourly.memory_bytes_total + $5
;

-- name: UpsertPodUsedCPU :batchexec
INSERT INTO pod_usage_hourly (timestamp, namespace, name, node_name, cpu_cores_max, cpu_cores_min, cpu_cores_total,
                              cpu_cores_total_readings)
VALUES ($1, $2, $3, $4, $5, $5, $5, 1)
ON CONFLICT (timestamp, namespace, name, node_name)
    DO UPDATE SET cpu_cores_total_readings = pod_usage_hourly.cpu_cores_total_readings + 1,
                  cpu_cores_max            = CASE
                                                 WHEN pod_usage_hourly.cpu_cores_max > $5
                                                     THEN pod_usage_hourly.cpu_cores_max
                                                 ELSE $5 END,
                  cpu_cores_min            = CASE
                                                 WHEN pod_usage_hourly.cpu_cores_min < $5 AND
                                                      pod_usage_hourly.cpu_cores_min != 0
                                                     THEN pod_usage_hourly.cpu_cores_min
                                                 ELSE $5 END,
                  cpu_cores_total          = pod_usage_hourly.cpu_cores_total + $5
;

-- name: ListPodUsageHourly :many
SELECT * FROM pod_usage_hourly ORDER BY timestamp DESC LIMIT 100;
