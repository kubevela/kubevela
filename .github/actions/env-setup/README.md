# Kubevela Test Environment Setup Action

A GitHub Actions composite action that sets up a complete testing environment for Kubevela projects with Go, Kubernetes tools, and the Ginkgo testing framework.

## Features

- 🛠️ **System Dependencies**: Installs essential build tools (make, gcc, jq, curl, etc.)
- ☸️ **Kubernetes Tools**: Sets up kubectl and Helm for cluster operations
- 🐹 **Go Environment**: Configurable Go version with module caching
- 📦 **Dependency Management**: Downloads and verifies Go module dependencies
- 🧪 **Testing Framework**: Installs Ginkgo v2 for BDD-style testing

## Usage

```yaml
- name: Setup Kubevela Test Environment
  uses: ./path/to/this/action
  with:
    go-version: '1.23.8'      # Optional: Go version (default: 1.23.8)
```

### Example Workflow

```yaml
name: Kubevela Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup Test Environment
        uses: ./path/to/this/action
        with:
          go-version: '1.21'
          
      - name: Run Tests
        run: |
          ginkgo -r ./tests/e2e/
```

## Inputs

| Input | Description | Required | Default | Usage |
|-------|-------------|----------|---------|-------|
| `go-version` | Go version to install and use | No | `1.23.8` | Specify Go version for your project |

## What This Action Installs

### System Tools
- **make**: Build automation tool
- **gcc**: GNU Compiler Collection
- **jq**: JSON processor for shell scripts
- **ca-certificates**: SSL/TLS certificates
- **curl**: HTTP client for downloads
- **gnupg**: GNU Privacy Guard for security

### Kubernetes Ecosystem
- **kubectl**: Kubernetes command-line tool (latest stable)
- **helm**: Kubernetes package manager (latest stable)

### Go Development
- **Go Runtime**: Specified version with module caching enabled
- **Go Modules**: Downloaded and verified dependencies
- **Ginkgo v2.14.0**: BDD testing framework for Go


## Version History

- **v1.0.0** (2025-09-01): Initial release with environment setup