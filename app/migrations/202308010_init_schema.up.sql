create table cluster
(
    id         smallserial primary key,
    name       text                     not null unique,
    created_at timestamp with time zone not null default now()
);

create table object
(
    cluster_id smallint                 not null,
    uid        uuid primary key,
    kind       text  not null,
    namespace  text  not null,
    name       text  not null,
    data       jsonb not null default '{}',
    deleted_at timestamp with time zone
);

create table pod_usage_hourly
(
    cluster_id                  smallint                 not null,
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
    single_row                      boolean primary key       default true,
    default_price_cpu_core_hour     double precision not null default 0,
    default_price_memory_byte_hour  double precision not null default 0,
    default_price_storage_byte_hour double precision not null default 0,
    price_cpu_core_hour             double precision null,
    price_memory_byte_hour          double precision null,
    price_storage_byte_hour         double precision null,
    check (single_row)
);

insert into config (default_price_cpu_core_hour, default_price_memory_byte_hour, default_price_storage_byte_hour)
values (0.03398, 0.00456 / (2^30), 0.17 / 30 / 24 / (2^30));
-- core/memory pricing are for google compute on-demand C3  https://cloud.google.com/compute/vm-instance-pricing#general-purpose_machine_type_family
-- storage price is SSD provisioned space https://cloud.google.com/compute/disks-image-pricing#disk


create view object_controller as
with controller as (select uid,
                           owner_ref ->> 'kind' as controller_kind,
        owner_ref ->> 'name' as controller_name,
        (owner_ref ->> 'uid') ::uuid as controller_uid
        from (select uid, jsonb_array_elements(data -> 'metadata' -> 'ownerReferences') as owner_ref
        from object) owners
        where owner_ref ->> 'controller' = 'true')
select controller.uid,
       coalesce(controller_controller.controller_kind, controller.controller_kind) as controller_kind,
       coalesce(controller_controller.controller_name, controller.controller_name) as controller_name,
       coalesce(controller_controller.controller_uid, controller.controller_uid)   as controller_uid
from controller
         left join controller controller_controller on controller.controller_uid = controller_controller.uid;


create
or replace function parse_bytes(input varchar) returns bigint as
$$
declare
num_part  bigint;
    unit_part
varchar;
    result
bigint;
begin
    -- Extract numeric part
    num_part
:= substring(input from '^[0-9]+')::BIGINT;
    -- Extract unit part
    unit_part
:= substring(input from '[a-zA-Z]+$');
    -- Calculate bytes based on unit
case unit_part when 'E' then result := num_part * 1000000000000000000;
when 'P' then result := num_part * 1000000000000000;
when 'T' then result := num_part * 1000000000000;
when 'G' then result := num_part * 1000000000;
when 'M' then result := num_part * 1000000;
when 'K' then result := num_part * 1000;
when 'Ei' then result := num_part * 1024 ^ 6;
when 'Pi' then result := num_part * 1024 ^ 5;
when 'Ti' then result := num_part * 1024 ^ 4;
when 'Gi' then result := num_part * 1024 ^ 3;
when 'Mi' then result := num_part * 1024 ^ 2;
when 'Ki' then result := num_part * 1024;
else result := num_part; -- Assuming no suffix means bytes
end
case;

return result;
end;
$$
language plpgsql;


create
or replace function parse_cores(input varchar) returns numeric as
$$
declare
num_part     numeric;
    is_millicore
boolean;
begin
    -- Check if input is in millicores
    is_millicore
:= input like '%m';
    -- Extract numeric part
    if
is_millicore then num_part := substring(input from '^[0-9]+')::numeric;
else num_part := input::numeric;
end if;
    -- Convert millicores to cores if necessary
    if
is_millicore then return num_part / 1000;
else return num_part;
end if;
end;
$$
language plpgsql;

create view pod as
select *,
       (data -> 'status' ->> 'starttime'):: timestamp as start_time,
        data -> 'spec' ->> 'nodeName' as node_name,
        data -> 'metadata' -> 'labels' as labels,
        data -> 'metadata' -> 'annotations' as annotations,
        coalesce (( select sum (parse_cores(replace(value ::text, '"', '')))
        from jsonb_path_query(data, '$.spec.containers[*].resources.requests.cpu') as value ),
        0) as request_cpu_cores,
        coalesce (( select sum (parse_bytes(replace(value ::text, '"', '')))
        from jsonb_path_query(data, '$.spec.containers[*].resources.requests.memory') as value ),
        0) as request_memory_bytes,
        coalesce (( select sum (parse_bytes(pvc.data -> 'spec' -> 'resources' -> 'requests' ->> 'storage'))
        from object as pvc
        where pvc.kind = 'PersistentVolumeClaim'
        and pvc.name in
        ( select replace(jsonb_path_query(p.data, '$.spec.volumes[*].persistentVolumeClaim.claimName')::text,
        '"', '')
        from object
        where uid = p.uid
        and kind = 'Pod' )
        and pvc.namespace = p.data -> 'metadata' ->> 'namespace' ), 0) as request_storage_bytes
        from object p
        where kind = 'Pod';

create view pod_usage_request_hourly as
select
        timestamp,
            pod.uid,
            pod.cluster_id,
            pod.namespace,
            pod.name,
            pod.node_name,
            pod.request_cpu_cores,
            pod.request_memory_bytes,
            pod.request_storage_bytes,
            pod.labels,
            pod.annotations,
            object_controller.controller_uid,
            object_controller.controller_kind as controller_kind,
            object_controller.controller_name,
            cpu_cores_avg,
            memory_bytes_avg,
            extract (epoch from (least(pod_usage_hourly.timestamp + interval '1 hour', pod.deleted_at, now()) - greatest(pod_usage_hourly.timestamp, pod.start_time)) / 3600) as hours
        from pod_usage_hourly
        inner join pod on (pod_usage_hourly.pod_uid = pod.uid)
        inner join object_controller
        on (pod_uid = object_controller.uid::uuid);

