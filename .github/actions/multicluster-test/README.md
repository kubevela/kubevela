# Kubevela K8s Upgrade Multicluster E2E Test Action

A comprehensive GitHub Actions composite action for running Kubevela Kubernetes upgrade multicluster end-to-end tests with automated coverage reporting and failure diagnostics.

## Usage


```yaml
name: Kubevela Multicluster E2E Tests
on:
  push:
    branches: [main, develop]
  pull_request:
    branches: [main]

jobs:
  multicluster-e2e:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout repository
        uses: actions/checkout@v4

      - name: Run Multicluster E2E Tests
        uses: ./.github/actions/kubevela-multicluster-e2e
        with:
          codecov-enable: 'true'
          codecov-token: ${{ secrets.CODECOV_TOKEN }}
```

## Inputs

| Input | Description | Required | Default | Type |
|-------|-------------|----------|---------|------|
| `codecov-token` | Codecov token for uploading coverage reports | No | `''` | string |
| `codecov-enable` | Enable codecov coverage upload | No | `'false'` | string |
