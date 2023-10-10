CREATE TABLE pod
(
    pod_uid              UUID PRIMARY KEY,
    namespace            TEXT                     NOT NULL,
    name                 TEXT                     NOT NULL,
    node_name            TEXT                     NOT NULL,
    created_at           TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at           TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at           TIMESTAMP WITH TIME ZONE NULL,
    request_cpu_cores    DOUBLE PRECISION         NOT NULL DEFAULT 0,
    request_memory_bytes DOUBLE PRECISION         NOT NULL DEFAULT 0,
    controller_kind           TEXT                     NOT NULL DEFAULT '',
    controller_name           TEXT                     NOT NULL DEFAULT '',
    controller_uid            UUID NULL,
    labels               JSONB                    NOT NULL DEFAULT '{}',
    annotations          JSONB                    NOT NULL DEFAULT '{}'
);

CREATE TABLE replica_set
(
    replica_set_uid UUID PRIMARY KEY,
    namespace       TEXT                     NOT NULL,
    name            TEXT                     NOT NULL,
    controller_kind      TEXT                     NOT NULL DEFAULT '',
    controller_name      TEXT                     NOT NULL DEFAULT '',
    controller_uid       UUID NULL,
    created_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMP WITH TIME ZONE NULL,
    labels          JSONB                    NOT NULL DEFAULT '{}',
    annotations     JSONB                    NOT NULL DEFAULT '{}'
);

CREATE TABLE job
(
    job_uid     UUID PRIMARY KEY,
    namespace   TEXT                     NOT NULL,
    name        TEXT                     NOT NULL,
    controller_kind  TEXT                     NOT NULL DEFAULT '',
    controller_name  TEXT                     NOT NULL DEFAULT '',
    controller_uid   UUID NULL,
    created_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at  TIMESTAMP WITH TIME ZONE NULL,
    labels      JSONB                    NOT NULL DEFAULT '{}',
    annotations JSONB                    NOT NULL DEFAULT '{}'
);

CREATE TABLE pod_usage_hourly
(
    pod_uid                     UUID NULL,
    timestamp                   TIMESTAMP WITH TIME ZONE NOT NULL,
    namespace                   TEXT                     NOT NULL,
    name                        TEXT                     NOT NULL,
    node_name                   TEXT                     NOT NULL,
    memory_bytes_max            DOUBLE PRECISION         NOT NULL DEFAULT 0,
    memory_bytes_min            DOUBLE PRECISION         NOT NULL DEFAULT 0,
    memory_bytes_total          DOUBLE PRECISION         NOT NULL DEFAULT 0,
    memory_bytes_total_readings INT                      NOT NULL DEFAULT 0,
    memory_bytes_avg            DOUBLE PRECISION         NOT NULL GENERATED ALWAYS AS (CASE
                                                                                           WHEN memory_bytes_total_readings = 0
                                                                                               THEN 0
                                                                                           ELSE memory_bytes_total / memory_bytes_total_readings END
        ) STORED,
    cpu_cores_max               DOUBLE PRECISION         NOT NULL DEFAULT 0,
    cpu_cores_min               DOUBLE PRECISION         NOT NULL DEFAULT 0,
    cpu_cores_total             DOUBLE PRECISION         NOT NULL DEFAULT 0,
    cpu_cores_total_readings    INT                      NOT NULL DEFAULT 0,
    cpu_cores_avg               DOUBLE PRECISION         NOT NULL GENERATED ALWAYS AS (CASE
                                                                                           WHEN cpu_cores_total_readings = 0
                                                                                               THEN 0
                                                                                           ELSE cpu_cores_total / cpu_cores_total_readings END) STORED,
    PRIMARY KEY (timestamp, namespace, name, node_name)
);


CREATE
OR REPLACE FUNCTION update_pod_uid()
RETURNS TRIGGER AS $$
BEGIN
    IF
NEW.pod_uid IS NOT NULL THEN RETURN NEW;
END IF;
    -- Fetching the pod_uid from the pod table using name and namespace
    NEW.pod_uid
:= (SELECT pod_uid FROM pod WHERE name = NEW.name AND namespace = NEW.namespace AND pod.started_at < NEW.timestamp ORDER BY EXTRACT(EPOCH FROM (NEW.timestamp - pod.started_at)) ASC LIMIT 1);
RETURN NEW;
END;
$$
LANGUAGE plpgsql;