create view node as
select *,
       parse_bytes(object.data -> 'status' -> 'allocatable' ->> 'memory') as allocatable_memory_bytes,
       parse_cores(object.data -> 'status' -> 'allocatable' ->> 'cpu')    as allocatable_cpu_cores,
       parse_bytes(object.data -> 'status' -> 'capacity' ->> 'memory')    as capacity_memory_bytes,
       parse_cores(object.data -> 'status' -> 'capacity' ->> 'cpu')       as capacity_cpu_cores,
       data -> 'metadata' -> 'labels'                                     as labels,
       data -> 'metadata' -> 'annotations'                                as annotations,
       (data -> 'metadata' ->> 'creationTimestamp') ::timestamp as creation_timestamp
from object
where kind = 'Node';

create view node_hourly as
select gs.timestamp                                                                   as timestamp,
       node.*,
       extract(epoch from (least(gs.timestamp + interval '1 hour', node.deleted_at, now()) -
                            greatest(gs.timestamp, node.creation_timestamp))) / 3600 as hours
from node,
     generate_series(date_trunc('hour', ( select min(creation_timestamp) from node )), date_trunc('hour', now()),
                     '1 hour'::interval) gs(timestamp)
where (node.deleted_at is null or gs.timestamp < node.deleted_at);


create view cost_node_idle_hourly as
select node.timestamp                                                                          as timestamp,
       node.uid                                                                                as uid,
       node.cluster_id                                                                         as cluster_id,
       '__idle__'                                                                              as namespace,
       '__idle__'                                                                              as name,
       node.name                                                                               as node_name,
       allocatable_cpu_cores - coalesce(node_usage_hourly.request_cpu_cores, 0)                as request_cpu_cores,
       allocatable_memory_bytes - coalesce(node_usage_hourly.request_memory_bytes, 0)          as request_memory_bytes,
       0                                                                                       as request_storage_bytes,
       node.labels                                                                             as labels,
       node.annotations                                                                        as annotations,
       null::uuid                                                                              as controller_uid,
       '__idle__'                                                                              as controller_kind,
       '__idle__'                                                                              as controller_name,
       allocatable_cpu_cores - coalesce(node_usage_hourly.cpu_cores, 0)                        as cpu_cores_avg,
       allocatable_memory_bytes - coalesce(node_usage_hourly.memory_bytes, 0)                  as memory_bytes_avg,
       node.hours                                                                              as hours,
       (allocatable_cpu_cores - greatest(node_usage_hourly.request_cpu_cores, node_usage_hourly.cpu_cores, 0)) * ( select coalesce(price_cpu_core_hour, default_price_cpu_core_hour) from config ) as cpu_cost,
       (allocatable_memory_bytes - greatest(node_usage_hourly.request_memory_bytes, allocatable_memory_bytes, 0)) * ( select coalesce(price_memory_byte_hour, default_price_memory_byte_hour) from config ) as memory_cost,
       0                                                                                       as storage_cost
from node_hourly node
         left join ( select node_name,
                            timestamp,
                            sum(request_cpu_cores * hours)                    as request_cpu_cores,
                            sum(request_memory_bytes * hours)                 as request_memory_bytes,
                            sum(cpu_cores_avg * hours)                        as cpu_cores,
                            sum(memory_bytes_avg * hours)                     as memory_bytes
                     from pod_usage_request_hourly
                     group by node_name, timestamp ) node_usage_hourly
                   on (node_usage_hourly.node_name = node.name and node_usage_hourly.timestamp = node.timestamp);


create view cost_node_system_hourly as
select timestamp,
       uid                                                                                     as uid,
       cluster_id                                                                              as cluster_id,
       '__system__'                                                                            as namespace,
       '__system__'                                                                            as name,
       name                                                                                    as node_name,
       capacity_cpu_cores - allocatable_cpu_cores                                              as request_cpu_cores,
       capacity_memory_bytes - allocatable_memory_bytes                                        as request_memory_bytes,
       0                                                                                       as request_storage_bytes,
       labels                                                                                  as labels,
       annotations                                                                             as annotations,
       null::uuid                                                                              as controller_uid,
       '__system__'                                                                            as controller_kind,
       '__system__'                                                                            as controller_name,
       0                                                                                       as cpu_cores_avg,
       0                                                                                       as memory_bytes_avg,
       hours                                                                                   as hours,
       hours * (capacity_cpu_cores - allocatable_cpu_cores) *
        ( select coalesce(price_cpu_core_hour, default_price_cpu_core_hour) from config )      as cpu_cost,
       hours * (capacity_memory_bytes - allocatable_memory_bytes) *
       ( select coalesce(price_memory_byte_hour, default_price_memory_byte_hour) from config ) as memory_cost,
       0                                                                                       as storage_cost
from node_hourly;

create view cost_pod_hourly as
select *,
       greatest(request_cpu_cores, cpu_cores_avg) *
       (select coalesce(price_cpu_core_hour, default_price_cpu_core_hour) from config) * hours       as cpu_cost,
       greatest(request_memory_bytes, memory_bytes_avg) *
       (select coalesce(price_memory_byte_hour, default_price_memory_byte_hour) from config) * hours as memory_cost,
       request_storage_bytes * (select coalesce(price_storage_byte_hour, default_price_storage_byte_hour) from config) *
       hours                                                                                         as storage_cost
from pod_usage_request_hourly;

create view cost_hourly as
select *
from cost_pod_hourly
union all
select *
from cost_node_idle_hourly
union all
select *
from cost_node_system_hourly;
