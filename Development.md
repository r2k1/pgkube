# Development


This project utilizes [magefile](https://magefile.org/) to automate various development tasks. To view a list of available commands, run `mage -l`.

## Running locally against a kubernetes cluster

1.
    - Local Server: `docker run --rm -d -p 5432:5432 --name kube-postgres -e POSTGRES_USER=pgkube-user -e POSTGRES_PASSWORD=pgkube-password postgres:15.4-alpine`
    - Existing server in Kubernetes `kubectl port-forward svc/postgres 5432:5432`
2. Modify `app/.env` file. Set `DATABASE_URL` (e.g. `postgres://pgkube-user:pgkube-password@localhost:5432/postgres`)
3. Run locally. `cd app && go run ./...`. A default context from kubeconfig will be used to connect to kubernetes cluster.

## Creating a local kubernetes cluster

```sh
mage recreateKindCluster
```

## Testing docker image.

```bash
cp magefiles/.env.example magefiles/.env
# Edit .env to match your environment. You may need to create a docker registry.
mage DockerPush
mage KubeApply
```
