## Views


### View: pod_controller

Provides an information about pod controller. Returns controller of controller if present (useful for cronjob and deployment).

| Column          | Type | Description                                                   |
|-----------------|------|---------------------------------------------------------------|
| pod_uid         | UUID | Unique identifier for the Pod                                 |
| name            | TEXT | Name of the Pod                                               |
| namespace       | TEXT | Namespace where the Pod resides                               |
| controller_uid  | UUID | Unique identifier for the controller                          |
| controller_kind | TEXT | Type of the controller (ReplicaSet, Job, Deployment, CronJob) |
| controller_name | TEXT | Name of the controller                                        |


### View: cost_pod_hourly

Calculates and aggregates the hourly costs and resource utilization metrics for each Pod.


| Column               | Type      | Description                                                  |
|----------------------|-----------|--------------------------------------------------------------|
| timestamp            | TIMESTAMP | The time at which the data was recorded, rounded to the hour |
| pod_uid              | UUID      | Unique identifier for the Pod                                |
| namespace            | TEXT      | Namespace where the Pod is located                           |
| name                 | TEXT      | Name of the Pod                                              |
| node_name            | TEXT      | Node where the Pod is running                                |
| created_at           | TIMESTAMP | Creation time of the Pod                                     |
| started_at           | TIMESTAMP | Start time of the Pod                                        |
| deleted_at           | TIMESTAMP | Deletion time of the Pod, NULL if not deleted                |
| request_memory_bytes | DOUBLE    | Memory requested by the Pod in bytes                         |
| request_cpu_cores    | DOUBLE    | CPU cores requested by the Pod                               |
| labels               | JSONB     | Labels attached to the Pod                                   |
| annotations          | JSONB     | Annotations attached to the Pod                              |
| controller_uid       | UUID      | Unique identifier for the controller                         |
| controller_kind      | TEXT      | Kind of the controller (e.g., ReplicaSet, Job)               |
| controller_name      | TEXT      | Name of the controller                                       |
| cpu_cores_avg        | DOUBLE    | Average CPU cores used by the Pod                            |
| cpu_cores_max        | DOUBLE    | Maximum CPU cores used by the Pod                            |
| memory_bytes_avg     | DOUBLE    | Average memory used by the Pod in bytes                      |
| memory_bytes_max     | DOUBLE    | Maximum memory used by the Pod in bytes                      |
| pod_hours            | DOUBLE    | Duration the Pod was running in the hour                     |
| memory_cost          | DOUBLE    | Calculated cost for memory usage of the Pod in the hour      |
| cpu_cost             | DOUBLE    | Calculated cost for CPU usage of the Pod in the hour         |


### View: cost_workload_daily

Aggregates costs and resource usage metrics on a per-workload (controller) basis, grouped by day.


| Column                   | Type      | Description                                                    |
|--------------------------|-----------|----------------------------------------------------------------|
| timestamp                | TIMESTAMP | The starting time of the day (00:00) in UTC                    |
| namespace                | TEXT      | Namespace where the controller is located                      |
| controller_kind          | TEXT      | Kind of the controller (e.g., ReplicaSet, Job)                 |
| controller_name          | TEXT      | Name of the controller                                         |
| memory_bytes_avg         | DOUBLE    | Average memory usage (bytes) across Pods in the workload       |
| memory_bytes_max         | DOUBLE    | Maximum memory usage (bytes) in the workload                   |
| request_memory_bytes_avg | DOUBLE    | Average requested memory (bytes) across Pods in the workload   |
| cpu_cores_avg            | DOUBLE    | Average CPU usage (cores) across Pods in the workload          |
| cpu_cores_max            | DOUBLE    | Maximum CPU usage (cores) in the workload                      |
| request_cpu_cores_avg    | DOUBLE    | Average requested CPU (cores) across Pods in the workload      |
| pod_hours                | DOUBLE    | Total Pod hours consumed by the workload                       |
| memory_cost              | DOUBLE    | Total cost for memory consumption in the workload              |
| cpu_cost                 | DOUBLE    | Total cost for CPU consumption in the workload                 |
| total_cost               | DOUBLE    | Total cost for both CPU and memory consumption in the workload |
