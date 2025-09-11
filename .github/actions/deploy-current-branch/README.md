# Deploy Current Branch Action

This GitHub composite action builds a Docker image from the current branch commit and deploys it to a KubeVela cluster for development testing.

## What it does

- Generates a unique image tag from the latest commit hash
- Builds and loads the Docker image into a KinD cluster
- Applies KubeVela CRDs for upgrade safety
- Upgrades the KubeVela Helm release to use the local development image
- Verifies deployment status and the running image version

## Usage

```yaml
- name: Deploy Current Branch
  uses: ./path/to/this/action
```

## Requirements

- Docker, Helm, kubectl, and KinD must be available in your runner environment
- Kubernetes cluster access
- `charts/vela-core/crds` directory with CRDs
- Valid Helm chart at `charts/vela-core`

## Steps performed

1. **Generate commit hash for image tag**
2. **Build & load Docker image into KinD**
3. **Pre-apply chart CRDs**
4. **Upgrade KubeVela using local image**
5. **Verify deployment and image version**

---