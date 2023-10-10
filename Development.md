# Development

Pre-requisites:
- Docker
- [magefile](https://magefile.org/)
- A kubernetes cluster

Run `mage -l` to get a list of useful for development commands.

To build and deploy local version to kubernetes cluster:
```bash
cp magefiles/.env.example magefiles/.env
# Edit .env to match your environment. You may need to create a docker registry.
mage DockerPublish test-tag
mage KubeApply test-tag
```

