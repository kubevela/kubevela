# Setup Kind Cluster Action

A GitHub Action that sets up a Kubernetes testing environment using Kind (Kubernetes in Docker) for E2E testing.

## Inputs

| Input | Description | Required | Default |
|-------|-------------|----------|---------|
| `k8s-version` | Kubernetes version for the kind cluster | No | `v1.31.9` |
| `name` | Name of the kind cluster; a named cluster skips the image load | No | |
| `load-image` | Build the vela-core e2e image and load it into the cluster. Set `'false'` when the job loads a prebuilt image later, as the E2E Test workflow does with `make image-load-archive` | No | `'true'` |

## Quick Start

```yaml
name: E2E Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'
      
      - name: Setup Kind Cluster
        uses: ./.github/actions/setup-kind-cluster
        with:
          k8s-version: 'v1.31.9'
      
      - name: Run tests
        run: |
          kubectl cluster-info
          make test-e2e
```

## What it does

1. **Installs Kind CLI** - Downloads Kind v0.29.0 using Go
2. **Cleans up** - Removes any existing Kind clusters
3. **Creates cluster** - Spins up Kubernetes v1.31.9 cluster
4. **Sets up environment** - Configures KUBECONFIG for kubectl access
5. **Loads images** - Builds and loads Docker images using `make image-load`,
   unless `load-image` is `'false'`. The E2E Test workflow builds the image once
   in a separate job (`make image-archive`) and each suite job loads that
   archive with `make image-load-archive` instead.

## File Structure

The action is defined in [`action.yaml`](action.yaml) in this directory; read
it there rather than from a copy here, so the steps and inputs stay current.