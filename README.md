# pgkube

pgkube is an open-source Kubernetes workload analyzer that stores and processes data in a PostgreSQL database.

[Demo](http://20.69.144.9/workload?col=namespace&col=controller_kind&col=controller_name&col=request_cpu_cores&col=used_cpu_cores&col=request_memory_bytes&col=used_memory_bytes&col=total_cost&col=request_cpu_core_hours&col=used_cpu_core_hours&col=request_memory_gb_hours&col=used_memory_gb_hours&col=label_app&orderby=namespace&range=24h)

## Features

- Collects workload information from a Kubernetes cluster and stores it in a PostgreSQL database.
- Provides a user interface for querying and visualizing the collected data.
- Allows exporting data in CSV format.

## Installation

### Option 1. Setting up pgkube with a new PostgreSQL Database

```sh
curl https://raw.githubusercontent.com/r2k1/pgkube/main/kube/postgres.yaml --output postgres.yaml
# It's highly recommended to modify postgres.yaml and change default PostgreSQL user, and password before applying it
curl https://raw.githubusercontent.com/r2k1/pgkube/main/kube/pgkube.yaml --output pgkube.yaml
# Modify pgkube.yaml to adjust the namespace and PostgreSQL connection string (DATABASE_URL).
kubectl apply -f postgres.yaml
kubectl apply -f pgkube.yaml
```

### Option 2. Using an existing PostgreSQL Database

pgkube automatically creates the necessary tables in the database. It's recommended to create a separate database or schema specifically for pgkube.

```sh
curl https://raw.githubusercontent.com/r2k1/pgkube/main/kube/pgkube.yaml --output pgkube.yaml
# Modify pgkube.yaml to adjust the namespace and PostgreSQL connection string (DATABASE_URL).
kubectl apply -f pgkube.yaml
```

### Check UI

```sh
kubectl port-forward -n pgkube svc/pgkube 8080:80
```

Open [http://localhost:8080](http://localhost:8080 ) in your browser.

Use your favorite tool (such as PgAdmin, DBeaver, Grafana, PowerBI, Tableau, Apache Superset, JetBrains IDE, Redash, etc.) to query the database and analyze the data. You can also use the built-in UI to generate more advanced queries.

A good starting point is:

```sql
SELECT * FROM cost_hourly;
```
