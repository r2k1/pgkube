create table pod
(
    pod_uid              uuid primary key,
    namespace            text                     not null,
    name                 text                     not null,
    node_name            text                     not null,
    created_at           timestamp with time zone not null default current_timestamp,
    started_at           timestamp with time zone not null default current_timestamp,
    deleted_at           timestamp with time zone null,
    request_cpu_cores    double precision         not null default 0,
    request_memory_bytes double precision         not null default 0,
    controller_kind      text                     not null default '',
    controller_name      text                     not null default '',
    controller_uid       uuid null,
    labels               jsonb                    not null default '{}',
    annotations          jsonb                    not null default '{}'
);

create table replica_set
(
    replica_set_uid uuid primary key,
    namespace       text                     not null,
    name            text                     not null,
    controller_kind text                     not null default '',
    controller_name text                     not null default '',
    controller_uid  uuid null,
    created_at      timestamp with time zone not null default current_timestamp,
    deleted_at      timestamp with time zone null,
    labels          jsonb                    not null default '{}',
    annotations     jsonb                    not null default '{}'
);

create table job
(
    job_uid         uuid primary key,
    namespace       text                     not null,
    name            text                     not null,
    controller_kind text                     not null default '',
    controller_name text                     not null default '',
    controller_uid  uuid null,
    created_at      timestamp with time zone not null default current_timestamp,
    deleted_at      timestamp with time zone null,
    labels          jsonb                    not null default '{}',
    annotations     jsonb                    not null default '{}'
);

create table pod_usage_hourly
(
    pod_uid                     uuid null,
    timestamp                   timestamp with time zone not null,
    namespace                   text                     not null,
    name                        text                     not null,
    node_name                   text                     not null,
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
    primary key (timestamp, namespace, name, node_name)
);


create
or replace function update_pod_uid()
returns trigger as $$
begin
    if
new.pod_uid is not null then return new;
end if;
    -- fetching the pod_uid from the pod table using name and namespace
    new.pod_uid
:= (select pod_uid from pod where name = new.name and namespace = new.namespace and pod.started_at < new.timestamp order by extract(epoch from (new.timestamp - pod.started_at)) asc limit 1);
return new;
end;
$$
language plpgsql;

create trigger trigger_update_pod_uid
    before insert or
update on pod_usage_hourly
    for each row
    execute function update_pod_uid();

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

create view pod_controller as
select pod.pod_uid,
       pod.name,
       pod.namespace,
       coalesce(replica_set.controller_uid, job.controller_uid, pod.controller_uid)    as controller_uid,
       coalesce(replica_set.controller_kind, job.controller_kind, pod.controller_kind) as controller_kind,
       coalesce(replica_set.controller_name, job.controller_name, pod.controller_name) as controller_name
from pod
         left join replica_set
                   on pod.controller_kind = 'replicaset' and pod.controller_uid = replica_set.replica_set_uid
         left join job on pod.controller_kind = 'job' and pod.controller_uid = job.job_uid;

create view cost_pod_hourly as
select *,
       greatest(request_memory_bytes, memory_bytes_avg) *
       (select coalesce(price_memory_gigabyte_hour, default_price_memory_gb_hour) from config) / 1000000000 *
       pod_hours as memory_cost,
       greatest(request_cpu_cores, cpu_cores_avg) *
       (select coalesce(price_cpu_core_hour, default_price_cpu_core_hour) from config) *
       pod_hours as cpu_cost
from (select timestamp, pod.pod_uid, pod.namespace, pod.name, pod.node_name, pod.created_at, pod.started_at, pod.deleted_at, pod.request_memory_bytes, pod.request_cpu_cores, pod.labels, pod.annotations, pod_controller.controller_uid, pod_controller.controller_kind, pod_controller.controller_name, cpu_cores_avg, cpu_cores_max, memory_bytes_avg, memory_bytes_max, extract (epoch from
          least(pod_usage_hourly.timestamp + interval '1 hour', pod.deleted_at, now()) -
          greatest(pod_usage_hourly.timestamp, pod.started_at)) / 3600 as pod_hours
      from pod_usage_hourly
          inner join pod using (pod_uid)
          inner join pod_controller using (pod_uid)) pod_usage;



create view cost_workload_daily as
select date_trunc('day', timestamp) as timestamp,
       namespace,
       controller_kind,
       controller_name,
       sum(memory_bytes_avg * cost_pod_hourly.pod_hours) /
       sum(pod_hours)               as memory_bytes_avg,
       max(memory_bytes_max)        as memory_bytes_max,
       sum(request_memory_bytes * cost_pod_hourly.pod_hours) /
       sum(pod_hours)               as request_memory_bytes_avg,
       sum(cpu_cores_avg * cost_pod_hourly.pod_hours) /
       sum(pod_hours)               as cpu_cores_avg,
       max(cpu_cores_max)           as cpu_cores_max,
       sum(request_cpu_cores * cost_pod_hourly.pod_hours) /
       sum(pod_hours)               as request_cpu_cores_avg,
       sum(pod_hours)               as pod_hours,
       sum(memory_cost)             as memory_cost,
       sum(cpu_cost)                as cpu_cost,
       sum(memory_cost + cpu_cost)  as total_cost
from cost_pod_hourly
group by 1, namespace, controller_kind, controller_name
order by timestamp asc, total_cost desc;

