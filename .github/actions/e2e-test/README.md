# Kubevela K8s Upgrade E2E Test Action

A comprehensive GitHub composite action for running KubeVela Kubernetes upgrade end-to-end (E2E) tests with complete environment setup, multiple test suites, and failure diagnostics.


> **Note**: This action requires the `GO_VERSION` environment variable to be set in your workflow.

## Quick Start

### Basic Usage

```yaml
name: E2E Tests
on: [push, pull_request]

jobs:
  e2e-tests:
    runs-on: ubuntu-latest
    env:
      GO_VERSION: '1.23.8'
    steps:
      - name: Checkout code
        uses: actions/checkout@v4
        
      - name: Run KubeVela E2E Tests
        uses: ./.github/actions/upgrade-e2e-test
```

## Test Flow Diagram

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│ Environment     │    │ E2E Environment  │    │ Test Execution  │
│ Setup           │───▶│ Preparation      │───▶│ (3 Suites)      │
│                 │    │                  │    │                 │
│ • Install tools │    │ • Cleanup        │    │ • API tests     │
│ • Setup Go      │    │ • Core setup     │    │ • Addon tests   │
│ • Dependencies  │    │ • Helm tests     │    │ • General tests │
│ • Build project │    │                  │    │                 │
└─────────────────┘    └──────────────────┘    └─────────────────┘
                                                        │
                                                        ▼
                                                ┌─────────────────┐
                                                │ Diagnostics     │
                                                │ (On Failure)    │
                                                │                 │
                                                │ • Cluster logs  │
                                                │ • System events │
                                                │ • Test artifacts│
                                                └─────────────────┘
```

**Last Updated**: September 1, 2025