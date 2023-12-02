create table object
(
    kind       text  not null,
    metadata   jsonb not null default '{}',
    spec       jsonb not null default '{}',
    status     jsonb not null default '{}',
    deleted_at timestamp with time zone
);

create unique index idx_unique_metadata_uid on object ((metadata ->> 'uid'));

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
with recursive controller_hierarchy as (select metadata ->> 'uid' as object_uid,
        jsonb_array_elements(metadata -> 'ownerReferences') as owner_ref,
        1 as level,
        kind
        from object
        where metadata -> 'ownerReferences' is not null
        union all
select ch.object_uid,
       k_owner_ref,
       ch.level + 1 as level,
       ch.kind
from controller_hierarchy ch
         join object k on ch.owner_ref ->> 'uid' = k.metadata ->> 'uid'
    cross join lateral jsonb_array_elements(k.metadata -> 'ownerReferences') k_owner_ref
where ch.owner_ref ->> 'controller' = 'true')
select kind,
       object_uid::uuid as object_uid,
       (owner_ref ->> 'uid')::uuid as controller_uid,
    owner_ref ->> 'kind' as controller_type,
    owner_ref ->> 'name' as controller_name,
    max (level) over (partition by object_uid) as controller_level
from controller_hierarchy
where level = (select max (level) from controller_hierarchy ch2 where ch2.object_uid = controller_hierarchy.object_uid)
order by object_uid;


create view pod as
select t.metadata ->> 'name' AS pod_name,
        (t.metadata ->> 'uid')::uuid AS pod_uid,
        t.metadata ->> 'namespace' AS namespace,
        (t.status ->> 'startTime')::timestamp AS start_time,
        t.spec ->> 'nodeName' AS node_name,
        (t.metadata ->> 'labels')::jsonb as labels,
        (t.metadata ->> 'annotations')::jsonb as annotations,
        t.deleted_at::timestamp,
        res.request_memory_bytes,
        res.request_cpu_cores
        from object t
        cross join lateral (
        select sum (
        case
    -- Memory conversion cases
        when container.value -> 'resources' -> 'requests' ->> 'memory' SIMILAR to '%[0-9]+Ei' then
        (substring (container.value -> 'resources' -> 'requests' ->> 'memory' from
        '[0-9]+'):: numeric * 1024 ^ 6)
    -- Similar cases for Pi, Ti, Gi, Mi, Ki, M, K, and default
        end
        ) AS request_memory_bytes,
        sum (
        case
    -- CPU conversion cases
        when container.value -> 'resources' -> 'requests' ->> 'cpu' like '%m' then
        (left (container.value -> 'resources' -> 'requests' ->> 'cpu', -1):: numeric / 1000)
        else
        (container.value -> 'resources' -> 'requests' ->> 'cpu'):: numeric
        end
        ) AS request_cpu_cores
        from jsonb_array_elements(t.spec -> 'containers') as container(value)
        ) AS res
        where kind = 'Pod';

create view cost_pod_hourly as
select *,
       greatest(request_memory_bytes, memory_bytes_avg) *
       (select coalesce(price_memory_gigabyte_hour, default_price_memory_gb_hour) from config) / 1000000000 *
       pod_hours as memory_cost,
       greatest(request_cpu_cores, cpu_cores_avg) *
       (select coalesce(price_cpu_core_hour, default_price_cpu_core_hour) from config) *
       pod_hours as cpu_cost
from (select timestamp,
          pod.pod_uid,
          pod.namespace,
          pod.pod_name,
          pod.node_name,
          pod.start_time,
          pod.deleted_at,
          pod.request_memory_bytes,
          pod.request_cpu_cores,
          pod.labels,
          pod.annotations,
          object_controller.controller_uid,
          object_controller.controller_type as controller_kind,
          object_controller.controller_name,
          cpu_cores_avg,
          cpu_cores_max,
          memory_bytes_avg,
          memory_bytes_max,
          extract(epoch from
          least(pod_usage_hourly.timestamp + interval '1 hour', pod.deleted_at, now()) -
          greatest(pod_usage_hourly.timestamp, pod.start_time)) / 3600 as pod_hours
      from pod_usage_hourly
          inner join pod using (pod_uid)
          inner join object_controller on (pod_uid = object_controller.object_uid::uuid)) pod_usage;
