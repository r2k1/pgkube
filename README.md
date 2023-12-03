pgkube captures metrics and usage information from your Kubernetes cluster and stores it in a PostgreSQL database. It helps administrators understand resource utilization and estimate costs.

## Installation

### Using an existing PostgreSQL Database


```sh
curl https://raw.githubusercontent.com/r2k1/pgkube/main/kube/pgkube.yaml --output pgkube.yaml
# Modify pgkube.yaml to adjust the namespace and PostgreSQL connection string (DATABASE_URL).
kubectl apply -f pgkube.yaml
```

### Setting up pgkube with a local PostgreSQL Database

```sh
curl https://raw.githubusercontent.com/r2k1/pgkube/main/kube/postgres.yaml --output postgres.yaml
# Modify postgres.yaml to adjust the namespace, PostgreSQL user, and password.
curl https://raw.githubusercontent.com/r2k1/pgkube/main/kube/pgkube.yaml --output pgkube.yaml
# Modify pgkube.yaml to adjust the namespace and PostgreSQL connection string (DATABASE_URL).
kubectl apply -f postgres.yaml
kubectl apply -f pgkube.yaml
```

### Usage

To query the data stored by pgkube, you can use your preferred PostgreSQL querying tool. As an example, to retrieve the total cost of all workloads in a specific namespace:

```sql
SELECT * FROM cost_workload_daily WHERE namespace = 'kube-system';
```
