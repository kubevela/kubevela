# Setup Kind Cluster Action

A GitHub Action that sets up a Kubernetes testing environment using Kind (Kubernetes in Docker) for E2E testing.

## Inputs

| Input | Description | Required | Default |
|-------|-------------|----------|---------|
| `k8s-version` | Kubernetes version for the kind cluster | No | `v1.31.9` |

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
5. **Loads images** - Builds and loads Docker images using `make image-load`

## File Structure

Save as `.github/actions/setup-kind-cluster/action.yaml`:

```yaml
name: 'SetUp kind cluster'
description: 'Sets up complete testing environment for Kubevela with Go, Kubernetes tools, and Ginkgo framework for E2E testing.'

inputs:
  k8s-version:
    description: 'Kubernetes version for the kind cluster'
    required: false
    default: 'v1.31.9'

runs:
  using: 'composite'
  steps:
    # ========================================================================
    # Kind cluster Setup
    # ========================================================================
    - name: Setup KinD
      run: |
        go install sigs.k8s.io/kind@v0.29.0
        kind delete cluster || true
        kind create cluster --image=kindest/node:${{ inputs.k8s-version }}
      shell: bash

    - name: Load image
      run: |
        mkdir -p $HOME/tmp/
        TMPDIR=$HOME/tmp/ make image-load
      shell: bash
```