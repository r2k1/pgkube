create table object
(
    uid        uuid primary key,
    kind       text not null,
    namespace  text not null,
    name       text not null,
    data       jsonb not null default '{}',
    deleted_at timestamp with time zone
);

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
    single_row                    boolean primary key       default true,
    default_price_cpu_core_hour   double precision not null default 0,
    default_price_memory_gb_hour  double precision not null default 0,
    default_price_storage_gb_hour double precision not null default 0,
    price_cpu_core_hour           double precision null,
    price_memory_gigabyte_hour    double precision null,
    price_storage_gb_hour         double precision null,
    check (single_row)
);

insert into config (default_price_cpu_core_hour, default_price_memory_gb_hour, default_price_storage_gb_hour)
values (0.031611, 0.004237, 0.00005479452);


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


create or replace function parse_bytes(input varchar) returns bigint as
$$
declare
    num_part  bigint;
    unit_part varchar;
    result    bigint;
begin
    -- Extract numeric part
    num_part := substring(input from '^[0-9]+')::BIGINT;
    -- Extract unit part
    unit_part := substring(input from '[a-zA-Z]+$');
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
end case;

return result;
end;
$$ language plpgsql;


create or replace function parse_cores(input varchar) returns numeric as
$$
declare
num_part     numeric;
    is_millicore boolean;
begin
    -- Check if input is in millicores
    is_millicore := input like '%m';
    -- Extract numeric part
    if is_millicore then num_part := substring(input from '^[0-9]+')::numeric; else num_part := input::numeric; end if;
    -- Convert millicores to cores if necessary
    if is_millicore then return num_part / 1000; else return num_part; end if;
end;
$$ language plpgsql;


create view pod as
select data -> 'metadata' ->> 'name'                                                                           as name,
       uid,
       data -> 'metadata' ->> 'namespace'                                                                      as namespace,
       (data -> 'status' ->> 'starttime'):: timestamp                                                          as start_time,
       data -> 'spec' ->> 'nodeName'                                                                           as node_name,
       data -> 'metadata' -> 'labels'                                                                          as labels,
       data -> 'metadata' -> 'annotations'                                                                     as annotations,
       deleted_at:: timestamp,
       coalesce(( select sum(parse_cores(replace(value ::text, '"', '')))
                  from jsonb_path_query(data, '$.spec.containers[*].resources.requests.cpu') as value ),
                0)                                                                                             as request_cpu_cores,
       coalesce(( select sum(parse_bytes(replace(value ::text, '"', '')))
                  from jsonb_path_query(data, '$.spec.containers[*].resources.requests.memory') as value ),
                0)                                                                                             as request_memory_bytes,
       coalesce(( select sum(parse_bytes(pvc.data -> 'spec' -> 'resources' -> 'requests' ->> 'storage'))
         from object as pvc
         where pvc.kind = 'PersistentVolumeClaim'
           and pvc.name in
               ( select replace(jsonb_path_query(p.data, '$.spec.volumes[*].persistentVolumeClaim.claimName')::text,
                                '"', '')
                 from object
                 where uid = p.uid
                   and kind = 'Pod' )
           and pvc.namespace = p.data -> 'metadata' ->> 'namespace' ), 0)                                       as request_storage_bytes
from object p
where kind = 'Pod';



create view cost_pod_hourly as
select *,
       greatest(request_memory_bytes, memory_bytes_avg) * (select coalesce(price_memory_gigabyte_hour, default_price_memory_gb_hour) from config) / 1000000000 * pod_hours as memory_cost,
       greatest(request_cpu_cores, cpu_cores_avg) * (select coalesce(price_cpu_core_hour, default_price_cpu_core_hour) from config) * pod_hours as cpu_cost,
       request_storage_bytes * (select coalesce(price_storage_gb_hour, default_price_storage_gb_hour) from config) / 1000000000 * pod_hours as storage_cost
from (select timestamp, pod.uid, pod.namespace, pod.name as pod_name, pod.node_name, pod.start_time, pod.deleted_at, pod.request_memory_bytes, pod.request_cpu_cores, pod.request_storage_bytes, pod.labels, pod.annotations, object_controller.controller_uid, object_controller.controller_kind as controller_kind, object_controller.controller_name, cpu_cores_avg, cpu_cores_max, memory_bytes_avg, memory_bytes_max, extract (epoch from
          least(pod_usage_hourly.timestamp + interval '1 hour', pod.deleted_at, now()) -
          greatest(pod_usage_hourly.timestamp, pod.start_time)) / 3600 as pod_hours
      from pod_usage_hourly
          inner join pod on (pod_usage_hourly.pod_uid = pod.uid)
          inner join object_controller
      on (pod_uid = object_controller.uid::uuid)) pod_usage;
