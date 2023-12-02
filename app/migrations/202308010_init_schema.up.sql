create table object
(
    kind       text  not null,
    metadata   jsonb not null default '{}',
    spec       jsonb not null default '{}',
    status     jsonb not null default '{}',
    deleted_at timestamp with time zone
);

create unique index idx_unique_metadata_uid on object (((metadata ->> 'uid')::uuid));

create table pod_usage_hourly
(
    pod_uid                     uuid                     not null,
    timestamp                   timestamp with time zone not null,
    memory_bytes_max            double precision         not null default 0,
    memory_bytes_min            double precision         not null default 0,
    memory_bytes_total          double precision         not null default 0,
    memory_bytes_total_readings int                      not null default 0,
    memory_bytes_avg            double precision         not null generated always as (case
                                                                                           when memory_bytes_total_readings = 0
                                                                                               then 0
                                                                                           else memory_bytes_total / memory_bytes_total_readings end
        ) stored,
    cpu_cores_max               double precision         not null default 0,
    cpu_cores_min               double precision         not null default 0,
    cpu_cores_total             double precision         not null default 0,
    cpu_cores_total_readings    int                      not null default 0,
    cpu_cores_avg               double precision         not null generated always as (case
                                                                                           when cpu_cores_total_readings = 0
                                                                                               then 0
                                                                                           else cpu_cores_total / cpu_cores_total_readings end) stored,
    primary key (pod_uid, timestamp)
);

create table config
(
    single_row                   boolean primary key       default true,
    default_price_cpu_core_hour  double precision not null default 0,
    default_price_memory_gb_hour double precision not null default 0,
    price_cpu_core_hour          double precision null,
    price_memory_gigabyte_hour   double precision null,
    check (single_row)
);

insert into config (default_price_cpu_core_hour, default_price_memory_gb_hour)
values (0.031611, 0.004237);


create view object_controller as
with controller as (select object_uid,
                           owner_ref ->> 'kind' as controller_kind,
                           owner_ref ->> 'name' as controller_name,
                           (owner_ref ->> 'uid') ::uuid as controller_uid
                    from (select (metadata ->> 'uid')::uuid                          as object_uid, jsonb_array_elements(metadata -> 'ownerReferences') as owner_ref
                          from object) owners
                    where owner_ref ->> 'controller' = 'true')
select controller.object_uid,
       coalesce(controller_controller.controller_kind, controller.controller_kind) as controller_kind,
       coalesce(controller_controller.controller_name, controller.controller_name) as controller_name,
       coalesce(controller_controller.controller_uid, controller.controller_uid)   as controller_uid
from controller
         left join controller controller_controller on controller.controller_uid = controller_controller.object_uid;


CREATE
OR REPLACE FUNCTION convert_memory_to_bytes(input VARCHAR)
RETURNS BIGINT AS $$
DECLARE
num_part BIGINT;
    unit_part
VARCHAR;
    result
BIGINT;
BEGIN
    -- Extract numeric part
    num_part
:= SUBSTRING(input FROM '^[0-9]+')::BIGINT;

    -- Extract unit part
    unit_part
:= SUBSTRING(input FROM '[a-zA-Z]+$');

    -- Calculate bytes based on unit
CASE unit_part
WHEN 'E'  THEN result := num_part * 1000000000000000000;
WHEN 'P'  THEN result := num_part * 1000000000000000;
WHEN 'T'  THEN result := num_part * 1000000000000;
WHEN 'G'  THEN result := num_part * 1000000000;
WHEN 'M'  THEN result := num_part * 1000000;
WHEN 'K'  THEN result := num_part * 1000;
WHEN 'Ei' THEN result := num_part * 1024^6;
WHEN 'Pi' THEN result := num_part * 1024^5;
WHEN 'Ti' THEN result := num_part * 1024^4;
WHEN 'Gi' THEN result := num_part * 1024^3;
WHEN 'Mi' THEN result := num_part * 1024^2;
WHEN 'Ki' THEN result := num_part * 1024;
ELSE result := num_part; -- Assuming no suffix means bytes
END
CASE;

RETURN result;
END;
$$
LANGUAGE plpgsql;


CREATE
OR REPLACE FUNCTION convert_cpu_to_cores(input VARCHAR)
RETURNS NUMERIC AS $$
DECLARE
num_part NUMERIC;
    is_millicore
BOOLEAN;
BEGIN
    -- Check if input is in millicores
    is_millicore
:= input LIKE '%m';

    -- Extract numeric part
    IF
is_millicore THEN
        num_part := SUBSTRING(input FROM '^[0-9]+')::NUMERIC;
ELSE
        num_part := input::NUMERIC;
END IF;

    -- Convert millicores to cores if necessary
    IF
is_millicore THEN
        RETURN num_part / 1000;
ELSE
        RETURN num_part;
END IF;
END;
$$
LANGUAGE plpgsql;


create view pod as
select
    metadata ->> 'name' as pod_name,
    (metadata ->> 'uid')::uuid as pod_uid,
    metadata ->> 'namespace' as namespace,
    (status ->> 'starttime'):: timestamp as start_time,
    spec ->> 'nodeName' as node_name,
    metadata -> 'labels' as labels,
    metadata -> 'annotations' as annotations,
    deleted_at:: timestamp,
    coalesce((select sum (convert_cpu_to_cores(replace(value ::text, '"', ''))) from jsonb_path_query(spec, '$.containers[*].resources.requests.cpu') as value), 0) as request_cpu_cores,
    coalesce((select sum (convert_memory_to_bytes(replace(value ::text, '"', ''))) from jsonb_path_query(spec, '$.containers[*].resources.requests.memory') as value), 0) as request_memory_bytes
from object
where kind = 'Pod';


create view cost_pod_hourly as
select *,
       greatest(request_memory_bytes, memory_bytes_avg) *
       (select coalesce(price_memory_gigabyte_hour, default_price_memory_gb_hour) from config) / 1000000000 *
       pod_hours as memory_cost,
       greatest(request_cpu_cores, cpu_cores_avg) *
       (select coalesce(price_cpu_core_hour, default_price_cpu_core_hour) from config) *
       pod_hours as cpu_cost
from (select timestamp, pod.pod_uid, pod.namespace, pod.pod_name, pod.node_name, pod.start_time, pod.deleted_at, pod.request_memory_bytes, pod.request_cpu_cores, pod.labels, pod.annotations, object_controller.controller_uid, object_controller.controller_kind as controller_kind, object_controller.controller_name, cpu_cores_avg, cpu_cores_max, memory_bytes_avg, memory_bytes_max, extract (epoch from
          least(pod_usage_hourly.timestamp + interval '1 hour', pod.deleted_at, now()) -
          greatest(pod_usage_hourly.timestamp, pod.start_time)) / 3600 as pod_hours
      from pod_usage_hourly
          inner join pod using (pod_uid)
          inner join object_controller
      on (pod_uid = object_controller.object_uid::uuid)) pod_usage;
