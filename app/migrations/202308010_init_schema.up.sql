create table object
(
    kind       text  not null,
    metadata   jsonb not null default '{}',
    spec       jsonb not null default '{}',
    status     jsonb not null default '{}',
    deleted_at timestamp with time zone
);

create unique index idx_unique_metadata_uid on object ((metadata ->> 'uid') );

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
       object_uid,
       owner_ref ->> 'uid' as controller_uid, owner_ref ->> 'kind' as controller_type, owner_ref ->> 'name' as controller_name, max (level) over (partition by object_uid) as controller_level
from controller_hierarchy
where level = (select max (level) from controller_hierarchy ch2 where ch2.object_uid = controller_hierarchy.object_uid)
order by object_uid;
