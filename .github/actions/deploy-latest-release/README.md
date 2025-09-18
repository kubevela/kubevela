# Install Latest KubeVela Release Action

This GitHub composite action installs the latest stable KubeVela release from the official Helm repository and verifies its deployment status.

## What it does

- Discovers the latest stable KubeVela release tag from GitHub
- Adds and updates the official KubeVela Helm chart repository
- Installs KubeVela into the `vela-system` namespace (using Helm)
- Verifies pod status and deployment rollout for successful installation

## Usage

```yaml
- name: Install Latest KubeVela Release
  uses: ./path/to/this/action
```

## Requirements

- Helm, kubectl, jq, and curl must be available in your runner environment
- Kubernetes cluster access

## Steps performed

1. **Release Tag Discovery:** Fetches latest stable tag (without `v` prefix)
2. **Helm Repo Setup:** Adds/updates KubeVela Helm chart repo
3. **Install KubeVela:** Installs latest release in the `vela-system` namespace
4. **Status Verification:** Checks pod status and rollout for readiness

---

