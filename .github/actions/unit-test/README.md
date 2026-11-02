# Kubevela K8s Upgrade Unit Test Action

A comprehensive GitHub composite action for running KubeVela Kubernetes upgrade unit tests with coverage reporting and failure diagnostics.

## Inputs

| Input | Description | Required | Default |
|-------|-------------|----------|---------|
| `codecov-token` | Codecov token for uploading coverage reports | ❌ | `''` |
| `codecov-enable` | Enable Codecov coverage upload (`'true'` or `'false'`) | ❌ | `'false'` |

## Quick Start

### Basic Usage

```yaml
name: Unit Tests with Coverage
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v4
        
      - name: Run KubeVela Unit Tests
        uses: ./.github/actions/unit-test
        with:
          codecov-enable: 'true'
          codecov-token: ${{ secrets.CODECOV_TOKEN }}
```