CREATE TRIGGER trigger_update_pod_uid
    BEFORE INSERT OR
UPDATE ON pod_usage_hourly
    FOR EACH ROW
    EXECUTE FUNCTION update_pod_uid();

CREATE TABLE config
(
    single_row                   BOOLEAN PRIMARY KEY       DEFAULT TRUE,
    default_price_cpu_core_hour  DOUBLE PRECISION NOT NULL DEFAULT 0,
    default_price_memory_gb_hour DOUBLE PRECISION NOT NULL DEFAULT 0,
    price_cpu_core_hour          DOUBLE PRECISION NULL,
    price_memory_gigabyte_hour   DOUBLE PRECISION NULL,
    CHECK (single_row)
);

INSERT INTO config (default_price_cpu_core_hour, default_price_memory_gb_hour)
VALUES (0.031611, 0.004237);

CREATE VIEW pod_controller AS
SELECT pod.pod_uid,
       pod.name,
       pod.namespace,
       COALESCE(replica_set.controller_uid, job.controller_uid, pod.controller_uid)    AS controller_uid,
       COALESCE(replica_set.controller_kind, job.controller_kind, pod.controller_kind) AS controller_kind,
       COALESCE(replica_set.controller_name, job.controller_name, pod.controller_name) AS controller_name
FROM pod
         LEFT JOIN replica_set ON pod.controller_kind = 'ReplicaSet' AND pod.controller_uid = replica_set.replica_set_uid
         LEFT JOIN job ON pod.controller_kind = 'Job' AND pod.controller_uid = job.job_uid;

CREATE VIEW cost_pod_hourly AS
SELECT *,
       GREATEST(request_memory_bytes, memory_bytes_avg) *
       (SELECT COALESCE(price_memory_gigabyte_hour, default_price_memory_gb_hour) FROM config) / 1000000000 *
       pod_hours as memory_cost,
       GREATEST(request_cpu_cores, cpu_cores_avg) *
       (SELECT COALESCE(price_cpu_core_hour, default_price_cpu_core_hour) FROM config) *
       pod_hours as cpu_cost
FROM (SELECT timestamp, pod.pod_uid, pod.namespace, pod.name, pod.node_name, pod.created_at, pod.started_at, pod.deleted_at, pod.request_memory_bytes, pod.request_cpu_cores, pod.labels, pod.annotations, pod_controller.controller_uid, pod_controller.controller_kind, pod_controller.controller_name, cpu_cores_avg, cpu_cores_max, memory_bytes_avg, memory_bytes_max, EXTRACT (EPOCH FROM
          LEAST(pod_usage_hourly.timestamp + interval '1 hour', pod.deleted_at, now()) -
          GREATEST(pod_usage_hourly.timestamp, pod.started_at)) / 3600 AS pod_hours
      FROM pod_usage_hourly
          INNER JOIN pod USING (pod_uid)
          INNER JOIN pod_controller USING (pod_uid)) pod_usage;



CREATE VIEW cost_workload_daily AS
SELECT date_trunc('day', timestamp) AS timestamp,
       namespace,
       controller_kind,
       controller_name,
       SUM(memory_bytes_avg * cost_pod_hourly.pod_hours) /
       SUM(pod_hours)               AS memory_bytes_avg,
       MAX(memory_bytes_max)        AS memory_bytes_max,
       SUM(request_memory_bytes * cost_pod_hourly.pod_hours) /
       SUM(pod_hours)               AS request_memory_bytes_avg,
       SUM(cpu_cores_avg * cost_pod_hourly.pod_hours) /
       SUM(pod_hours)               AS cpu_cores_avg,
       MAX(cpu_cores_max)           AS cpu_cores_max,
       SUM(request_cpu_cores * cost_pod_hourly.pod_hours) /
       SUM(pod_hours)               AS request_cpu_cores_avg,
       SUM(pod_hours)               AS pod_hours,
       SUM(memory_cost)             AS memory_cost,
       SUM(cpu_cost)                AS cpu_cost,
       SUM(memory_cost + cpu_cost)  AS total_cost
FROM cost_pod_hourly
GROUP BY 1, namespace, controller_kind, controller_name
ORDER BY timestamp ASC, total_cost DESC;

