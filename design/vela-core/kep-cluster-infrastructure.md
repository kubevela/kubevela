# KEP: Cluster Infrastructure Abstraction

**Authors**: KubeVela Maintainers
**Status**: Draft
**Created**: 2024-12-24
**Last Updated**: 2024-12-24

## Table of Contents

- [Introduction](#introduction)
- [Background](#background)
- [Goals](#goals)
- [Non-Goals](#non-goals)
- [Proposal](#proposal)
  - [Core CRDs](#core-crds)
    - [Cluster](#1-cluster)
    - [ClusterPlane](#2-clusterplane)
    - [ClusterPlane Versioning Strategy](#clusterplane-versioning-strategy)
    - [ClusterBlueprint](#3-clusterblueprint)
    - [ClusterBlueprint Versioning Strategy](#clusterblueprint-versioning-strategy)
    - [ClusterRollout (Optional)](#4-clusterrollout-optional---for-emergencymanual-overrides)
    - [ClusterRolloutStrategy](#5-clusterrolloutstrategy)
  - [Cluster Lifecycle Management](#cluster-lifecycle-management)
    - [Mode 1: Provision](#mode-1-provision---create-new-cluster)
    - [Mode 2: Adopt](#mode-2-adopt---take-over-existing-cluster)
    - [Mode 3: Connect](#mode-3-connect---manage-existing-cluster)
    - [ClusterProviderDefinition](#clusterproviderinition)
  - [Definition Types](#definition-types)
  - [Workflow and Rollout](#workflow-and-rollout)
  - [Multi-Tenancy and Team Ownership](#multi-tenancy-and-team-ownership)
  - [Health Checking and Observability](#health-checking-and-observability)
    - [Health Hierarchy](#health-hierarchy)
    - [ObservabilityProviderDefinition](#observabilityproviderdefinition)
    - [Health Check Configuration](#health-check-configuration-in-clusterplane)
  - [Drift Detection and Remediation](#drift-detection-and-remediation)
    - [Drift Detection CLI](#drift-detection-cli)
    - [What-If Blueprint Comparison](#what-if-blueprint-comparison)
    - [Fleet-Wide Drift Analysis](#fleet-wide-drift-analysis)
    - [Drift Exceptions](#drift-exceptions)
- [Use Cases](#use-cases)
- [Edge Cases and Considerations](#edge-cases-and-considerations)
- [API Reference](#api-reference)
- [Implementation Plan](#implementation-plan)

---

## Introduction

This KEP proposes a new set of CRDs that bring OAM's abstraction model to **cluster infrastructure** itself. While KubeVela's `Application` CRD excels at deploying workloads, platform teams today lack a unified, declarative way to:

1. Define and compose cluster-level infrastructure (networking, storage, security, observability)
2. Version and release infrastructure changes across fleet of clusters
3. Enable different platform sub-teams to own their domain (networking team owns ingress, security team owns policies)
4. Roll out infrastructure changes safely with canary, monitoring, and automatic rollback

We introduce five primary CRDs:

- **`Cluster`** - First-class representation of a managed cluster with full inventory and status
- **`ClusterPlane`** - A composable infrastructure layer owned by a team (e.g., networking plane, security plane)
- **`ClusterBlueprint`** - A complete cluster specification composed of multiple ClusterPlanes
- **`ClusterRolloutStrategy`** - Shared rollout strategy that defines wave-based progression across cluster fleet
- **`ClusterRollout`** - (Optional) Imperative rollout for emergency/manual overrides

---

## Background

### The Problem with Application-Centric Only

KubeVela's `Application` CRD is designed for workload deployment. It assumes:

- A cluster already exists and is configured
- Cluster-level infrastructure (CNI, ingress, cert-manager, etc.) is pre-installed
- Platform capabilities are available

But who configures the cluster? Today, platform teams use:

- **Terraform/Pulumi** - For cloud resources, but not Kubernetes-native
- **Helm charts** - No composition, no progressive rollout, version conflicts
- **GitOps tools** - Apply manifests, but no abstraction or rollout strategies
- **Custom scripts** - Fragile, hard to maintain

Each approach lacks:

1. **Composability** - Can't easily combine networking + security + observability
2. **Ownership boundaries** - No clear "this team owns this layer"
3. **Progressive rollout** - All-or-nothing applies, no canary for infra changes
4. **Unified abstraction** - Different tools for different layers

### What Platform Teams Need

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         PLATFORM TEAM STRUCTURE                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐             │
│  │  Networking     │  │    Security     │  │  Observability  │             │
│  │     Team        │  │      Team       │  │      Team       │             │
│  │                 │  │                 │  │                 │             │
│  │  - Ingress      │  │  - OPA/Gatekeeper│ │  - Prometheus   │             │
│  │  - CNI          │  │  - Cert-manager │  │  - Grafana      │             │
│  │  - Service Mesh │  │  - Secrets mgmt │  │  - Logging      │             │
│  │  - DNS          │  │  - Network Pol  │  │  - Tracing      │             │
│  └────────┬────────┘  └────────┬────────┘  └────────┬────────┘             │
│           │                    │                    │                       │
│           ▼                    ▼                    ▼                       │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                        ClusterBlueprint                              │   │
│  │                                                                      │   │
│  │   Composes: NetworkingPlane + SecurityPlane + ObservabilityPlane    │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                         ClusterRollout                               │   │
│  │                                                                      │   │
│  │   Strategy: Canary 10% → 50% → 100%                                 │   │
│  │   Monitoring: Error rate < 1%, Latency p99 < 100ms                  │   │
│  │   Rollback: Automatic on SLO breach                                 │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ▼                                        │
│           ┌────────────┬────────────┬────────────┬────────────┐            │
│           │ cluster-1  │ cluster-2  │ cluster-3  │ cluster-N  │            │
│           └────────────┴────────────┴────────────┴────────────┘            │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Goals

1. **Enable infrastructure-as-code with OAM patterns** - Components, traits, policies, workflows for cluster infrastructure
2. **Full cluster lifecycle management** - Provision new clusters, adopt existing ones, or connect to pre-existing clusters
3. **Team ownership boundaries** - Each ClusterPlane is owned by a team, versioned independently
4. **Composable blueprints** - Combine planes into complete cluster specifications
5. **Progressive rollout** - Canary, blue-green, rolling updates for infrastructure changes
6. **Observability-driven rollout** - Automatic rollback based on SLO breaches
7. **Multi-cluster fleet management** - Apply blueprints across cluster groups
8. **Minimal bootstrapping requirements** - Create clusters with just cloud credentials; everything else is inferred or defaulted
9. **Compatibility with existing KubeVela** - Reuse definition system, workflow engine, policy framework

## Non-Goals

1. **Replacing Application CRD** - This is complementary, not a replacement
2. **Node-level configuration** - We focus on Kubernetes API objects, not OS-level config
3. **Full GitOps implementation** - We provide the CRDs; GitOps tools can manage them
4. **Implementing cloud provider APIs** - We integrate with existing providers (Crossplane, Terraform) rather than reimplementing

---

## Proposal

### Core CRDs

#### 1. Cluster

The `Cluster` CRD is the **first-class representation** of a managed cluster. It provides a single source of truth for cluster state, inventory, and applied infrastructure.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Cluster
metadata:
  name: production-us-east-1
  namespace: vela-system
  labels:
    # Discovery labels
    environment: production
    region: us-east-1
    provider: aws
    tier: standard
spec:
  # Cluster access configuration
  credential:
    # Reference to kubeconfig secret
    secretRef:
      name: production-us-east-1-kubeconfig
      namespace: vela-system
    # Or inline (not recommended for production)
    # kubeconfig: |
    #   apiVersion: v1
    #   kind: Config
    #   ...

  # Which blueprint this cluster should follow
  blueprintRef:
    name: production-standard
    # Optional: pin to specific revision (otherwise uses latest)
    revision: production-standard-v2.3.0

  # Cluster-specific overrides for the blueprint
  patches:
    # Override plane component properties for this cluster
    - plane: networking
      component: ingress-nginx
      properties:
        values:
          controller:
            replicaCount: 5  # This cluster needs more replicas

  # Cluster metadata (synced from actual cluster)
  clusterInfo:
    # These are auto-discovered but can be overridden
    displayName: "Production US East 1"
    description: "Primary production cluster for US East region"
    contactEmail: "platform-us-east@example.com"

  # Rollout strategy reference - determines how this cluster receives updates
  # The cluster's labels determine which wave it belongs to
  rolloutStrategyRef:
    name: production-rollout
    # Optional: cluster-specific overrides
    overrides:
      # Stricter analysis thresholds for this cluster
      analysis:
        metrics:
          - name: error-rate
            thresholds:
              - condition: "< 0.5%"  # Stricter than strategy default
      # Skip certain waves (useful for canary clusters)
      # skipWaves: [non-critical, critical]

  # Maintenance windows for this cluster
  maintenance:
    windows:
      - start: "02:00"
        end: "06:00"
        timezone: "America/New_York"
        days: [Sat, Sun]
    # Allow emergency updates outside window
    allowEmergencyUpdates: true
    # Enforce window strictly (block updates outside window)
    enforceWindow: true

  # Cluster-level policies (in addition to blueprint policies)
  policies:
    - name: backup-retention
      type: velero-backup
      properties:
        schedule: "0 2 * * *"
        retention: "30d"

status:
  # Connection status
  connectionStatus: Connected  # Connected, Disconnected, Unknown
  lastProbeTime: "2024-12-24T10:00:00Z"
  latency: "45ms"

  # Cluster information (auto-discovered)
  clusterInfo:
    kubernetesVersion: "v1.28.5"
    platform: "eks"  # eks, gke, aks, kind, k3s, etc.
    region: "us-east-1"
    nodeCount: 12
    totalCPU: "96"
    totalMemory: "384Gi"
    apiServerEndpoint: "https://XXXXX.eks.amazonaws.com"

  # Applied blueprint status
  blueprint:
    name: production-standard
    revision: production-standard-v2.3.0
    appliedAt: "2024-12-24T08:00:00Z"
    status: Synced  # Synced, OutOfSync, Updating, Failed

  # Per-plane inventory and status
  planes:
    - name: networking
      revision: networking-v2.3.1
      status: Running
      lastUpdated: "2024-12-24T08:00:00Z"
      components:
        - name: ingress-nginx
          type: helm-release
          status: Running
          version: "4.8.3"
          healthy: true
          resources:
            - apiVersion: apps/v1
              kind: Deployment
              name: ingress-nginx-controller
              namespace: ingress-nginx
              ready: "3/3"
            - apiVersion: v1
              kind: Service
              name: ingress-nginx-controller
              namespace: ingress-nginx
              type: LoadBalancer
              externalIP: "52.x.x.x"
        - name: cilium
          type: helm-release
          status: Running
          version: "1.14.4"
          healthy: true
          resources:
            - apiVersion: apps/v1
              kind: DaemonSet
              name: cilium
              namespace: kube-system
              ready: "12/12"
        - name: external-dns
          type: helm-release
          status: Running
          version: "1.14.3"
          healthy: true

    - name: security
      revision: security-v1.8.0
      status: Running
      components:
        - name: cert-manager
          type: helm-release
          status: Running
          version: "1.13.3"
          healthy: true
        - name: gatekeeper
          type: helm-release
          status: Running
          version: "3.14.0"
          healthy: true

    - name: observability
      revision: observability-v3.1.0
      status: Running
      components:
        - name: prometheus-stack
          type: helm-release
          status: Running
          version: "55.5.0"
          healthy: true
        - name: loki
          type: helm-release
          status: Running
          version: "5.41.0"
          healthy: true

  # Aggregated health
  health:
    status: Healthy  # Healthy, Degraded, Unhealthy, Unknown
    planesHealthy: 3
    planesTotal: 3
    componentsHealthy: 8
    componentsTotal: 8

  # Drift detection
  drift:
    detected: false
    lastCheckTime: "2024-12-24T10:00:00Z"
    # If drift detected:
    # driftedResources:
    #   - resource: "Deployment/ingress-nginx-controller"
    #     field: "spec.replicas"
    #     expected: 5
    #     actual: 3

  # Resource usage summary
  resources:
    cpu:
      capacity: "96"
      allocatable: "94"
      requested: "45"
      usage: "32"
    memory:
      capacity: "384Gi"
      allocatable: "380Gi"
      requested: "180Gi"
      usage: "145Gi"
    pods:
      capacity: 1100
      running: 487

  # Conditions
  conditions:
    - type: Connected
      status: "True"
      lastTransitionTime: "2024-12-24T00:00:00Z"
    - type: BlueprintApplied
      status: "True"
      lastTransitionTime: "2024-12-24T08:00:00Z"
    - type: Healthy
      status: "True"
      lastTransitionTime: "2024-12-24T08:05:00Z"
    - type: DriftFree
      status: "True"
      lastTransitionTime: "2024-12-24T10:00:00Z"

  # History of changes
  history:
    - revision: production-standard-v2.3.0
      appliedAt: "2024-12-24T08:00:00Z"
      appliedBy: "rollout/ingress-upgrade-v2.3"
      status: Succeeded
    - revision: production-standard-v2.2.0
      appliedAt: "2024-12-20T08:00:00Z"
      appliedBy: "rollout/security-patch"
      status: Succeeded
```

**Key Design Decisions for Cluster CRD:**

1. **Single source of truth** - All cluster information in one place
2. **Rich inventory** - Full component and resource inventory with versions
3. **Auto-discovery** - Cluster info, node count, versions are discovered automatically
4. **Blueprint binding** - Each cluster declares which blueprint it follows
5. **Override support** - Cluster-specific patches for blueprint customization
6. **Health aggregation** - Roll-up health status from planes and components
7. **History tracking** - Full audit trail of what was applied when

#### 2. ClusterPlane

A `ClusterPlane` represents a composable infrastructure layer, typically owned by one team.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterPlane
metadata:
  name: networking
  namespace: vela-system
  labels:
    plane.oam.dev/owner: networking-team
    plane.oam.dev/category: networking
spec:
  # Version of this plane (semantic versioning required)
  version: "2.3.1"

  # Description for documentation
  description: "Core networking infrastructure including ingress, CNI, and service mesh"

  # Changelog for this version (optional but recommended)
  changelog: |
    ## 2.3.1
    - Updated ingress-nginx to 4.8.3 (security patch)
    - Fixed Cilium hubble relay configuration

    ## 2.3.0
    - Added external-dns component
    - Upgraded Cilium to 1.14.4

  # Components that make up this plane (like Application components)
  components:
    - name: ingress-nginx
      type: helm-release
      properties:
        chart: ingress-nginx
        repo: https://kubernetes.github.io/ingress-nginx
        version: "4.8.3"
        namespace: ingress-nginx
        values:
          controller:
            replicaCount: 2
            metrics:
              enabled: true
      traits:
        - type: resource-quota
          properties:
            cpu: "2"
            memory: "4Gi"

    - name: cilium
      type: helm-release
      properties:
        chart: cilium
        repo: https://helm.cilium.io/
        version: "1.14.4"
        namespace: kube-system
        values:
          hubble:
            enabled: true
            relay:
              enabled: true

    - name: external-dns
      type: helm-release
      properties:
        chart: external-dns
        repo: https://kubernetes-sigs.github.io/external-dns/
        version: "1.14.3"
        namespace: external-dns

  # Plane-level policies (applied to all components in this plane)
  policies:
    - name: health-check
      type: health
      properties:
        probeTimeout: 300s
        probeInterval: 10s

    - name: dependency-order
      type: apply-order
      properties:
        # Cilium must be ready before ingress
        rules:
          - component: ingress-nginx
            dependsOn: [cilium]

  # Outputs exposed to other planes or blueprints
  outputs:
    - name: ingressClass
      valueFrom:
        component: ingress-nginx
        fieldPath: status.ingressClassName

    - name: clusterDNS
      valueFrom:
        component: external-dns
        fieldPath: status.dnsZone

status:
  phase: Running  # Pending, Provisioning, Running, Failed, Updating

  # Current active revision
  currentRevision: networking-v2.3.1
  currentVersion: "2.3.1"

  # Revision history (immutable snapshots)
  revisions:
    - name: networking-v2.3.1
      version: "2.3.1"
      created: "2024-12-24T10:00:00Z"
      createdBy: "jane@company.com"
      digest: "sha256:abc123..."  # Hash of spec for integrity
      changelog: "Updated ingress-nginx to 4.8.3 (security patch)"
      active: true  # Currently deployed

    - name: networking-v2.3.0
      version: "2.3.0"
      created: "2024-12-20T14:30:00Z"
      createdBy: "bob@company.com"
      digest: "sha256:def456..."
      changelog: "Added external-dns component"
      active: false

    - name: networking-v2.2.0
      version: "2.2.0"
      created: "2024-11-15T09:00:00Z"
      createdBy: "jane@company.com"
      digest: "sha256:ghi789..."
      changelog: "Upgraded Cilium to 1.14.x"
      active: false

  # How many revisions to keep
  revisionHistoryLimit: 10

  components:
    - name: ingress-nginx
      healthy: true
      message: "3/3 replicas ready"
    - name: cilium
      healthy: true
      message: "DaemonSet running on all nodes"
    - name: external-dns
      healthy: true
      message: "1/1 replicas ready"
  outputs:
    ingressClass: nginx
    clusterDNS: "cluster.example.com"
  observedGeneration: 3
  lastUpdated: "2024-12-24T10:00:00Z"
```

##### ClusterPlane Versioning Strategy

ClusterPlane uses semantic versioning with immutable revisions:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      CLUSTERPLANE VERSIONING MODEL                           │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  VERSION SEMANTICS (SemVer):                                                │
│  ───────────────────────────                                                │
│  MAJOR.MINOR.PATCH (e.g., 2.3.1)                                           │
│                                                                             │
│  • MAJOR: Breaking changes (component removed, incompatible config)         │
│  • MINOR: New features (new component added, new capability)                │
│  • PATCH: Bug fixes, security patches, version bumps                        │
│                                                                             │
│  REVISION MODEL:                                                            │
│  ──────────────────                                                         │
│  • Each version change creates an immutable revision                        │
│  • Revision name: {plane-name}-v{version} (e.g., networking-v2.3.1)        │
│  • Revisions are stored in status.revisions                                 │
│  • Blueprints can pin to specific revisions                                 │
│                                                                             │
│  MUTABLE vs IMMUTABLE:                                                      │
│  ─────────────────────                                                      │
│  • spec.version change → creates new revision (immutable snapshot)          │
│  • spec change without version change → REJECTED (must bump version)        │
│  • Exception: spec.description, metadata changes allowed without bump       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Version Bump Enforcement:**

```yaml
# Admission webhook validates version changes
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: clusterplane-version-validation
webhooks:
  - name: version.clusterplane.oam.dev
    rules:
      - apiGroups: ["core.oam.dev"]
        resources: ["clusterplanes"]
        operations: ["UPDATE"]
    # Rejects updates where spec changes but version doesn't
```

**How Teams Set Versions:**

```yaml
# networking-team updates the plane
apiVersion: core.oam.dev/v1beta1
kind: ClusterPlane
metadata:
  name: networking
spec:
  # Team bumps version when making changes
  version: "2.4.0"  # Was 2.3.1

  # Changelog documents what changed
  changelog: |
    ## 2.4.0
    - Added Gateway API support
    - Upgraded ingress-nginx to 4.9.0
    - BREAKING: Removed legacy annotation support

  components:
    - name: ingress-nginx
      properties:
        version: "4.9.0"  # Updated
    # ... rest of components
```

**Referencing Versions in Blueprints:**

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterBlueprint
metadata:
  name: production-standard
spec:
  planes:
    # Option 1: Pin to exact revision (recommended for production)
    - name: networking
      ref:
        name: networking
        revision: networking-v2.3.1  # Explicit revision

    # Option 2: Pin to version (resolves to revision)
    - name: security
      ref:
        name: security
        version: "1.8.0"  # Resolves to security-v1.8.0

    # Option 3: Use latest (for dev/staging, auto-updates)
    - name: observability
      ref:
        name: observability
        # No revision or version = latest

    # Option 4: Version constraint (auto-upgrade within range)
    - name: storage
      ref:
        name: storage
        versionConstraint: ">=1.0.0 <2.0.0"  # Any 1.x version
```

**CLI Commands for Versioning:**

```bash
# List all revisions of a plane
$ vela plane revisions networking

REVISION              VERSION   CREATED                 BY                  ACTIVE
networking-v2.3.1     2.3.1     2024-12-24 10:00:00    jane@company.com    ✓
networking-v2.3.0     2.3.0     2024-12-20 14:30:00    bob@company.com
networking-v2.2.0     2.2.0     2024-11-15 09:00:00    jane@company.com

# Show diff between versions
$ vela plane diff networking --from v2.3.0 --to v2.3.1

--- networking-v2.3.0
+++ networking-v2.3.1
@@ spec.components[0].properties @@
-  version: "4.8.2"
+  version: "4.8.3"

@@ spec.components[1].properties.values.hubble.relay @@
+  enabled: true

# Rollback to previous version (creates new revision)
$ vela plane rollback networking --to-revision networking-v2.3.0

Rolling back networking to v2.3.0...
  → Creating new revision networking-v2.3.2 (based on v2.3.0)
  → Version will be 2.3.2 (patch bump from current)

Proceed? [y/N]: y
✓ Rollback complete. New revision: networking-v2.3.2

# Promote a plane version to blueprints
$ vela plane promote networking --version 2.4.0 --blueprint production-standard

Promoting networking v2.4.0 to blueprint production-standard...
  → Blueprint currently uses: networking-v2.3.1
  → Will update to: networking-v2.4.0

Changes in v2.4.0:
  - Added Gateway API support
  - Upgraded ingress-nginx to 4.9.0
  - BREAKING: Removed legacy annotation support

⚠ This is a MAJOR version change. Proceed with caution.
Proceed? [y/N]:
```

#### 3. ClusterBlueprint

A `ClusterBlueprint` composes multiple `ClusterPlanes` into a complete cluster specification.

**Important**: The ClusterBlueprint defines *what* a cluster should look like. Individual `Cluster` resources declare which blueprint they follow via `spec.blueprintRef`. This inverts the relationship - clusters pull blueprints rather than blueprints pushing to clusters.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterBlueprint
metadata:
  name: production-standard
  namespace: vela-system
  labels:
    tier: production
spec:
  # Version of this blueprint (semantic versioning required)
  version: "2.3.0"

  description: "Standard production cluster configuration"

  # Changelog for this version
  changelog: |
    ## 2.3.0
    - Updated networking plane to v2.3.1 (security patches)
    - Added storage plane for AWS clusters
    - Increased ingress replica count to 3

    ## 2.2.0
    - Added observability plane
    - Updated security plane to v1.8.0

  # Reference planes with optional version pinning
  planes:
    - name: networking
      ref:
        name: networking
        # Optional: pin to specific revision
        revision: networking-v2.3.1
      # Override plane-level settings
      patches:
        - component: ingress-nginx
          properties:
            values:
              controller:
                replicaCount: 3  # Override for production

    - name: security
      ref:
        name: security
        revision: security-v1.8.0

    - name: observability
      ref:
        name: observability

    - name: storage
      ref:
        name: storage
      # Conditional inclusion based on cluster properties
      when: "context.cluster.labels.provider == 'aws'"

  # Blueprint-level policies
  policies:
    - name: resource-governance
      type: resource-limits
      properties:
        maxTotalCPU: "100"
        maxTotalMemory: "200Gi"

  # Blueprint-level workflow (orchestrates plane deployment)
  workflow:
    steps:
      - name: deploy-networking
        type: apply-plane
        properties:
          plane: networking

      - name: deploy-security
        type: apply-plane
        properties:
          plane: security
        # Security can deploy in parallel with networking
        dependsOn: []

      - name: wait-for-core
        type: suspend
        properties:
          duration: "60s"
          message: "Waiting for core infrastructure to stabilize"

      - name: deploy-observability
        type: apply-plane
        properties:
          plane: observability
        dependsOn:
          - deploy-networking
          - deploy-security

      - name: validation
        type: validate-cluster
        properties:
          checks:
            - name: dns-resolution
              type: dns-probe
              endpoint: "kubernetes.default.svc"
            - name: ingress-health
              type: http-probe
              endpoint: "http://ingress-nginx.ingress-nginx.svc/healthz"

status:
  phase: Running

  # Current active revision
  currentRevision: production-standard-v2.3.0
  currentVersion: "2.3.0"

  # Resolved plane revisions for this blueprint version
  planes:
    - name: networking
      revision: networking-v2.3.1
      version: "2.3.1"
      status: Running
    - name: security
      revision: security-v1.8.0
      version: "1.8.0"
      status: Running
    - name: observability
      revision: observability-v3.1.0
      version: "3.1.0"
      status: Running

  # Revision history
  revisions:
    - name: production-standard-v2.3.0
      version: "2.3.0"
      created: "2024-12-24T10:00:00Z"
      createdBy: "sre-team@company.com"
      digest: "sha256:abc123..."
      changelog: "Updated networking plane, added storage plane"
      active: true
      planeRevisions:  # Snapshot of which plane versions were used
        networking: networking-v2.3.1
        security: security-v1.8.0
        observability: observability-v3.1.0
        storage: storage-v1.2.0

    - name: production-standard-v2.2.0
      version: "2.2.0"
      created: "2024-12-01T14:30:00Z"
      createdBy: "platform-lead@company.com"
      digest: "sha256:def456..."
      changelog: "Added observability plane"
      active: false
      planeRevisions:
        networking: networking-v2.2.0
        security: security-v1.8.0
        observability: observability-v3.1.0

  revisionHistoryLimit: 10

  # List of clusters using this blueprint (computed from Cluster CRs)
  clusters:
    total: 5
    byRevision:
      production-standard-v2.3.0: 3  # Already on latest
      production-standard-v2.2.0: 2  # Still updating
    synced: 3
    updating: 2
    failed: 0
  observedGeneration: 5
```

##### ClusterBlueprint Versioning Strategy

ClusterBlueprint versioning follows the same model as ClusterPlane, with additional tracking of composed plane versions:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                   CLUSTERBLUEPRINT VERSIONING MODEL                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  BLUEPRINT VERSION = Composition of plane versions + overrides + policies  │
│                                                                             │
│  Example:                                                                   │
│  ─────────                                                                  │
│  production-standard v2.3.0:                                                │
│    ├── networking-v2.3.1 (pinned)                                          │
│    ├── security-v1.8.0 (pinned)                                            │
│    ├── observability-v3.1.0 (latest at time of creation)                   │
│    ├── storage-v1.2.0 (conditional, AWS only)                              │
│    └── patches: ingress replicaCount=3                                     │
│                                                                             │
│  WHEN TO BUMP BLUEPRINT VERSION:                                            │
│  ─────────────────────────────────                                          │
│  • Change plane reference (pin to different version)                        │
│  • Add/remove a plane                                                       │
│  • Change patches or policies                                               │
│  • Change workflow steps                                                    │
│                                                                             │
│  WHEN NOT TO BUMP (automatic):                                              │
│  ─────────────────────────────────                                          │
│  • Unpinned plane gets new version (tracked in status.planes)              │
│  • Description/metadata changes                                             │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Cluster Reference Options:**

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Cluster
metadata:
  name: production-us-east-1
spec:
  blueprintRef:
    # Option 1: Pin to exact revision (recommended for production)
    name: production-standard
    revision: production-standard-v2.3.0

    # Option 2: Pin to version
    # name: production-standard
    # version: "2.3.0"

    # Option 3: Use latest (for dev/staging clusters)
    # name: production-standard
    # (no revision or version = always use latest)

    # Option 4: Version constraint
    # name: production-standard
    # versionConstraint: ">=2.0.0 <3.0.0"  # Any 2.x version
```

**CLI Commands for Blueprint Versioning:**

```bash
# List all blueprint revisions
$ vela blueprint revisions production-standard

REVISION                        VERSION   CREATED                 CLUSTERS   STATUS
production-standard-v2.3.0      2.3.0     2024-12-24 10:00:00    3/5        Active
production-standard-v2.2.0      2.2.0     2024-12-01 14:30:00    2/5        Updating
production-standard-v2.1.0      2.1.0     2024-11-15 09:00:00    0/5        Archived

# Show what planes are in each blueprint version
$ vela blueprint show production-standard --revision v2.3.0

Blueprint: production-standard v2.3.0
Created: 2024-12-24 10:00:00 by sre-team@company.com

Planes:
  NAME            REVISION              VERSION   PINNED
  networking      networking-v2.3.1     2.3.1     ✓
  security        security-v1.8.0       1.8.0     ✓
  observability   observability-v3.1.0  3.1.0
  storage         storage-v1.2.0        1.2.0     (AWS only)

Patches:
  - networking/ingress-nginx: replicaCount=3

Clusters using this revision: 3
  - production-us-east-1 (synced)
  - production-us-west-2 (synced)
  - production-eu-west-1 (synced)

# Diff between blueprint versions
$ vela blueprint diff production-standard --from v2.2.0 --to v2.3.0

--- production-standard-v2.2.0
+++ production-standard-v2.3.0

Plane changes:
  networking: v2.2.0 → v2.3.1
  + storage: v1.2.0 (new, conditional: AWS only)

Patch changes:
  + networking/ingress-nginx.replicaCount: 3

# Upgrade clusters to new blueprint version
$ vela blueprint upgrade production-standard --to-version 2.3.0 \
    --clusters production-us-east-1,production-us-west-2

Upgrading 2 clusters to production-standard v2.3.0...

Cluster                   Current      Target       Status
production-us-east-1      v2.2.0       v2.3.0       Pending
production-us-west-2      v2.2.0       v2.3.0       Pending

This will trigger ClusterRolloutStrategy: production-canary
  Wave 1: production-us-west-2 (canary)
  Wave 2: production-us-east-1 (after validation)

Proceed? [y/N]:

# Create new blueprint version from current state
$ vela blueprint release production-standard --version 2.4.0 \
    --changelog "Upgraded observability to v4.0.0"

Creating new revision production-standard-v2.4.0...
  → Snapshotting current plane references
  → Recording changelog

✓ Created production-standard-v2.4.0
```

**Relationship between Cluster and ClusterBlueprint:**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     CLUSTER ← BLUEPRINT RELATIONSHIP                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ClusterBlueprint                    Cluster CRs                            │
│  ─────────────────                   ───────────                            │
│                                                                             │
│  ┌─────────────────────┐            ┌─────────────────────┐                │
│  │ production-standard │◄───────────│ production-us-east-1│                │
│  │                     │            │ blueprintRef:       │                │
│  │ planes:             │            │   name: production- │                │
│  │   - networking      │◄───────────│         standard    │                │
│  │   - security        │            └─────────────────────┘                │
│  │   - observability   │                                                   │
│  └─────────────────────┘            ┌─────────────────────┐                │
│           ▲                         │ production-us-west-2│                │
│           │                         │ blueprintRef:       │                │
│           └─────────────────────────│   name: production- │                │
│                                     │         standard    │                │
│                                     └─────────────────────┘                │
│                                                                             │
│  The Blueprint defines WHAT.        Clusters declare WHICH blueprint.      │
│  Clusters reference blueprints.     The Cluster controller reconciles.     │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### 4. ClusterRollout (Optional - For Emergency/Manual Overrides)

> **Note**: With the introduction of `ClusterRolloutStrategy`, the `ClusterRollout` CRD becomes **optional** and is primarily used for:
> - **Emergency rollouts** that bypass normal wave progression
> - **Manual overrides** for specific clusters or cluster groups
> - **One-time operations** that don't follow the standard strategy
>
> For normal operations, clusters reference a `ClusterRolloutStrategy` via `rolloutStrategyRef`. The strategy controller automatically progresses through waves when blueprint updates are detected.

A `ClusterRollout` manages **imperative/emergency** progressive delivery of `ClusterBlueprint` changes, overriding the normal `ClusterRolloutStrategy` behavior.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterRollout
metadata:
  name: production-upgrade-v2.4
  namespace: vela-system
spec:
  # Target blueprint and revision
  targetBlueprint:
    name: production-standard
    revision: production-standard-v2.4.0

  # Source (current state) - auto-detected if not specified
  sourceBlueprint:
    name: production-standard
    revision: production-standard-v2.3.0

  # Rollout strategy
  strategy:
    type: canary  # canary, blueGreen, rolling
    canary:
      # Cluster-level canary (not pod-level)
      steps:
        - weight: 10
          # Which clusters get this batch
          clusterSelector:
            matchLabels:
              canary: "true"
          pause:
            duration: "30m"

        - weight: 50
          clusterSelector:
            matchLabels:
              tier: non-critical
          pause:
            duration: "2h"

        - weight: 100
          # All remaining clusters

  # Analysis and SLO monitoring
  analysis:
    # Metrics to monitor during rollout
    metrics:
      - name: error-rate
        provider: prometheus
        query: |
          sum(rate(nginx_ingress_controller_requests{status=~"5.."}[5m]))
          / sum(rate(nginx_ingress_controller_requests[5m])) * 100
        thresholds:
          - condition: "< 1"      # Must be less than 1%
            failureLimit: 3       # Allow 3 failures before rollback

      - name: p99-latency
        provider: prometheus
        query: |
          histogram_quantile(0.99,
            sum(rate(nginx_ingress_controller_request_duration_seconds_bucket[5m]))
            by (le))
        thresholds:
          - condition: "< 0.5"    # p99 < 500ms
            failureLimit: 2

      - name: pod-restarts
        provider: kubernetes
        query: |
          sum(increase(kube_pod_container_status_restarts_total{namespace=~"ingress-nginx|cert-manager|monitoring"}[10m]))
        thresholds:
          - condition: "< 5"
            failureLimit: 1

    # How often to check metrics
    interval: "1m"

    # Initial delay before starting analysis
    initialDelay: "5m"

  # Rollback configuration
  rollback:
    # Automatic rollback on SLO breach
    automatic: true

    # How to rollback
    strategy: immediate  # immediate, gradual

    # Notification on rollback
    notification:
      - type: slack
        channel: "#platform-alerts"
        template: |
          :rotating_light: Rollout {{.Name}} automatically rolled back
          Reason: {{.Reason}}
          Failed Metric: {{.FailedMetric}}
          Clusters Affected: {{.AffectedClusters}}

  # Manual controls
  paused: false

  # Approval gates
  approvals:
    - stage: "50%"
      approvers:
        - platform-leads
      timeout: "24h"
      autoApproveAfter: "48h"  # Optional: auto-approve if no response

status:
  phase: Progressing  # Pending, Progressing, Paused, Succeeded, Failed, RolledBack

  currentStep: 1
  currentWeight: 10

  clusters:
    - name: production-canary-1
      status: Updated
      revision: production-standard-v2.4.0
      updatedAt: "2024-12-24T10:00:00Z"
    - name: production-us-east-1
      status: Pending
      revision: production-standard-v2.3.0
    - name: production-us-west-2
      status: Pending
      revision: production-standard-v2.3.0

  analysis:
    lastAnalysisTime: "2024-12-24T10:30:00Z"
    metrics:
      - name: error-rate
        value: 0.2
        status: Passing
      - name: p99-latency
        value: 0.12
        status: Passing
      - name: pod-restarts
        value: 0
        status: Passing

  conditions:
    - type: Progressing
      status: "True"
      reason: CanaryStepCompleted
      message: "Canary step 1 (10%) completed successfully"
    - type: AnalysisPassing
      status: "True"
      reason: AllMetricsHealthy
      message: "All SLO metrics within thresholds"

  history:
    - revision: production-standard-v2.3.0
      phase: Succeeded
      startTime: "2024-12-20T10:00:00Z"
      endTime: "2024-12-20T14:00:00Z"
```

#### 5. ClusterRolloutStrategy

A `ClusterRolloutStrategy` defines **how blueprint updates are rolled out across a fleet** of clusters. Clusters reference this strategy via `rolloutStrategyRef`, enabling coordinated updates where Cluster B only updates after Cluster A succeeds.

This design eliminates conflicts between per-cluster update policies and fleet-wide rollouts by having a **single source of truth** for rollout behavior.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterRolloutStrategy
metadata:
  name: production-rollout
  namespace: vela-system
spec:
  description: "Production fleet rollout strategy with wave-based progression"

  # Wave definitions - clusters are grouped into waves by label selector
  waves:
    - name: canary
      order: 1
      description: "Canary clusters for initial validation"
      clusterSelector:
        matchLabels:
          tier: canary
      # Optional: limit how many clusters in this wave
      maxClusters: 2
      # Pause after this wave completes
      pause:
        duration: "4h"
        # Or require manual approval
        # approval:
        #   required: true

    - name: staging
      order: 2
      description: "Staging clusters for extended validation"
      clusterSelector:
        matchLabels:
          tier: staging
      # Only proceed if previous wave succeeded
      waitFor:
        wave: canary
        # How long canary must be healthy before proceeding
        healthyDuration: "4h"
        # What health checks must pass
        healthChecks:
          - type: allClustersHealthy
          - type: analysisPass
      pause:
        duration: "12h"

    - name: non-critical
      order: 3
      description: "Non-critical production clusters"
      clusterSelector:
        matchLabels:
          tier: non-critical
      waitFor:
        wave: staging
        healthyDuration: "12h"
      # Batch updates within this wave
      batching:
        size: 5  # Update 5 clusters at a time
        interval: "30m"  # Wait 30m between batches

    - name: critical
      order: 4
      description: "Critical production clusters - requires approval"
      clusterSelector:
        matchLabels:
          tier: critical
      waitFor:
        wave: non-critical
        healthyDuration: "24h"
      # Require human approval before proceeding
      approval:
        required: true
        approvers:
          - group: platform-leads
          - user: oncall@example.com
        timeout: "48h"
        # Auto-approve if no response (optional)
        # autoApproveAfter: "72h"
      # Extra strict batching for critical
      batching:
        size: 1  # One cluster at a time
        interval: "2h"

  # Maintenance window behavior
  maintenanceWindows:
    # Respect individual cluster maintenance windows
    respectClusterWindows: true
    # If true, skip clusters outside their window (proceed with others)
    # If false, wait for all clusters in wave to be in their window
    skipIfOutsideWindow: true
    # Maximum time to wait for a maintenance window
    maxWaitTime: "168h"  # 1 week

  # Per-cluster rollout behavior (within each cluster)
  clusterUpdateBehavior:
    # How to update components within a single cluster
    strategy: canary  # canary, rolling, blueGreen, allAtOnce
    canary:
      steps:
        - weight: 10
          pause:
            duration: "5m"
        - weight: 50
          pause:
            duration: "15m"
        - weight: 100
    # Timeout for updating a single cluster
    timeout: "30m"

  # Analysis configuration for rollout validation
  analysis:
    # Default metrics applied to all waves
    metrics:
      - name: error-rate
        provider: prometheus
        query: |
          sum(rate(http_requests_total{status=~"5.."}[5m]))
          / sum(rate(http_requests_total[5m])) * 100
        thresholds:
          - condition: "< 1"
            failureLimit: 3
      - name: p99-latency
        provider: prometheus
        query: |
          histogram_quantile(0.99, sum(rate(request_duration_seconds_bucket[5m])) by (le))
        thresholds:
          - condition: "< 0.5"
            failureLimit: 2
      - name: pod-restarts
        provider: kubernetes
        query: |
          sum(increase(kube_pod_container_status_restarts_total[10m]))
        thresholds:
          - condition: "< 5"
    # How often to run analysis
    interval: "1m"
    # Delay before starting analysis after update
    initialDelay: "5m"

  # Rollback configuration
  rollback:
    # Automatic rollback on SLO breach
    automatic: true
    # How to rollback
    strategy: immediate  # immediate, gradual
    # Scope of rollback
    scope: wave  # wave, cluster, fleet
    # Notification on rollback
    notification:
      channels:
        - type: slack
          target: "#platform-alerts"
        - type: pagerduty
          target: "platform-oncall"
      template: |
        :rotating_light: Rollout failed in wave {{.Wave}}
        Cluster: {{.Cluster}}
        Reason: {{.Reason}}
        Failed Metric: {{.FailedMetric}}
        Action: {{.RollbackAction}}

  # Global pausing
  paused: false

status:
  # Current state of the strategy
  phase: Active  # Active, Paused, Superseded

  # Current rollout progress (when a blueprint update is in progress)
  currentRollout:
    blueprintRevision: production-standard-v2.4.0
    previousRevision: production-standard-v2.3.0
    startedAt: "2024-12-24T10:00:00Z"
    currentWave: staging
    waveProgress:
      - wave: canary
        status: Completed
        clustersUpdated: 2
        clustersTotal: 2
        completedAt: "2024-12-24T14:00:00Z"
      - wave: staging
        status: InProgress
        clustersUpdated: 1
        clustersTotal: 3
        startedAt: "2024-12-24T14:00:00Z"
      - wave: non-critical
        status: Pending
        clustersTotal: 8
      - wave: critical
        status: Pending
        clustersTotal: 5
        awaitingApproval: false

  # Clusters referencing this strategy
  clusters:
    total: 18
    byWave:
      canary: 2
      staging: 3
      non-critical: 8
      critical: 5

  # Analysis state
  analysis:
    lastCheckTime: "2024-12-24T15:00:00Z"
    passing: true
    metrics:
      - name: error-rate
        value: 0.2
        status: Passing
      - name: p99-latency
        value: 0.12
        status: Passing

  conditions:
    - type: Ready
      status: "True"
      message: "Strategy is active and being used by 18 clusters"
    - type: RolloutInProgress
      status: "True"
      message: "Rolling out production-standard-v2.4.0, wave 2/4 in progress"
```

**Relationship: Cluster → ClusterRolloutStrategy → ClusterBlueprint**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│              CLUSTER-DRIVEN ROLLOUT WITH SHARED STRATEGY                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ClusterBlueprint                ClusterRolloutStrategy                     │
│  ─────────────────                ───────────────────────                   │
│  "What to deploy"                 "How to roll out"                         │
│                                                                             │
│  ┌─────────────────┐              ┌─────────────────────┐                  │
│  │ production-     │              │ production-rollout  │                  │
│  │ standard        │              │                     │                  │
│  │                 │              │ waves:              │                  │
│  │ revision: v2.4  │              │  1. canary          │                  │
│  │                 │              │  2. staging         │                  │
│  │ planes:         │              │  3. non-critical    │                  │
│  │  - networking   │              │  4. critical        │                  │
│  │  - security     │              │                     │                  │
│  │  - observability│              │ analysis:           │                  │
│  └────────┬────────┘              │  - error-rate < 1%  │                  │
│           │                       │  - p99 < 500ms      │                  │
│           │                       └──────────┬──────────┘                  │
│           │                                  │                              │
│           │              ┌───────────────────┼───────────────────┐         │
│           │              │                   │                   │         │
│           │              ▼                   ▼                   ▼         │
│           │     ┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐│
│           │     │ cluster-canary  │ │ cluster-staging │ │ cluster-prod-1  ││
│           │     │                 │ │                 │ │                 ││
│           │     │ tier: canary    │ │ tier: staging   │ │ tier: critical  ││
│           │     │                 │ │                 │ │                 ││
│           └────►│ blueprintRef:   │ │ blueprintRef:   │ │ blueprintRef:   ││
│                 │   production-   │ │   production-   │ │   production-   ││
│                 │   standard      │ │   standard      │ │   standard      ││
│                 │                 │ │                 │ │                 ││
│                 │ rolloutStrategy │ │ rolloutStrategy │ │ rolloutStrategy ││
│                 │ Ref: production │ │ Ref: production │ │ Ref: production ││
│                 │ -rollout        │ │ -rollout        │ │ -rollout        ││
│                 │                 │ │                 │ │                 ││
│                 │ maintenance:    │ │ maintenance:    │ │ maintenance:    ││
│                 │  anytime        │ │  weekends       │ │  Sat 2-6am      ││
│                 └─────────────────┘ └─────────────────┘ └─────────────────┘│
│                         │                   │                   │          │
│                         │                   │                   │          │
│  WAVE 1 ───────────────►│                   │                   │          │
│  Updates immediately    │                   │                   │          │
│                         │                   │                   │          │
│  WAVE 2 ────────────────┼──────────────────►│                   │          │
│  Waits 4h after canary  │                   │                   │          │
│  is healthy             │                   │                   │          │
│                         │                   │                   │          │
│  WAVE 4 ────────────────┼───────────────────┼──────────────────►│          │
│  Waits for approval     │                   │                   │          │
│  + maintenance window   │                   │                   │          │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

### Cluster Lifecycle Management

A key design goal is supporting the **full cluster lifecycle** - from provisioning brand new clusters to adopting existing ones. The `Cluster` CRD supports three modes of operation.

#### Cluster Modes

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         CLUSTER LIFECYCLE MODES                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  MODE 1: PROVISION                MODE 2: ADOPT                             │
│  ─────────────────                ────────────────                          │
│  "Create a new cluster            "Take over an existing                    │
│   from scratch"                    cluster created elsewhere"               │
│                                                                             │
│  ┌─────────────────┐              ┌─────────────────┐                      │
│  │ Cloud Creds     │              │ Kubeconfig OR   │                      │
│  │ + Region        │              │ Terraform State │                      │
│  │ + Blueprint     │              │ + Blueprint     │                      │
│  └────────┬────────┘              └────────┬────────┘                      │
│           │                                │                                │
│           ▼                                ▼                                │
│  ┌─────────────────┐              ┌─────────────────┐                      │
│  │ VPC Created     │              │ Discovery &     │                      │
│  │ EKS Provisioned │              │ Inventory Scan  │                      │
│  │ Nodes Launched  │              │ State Import    │                      │
│  └────────┬────────┘              └────────┬────────┘                      │
│           │                                │                                │
│           ▼                                ▼                                │
│  ┌─────────────────┐              ┌─────────────────┐                      │
│  │ Blueprint       │              │ Blueprint       │                      │
│  │ Applied         │              │ Reconciled      │                      │
│  └─────────────────┘              └─────────────────┘                      │
│                                                                             │
│  MODE 3: CONNECT                                                            │
│  ───────────────                                                            │
│  "Just manage what's in the cluster, don't provision infrastructure"       │
│                                                                             │
│  ┌─────────────────┐                                                       │
│  │ Kubeconfig      │                                                       │
│  │ + Blueprint     │                                                       │
│  │ (optional)      │                                                       │
│  └────────┬────────┘                                                       │
│           │                                                                 │
│           ▼                                                                 │
│  ┌─────────────────┐                                                       │
│  │ Inventory Scan  │                                                       │
│  │ Blueprint Apply │                                                       │
│  └─────────────────┘                                                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### Mode 1: Provision - Create New Cluster

Create a brand new cluster with minimal input. Only cloud credentials and desired region are required; everything else uses smart defaults.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Cluster
metadata:
  name: production-us-east-1
  namespace: vela-system
spec:
  # MODE: Provision new cluster
  mode: provision

  # Cloud provider configuration
  provider:
    type: aws  # aws, gcp, azure, kind, k3s

    # Reference to cloud credentials secret
    credentialRef:
      name: aws-platform-credentials
      namespace: vela-system

    # Region (only required field besides credentials)
    region: us-east-1

    # Everything below is OPTIONAL with smart defaults

  # Cluster specification (all optional - uses defaults)
  clusterSpec:
    # Kubernetes version (default: latest stable)
    kubernetesVersion: "1.28"

    # Node configuration (default: managed node group with reasonable sizes)
    nodePools:
      - name: default
        instanceType: m5.large    # Default based on blueprint requirements
        minSize: 3                # Default: 3
        maxSize: 10               # Default: 10
        # If not specified, auto-calculated from blueprint resource requirements

    # Networking (default: create new VPC with standard CIDR)
    networking:
      # Leave empty to auto-create VPC
      # vpcId: vpc-xxx          # Optional: use existing VPC
      # subnetIds: [...]        # Optional: use existing subnets

      # CIDR ranges (defaults if VPC is auto-created)
      vpcCidr: "10.0.0.0/16"
      podCidr: "10.244.0.0/16"
      serviceCidr: "10.96.0.0/12"

    # Additional features (defaults based on blueprint)
    features:
      # Auto-enabled based on blueprint planes
      privateEndpoint: true
      publicEndpoint: false
      logging:
        - api
        - audit
        - authenticator

  # Blueprint to apply after provisioning
  blueprintRef:
    name: production-standard

  # Provisioning workflow (optional - uses default)
  provisionWorkflow:
    # Default workflow: provision → wait → connect → apply blueprint
    # Can be customized for special requirements
    timeout: "30m"

status:
  mode: provision
  provisioningStatus:
    phase: Provisioning  # Pending, Provisioning, Ready, Failed

    # Provisioning progress
    infrastructure:
      vpc:
        status: Created
        id: vpc-0123456789
        cidr: "10.0.0.0/16"
      subnets:
        - id: subnet-aaa
          az: us-east-1a
          cidr: "10.0.1.0/24"
        - id: subnet-bbb
          az: us-east-1b
          cidr: "10.0.2.0/24"
      securityGroups:
        - id: sg-xxx
          name: production-us-east-1-cluster

    cluster:
      status: Creating
      arn: "arn:aws:eks:us-east-1:123456789:cluster/production-us-east-1"
      endpoint: ""  # Populated when ready

    nodePools:
      - name: default
        status: Pending
        desiredSize: 3
        readyNodes: 0

    # Timeline
    startedAt: "2024-12-24T10:00:00Z"
    estimatedCompletion: "2024-12-24T10:25:00Z"

    # Events
    events:
      - time: "2024-12-24T10:00:00Z"
        message: "Started VPC creation"
      - time: "2024-12-24T10:02:00Z"
        message: "VPC created, starting subnet creation"
```

**Minimal Provision Example** - Just credentials and region:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Cluster
metadata:
  name: dev-cluster
spec:
  mode: provision
  provider:
    type: aws
    credentialRef:
      name: aws-credentials
    region: us-west-2
  blueprintRef:
    name: dev-minimal

# That's it! Everything else uses smart defaults:
# - VPC: Auto-created with 10.0.0.0/16
# - Subnets: 3 AZs, public + private
# - Node pool: 3x m5.large (auto-scaled 3-10)
# - K8s version: Latest stable
# - Security: Private endpoint, no public access
```

#### Mode 2: Adopt - Take Over Existing Cluster

Adopt a cluster that was created by Terraform, CloudFormation, or manually. Vela discovers existing infrastructure and takes over management.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Cluster
metadata:
  name: legacy-production
  namespace: vela-system
spec:
  # MODE: Adopt existing cluster
  mode: adopt

  # How to connect to the cluster
  credential:
    # Option A: Kubeconfig
    secretRef:
      name: legacy-production-kubeconfig

    # Option B: Cloud provider discovery (finds cluster by name/tags)
    # provider:
    #   type: aws
    #   credentialRef:
    #     name: aws-credentials
    #   discovery:
    #     clusterName: legacy-production
    #     # Or by tags:
    #     # tags:
    #     #   environment: production
    #     #   team: platform

  # Adoption configuration
  adoption:
    # What to do with existing resources
    existingResources:
      # discover: Find and inventory existing resources
      # reconcile: Bring into desired state (may modify)
      # ignore: Don't touch, just track
      mode: discover  # First run: just discover

    # Import infrastructure state (optional)
    terraformState:
      # Import from Terraform state for drift detection
      backend:
        type: s3
        config:
          bucket: terraform-state
          key: clusters/legacy-production/terraform.tfstate
          region: us-east-1

    # Map existing components to planes
    componentMapping:
      # Auto-discover and map to planes
      autoDiscover: true

      # Or explicit mapping
      mappings:
        - namespace: ingress-nginx
          plane: networking
          component: ingress-nginx
        - namespace: cert-manager
          plane: security
          component: cert-manager
        - namespace: monitoring
          plane: observability
          component: prometheus-stack

  # Blueprint to reconcile towards (optional for adoption)
  blueprintRef:
    name: production-standard
    # Don't apply immediately - just compare
    reconcileMode: dryRun  # dryRun, gradual, immediate

status:
  mode: adopt
  adoptionStatus:
    phase: Discovered  # Discovering, Discovered, Reconciling, Adopted

    # What was discovered
    discoveredInfrastructure:
      provider: aws
      region: us-east-1
      vpcId: vpc-legacy123
      clusterName: legacy-production
      kubernetesVersion: "v1.27.8"
      nodeCount: 8

    # Discovered components mapped to planes
    discoveredComponents:
      - namespace: ingress-nginx
        resources:
          - kind: Deployment
            name: ingress-nginx-controller
            version: "4.7.1"  # Detected from image
        suggestedPlane: networking
        suggestedComponent: ingress-nginx
        status: Mapped

      - namespace: cert-manager
        resources:
          - kind: Deployment
            name: cert-manager
            version: "1.12.0"
        suggestedPlane: security
        suggestedComponent: cert-manager
        status: Mapped

      - namespace: kube-system
        resources:
          - kind: DaemonSet
            name: aws-node  # AWS CNI
            version: "1.14.0"
        suggestedPlane: networking
        suggestedComponent: aws-cni
        status: Mapped

      - namespace: custom-app
        resources:
          - kind: Deployment
            name: legacy-service
        suggestedPlane: null  # Not infrastructure
        status: Ignored

    # Drift from target blueprint
    blueprintDrift:
      hasDrift: true
      driftSummary:
        - component: ingress-nginx
          currentVersion: "4.7.1"
          targetVersion: "4.8.3"
          action: "Upgrade available"
        - component: prometheus-stack
          status: "Missing"
          action: "Will be installed"
        - component: gatekeeper
          status: "Missing"
          action: "Will be installed"

    # Terraform state sync (if configured)
    terraformSync:
      lastSyncTime: "2024-12-24T10:00:00Z"
      resourcesTracked: 47
      driftDetected: false
```

**Adoption Workflow:**

```yaml
# Step 1: Discover (dry-run)
apiVersion: core.oam.dev/v1beta1
kind: Cluster
metadata:
  name: legacy-production
spec:
  mode: adopt
  credential:
    secretRef:
      name: legacy-kubeconfig
  adoption:
    existingResources:
      mode: discover
  blueprintRef:
    name: production-standard
    reconcileMode: dryRun  # Just show what would change

---
# Step 2: Review discovered state, then reconcile gradually
# After reviewing status.adoptionStatus.blueprintDrift:

apiVersion: core.oam.dev/v1beta1
kind: Cluster
metadata:
  name: legacy-production
spec:
  mode: adopt
  credential:
    secretRef:
      name: legacy-kubeconfig
  adoption:
    existingResources:
      mode: reconcile  # Now actually reconcile
  blueprintRef:
    name: production-standard
    reconcileMode: gradual  # Apply changes incrementally

    # Gradual reconciliation settings
    gradualReconcile:
      # Order of operations
      order:
        - action: upgrade
          components: [ingress-nginx, cert-manager]  # Upgrade existing first
        - action: install
          components: [gatekeeper]  # Then add missing security
        - action: install
          components: [prometheus-stack, loki]  # Finally observability

      # Pause between phases for validation
      pauseBetweenPhases: "1h"

      # Automatic or manual progression
      progression: manual  # manual, automatic
```

#### Mode 3: Connect - Manage Existing Cluster

Simply connect to an existing cluster without adopting infrastructure management. The cluster's underlying infrastructure (VPC, nodes) remains managed externally.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Cluster
metadata:
  name: partner-cluster
  namespace: vela-system
spec:
  # MODE: Just connect and manage Kubernetes resources
  mode: connect

  # Cluster access
  credential:
    secretRef:
      name: partner-cluster-kubeconfig

  # What to manage
  managementScope:
    # Only manage resources in these namespaces
    namespaces:
      include:
        - vela-managed-*
        - platform-*
      exclude:
        - kube-system
        - kube-public

    # Only manage resources with these labels
    labelSelector:
      matchLabels:
        managed-by: vela

  # Blueprint (optional)
  blueprintRef:
    name: partner-minimal

status:
  mode: connect
  connectionStatus: Connected
  managementScope:
    managedNamespaces: 5
    managedResources: 47
```

#### ClusterProviderDefinition

To support multiple cloud providers, we introduce `ClusterProviderDefinition`:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterProviderDefinition
metadata:
  name: aws-eks
  namespace: vela-system
spec:
  description: "Provision EKS clusters on AWS"

  # Provider type
  provider: aws

  # What credentials are required
  credentials:
    required:
      - name: accessKeyId
        description: "AWS Access Key ID"
      - name: secretAccessKey
        description: "AWS Secret Access Key"
    optional:
      - name: sessionToken
        description: "AWS Session Token (for temporary credentials)"
      - name: roleArn
        description: "IAM Role ARN to assume"

  # Provisioning schematic
  schematic:
    # Option 1: Use Crossplane
    crossplane:
      compositionRef:
        name: eks-cluster-composition

    # Option 2: Use Terraform (via tf-controller)
    # terraform:
    #   moduleSource: "terraform-aws-modules/eks/aws"
    #   version: "19.0.0"

    # Option 3: Native implementation
    # native:
    #   controllerImage: "kubevela/cluster-provider-aws:v1.0.0"

  # Default values for this provider
  defaults:
    kubernetesVersion: "1.28"
    instanceType: "m5.large"
    minNodes: 3
    maxNodes: 10
    networking:
      vpcCidr: "10.0.0.0/16"
      privateSubnets: true
      publicSubnets: true
      natGateway: single  # single, perAz, none

  # Capabilities this provider supports
  capabilities:
    - privateEndpoint
    - publicEndpoint
    - managedNodeGroups
    - fargateProfiles
    - spotInstances
    - gpuNodes
    - armNodes

  # Status mappings
  statusMappings:
    # Map provider-specific status to standard status
    provisioning:
      - CREATING
      - UPDATING
    ready:
      - ACTIVE
    failed:
      - FAILED
      - DELETING

---
# GCP GKE Provider
apiVersion: core.oam.dev/v1beta1
kind: ClusterProviderDefinition
metadata:
  name: gcp-gke
spec:
  provider: gcp
  schematic:
    crossplane:
      compositionRef:
        name: gke-cluster-composition
  defaults:
    kubernetesVersion: "1.28"
    machineType: "e2-standard-4"
    minNodes: 3

---
# Azure AKS Provider
apiVersion: core.oam.dev/v1beta1
kind: ClusterProviderDefinition
metadata:
  name: azure-aks
spec:
  provider: azure
  schematic:
    crossplane:
      compositionRef:
        name: aks-cluster-composition
  defaults:
    kubernetesVersion: "1.28"
    vmSize: "Standard_D4s_v3"
    minNodes: 3

---
# Local development (kind)
apiVersion: core.oam.dev/v1beta1
kind: ClusterProviderDefinition
metadata:
  name: kind
spec:
  provider: kind
  schematic:
    native:
      controllerImage: "kubevela/cluster-provider-kind:v1.0.0"
  defaults:
    kubernetesVersion: "1.28"
    nodes: 1
  capabilities:
    - localDevelopment
```

#### CLI for Cluster Lifecycle

```bash
# ============================================
# PROVISION NEW CLUSTER
# ============================================

# Minimal - just credentials and region
vela cluster create production-us-east-1 \
  --provider aws \
  --credentials aws-platform-creds \
  --region us-east-1 \
  --blueprint production-standard

# With options
vela cluster create production-us-east-1 \
  --provider aws \
  --credentials aws-platform-creds \
  --region us-east-1 \
  --kubernetes-version 1.28 \
  --node-type m5.xlarge \
  --min-nodes 5 \
  --max-nodes 20 \
  --blueprint production-standard

# Watch provisioning progress
vela cluster watch production-us-east-1
Cluster: production-us-east-1
Phase: Provisioning (12m elapsed)

Infrastructure:
  ✓ VPC created (vpc-0123456789)
  ✓ Subnets created (3 AZs)
  ✓ Security groups configured
  ⟳ EKS cluster creating... (est. 8m remaining)
  ○ Node group pending
  ○ Blueprint pending

# ============================================
# ADOPT EXISTING CLUSTER
# ============================================

# Step 1: Discover what's in the cluster
vela cluster adopt legacy-production \
  --kubeconfig ./legacy-kubeconfig \
  --blueprint production-standard \
  --dry-run

Adopting cluster: legacy-production
Mode: Discovery (dry-run)

Cluster Info:
  Provider:    AWS EKS
  Region:      us-east-1
  K8s Version: v1.27.8
  Nodes:       8

Discovered Components:
  ┌────────────────────┬─────────────────┬───────────┬─────────────────────────────┐
  │ COMPONENT          │ VERSION         │ PLANE     │ BLUEPRINT STATUS            │
  ├────────────────────┼─────────────────┼───────────┼─────────────────────────────┤
  │ ingress-nginx      │ 4.7.1           │ networking│ Upgrade available (→4.8.3)  │
  │ aws-cni            │ 1.14.0          │ networking│ ✓ Matches                   │
  │ cert-manager       │ 1.12.0          │ security  │ Upgrade available (→1.13.3) │
  │ prometheus         │ (custom)        │ -         │ ⚠ Non-standard deployment   │
  └────────────────────┴─────────────────┴───────────┴─────────────────────────────┘

Missing from Blueprint:
  - gatekeeper (security plane)
  - loki (observability plane)
  - prometheus-stack (observability plane) - replaces custom prometheus

Recommended Actions:
  1. Upgrade ingress-nginx: 4.7.1 → 4.8.3
  2. Upgrade cert-manager: 1.12.0 → 1.13.3
  3. Install gatekeeper for policy enforcement
  4. Replace custom prometheus with prometheus-stack
  5. Install loki for logging

Proceed with adoption? (y/n):

# Step 2: Actually adopt
vela cluster adopt legacy-production \
  --kubeconfig ./legacy-kubeconfig \
  --blueprint production-standard \
  --reconcile gradual \
  --confirm

# Step 3: Monitor adoption
vela cluster adoption-status legacy-production
Adoption Progress:
  Phase 1: Upgrades (in progress)
    ✓ ingress-nginx upgraded to 4.8.3
    ⟳ cert-manager upgrading... (1.12.0 → 1.13.3)

  Phase 2: Security (pending)
    ○ gatekeeper installation pending

  Phase 3: Observability (pending)
    ○ prometheus migration pending
    ○ loki installation pending

# ============================================
# CONNECT TO EXISTING CLUSTER
# ============================================

vela cluster connect partner-cluster \
  --kubeconfig ./partner-kubeconfig \
  --managed-namespaces "platform-*" \
  --blueprint partner-minimal

# ============================================
# IMPORT FROM TERRAFORM
# ============================================

vela cluster import-terraform production-us-east-1 \
  --state-backend s3 \
  --bucket terraform-state \
  --key clusters/production/terraform.tfstate \
  --blueprint production-standard
```

#### Provisioning Integration Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    CLUSTER PROVISIONING ARCHITECTURE                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌───────────────┐                                                         │
│  │    Cluster    │                                                         │
│  │   Controller  │                                                         │
│  └───────┬───────┘                                                         │
│          │                                                                  │
│          │ Reads ClusterProviderDefinition                                 │
│          │ to determine provisioning method                                │
│          │                                                                  │
│          ▼                                                                  │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    PROVISIONING BACKENDS                             │   │
│  ├─────────────────────────────────────────────────────────────────────┤   │
│  │                                                                      │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐              │   │
│  │  │  Crossplane  │  │  Terraform   │  │    Native    │              │   │
│  │  │              │  │  Controller  │  │   Provider   │              │   │
│  │  │  - AWS EKS   │  │              │  │              │              │   │
│  │  │  - GCP GKE   │  │  - Any TF    │  │  - kind      │              │   │
│  │  │  - Azure AKS │  │    module    │  │  - k3s       │              │   │
│  │  │              │  │              │  │  - custom    │              │   │
│  │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘              │   │
│  │         │                 │                 │                       │   │
│  └─────────┼─────────────────┼─────────────────┼───────────────────────┘   │
│            │                 │                 │                            │
│            ▼                 ▼                 ▼                            │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    CLOUD PROVIDERS                                   │   │
│  │                                                                      │   │
│  │   ┌─────────┐    ┌─────────┐    ┌─────────┐    ┌─────────┐        │   │
│  │   │   AWS   │    │   GCP   │    │  Azure  │    │  Local  │        │   │
│  │   │   EKS   │    │   GKE   │    │   AKS   │    │  kind   │        │   │
│  │   └─────────┘    └─────────┘    └─────────┘    └─────────┘        │   │
│  │                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  After Provisioning:                                                        │
│  ──────────────────                                                        │
│  1. Cluster controller obtains kubeconfig                                  │
│  2. Updates Cluster status with connection info                            │
│  3. Triggers Blueprint controller to apply planes                          │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

### Definition Types

We introduce new X-Definition types for cluster infrastructure:

#### PlaneComponentDefinition

Defines component types available in ClusterPlanes.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: PlaneComponentDefinition
metadata:
  name: helm-release
  namespace: vela-system
spec:
  description: "Deploy a Helm chart as a plane component"

  workload:
    type: autodetect  # The Helm chart determines the workload type

  schematic:
    cue:
      template: |
        import "encoding/yaml"

        output: {
          apiVersion: "helm.toolkit.fluxcd.io/v2beta1"
          kind: "HelmRelease"
          metadata: {
            name: context.name
            namespace: parameter.namespace
          }
          spec: {
            interval: parameter.interval
            chart: {
              spec: {
                chart: parameter.chart
                version: parameter.version
                sourceRef: {
                  kind: "HelmRepository"
                  name: context.name + "-repo"
                }
              }
            }
            if parameter.values != _|_ {
              values: parameter.values
            }
          }
        }

        outputs: {
          "helm-repo": {
            apiVersion: "source.toolkit.fluxcd.io/v1beta2"
            kind: "HelmRepository"
            metadata: {
              name: context.name + "-repo"
              namespace: parameter.namespace
            }
            spec: {
              interval: "1h"
              url: parameter.repo
            }
          }
        }

        parameter: {
          chart: string
          repo: string
          version: string
          namespace: *"default" | string
          interval: *"5m" | string
          values?: {...}
        }
```

#### PlaneTraitDefinition

Defines traits that can be applied to plane components.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: PlaneTraitDefinition
metadata:
  name: resource-quota
  namespace: vela-system
spec:
  description: "Apply resource quotas to plane component namespace"

  appliesToWorkloads:
    - helm-release
    - kustomization

  schematic:
    cue:
      template: |
        outputs: {
          "resource-quota": {
            apiVersion: "v1"
            kind: "ResourceQuota"
            metadata: {
              name: context.name + "-quota"
              namespace: context.output.metadata.namespace
            }
            spec: {
              hard: {
                if parameter.cpu != _|_ {
                  "requests.cpu": parameter.cpu
                  "limits.cpu": parameter.cpu
                }
                if parameter.memory != _|_ {
                  "requests.memory": parameter.memory
                  "limits.memory": parameter.memory
                }
                if parameter.pods != _|_ {
                  pods: parameter.pods
                }
              }
            }
          }
        }

        parameter: {
          cpu?: string
          memory?: string
          pods?: string
        }
```

#### PlanePolicyDefinition

Defines policies applicable at the plane or blueprint level.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: PlanePolicyDefinition
metadata:
  name: apply-order
  namespace: vela-system
spec:
  description: "Define component apply order within a plane"

  schematic:
    cue:
      template: |
        // This policy is processed by the plane controller
        // to determine component ordering

        #ApplyOrderPolicy: {
          rules: [...{
            component: string
            dependsOn: [...string]
          }]
        }

        output: #ApplyOrderPolicy & {
          rules: parameter.rules
        }

        parameter: {
          rules: [...{
            component: string
            dependsOn: [...string]
          }]
        }
```

#### ClusterWorkflowStepDefinition

Defines workflow steps for cluster/plane operations.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterWorkflowStepDefinition
metadata:
  name: apply-plane
  namespace: vela-system
spec:
  description: "Apply a ClusterPlane to target clusters"

  schematic:
    cue:
      template: |
        import "vela/op"

        // Get plane reference
        plane: op.#Read & {
          value: {
            apiVersion: "core.oam.dev/v1beta1"
            kind: "ClusterPlane"
            metadata: {
              name: parameter.plane
              namespace: "vela-system"
            }
          }
        }

        // Apply to each target cluster
        apply: op.#Steps & {
          for cluster in context.clusters {
            "apply-\(cluster.name)": op.#Apply & {
              cluster: cluster.name
              value: plane.value
            }
          }
        }

        // Wait for health
        wait: op.#ConditionalWait & {
          continue: apply.status.healthy == true
        }

        parameter: {
          plane: string
          timeout?: string
        }
```

---

### Workflow and Rollout

#### Plane Deployment Workflow

The workflow engine is extended to support plane-level operations:

```yaml
workflow:
  steps:
    # Deploy a plane
    - name: deploy-networking
      type: apply-plane
      properties:
        plane: networking
        waitForHealthy: true
        timeout: "10m"

    # Validate deployment
    - name: validate-networking
      type: validate-plane
      properties:
        plane: networking
        checks:
          - type: pods-ready
            namespace: ingress-nginx
          - type: service-available
            service: ingress-nginx-controller
            namespace: ingress-nginx

    # Conditional step
    - name: deploy-istio
      type: apply-plane
      if: context.blueprint.features.serviceMesh == true
      properties:
        plane: service-mesh

    # Human approval for production
    - name: production-approval
      type: suspend
      if: context.clusters[0].labels.environment == "production"
      properties:
        message: "Approve deployment to production clusters"
        approvers:
          - platform-leads

    # Parallel deployment to remaining planes
    - name: deploy-remaining
      type: step-group
      subSteps:
        - name: observability
          type: apply-plane
          properties:
            plane: observability
        - name: security
          type: apply-plane
          properties:
            plane: security
```

#### Rollout State Machine

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         CLUSTER ROLLOUT STATE MACHINE                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│                              ┌──────────┐                                   │
│                              │ Pending  │                                   │
│                              └────┬─────┘                                   │
│                                   │ Start rollout                           │
│                                   ▼                                         │
│                           ┌──────────────┐                                  │
│                           │ Initializing │                                  │
│                           └──────┬───────┘                                  │
│                                  │ Select first batch                       │
│                                  ▼                                          │
│     ┌───────────────────────────────────────────────────────────────┐      │
│     │                      BATCH LOOP                                │      │
│     │  ┌─────────────┐     ┌─────────────┐     ┌─────────────┐     │      │
│     │  │   Updating  │────▶│  Analyzing  │────▶│   Paused    │     │      │
│     │  │   Cluster   │     │   Metrics   │     │ (optional)  │     │      │
│     │  └─────────────┘     └──────┬──────┘     └──────┬──────┘     │      │
│     │                             │                    │            │      │
│     │         ┌───────────────────┼────────────────────┘            │      │
│     │         │                   │                                 │      │
│     │         │    SLO Pass       │    SLO Fail                     │      │
│     │         ▼                   ▼                                 │      │
│     │  ┌─────────────┐     ┌─────────────┐                         │      │
│     │  │ Next Batch  │     │ RollingBack │                         │      │
│     │  └──────┬──────┘     └──────┬──────┘                         │      │
│     │         │                   │                                 │      │
│     └─────────┼───────────────────┼─────────────────────────────────┘      │
│               │                   │                                         │
│               │ All batches       │                                         │
│               │ complete          │                                         │
│               ▼                   ▼                                         │
│        ┌──────────┐        ┌────────────┐                                  │
│        │Succeeded │        │ RolledBack │                                  │
│        └──────────┘        └────────────┘                                  │
│                                                                             │
│  Manual Controls:                                                           │
│  - Pause: Enter Paused state at any batch                                  │
│  - Resume: Continue from Paused state                                      │
│  - Abort: Cancel rollout, remain at current state                          │
│  - Rollback: Manually trigger rollback                                     │
│  - Promote: Skip remaining batches, apply to all                           │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

### Multi-Tenancy and Team Ownership

#### Ownership Model

```yaml
# Networking team's plane
apiVersion: core.oam.dev/v1beta1
kind: ClusterPlane
metadata:
  name: networking
  namespace: platform-networking  # Team's namespace
  labels:
    plane.oam.dev/owner: networking-team
    plane.oam.dev/category: networking
  annotations:
    plane.oam.dev/contact: "networking@example.com"
    plane.oam.dev/oncall: "https://pagerduty.com/networking"
spec:
  # Only networking team can modify this plane
  # Enforced via RBAC on the namespace
```

#### RBAC Configuration

```yaml
# Role for plane owners
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: plane-owner
  namespace: platform-networking
rules:
  - apiGroups: ["core.oam.dev"]
    resources: ["clusterplanes"]
    verbs: ["*"]
  - apiGroups: ["core.oam.dev"]
    resources: ["planecomponentdefinitions", "planetraitdefinitions"]
    verbs: ["get", "list", "watch"]

---
# Role for blueprint composers (SRE/Platform leads)
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: blueprint-composer
rules:
  - apiGroups: ["core.oam.dev"]
    resources: ["clusterblueprints", "clusterrollouts"]
    verbs: ["*"]
  - apiGroups: ["core.oam.dev"]
    resources: ["clusterplanes"]
    verbs: ["get", "list", "watch"]  # Can reference but not modify
```

---

### Health Checking and Observability

A critical requirement is understanding the health of clusters at multiple levels - from the overall cluster down to individual resources. The health model must support:

1. **Hierarchical health aggregation** - Cluster health rolls up from planes, planes from components, components from resources
2. **Pluggable observability providers** - Support Prometheus, Datadog, New Relic, Dynatrace, CloudWatch, and custom providers
3. **Drill-down capability** - Quickly isolate issues by navigating the health hierarchy
4. **Multiple health dimensions** - Availability, performance, saturation, errors

#### Health Hierarchy

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           HEALTH HIERARCHY                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  CLUSTER LEVEL                                                              │
│  ─────────────                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ Cluster: production-us-east-1                                        │   │
│  │ Health: Degraded (1 of 3 planes unhealthy)                          │   │
│  │                                                                      │   │
│  │ Aggregated from:                                                     │   │
│  │   ✓ networking: Healthy                                              │   │
│  │   ✗ security: Degraded (cert-manager unhealthy)                     │   │
│  │   ✓ observability: Healthy                                           │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│           │                                                                 │
│           ▼                                                                 │
│  PLANE LEVEL (drill down into security plane)                              │
│  ───────────                                                               │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ Plane: security                                                      │   │
│  │ Health: Degraded (1 of 3 components unhealthy)                      │   │
│  │                                                                      │   │
│  │ Aggregated from:                                                     │   │
│  │   ✓ gatekeeper: Healthy                                              │   │
│  │   ✗ cert-manager: Unhealthy (certificate renewal failing)          │   │
│  │   ✓ external-secrets: Healthy                                        │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│           │                                                                 │
│           ▼                                                                 │
│  COMPONENT LEVEL (drill down into cert-manager)                            │
│  ───────────────                                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ Component: cert-manager                                              │   │
│  │ Health: Unhealthy                                                    │   │
│  │                                                                      │   │
│  │ Health Checks:                                                       │   │
│  │   ✓ Deployment ready: 3/3 replicas                                  │   │
│  │   ✓ Pod health: All pods running                                    │   │
│  │   ✗ Functional: Certificate renewal error rate > 5%                 │   │
│  │   ✗ SLO: ACME challenge success rate < 99%                          │   │
│  │                                                                      │   │
│  │ Resources:                                                           │   │
│  │   ✓ Deployment/cert-manager: 3/3 ready                              │   │
│  │   ✓ Deployment/cert-manager-webhook: 1/1 ready                      │   │
│  │   ✓ Deployment/cert-manager-cainjector: 1/1 ready                   │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### ObservabilityProviderDefinition

To support multiple observability backends, we introduce `ObservabilityProviderDefinition`:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ObservabilityProviderDefinition
metadata:
  name: prometheus
  namespace: vela-system
spec:
  description: "Prometheus metrics provider"

  # Provider type identifier
  type: prometheus

  # Connection configuration schema
  connectionSpec:
    properties:
      endpoint:
        type: string
        description: "Prometheus server URL"
        required: true
      # Authentication options
      auth:
        type: object
        properties:
          type:
            type: string
            enum: [none, basic, bearer, oauth2]
          secretRef:
            type: object
            description: "Reference to credentials secret"

  # Query template - how to execute queries
  queryTemplate: |
    // CUE template for query execution
    query: {
      type: "instant" | "range"
      promql: string
      time?: string
      start?: string
      end?: string
      step?: string
    }

  # Response parsing template
  responseTemplate: |
    // CUE template for parsing response
    result: {
      value: number
      labels: [string]: string
      timestamp: string
    }

  # Built-in metric templates
  builtinMetrics:
    - name: error-rate
      description: "HTTP error rate percentage"
      query: |
        sum(rate(http_requests_total{status=~"5.."}[5m]))
        / sum(rate(http_requests_total[5m])) * 100
      unit: "percent"

    - name: p99-latency
      description: "99th percentile latency"
      query: |
        histogram_quantile(0.99, sum(rate(http_request_duration_seconds_bucket[5m])) by (le))
      unit: "seconds"

    - name: availability
      description: "Service availability based on successful requests"
      query: |
        sum(rate(http_requests_total{status!~"5.."}[5m]))
        / sum(rate(http_requests_total[5m])) * 100
      unit: "percent"

---
# Datadog provider
apiVersion: core.oam.dev/v1beta1
kind: ObservabilityProviderDefinition
metadata:
  name: datadog
  namespace: vela-system
spec:
  description: "Datadog metrics and APM provider"
  type: datadog

  connectionSpec:
    properties:
      site:
        type: string
        description: "Datadog site (e.g., datadoghq.com, datadoghq.eu)"
        default: "datadoghq.com"
      apiKeyRef:
        type: object
        description: "Reference to API key secret"
        required: true
      appKeyRef:
        type: object
        description: "Reference to Application key secret"
        required: true

  queryTemplate: |
    query: {
      type: "timeseries" | "scalar"
      metric: string
      scope?: string
      groupBy?: [...string]
      from: int  // Unix timestamp
      to: int
    }

  builtinMetrics:
    - name: error-rate
      description: "APM error rate"
      query: "avg:trace.http.request.errors{*} / avg:trace.http.request.hits{*} * 100"
      unit: "percent"

    - name: p99-latency
      description: "APM p99 latency"
      query: "p99:trace.http.request.duration{*}"
      unit: "seconds"

---
# New Relic provider
apiVersion: core.oam.dev/v1beta1
kind: ObservabilityProviderDefinition
metadata:
  name: newrelic
  namespace: vela-system
spec:
  description: "New Relic observability provider"
  type: newrelic

  connectionSpec:
    properties:
      accountId:
        type: string
        required: true
      apiKeyRef:
        type: object
        required: true
      region:
        type: string
        enum: [US, EU]
        default: "US"

  queryTemplate: |
    query: {
      nrql: string  // NRQL query
    }

  builtinMetrics:
    - name: error-rate
      query: "SELECT percentage(count(*), WHERE error IS true) FROM Transaction"
      unit: "percent"

    - name: apdex
      query: "SELECT apdex(duration, 0.5) FROM Transaction"
      unit: "score"

---
# CloudWatch provider
apiVersion: core.oam.dev/v1beta1
kind: ObservabilityProviderDefinition
metadata:
  name: cloudwatch
  namespace: vela-system
spec:
  description: "AWS CloudWatch metrics provider"
  type: cloudwatch

  connectionSpec:
    properties:
      region:
        type: string
        required: true
      credentialRef:
        type: object
        description: "Reference to AWS credentials"

  queryTemplate: |
    query: {
      namespace: string
      metricName: string
      dimensions: [...{name: string, value: string}]
      statistic: "Average" | "Sum" | "Minimum" | "Maximum" | "SampleCount"
      period: int  // seconds
    }

---
# Custom/webhook provider for any backend
apiVersion: core.oam.dev/v1beta1
kind: ObservabilityProviderDefinition
metadata:
  name: custom-webhook
  namespace: vela-system
spec:
  description: "Custom webhook-based observability provider"
  type: webhook

  connectionSpec:
    properties:
      endpoint:
        type: string
        required: true
      method:
        type: string
        enum: [GET, POST]
        default: "POST"
      headers:
        type: object
        additionalProperties:
          type: string
      authSecretRef:
        type: object

  queryTemplate: |
    // Request body template
    request: {
      query: string
      params: [string]: _
    }

  responseTemplate: |
    // Expected response format
    response: {
      success: bool
      value: number
      message?: string
    }
```

#### ObservabilityProvider Instance

Create instances of providers with connection details:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ObservabilityProvider
metadata:
  name: central-prometheus
  namespace: vela-system
spec:
  # Reference to provider definition
  definitionRef:
    name: prometheus

  # Connection configuration
  connection:
    endpoint: "http://prometheus.monitoring.svc:9090"
    auth:
      type: none

  # Health check for the provider itself
  healthCheck:
    interval: "30s"
    timeout: "10s"

---
apiVersion: core.oam.dev/v1beta1
kind: ObservabilityProvider
metadata:
  name: datadog-prod
  namespace: vela-system
spec:
  definitionRef:
    name: datadog

  connection:
    site: "datadoghq.com"
    apiKeyRef:
      name: datadog-credentials
      key: api-key
    appKeyRef:
      name: datadog-credentials
      key: app-key

---
apiVersion: core.oam.dev/v1beta1
kind: ObservabilityProvider
metadata:
  name: newrelic-prod
  namespace: vela-system
spec:
  definitionRef:
    name: newrelic

  connection:
    accountId: "1234567"
    region: "US"
    apiKeyRef:
      name: newrelic-credentials
      key: api-key
```

#### Health Check Configuration in ClusterPlane

Each `ClusterPlane` can define health checks using any registered observability provider:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterPlane
metadata:
  name: networking
spec:
  components:
    - name: ingress-nginx
      type: helm-release
      properties:
        chart: ingress-nginx
        version: "4.8.3"

      # Component-level health checks
      healthChecks:
        # Kubernetes resource health (built-in)
        - name: deployment-ready
          type: kubernetes
          kubernetes:
            # Check deployment readiness
            resourceRef:
              apiVersion: apps/v1
              kind: Deployment
              name: ingress-nginx-controller
              namespace: ingress-nginx
            condition:
              type: Available
              status: "True"

        # Prometheus-based SLO check
        - name: error-rate-slo
          type: metrics
          metrics:
            providerRef:
              name: central-prometheus
            query: |
              sum(rate(nginx_ingress_controller_requests{status=~"5.."}[5m]))
              / sum(rate(nginx_ingress_controller_requests[5m])) * 100
            threshold:
              operator: "<"
              value: 1  # Error rate < 1%
            for: "5m"  # Must be true for 5 minutes

        # Datadog APM check (alternative provider)
        - name: latency-slo
          type: metrics
          metrics:
            providerRef:
              name: datadog-prod
            query: "p99:nginx.http.request.duration{service:ingress-nginx}"
            threshold:
              operator: "<"
              value: 0.5  # p99 < 500ms

        # HTTP endpoint health check
        - name: healthz-endpoint
          type: http
          http:
            url: "http://ingress-nginx-controller.ingress-nginx.svc/healthz"
            method: GET
            expectedStatus: 200
            timeout: "5s"
            interval: "30s"

        # Custom CUE-based health policy (KubeVela native)
        - name: custom-policy
          type: cue
          cue:
            healthPolicy: |
              deployment: context.outputs.deployment
              isHealth: deployment.status.readyReplicas >= deployment.spec.replicas

  # Plane-level health configuration
  health:
    # How to aggregate component health
    aggregation:
      # Plane is healthy if all components healthy
      strategy: all  # all, any, majority, weighted

      # Or use weighted scoring
      # strategy: weighted
      # weights:
      #   ingress-nginx: 50
      #   cilium: 30
      #   external-dns: 20
      # threshold: 80  # Healthy if score >= 80%

    # Grace period before marking unhealthy
    gracePeriod: "2m"

    # How often to evaluate health
    checkInterval: "30s"
```

#### Health Status in Cluster CRD

The Cluster status provides comprehensive health at all levels:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Cluster
metadata:
  name: production-us-east-1
status:
  # Top-level health summary
  health:
    status: Degraded  # Healthy, Degraded, Unhealthy, Unknown, Progressing
    reason: "PlaneUnhealthy"
    message: "1 of 3 planes is unhealthy: security"
    lastCheckTime: "2024-12-24T10:00:00Z"

    # Aggregated scores (if using weighted)
    score: 85

    # Quick summary
    summary:
      planesHealthy: 2
      planesTotal: 3
      componentsHealthy: 7
      componentsTotal: 8

    # SLO status
    sloStatus:
      withinBudget: true
      errorBudgetRemaining: "45%"

  # Per-plane health (drill-down level 1)
  planes:
    - name: networking
      health:
        status: Healthy
        score: 100
        lastCheckTime: "2024-12-24T10:00:00Z"
      components:
        - name: ingress-nginx
          health:
            status: Healthy
            checks:
              - name: deployment-ready
                status: Passing
                message: "3/3 replicas ready"
              - name: error-rate-slo
                status: Passing
                value: 0.2
                threshold: "< 1%"
              - name: latency-slo
                status: Passing
                value: 0.12
                threshold: "< 0.5s"
        - name: cilium
          health:
            status: Healthy
            checks:
              - name: daemonset-ready
                status: Passing
                message: "12/12 nodes ready"

    - name: security
      health:
        status: Degraded
        reason: "ComponentUnhealthy"
        message: "cert-manager failing health checks"
        score: 66
      components:
        - name: gatekeeper
          health:
            status: Healthy
        - name: cert-manager
          health:
            status: Unhealthy
            reason: "HealthCheckFailed"
            message: "Certificate renewal error rate exceeds threshold"
            checks:
              - name: deployment-ready
                status: Passing
                message: "3/3 replicas ready"
              - name: certificate-renewal-rate
                status: Failing
                value: 8.5
                threshold: "< 5%"
                message: "ACME challenge failures detected"
                since: "2024-12-24T09:45:00Z"
        - name: external-secrets
          health:
            status: Healthy

    - name: observability
      health:
        status: Healthy
        score: 100
      components:
        - name: prometheus-stack
          health:
            status: Healthy
        - name: loki
          health:
            status: Healthy

  # Health history for trend analysis
  healthHistory:
    - timestamp: "2024-12-24T10:00:00Z"
      status: Degraded
      score: 85
    - timestamp: "2024-12-24T09:55:00Z"
      status: Degraded
      score: 85
    - timestamp: "2024-12-24T09:50:00Z"
      status: Healthy
      score: 100
    # ... last 24 hours

  # Active health alerts
  alerts:
    - name: cert-manager-renewal-failing
      severity: warning
      component: security/cert-manager
      message: "Certificate renewal error rate > 5%"
      since: "2024-12-24T09:45:00Z"
      runbook: "https://runbooks.example.com/cert-manager-renewal"
```

#### CLI for Health Inspection

```bash
# ============================================
# CLUSTER HEALTH OVERVIEW
# ============================================

$ vela cluster health production-us-east-1

Cluster: production-us-east-1
Status: Degraded
Score: 85/100

Planes:
  ┌────────────────┬──────────┬───────┬────────────────────────────────┐
  │ PLANE          │ STATUS   │ SCORE │ MESSAGE                        │
  ├────────────────┼──────────┼───────┼────────────────────────────────┤
  │ networking     │ Healthy  │ 100   │ All components healthy         │
  │ security       │ Degraded │ 66    │ cert-manager unhealthy         │
  │ observability  │ Healthy  │ 100   │ All components healthy         │
  └────────────────┴──────────┴───────┴────────────────────────────────┘

Active Alerts:
  ⚠ cert-manager-renewal-failing (warning) - since 15m ago
    Certificate renewal error rate > 5%

Use 'vela cluster health production-us-east-1 --plane security' to drill down

# ============================================
# DRILL DOWN INTO PLANE
# ============================================

$ vela cluster health production-us-east-1 --plane security

Plane: security
Status: Degraded (1 of 3 components unhealthy)

Components:
  ┌─────────────────┬───────────┬─────────────────────────────────────────┐
  │ COMPONENT       │ STATUS    │ HEALTH CHECKS                           │
  ├─────────────────┼───────────┼─────────────────────────────────────────┤
  │ gatekeeper      │ ✓ Healthy │ deployment-ready: ✓                     │
  │                 │           │ policy-violations: ✓ (< 10)             │
  ├─────────────────┼───────────┼─────────────────────────────────────────┤
  │ cert-manager    │ ✗ Unhealthy│ deployment-ready: ✓ (3/3)              │
  │                 │           │ certificate-renewal: ✗ (8.5% > 5%)     │
  │                 │           │ acme-success-rate: ✗ (91% < 99%)       │
  ├─────────────────┼───────────┼─────────────────────────────────────────┤
  │ external-secrets│ ✓ Healthy │ deployment-ready: ✓                     │
  │                 │           │ sync-success-rate: ✓ (99.9%)           │
  └─────────────────┴───────────┴─────────────────────────────────────────┘

Use 'vela cluster health production-us-east-1 --component security/cert-manager' for details

# ============================================
# DRILL DOWN INTO COMPONENT
# ============================================

$ vela cluster health production-us-east-1 --component security/cert-manager

Component: cert-manager
Plane: security
Status: Unhealthy
Since: 2024-12-24T09:45:00Z (15 minutes ago)

Health Checks:
  ┌──────────────────────┬────────┬──────────┬───────────┬─────────────────────┐
  │ CHECK                │ STATUS │ VALUE    │ THRESHOLD │ PROVIDER            │
  ├──────────────────────┼────────┼──────────┼───────────┼─────────────────────┤
  │ deployment-ready     │ ✓ Pass │ 3/3      │ all ready │ kubernetes          │
  │ webhook-ready        │ ✓ Pass │ 1/1      │ all ready │ kubernetes          │
  │ certificate-renewal  │ ✗ Fail │ 8.5%     │ < 5%      │ prometheus          │
  │ acme-success-rate    │ ✗ Fail │ 91%      │ > 99%     │ prometheus          │
  │ memory-usage         │ ✓ Pass │ 256Mi    │ < 512Mi   │ prometheus          │
  └──────────────────────┴────────┴──────────┴───────────┴─────────────────────┘

Resources:
  Deployment/cert-manager: 3/3 ready
  Deployment/cert-manager-webhook: 1/1 ready
  Deployment/cert-manager-cainjector: 1/1 ready

Recent Events:
  09:45:00  Warning  CertificateRenewalFailed  Failed to renew certificate: ACME challenge failed
  09:47:00  Warning  ACMEChallengeFailed       DNS-01 challenge: timeout waiting for DNS propagation
  09:50:00  Warning  CertificateRenewalFailed  Failed to renew certificate: ACME challenge failed

Suggested Actions:
  1. Check DNS provider connectivity
  2. Verify ACME account credentials
  3. Review cert-manager logs: kubectl logs -n cert-manager deploy/cert-manager

Runbook: https://runbooks.example.com/cert-manager-renewal

# ============================================
# FLEET-WIDE HEALTH VIEW
# ============================================

$ vela cluster health --all

Fleet Health Summary
Total Clusters: 18

  ┌──────────────────────────┬──────────┬───────┬─────────────────────────────┐
  │ CLUSTER                  │ STATUS   │ SCORE │ ISSUES                      │
  ├──────────────────────────┼──────────┼───────┼─────────────────────────────┤
  │ production-us-east-1     │ Degraded │ 85    │ security/cert-manager       │
  │ production-us-west-2     │ Healthy  │ 100   │ -                           │
  │ production-eu-west-1     │ Healthy  │ 100   │ -                           │
  │ staging-us-east-1        │ Healthy  │ 100   │ -                           │
  │ canary-us-east-1         │ Degraded │ 90    │ networking/ingress-nginx    │
  │ ...                      │          │       │                             │
  └──────────────────────────┴──────────┴───────┴─────────────────────────────┘

By Status:
  Healthy:    15 clusters
  Degraded:   2 clusters
  Unhealthy:  0 clusters
  Unknown:    1 cluster

Common Issues:
  1. security/cert-manager (2 clusters) - Certificate renewal failures
  2. networking/ingress-nginx (1 cluster) - High latency

# ============================================
# HEALTH USING SPECIFIC PROVIDER
# ============================================

$ vela cluster health production-us-east-1 --provider datadog-prod

Using Datadog provider: datadog-prod

Cluster: production-us-east-1
APM Health:
  Services:     12 monitored
  Error Rate:   0.3%
  Latency p99:  145ms
  Throughput:   1,250 req/s

Traces with Errors (last 15m):
  - POST /api/certificates/renew - 23 errors
  - GET /api/secrets - 2 errors

Dashboards:
  - https://app.datadoghq.com/dashboard/abc123
```

#### Health-Based Rollout Integration

The `ClusterRolloutStrategy` uses health status for wave progression:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterRolloutStrategy
metadata:
  name: production-rollout
spec:
  waves:
    - name: canary
      clusterSelector:
        matchLabels:
          tier: canary
      waitFor:
        # No previous wave
      healthChecks:
        # All providers can be used for rollout health
        - providerRef:
            name: central-prometheus
          metrics:
            - name: error-rate
              query: "sum(rate(http_requests_total{status=~'5..'}[5m])) / sum(rate(http_requests_total[5m])) * 100"
              threshold:
                operator: "<"
                value: 1
            - name: p99-latency
              query: "histogram_quantile(0.99, sum(rate(http_request_duration_seconds_bucket[5m])) by (le))"
              threshold:
                operator: "<"
                value: 0.5

        - providerRef:
            name: datadog-prod
          metrics:
            - name: apm-error-rate
              query: "avg:trace.http.request.errors{*} / avg:trace.http.request.hits{*} * 100"
              threshold:
                operator: "<"
                value: 1

    - name: staging
      waitFor:
        wave: canary
        healthyDuration: "4h"
        # Require health from multiple providers
        healthRequirements:
          - provider: central-prometheus
            mustPass: [error-rate, p99-latency]
          - provider: datadog-prod
            mustPass: [apm-error-rate]
```

---

### Drift Detection and Remediation

Drift detection is a critical capability for managing fleet-wide cluster infrastructure. It ensures clusters remain consistent with their assigned blueprints and enables proactive identification of configuration variance.

#### Drift Detection CLI

The `vela cluster drift` command provides comprehensive drift detection:

```bash
# Check drift against assigned blueprint
$ vela cluster drift production-us-east-1

# Output shows current state vs expected state from blueprint
Cluster: production-us-east-1
Blueprint: production-standard-v2.3.0
Status: No Drift Detected ✓
```

#### What-If Blueprint Comparison

A key feature is the `--blueprint` flag which enables **what-if analysis** - comparing a cluster against any blueprint, not just its assigned one. This is essential for:

- **Upgrade planning**: See what would change if you moved a cluster to a new blueprint version
- **Standardization analysis**: Compare a non-standard cluster against the standard blueprint
- **Migration assessment**: Evaluate impact before reassigning a cluster to a different blueprint

```bash
# Compare against a different blueprint (what-if analysis)
$ vela cluster drift production-us-east-1 --blueprint staging-standard

# This shows what would need to change if this cluster were to adopt staging-standard
```

#### Fleet-Wide Drift Analysis

For upgrade planning across an entire fleet:

```bash
# Compare ALL clusters against a specific blueprint
$ vela cluster drift --all --blueprint production-standard-v2.4.0

# Output shows upgrade impact summary:
# - Which clusters are already compliant
# - Which need updates and what changes are required
# - Estimated rollout waves based on changes needed
```

This enables platform teams to assess the impact of a new blueprint version before initiating a rollout.

#### Drift Exceptions

Not all drift is unintentional. Some clusters may have legitimate reasons for configuration differences (cost optimization, regional requirements, etc.). The drift exceptions feature allows teams to:

1. **Accept known drift**: Mark specific configuration differences as intentional
2. **Document reasons**: Provide context for why the drift exists
3. **Exclude from alerts**: Prevent false positives in monitoring

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterDriftException
metadata:
  name: production-eu-west-1-replica-exception
  namespace: platform-clusters
spec:
  clusterRef:
    name: production-eu-west-1
  exceptions:
    - resource:
        apiVersion: apps/v1
        kind: Deployment
        name: ingress-nginx-controller
        namespace: ingress-nginx
      fields:
        - path: spec.replicas
          reason: "Scaled down for cost optimization in EU region"
          approvedBy: platform-admin@example.com
          expiresAt: "2025-03-01T00:00:00Z"  # Optional expiration
```

#### ClusterDriftReport CRD

Drift detection results are persisted as `ClusterDriftReport` resources:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterDriftReport
metadata:
  name: production-us-east-1-drift-2024-12-24
  namespace: platform-clusters
spec:
  clusterRef:
    name: production-us-east-1
  blueprintRef:
    name: production-standard
    revision: v2.3.0
  comparisonType: assigned  # or "what-if"
status:
  driftDetected: true
  lastChecked: "2024-12-24T10:00:00Z"
  summary:
    totalPlanes: 3
    driftedPlanes: 1
    totalComponents: 8
    driftedComponents: 2
  planeDrifts:
    - planeName: networking
      status: drifted
      componentDrifts:
        - componentName: ingress-nginx
          resourceDrifts:
            - apiVersion: apps/v1
              kind: Deployment
              name: ingress-nginx-controller
              namespace: ingress-nginx
              fields:
                - path: spec.replicas
                  expected: 3
                  actual: 5
                  severity: warning
                  exception: false
    - planeName: security
      status: synced
    - planeName: observability
      status: synced
```

---

## Use Cases

### Use Case 1: Networking Team Updates Ingress Controller

```yaml
# 1. Networking team updates their plane
apiVersion: core.oam.dev/v1beta1
kind: ClusterPlane
metadata:
  name: networking
  namespace: platform-networking
spec:
  components:
    - name: ingress-nginx
      type: helm-release
      properties:
        chart: ingress-nginx
        version: "4.9.0"  # Updated from 4.8.3
        # ... rest unchanged

---
# 2. SRE creates a rollout for this change
apiVersion: core.oam.dev/v1beta1
kind: ClusterRollout
metadata:
  name: ingress-upgrade-4.9.0
spec:
  # Target the blueprint that references this plane
  targetBlueprint:
    name: production-standard

  # Only roll out clusters using this blueprint
  clusterSelector:
    matchLabels:
      blueprint: production-standard

  strategy:
    type: canary
    canary:
      steps:
        - weight: 10
          pause:
            duration: "1h"
        - weight: 100

  analysis:
    metrics:
      - name: ingress-error-rate
        provider: prometheus
        query: |
          sum(rate(nginx_ingress_controller_requests{status=~"5.."}[5m]))
          / sum(rate(nginx_ingress_controller_requests[5m]))
        thresholds:
          - condition: "< 0.01"
```

### Use Case 2: New Cluster Onboarding

```yaml
# 1. Register new cluster
apiVersion: cluster.core.oam.dev/v1alpha1
kind: ManagedCluster
metadata:
  name: production-ap-south-1
spec:
  kubeconfig:
    secretRef:
      name: prod-ap-south-1-kubeconfig
  labels:
    environment: production
    region: ap-south-1
    tier: standard
    blueprint: production-standard  # Auto-apply this blueprint

---
# 2. ClusterBlueprint controller detects new cluster matching selector
# and automatically applies the blueprint via workflow

# 3. Status shows onboarding progress
status:
  appliedClusters:
    - name: production-ap-south-1
      status: Provisioning
      workflow:
        currentStep: deploy-networking
        progress: "2/5 planes deployed"
```

### Use Case 3: Emergency Rollback

```yaml
# Manual rollback trigger
apiVersion: core.oam.dev/v1beta1
kind: ClusterRollout
metadata:
  name: ingress-upgrade-4.9.0
spec:
  # ... existing spec ...

  # Trigger immediate rollback
  rollback:
    trigger: manual
    reason: "Critical bug discovered in ingress-nginx 4.9.0"

---
# Or via CLI
# vela cluster rollout rollback ingress-upgrade-4.9.0 --reason "Critical bug"
```

### Use Case 4: Blue-Green for Major Upgrade

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterRollout
metadata:
  name: k8s-1.29-upgrade
spec:
  targetBlueprint:
    name: production-standard
    revision: production-standard-k8s-1.29

  strategy:
    type: blueGreen
    blueGreen:
      # Create parallel "green" environment
      greenClusters:
        # Provision new clusters or use standby clusters
        provision:
          count: 3
          template:
            labels:
              role: green
              k8sVersion: "1.29"

      # Traffic switching
      trafficSwitch:
        type: dns  # or loadBalancer
        provider: route53

      # Validation before switch
      preSwitch:
        - type: smoke-test
          properties:
            testSuite: production-smoke
        - type: load-test
          properties:
            rps: 1000
            duration: "10m"

      # Keep blue for rollback window
      blueRetention: "72h"
```

---

## Edge Cases and Considerations

### 1. Circular Dependencies Between Planes

**Problem**: Plane A depends on Plane B, and Plane B depends on Plane A.

**Solution**:
```yaml
# Validation webhook rejects circular dependencies
status:
  conditions:
    - type: Valid
      status: "False"
      reason: CircularDependency
      message: "Circular dependency detected: networking → security → networking"
```

**Implementation**:
- Build dependency graph at blueprint validation time
- Topological sort to detect cycles
- Reject blueprints with cycles

### 2. Partial Plane Failure

**Problem**: 2 out of 3 components in a plane fail to deploy.

**Solution**:
```yaml
spec:
  # Plane-level failure policy
  failurePolicy:
    type: failFast  # or continueOnError, rollbackOnError

    # Per-component override
    components:
      - name: external-dns
        critical: false  # Non-critical component failure won't fail plane

status:
  phase: PartiallyRunning
  components:
    - name: ingress-nginx
      status: Running
    - name: cilium
      status: Running
    - name: external-dns
      status: Failed
      message: "Route53 credentials missing"
      critical: false  # Marked non-critical
```

### 3. Version Conflicts Between Planes

**Problem**: Networking plane requires Kubernetes 1.28+, but cluster is 1.27.

**Solution**:
```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterPlane
metadata:
  name: networking
spec:
  # Compatibility requirements
  requirements:
    kubernetes:
      minVersion: "1.28.0"
      maxVersion: "1.30.x"

    # Required CRDs
    crds:
      - group: "gateway.networking.k8s.io"
        version: "v1"
        kind: "Gateway"

    # Required APIs
    apis:
      - group: "admissionregistration.k8s.io"
        version: "v1"

---
# Blueprint validation fails if requirements not met
status:
  conditions:
    - type: Valid
      status: "False"
      reason: IncompatibleCluster
      message: |
        Cluster production-legacy (v1.27.5) does not meet requirements:
        - networking plane requires kubernetes >= 1.28.0
```

### 4. Rollout During Active Incident

**Problem**: Rollout starts while there's an active incident affecting metrics.

**Solution**:
```yaml
spec:
  analysis:
    # Baseline comparison instead of absolute thresholds
    mode: baseline
    baseline:
      # Compare against pre-rollout metrics
      source: preRollout
      window: "30m"

    metrics:
      - name: error-rate
        thresholds:
          # Allow up to 10% increase from baseline
          - condition: "< baseline * 1.1"

    # Incident integration
    incidentIntegration:
      provider: pagerduty
      # Pause rollout if active P1/P2 incident
      pauseOnIncident:
        severities: [P1, P2]
      # Don't count metrics during incident window
      excludeIncidentWindow: true

status:
  phase: Paused
  conditions:
    - type: IncidentPause
      status: "True"
      reason: ActiveIncident
      message: "Paused due to active P1 incident INC-12345"
```

### 5. Plane Upgrade Requires Cluster Restart

**Problem**: CNI upgrade requires node drain/restart.

**Solution**:
```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterPlane
metadata:
  name: networking
spec:
  components:
    - name: cilium
      type: helm-release
      properties:
        version: "1.15.0"

      # Upgrade strategy for disruptive changes
      upgradeStrategy:
        type: nodeByNode
        nodeByNode:
          maxUnavailable: 1
          drainTimeout: "10m"
          # Skip nodes with critical pods
          skipNodesWithCriticalPods: true
          # PDB awareness
          respectPodDisruptionBudgets: true
```

### 6. Orphaned Resources After Plane Removal

**Problem**: Removing a component from a plane leaves resources behind.

**Solution**:
```yaml
spec:
  # Garbage collection policy
  garbageCollection:
    # What to do when component is removed from plane
    onComponentRemoval: delete  # delete, orphan, warn

    # Finalizer ensures cleanup before plane deletion
    onPlaneDelete: cascade  # cascade, orphan

    # Grace period before deletion
    deletionGracePeriod: "5m"

status:
  orphanedResources:
    - apiVersion: v1
      kind: ConfigMap
      name: legacy-config
      namespace: ingress-nginx
      reason: "Component 'legacy-ingress' removed in revision v2.0.0"
      action: PendingDeletion
      deleteAfter: "2024-12-24T15:00:00Z"
```

### 7. Multi-Cluster State Drift

**Problem**: Cluster 3 out of 10 drifts from desired state.

**Solution**:
```yaml
spec:
  # Drift detection and remediation
  driftDetection:
    enabled: true
    interval: "5m"

    # What counts as drift
    scope:
      - resources  # Spec changes
      - status     # Unhealthy status
      - missing    # Deleted resources

    # Auto-remediation
    remediation:
      enabled: true
      # Max remediations per hour to prevent loops
      maxRemediationsPerHour: 3
      # Exclude certain fields from drift detection
      ignoredFields:
        - "metadata.resourceVersion"
        - "status.observedGeneration"

status:
  drift:
    clusters:
      - name: production-us-east-1
        drifted: false
      - name: production-us-west-2
        drifted: true
        driftDetails:
          - resource: "Deployment/ingress-nginx-controller"
            field: "spec.replicas"
            expected: 3
            actual: 2
            reason: "Manual kubectl scale"
            lastRemediation: "2024-12-24T09:00:00Z"
            remediationCount: 2
```

### 8. Rollout to Clusters in Different Time Zones

**Problem**: Need to avoid rollouts during business hours in each region.

**Solution**:
```yaml
spec:
  strategy:
    type: canary
    canary:
      steps:
        - weight: 33
          clusterSelector:
            matchLabels:
              region: us-east
          # Maintenance window for US East
          maintenanceWindow:
            timezone: "America/New_York"
            windows:
              - start: "02:00"
                end: "06:00"
                days: [Mon, Tue, Wed, Thu, Fri]

        - weight: 66
          clusterSelector:
            matchLabels:
              region: eu-west
          maintenanceWindow:
            timezone: "Europe/London"
            windows:
              - start: "02:00"
                end: "06:00"

        - weight: 100
          clusterSelector:
            matchLabels:
              region: ap-south
          maintenanceWindow:
            timezone: "Asia/Kolkata"
            windows:
              - start: "02:00"
                end: "06:00"
```

### 9. Secrets and Credentials for Plane Components

**Problem**: Helm chart needs cloud provider credentials.

**Solution**:
```yaml
spec:
  components:
    - name: external-dns
      type: helm-release
      properties:
        chart: external-dns
        values:
          provider: aws
          # Reference to ExternalSecret or sealed secret
          aws:
            credentials:
              secretRef:
                name: aws-credentials
                # Secret is synced to each target cluster
                syncPolicy: clusterLocal

  # Secret distribution policy
  secrets:
    - name: aws-credentials
      source:
        # Source from hub cluster
        type: externalSecret
        externalSecret:
          secretStore: aws-secrets-manager
          key: platform/external-dns

      # How to distribute to managed clusters
      distribution:
        type: perCluster
        # Each cluster gets unique credentials
        template: |
          accessKeyId: {{ .Cluster.Name }}-external-dns-key
          secretAccessKey: {{ .Cluster.Annotations.externalDnsSecretPath }}
```

### 10. Cost Tracking and Showback

**Problem**: Need to track infrastructure costs per plane/team.

**Solution**:
```yaml
spec:
  # Cost allocation
  costAllocation:
    enabled: true

    # Cost center for this plane
    costCenter: "platform-networking"

    # Resource tagging for cloud cost tracking
    resourceTags:
      team: networking
      service: platform
      costCenter: CC-12345

status:
  costs:
    # Estimated monthly cost
    estimatedMonthlyCost:
      amount: 1250.00
      currency: USD

    # Per-component breakdown
    components:
      - name: ingress-nginx
        cost: 450.00
        resources:
          - type: LoadBalancer
            count: 3
            unitCost: 150.00
      - name: cilium
        cost: 0.00  # No cloud resources
      - name: external-dns
        cost: 50.00
        resources:
          - type: Route53Queries
            count: 1000000
            unitCost: 0.05
```

### 11. Cluster Provisioning Failure Mid-Way

**Problem**: VPC and subnets created, but EKS cluster creation fails. Need to clean up or retry.

**Solution**:
```yaml
spec:
  mode: provision

  # Provisioning behavior on failure
  provisioningPolicy:
    onFailure: retry  # retry, cleanup, pause

    retry:
      maxAttempts: 3
      backoff:
        initial: "5m"
        max: "30m"
        multiplier: 2

    # What to do with partial infrastructure
    partialInfrastructure:
      # Keep resources for debugging
      retain: true
      retainDuration: "24h"

status:
  provisioningStatus:
    phase: Failed
    failureReason: "EKS cluster creation failed: insufficient capacity in us-east-1a"
    retryCount: 2
    nextRetryAt: "2024-12-24T11:00:00Z"

    # Partial infrastructure created
    partialInfrastructure:
      vpc:
        id: vpc-xxx
        status: Created
        retainUntil: "2024-12-25T10:00:00Z"
      subnets:
        - id: subnet-aaa
          status: Created
      cluster:
        status: Failed
        error: "insufficient capacity"

    # Suggested actions
    suggestedActions:
      - "Try a different availability zone: us-east-1b or us-east-1c"
      - "Reduce initial node count from 5 to 3"
      - "Use a different instance type: m5.xlarge → m6i.large"
```

### 12. Adopting Cluster with Conflicting Components

**Problem**: Existing cluster has ingress-nginx 3.x (old) with incompatible API versions.

**Solution**:
```yaml
spec:
  mode: adopt
  adoption:
    # Conflict resolution strategy
    conflictResolution:
      # What to do when discovered component conflicts with blueprint
      strategy: prompt  # prompt, upgrade, skip, replace

      # Per-component overrides
      overrides:
        - component: ingress-nginx
          # Old version uses deprecated APIs
          action: replace
          migration:
            # Backup existing before replacement
            backup: true
            # Migrate existing IngressClass resources
            migrateResources:
              - kind: Ingress
                apiVersion: "networking.k8s.io/v1"
            # Validation after migration
            validate:
              - type: ingress-connectivity
                timeout: "5m"

status:
  adoptionStatus:
    conflicts:
      - component: ingress-nginx
        currentVersion: "3.35.0"
        currentAPIVersion: "extensions/v1beta1"  # Deprecated
        targetVersion: "4.8.3"
        targetAPIVersion: "networking.k8s.io/v1"
        incompatible: true
        suggestedAction: "replace"
        migrationRequired: true
        affectedResources:
          - kind: Ingress
            count: 47
            namespaces: [app-a, app-b, app-c]
```

### 13. Credential Rotation During Provisioning

**Problem**: AWS credentials expire or get rotated while cluster is provisioning.

**Solution**:
```yaml
spec:
  provider:
    credentialRef:
      name: aws-credentials

    # Credential management
    credentialPolicy:
      # Auto-refresh from external secret manager
      refresh:
        enabled: true
        interval: "45m"  # Refresh before 1h session expiry

      # What to do on credential failure
      onFailure: pause  # pause, retry, abort

      # External secret integration
      externalSecret:
        provider: aws-secrets-manager
        secretId: "platform/eks-provisioner"

status:
  provisioningStatus:
    credentialStatus:
      valid: true
      expiresAt: "2024-12-24T11:00:00Z"
      lastRefresh: "2024-12-24T10:15:00Z"
      nextRefresh: "2024-12-24T11:00:00Z"

  # If credentials fail
  conditions:
    - type: CredentialsValid
      status: "False"
      reason: "ExpiredCredentials"
      message: "AWS credentials expired, waiting for refresh"
```

### 14. Adopting Cluster with Unknown/Custom Components

**Problem**: Cluster has custom-built or unknown components that don't map to standard definitions.

**Solution**:
```yaml
spec:
  mode: adopt
  adoption:
    # How to handle unknown components
    unknownComponents:
      # discover: Track but don't manage
      # import: Create custom plane component
      # ignore: Don't track
      action: discover

      # Create custom definitions for discovered components
      autoCreateDefinitions: true

status:
  adoptionStatus:
    discoveredComponents:
      - namespace: custom-platform
        resources:
          - kind: Deployment
            name: custom-auth-proxy
            image: "internal-registry/auth-proxy:v2.1"
        identified: false  # No matching definition
        action: Tracked

        # Auto-generated definition suggestion
        suggestedDefinition:
          apiVersion: core.oam.dev/v1beta1
          kind: PlaneComponentDefinition
          metadata:
            name: custom-auth-proxy
          spec:
            description: "Auto-discovered: custom-auth-proxy"
            workload:
              type: deployments.apps
            schematic:
              # Generated from discovered deployment
              kube:
                template:
                  apiVersion: apps/v1
                  kind: Deployment
                  # ... extracted from existing deployment
```

### 15. Network Isolation During Provisioning

**Problem**: Provisioning in air-gapped environment with no internet access.

**Solution**:
```yaml
spec:
  mode: provision

  # Air-gapped / network-restricted environment
  networkPolicy:
    airgapped: true

    # Private container registry
    containerRegistry:
      mirror: "registry.internal.company.com"
      # Registry credentials
      credentialRef:
        name: internal-registry-creds

    # Private Helm repository
    helmRepository:
      url: "https://helm.internal.company.com"
      credentialRef:
        name: helm-repo-creds

    # No external endpoints
    externalAccess:
      enabled: false

    # Private endpoints for cloud provider
    privateEndpoints:
      eks: true
      ecr: true
      s3: true

  # Pre-downloaded artifacts
  artifacts:
    # Container images pre-pulled
    images:
      source: "s3://artifacts-bucket/images/"
    # Helm charts pre-downloaded
    charts:
      source: "s3://artifacts-bucket/charts/"
```

### 16. Cluster Deletion and Resource Cleanup

**Problem**: Deleting a cluster should clean up all cloud resources but preserve audit logs.

**Solution**:
```yaml
spec:
  mode: provision

  # Deletion policy
  deletionPolicy:
    # What to do when Cluster CR is deleted
    clusterDeletion: delete  # delete, retain, orphan

    # Resource cleanup order
    cleanupOrder:
      - applications  # Delete vela applications first
      - planes        # Then plane components
      - nodeGroups    # Then nodes
      - cluster       # Then EKS cluster
      - networking    # Finally VPC/subnets

    # Resources to retain after deletion
    retain:
      - type: cloudwatchLogs
        retention: "90d"
      - type: s3Backups
        retention: "365d"
      - type: costReports
        retention: "730d"

    # Confirmation required for production
    confirmation:
      required: true
      # Must type cluster name to confirm
      typeToConfirm: true

    # Grace period before deletion starts
    gracePeriod: "24h"

    # Notification before deletion
    notification:
      channels:
        - type: slack
          channel: "#platform-alerts"
        - type: email
          recipients: ["platform-team@company.com"]
      beforeDeletion: "24h"

status:
  deletionStatus:
    phase: PendingConfirmation  # PendingConfirmation, Deleting, Deleted
    scheduledAt: "2024-12-25T10:00:00Z"
    gracePeriodEnds: "2024-12-25T10:00:00Z"
    notificationSent: true
    confirmationReceived: false

    # Progress during deletion
    cleanup:
      - resource: "Applications"
        count: 15
        deleted: 12
        status: InProgress
      - resource: "Planes"
        count: 3
        deleted: 0
        status: Pending
```

---

## API Reference

### Cluster

| Field | Type | Description |
|-------|------|-------------|
| `spec.mode` | string | Cluster mode: `provision`, `adopt`, or `connect` |
| `spec.provider.type` | string | Cloud provider: `aws`, `gcp`, `azure`, `kind`, `k3s` |
| `spec.provider.credentialRef` | SecretRef | Reference to cloud credentials secret |
| `spec.provider.region` | string | Cloud region for provisioning |
| `spec.credential.secretRef` | SecretRef | Kubeconfig secret reference (for adopt/connect) |
| `spec.clusterSpec` | ClusterSpec | Kubernetes version, node pools, networking (provision mode) |
| `spec.blueprintRef` | BlueprintRef | Blueprint to apply |
| `spec.adoption` | AdoptionSpec | Adoption configuration (adopt mode) |
| `spec.patches` | []PlanePatch | Cluster-specific blueprint overrides |
| `spec.rolloutStrategyRef` | StrategyRef | Reference to ClusterRolloutStrategy |
| `spec.rolloutStrategyRef.overrides` | OverrideSpec | Cluster-specific rollout overrides |
| `spec.maintenance` | MaintenanceSpec | Maintenance windows |
| `spec.maintenance.enforceWindow` | bool | Block updates outside maintenance window |
| `status.mode` | string | Active mode |
| `status.connectionStatus` | string | Connection status: `Connected`, `Disconnected` |
| `status.provisioningStatus` | ProvisioningStatus | Infrastructure provisioning progress |
| `status.adoptionStatus` | AdoptionStatus | Adoption discovery and reconciliation status |
| `status.clusterInfo` | ClusterInfo | Discovered cluster information |
| `status.blueprint` | BlueprintStatus | Applied blueprint status |
| `status.planes` | []PlaneInventory | Full inventory of planes and components |
| `status.health` | HealthStatus | Aggregated health status |
| `status.drift` | DriftStatus | Drift detection results |
| `status.resources` | ResourceUsage | CPU, memory, pod usage |
| `status.history` | []HistoryEntry | Blueprint application history |

### ClusterPlane

| Field | Type | Description |
|-------|------|-------------|
| `spec.description` | string | Human-readable description |
| `spec.components` | []PlaneComponent | Components in this plane |
| `spec.policies` | []PlanePolicy | Plane-level policies |
| `spec.outputs` | []PlaneOutput | Values exposed to other planes |
| `spec.requirements` | Requirements | Compatibility requirements |
| `spec.failurePolicy` | FailurePolicy | How to handle component failures |
| `spec.garbageCollection` | GCPolicy | Resource cleanup policy |
| `status.phase` | string | Current phase |
| `status.components` | []ComponentStatus | Per-component status |
| `status.outputs` | map[string]string | Resolved output values |

### ClusterBlueprint

| Field | Type | Description |
|-------|------|-------------|
| `spec.planes` | []PlaneRef | Referenced planes with patches |
| `spec.policies` | []BlueprintPolicy | Blueprint-level policies |
| `spec.workflow` | Workflow | Deployment workflow |
| `status.planes` | []PlaneStatus | Per-plane status |
| `status.appliedClusters` | []ClusterStatus | Per-cluster status |

### ClusterRolloutStrategy

| Field | Type | Description |
|-------|------|-------------|
| `spec.description` | string | Human-readable description |
| `spec.waves` | []Wave | Wave definitions with ordering and selectors |
| `spec.waves[].name` | string | Wave identifier |
| `spec.waves[].order` | int | Wave execution order |
| `spec.waves[].clusterSelector` | LabelSelector | Which clusters belong to this wave |
| `spec.waves[].waitFor` | WaitCondition | Previous wave dependency |
| `spec.waves[].waitFor.wave` | string | Name of wave to wait for |
| `spec.waves[].waitFor.healthyDuration` | Duration | How long wave must be healthy |
| `spec.waves[].pause` | PauseSpec | Pause duration after wave |
| `spec.waves[].approval` | ApprovalSpec | Manual approval requirement |
| `spec.waves[].batching` | BatchSpec | Batch size and interval within wave |
| `spec.maintenanceWindows.respectClusterWindows` | bool | Respect per-cluster maintenance windows |
| `spec.maintenanceWindows.skipIfOutsideWindow` | bool | Skip clusters outside their window |
| `spec.clusterUpdateBehavior` | UpdateBehavior | Per-cluster rollout strategy |
| `spec.analysis` | AnalysisSpec | Metrics and thresholds |
| `spec.rollback` | RollbackSpec | Automatic rollback configuration |
| `status.phase` | string | `Active`, `Paused`, `Superseded` |
| `status.currentRollout` | RolloutProgress | Current rollout progress |
| `status.currentRollout.currentWave` | string | Currently updating wave |
| `status.currentRollout.waveProgress` | []WaveStatus | Per-wave status |
| `status.clusters` | ClusterCounts | Cluster counts by wave |
| `status.analysis` | AnalysisStatus | Current analysis results |

### ClusterRollout (Optional - Emergency/Manual Overrides)

| Field | Type | Description |
|-------|------|-------------|
| `spec.targetBlueprint` | BlueprintRef | Target blueprint/revision |
| `spec.sourceBlueprint` | BlueprintRef | Source blueprint (optional) |
| `spec.strategy` | RolloutStrategy | Canary/BlueGreen/Rolling |
| `spec.analysis` | AnalysisSpec | Metrics and thresholds |
| `spec.rollback` | RollbackSpec | Rollback configuration |
| `spec.approvals` | []ApprovalGate | Manual approval gates |
| `spec.overrideStrategy` | bool | Override cluster's rolloutStrategyRef |
| `status.phase` | string | Current phase |
| `status.currentStep` | int | Current rollout step |
| `status.clusters` | []ClusterRolloutStatus | Per-cluster status |
| `status.analysis` | AnalysisStatus | Current analysis results |

### ObservabilityProviderDefinition

| Field | Type | Description |
|-------|------|-------------|
| `spec.description` | string | Human-readable description |
| `spec.type` | string | Provider type: `prometheus`, `datadog`, `newrelic`, `cloudwatch`, `webhook` |
| `spec.connectionSpec` | ConnectionSchema | JSON schema for connection configuration |
| `spec.queryTemplate` | string | CUE template for query execution |
| `spec.responseTemplate` | string | CUE template for response parsing |
| `spec.builtinMetrics` | []MetricTemplate | Pre-defined metric queries |
| `spec.builtinMetrics[].name` | string | Metric name |
| `spec.builtinMetrics[].query` | string | Query in provider's query language |
| `spec.builtinMetrics[].unit` | string | Unit of measurement |

### ObservabilityProvider

| Field | Type | Description |
|-------|------|-------------|
| `spec.definitionRef` | DefinitionRef | Reference to ObservabilityProviderDefinition |
| `spec.connection` | Connection | Provider-specific connection configuration |
| `spec.connection.endpoint` | string | Provider endpoint URL |
| `spec.connection.auth` | AuthConfig | Authentication configuration |
| `spec.healthCheck.interval` | Duration | How often to check provider health |
| `spec.healthCheck.timeout` | Duration | Timeout for health checks |
| `status.phase` | string | `Ready`, `Unhealthy`, `Unknown` |
| `status.lastCheckTime` | Time | Last successful connection time |

### HealthCheck (Component-level)

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Health check identifier |
| `type` | string | Check type: `kubernetes`, `metrics`, `http`, `cue` |
| `kubernetes.resourceRef` | ResourceRef | Reference to Kubernetes resource |
| `kubernetes.condition` | Condition | Expected condition |
| `metrics.providerRef` | ProviderRef | Reference to ObservabilityProvider |
| `metrics.query` | string | Query in provider's query language |
| `metrics.threshold` | Threshold | Expected value threshold |
| `metrics.for` | Duration | Duration threshold must hold |
| `http.url` | string | HTTP endpoint to check |
| `http.expectedStatus` | int | Expected HTTP status code |
| `cue.healthPolicy` | string | CUE expression returning `isHealth: bool` |

### HealthStatus (Status structures)

| Field | Type | Description |
|-------|------|-------------|
| `status` | string | `Healthy`, `Degraded`, `Unhealthy`, `Unknown`, `Progressing` |
| `reason` | string | Machine-readable reason code |
| `message` | string | Human-readable message |
| `score` | int | Health score 0-100 (for weighted aggregation) |
| `lastCheckTime` | Time | Last health evaluation time |
| `checks` | []CheckResult | Individual health check results |
| `checks[].name` | string | Check name |
| `checks[].status` | string | `Passing`, `Failing`, `Unknown` |
| `checks[].value` | string | Current value |
| `checks[].threshold` | string | Expected threshold |
| `checks[].since` | Time | When current status began |

### ClusterDriftReport

| Field | Type | Description |
|-------|------|-------------|
| `spec.clusterRef` | ClusterRef | Reference to the cluster being analyzed |
| `spec.blueprintRef.name` | string | Blueprint name for comparison |
| `spec.blueprintRef.revision` | string | Blueprint revision for comparison |
| `spec.comparisonType` | string | `assigned` (default) or `what-if` |
| `status.driftDetected` | bool | Whether any drift was detected |
| `status.lastChecked` | Time | When drift was last evaluated |
| `status.summary.totalPlanes` | int | Total number of planes in blueprint |
| `status.summary.driftedPlanes` | int | Number of planes with drift |
| `status.summary.totalComponents` | int | Total number of components |
| `status.summary.driftedComponents` | int | Number of components with drift |
| `status.planeDrifts` | []PlaneDrift | Per-plane drift details |
| `status.planeDrifts[].planeName` | string | Name of the plane |
| `status.planeDrifts[].status` | string | `synced`, `drifted`, `missing`, `extra` |
| `status.planeDrifts[].componentDrifts` | []ComponentDrift | Per-component drift details |

### ClusterDriftException

| Field | Type | Description |
|-------|------|-------------|
| `spec.clusterRef` | ClusterRef | Reference to the cluster |
| `spec.exceptions` | []Exception | List of accepted drift exceptions |
| `spec.exceptions[].resource` | ResourceRef | Reference to the drifted resource |
| `spec.exceptions[].fields` | []FieldException | Specific fields to exclude from drift |
| `spec.exceptions[].fields[].path` | string | JSONPath to the field |
| `spec.exceptions[].fields[].reason` | string | Reason for accepting this drift |
| `spec.exceptions[].fields[].approvedBy` | string | Who approved the exception |
| `spec.exceptions[].fields[].expiresAt` | Time | Optional expiration for the exception |

---

## Implementation Plan

### Phase 1: Core CRDs and Controllers

1. Define and implement CRD schemas
   - Cluster (with mode: provision, adopt, connect)
   - ClusterPlane
   - ClusterBlueprint
   - ClusterRolloutStrategy
   - ClusterRollout (optional, for emergency overrides)
   - ClusterProviderDefinition

2. Implement Cluster controller
   - Connection management (kubeconfig handling)
   - Inventory discovery
   - Health aggregation
   - Status reconciliation

3. Implement ClusterPlane controller
   - Component rendering
   - Trait application
   - Health checking

4. Implement ClusterBlueprint controller
   - Plane composition
   - Workflow execution
   - Multi-cluster dispatch

### Phase 2: Cluster Lifecycle

1. Provisioning backend integration
   - Crossplane integration for AWS/GCP/Azure
   - Terraform controller integration
   - Native kind/k3s provider

2. Cluster provisioning workflow
   - VPC/networking creation
   - Cluster creation
   - Node pool management
   - Kubeconfig generation

3. Cluster adoption workflow
   - Component discovery
   - Version detection
   - Mapping to planes
   - Terraform state import

4. Connect mode
   - Kubeconfig validation
   - Namespace scoping
   - RBAC integration

### Phase 3: Definition System

1. PlaneComponentDefinition
2. PlaneTraitDefinition
3. PlanePolicyDefinition
4. ClusterWorkflowStepDefinition
5. Built-in definitions (helm-release, kustomization, etc.)
6. Built-in provider definitions (aws-eks, gcp-gke, azure-aks, kind)

### Phase 4: Rollout Engine

1. ClusterRolloutStrategy controller
   - Wave management and progression
   - Cluster-to-wave assignment via labels
   - waitFor dependency resolution
   - Maintenance window enforcement
   - Approval gate integration

2. Rollout progression logic
   - Blueprint change detection
   - Automatic wave progression
   - Batch processing within waves
   - Health duration tracking

3. ClusterRollout controller (emergency overrides)
   - Imperative rollout support
   - Strategy override capability

4. Analysis and metrics integration
   - Prometheus integration
   - Kubernetes metrics
   - Per-wave and per-cluster analysis

5. Automatic rollback
   - Wave-scoped rollback
   - Cluster-scoped rollback
   - Fleet-wide rollback

### Phase 5: Health Checking and Observability

1. ObservabilityProviderDefinition and ObservabilityProvider CRDs
   - Provider definitions for Prometheus, Datadog, New Relic, CloudWatch
   - Custom webhook provider for extensibility
   - Connection management and authentication

2. Hierarchical health aggregation
   - Cluster → Plane → Component → Resource health roll-up
   - Configurable aggregation strategies (all, any, majority, weighted)
   - Health scoring (0-100)

3. Health check types
   - Kubernetes resource checks (deployment ready, conditions)
   - Metrics-based checks (any observability provider)
   - HTTP endpoint checks
   - CUE-based custom health policies

4. Health CLI and API
   - `vela cluster health` with drill-down capability
   - Fleet-wide health dashboard
   - Health history and trend analysis

5. Operational features
   - Prometheus metrics for clusters/planes/blueprints/rollouts
   - Drift detection and remediation
   - Cost tracking integration
   - Active alerts and runbook integration

### Phase 6: Drift Detection and Remediation

1. ClusterDriftReport and ClusterDriftException CRDs
   - Drift report generation and persistence
   - Exception management for intentional drift
   - Automatic drift detection scheduling

2. Drift detection engine
   - Deep comparison of cluster state vs blueprint
   - Plane-level and component-level diff
   - Resource-level field comparison
   - Integration with Terraform state for adopted clusters

3. What-if blueprint comparison
   - Compare cluster against any blueprint (`--blueprint` flag)
   - Fleet-wide upgrade impact analysis (`--all --blueprint`)
   - Estimated rollout wave planning

4. Drift remediation
   - `vela cluster remediate` for automatic correction
   - Selective remediation by plane/component
   - Drift exception workflow

### Phase 7: CLI and Operations

1. CLI commands
   - `vela cluster create/adopt/connect` - Cluster lifecycle
   - `vela cluster health` - Health inspection with drill-down
   - `vela cluster drift` - Drift detection with `--blueprint` comparison
   - `vela cluster remediate` - Drift remediation
   - `vela plane` - Plane management
   - `vela rollout` - Rollout management
   - `vela observability-provider` - Provider management

2. Operational dashboards
   - Grafana dashboards for fleet health
   - Per-cluster detail views
   - Rollout progress visualization
   - Drift status and history

### Phase 8: Integration and Documentation

1. Integration tests
2. E2E tests (provisioning, adoption, rollout scenarios)
3. Documentation
4. Migration guide from Terraform/manual clusters

---

## Appendix: CLI Commands

### Cluster Management (kubectl compatible)

```bash
# List all clusters with summary
$ kubectl get clusters -A
NAMESPACE     NAME                    PROVIDER   VERSION    BLUEPRINT            STATUS     HEALTH    AGE
vela-system   production-us-east-1    eks        v1.28.5    production-standard  Synced     Healthy   45d
vela-system   production-us-west-2    eks        v1.28.5    production-standard  Synced     Healthy   45d
vela-system   production-eu-west-1    eks        v1.28.3    production-standard  Synced     Healthy   30d
vela-system   staging-us-east-1       eks        v1.29.0    staging-standard     Synced     Healthy   60d
vela-system   development-local       kind       v1.28.0    dev-minimal          Synced     Healthy   90d

# Get wide output with more details
$ kubectl get clusters -o wide
NAME                    PROVIDER   VERSION    NODES   CPU     MEMORY    BLUEPRINT            PLANES   COMPONENTS   STATUS
production-us-east-1    eks        v1.28.5    12      96      384Gi     production-standard  3        8            Synced
production-us-west-2    eks        v1.28.5    10      80      320Gi     production-standard  3        8            Synced

# Describe cluster for full inventory
$ kubectl describe cluster production-us-east-1
Name:         production-us-east-1
Namespace:    vela-system
Labels:       environment=production
              provider=aws
              region=us-east-1
              tier=standard

Spec:
  Blueprint Ref:
    Name:      production-standard
    Revision:  production-standard-v2.3.0
  Credential:
    Secret Ref:
      Name:       production-us-east-1-kubeconfig
      Namespace:  vela-system

Status:
  Connection Status:  Connected
  Latency:            45ms

  Cluster Info:
    Kubernetes Version:  v1.28.5
    Platform:            eks
    Region:              us-east-1
    Node Count:          12
    Total CPU:           96
    Total Memory:        384Gi

  Blueprint:
    Name:       production-standard
    Revision:   production-standard-v2.3.0
    Applied At: 2024-12-24T08:00:00Z
    Status:     Synced

  Planes:
    Name:          networking
    Revision:      networking-v2.3.1
    Status:        Running
    Last Updated:  2024-12-24T08:00:00Z
    Components:
      Name:     ingress-nginx
      Type:     helm-release
      Version:  4.8.3
      Status:   Running
      Healthy:  true
      Resources:
        - Deployment/ingress-nginx-controller (ingress-nginx) [3/3]
        - Service/ingress-nginx-controller (ingress-nginx) [LoadBalancer: 52.x.x.x]

      Name:     cilium
      Type:     helm-release
      Version:  1.14.4
      Status:   Running
      Healthy:  true
      Resources:
        - DaemonSet/cilium (kube-system) [12/12]
        - DaemonSet/cilium-operator (kube-system) [2/2]

    Name:          security
    Revision:      security-v1.8.0
    Status:        Running
    Components:
      Name:     cert-manager
      Type:     helm-release
      Version:  1.13.3
      Healthy:  true

      Name:     gatekeeper
      Type:     helm-release
      Version:  3.14.0
      Healthy:  true

    Name:          observability
    Revision:      observability-v3.1.0
    Status:        Running
    Components:
      Name:     prometheus-stack
      Type:     helm-release
      Version:  55.5.0
      Healthy:  true

      Name:     loki
      Type:     helm-release
      Version:  5.41.0
      Healthy:  true

  Health:
    Status:              Healthy
    Planes Healthy:      3/3
    Components Healthy:  8/8

  Drift:
    Detected:         false
    Last Check Time:  2024-12-24T10:00:00Z

  Resources:
    CPU:
      Capacity:     96
      Requested:    45
      Usage:        32
    Memory:
      Capacity:     384Gi
      Requested:    180Gi
      Usage:        145Gi
    Pods:
      Capacity:     1100
      Running:      487

Events:
  Type    Reason            Age   From              Message
  ----    ------            ----  ----              -------
  Normal  BlueprintApplied  2d    cluster-controller Blueprint production-standard-v2.3.0 applied successfully
  Normal  HealthCheck       5m    cluster-controller All planes and components healthy
```

### Vela CLI Commands

```bash
# ============================================
# CLUSTER OPERATIONS - Full inventory view
# ============================================

# List all clusters with health status
vela cluster list
NAME                    STATUS      BLUEPRINT            VERSION     PLANES   HEALTH
production-us-east-1    Connected   production-standard  v2.3.0      3        Healthy
production-us-west-2    Connected   production-standard  v2.3.0      3        Healthy
staging-us-east-1       Connected   staging-standard     v2.1.0      2        Healthy
dev-local               Connected   dev-minimal          v1.0.0      1        Healthy

# Show full cluster inventory
vela cluster show production-us-east-1
Cluster: production-us-east-1
  Provider:    eks
  Region:      us-east-1
  K8s Version: v1.28.5
  Nodes:       12

Blueprint:
  Name:     production-standard
  Revision: production-standard-v2.3.0
  Status:   Synced

Planes:
  ┌─────────────────┬──────────────────────┬──────────┬─────────────────────────────────┐
  │ PLANE           │ REVISION             │ STATUS   │ COMPONENTS                      │
  ├─────────────────┼──────────────────────┼──────────┼─────────────────────────────────┤
  │ networking      │ networking-v2.3.1    │ Running  │ ingress-nginx (4.8.3)          │
  │                 │                      │          │ cilium (1.14.4)                 │
  │                 │                      │          │ external-dns (1.14.3)           │
  ├─────────────────┼──────────────────────┼──────────┼─────────────────────────────────┤
  │ security        │ security-v1.8.0      │ Running  │ cert-manager (1.13.3)           │
  │                 │                      │          │ gatekeeper (3.14.0)             │
  ├─────────────────┼──────────────────────┼──────────┼─────────────────────────────────┤
  │ observability   │ observability-v3.1.0 │ Running  │ prometheus-stack (55.5.0)       │
  │                 │                      │          │ loki (5.41.0)                   │
  └─────────────────┴──────────────────────┴──────────┴─────────────────────────────────┘

Health: ✓ Healthy (3/3 planes, 8/8 components)
Drift:  ✓ No drift detected

# Show component versions across all clusters
vela cluster components --component ingress-nginx
CLUSTER                 PLANE        COMPONENT       VERSION   STATUS    HEALTHY
production-us-east-1    networking   ingress-nginx   4.8.3     Running   ✓
production-us-west-2    networking   ingress-nginx   4.8.3     Running   ✓
production-eu-west-1    networking   ingress-nginx   4.8.3     Running   ✓
staging-us-east-1       networking   ingress-nginx   4.9.0     Running   ✓

# Compare clusters
vela cluster diff production-us-east-1 production-us-west-2
Comparing: production-us-east-1 ↔ production-us-west-2

Differences:
  Cluster Info:
    - Nodes: 12 vs 10
    - Region: us-east-1 vs us-west-2

  Plane Patches:
    networking/ingress-nginx:
      - controller.replicaCount: 5 vs 3

  Blueprint: Same (production-standard-v2.3.0)
  Planes: Same versions
  Components: Same versions

# ============================================
# DRIFT DETECTION AND COMPARISON
# ============================================

# Check drift against cluster's assigned blueprint
$ vela cluster drift production-us-east-1

Cluster: production-us-east-1
Blueprint: production-standard-v2.3.0
Status: No Drift Detected ✓

Last Check: 2024-12-24T10:00:00Z
Next Check: 2024-12-24T10:05:00Z (in 4m)

# Check drift against a DIFFERENT blueprint (what-if analysis)
$ vela cluster drift production-us-east-1 --blueprint staging-standard

Cluster: production-us-east-1
Comparing against: staging-standard-v2.1.0 (NOT the assigned blueprint)

Drift Report:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

PLANES COMPARISON:
  ┌───────────────┬────────────────────────────┬────────────────────────────┐
  │ PLANE         │ CLUSTER (current)          │ BLUEPRINT (target)         │
  ├───────────────┼────────────────────────────┼────────────────────────────┤
  │ networking    │ networking-v2.3.1          │ networking-v2.1.0          │
  │               │ ⚠ Ahead of blueprint       │                            │
  ├───────────────┼────────────────────────────┼────────────────────────────┤
  │ security      │ security-v1.8.0            │ security-v1.8.0            │
  │               │ ✓ Match                    │                            │
  ├───────────────┼────────────────────────────┼────────────────────────────┤
  │ observability │ observability-v3.1.0       │ (not in blueprint)         │
  │               │ ⚠ Extra plane              │                            │
  └───────────────┴────────────────────────────┴────────────────────────────┘

COMPONENT DIFFERENCES:
  networking/ingress-nginx:
    - Current: 4.8.3, Target: 4.6.0
    - Drift: Version ahead by 2 minor versions

  networking/cilium:
    - Current: 1.14.4, Target: 1.13.0
    - Drift: Version ahead by 1 minor version

  observability/prometheus-stack:
    - Current: 55.5.0, Target: (missing)
    - Drift: Component not in target blueprint

CONFIGURATION DIFFERENCES:
  networking/ingress-nginx:
    spec.values.controller.replicaCount:
      Current: 5
      Target:  2
    spec.values.controller.metrics.enabled:
      Current: true
      Target:  false

SUMMARY:
  Planes:     1 match, 1 ahead, 1 extra
  Components: 2 version differences, 1 missing
  Config:     2 field differences

This is a comparison only. No changes will be made.
To apply this blueprint, use: vela cluster update production-us-east-1 --blueprint staging-standard

# Detailed drift with specific output format
$ vela cluster drift production-us-east-1 --blueprint production-standard-v2.4.0 --output yaml

apiVersion: core.oam.dev/v1beta1
kind: ClusterDriftReport
metadata:
  name: production-us-east-1-drift
  generatedAt: "2024-12-24T10:15:00Z"
spec:
  cluster: production-us-east-1
  currentBlueprint: production-standard-v2.3.0
  targetBlueprint: production-standard-v2.4.0
  comparisonType: upgrade  # upgrade, downgrade, lateral
status:
  hasDrift: true
  summary:
    planesMatching: 2
    planesDrifted: 1
    componentsDrifted: 3
    configurationDrifted: 5
  planes:
    - name: networking
      status: Drifted
      currentRevision: networking-v2.3.1
      targetRevision: networking-v2.4.0
      components:
        - name: ingress-nginx
          status: VersionBehind
          currentVersion: "4.8.3"
          targetVersion: "4.9.0"
          configDrift:
            - path: spec.values.controller.config.use-gzip
              current: null
              target: "true"
        - name: cilium
          status: Match
    - name: security
      status: Match
    - name: observability
      status: Match

# Check drift for all clusters
$ vela cluster drift --all

Fleet Drift Summary
Total Clusters: 18

  ┌──────────────────────────┬─────────────────────────────┬──────────┬─────────────────────────────────────┐
  │ CLUSTER                  │ BLUEPRINT                   │ DRIFT    │ DETAILS                             │
  ├──────────────────────────┼─────────────────────────────┼──────────┼─────────────────────────────────────┤
  │ production-us-east-1     │ production-standard-v2.3.0  │ ✓ None   │ -                                   │
  │ production-us-west-2     │ production-standard-v2.3.0  │ ✓ None   │ -                                   │
  │ production-eu-west-1     │ production-standard-v2.3.0  │ ⚠ Config │ ingress-nginx replicas: 3→2         │
  │ staging-us-east-1        │ staging-standard-v2.1.0     │ ✓ None   │ -                                   │
  │ canary-us-east-1         │ production-standard-v2.4.0  │ ⚠ Behind │ Updating to v2.4.0 (in progress)    │
  └──────────────────────────┴─────────────────────────────┴──────────┴─────────────────────────────────────┘

By Status:
  No Drift:       15 clusters
  Config Drift:   2 clusters
  Version Behind: 1 cluster

# Compare ALL clusters against a specific blueprint (upgrade planning)
$ vela cluster drift --all --blueprint production-standard-v2.4.0

Comparing 18 clusters against: production-standard-v2.4.0

Upgrade Impact Analysis:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

CLUSTERS ALREADY AT v2.4.0: 1
  - canary-us-east-1 (updating)

CLUSTERS NEEDING UPGRADE: 14
  From production-standard-v2.3.0 (12 clusters):
    Changes:
      - networking/ingress-nginx: 4.8.3 → 4.9.0
      - networking plane config: +use-gzip, +http2
    Impact: Low risk, minor version bump

  From production-standard-v2.2.0 (2 clusters):
    Changes:
      - networking/ingress-nginx: 4.7.0 → 4.9.0
      - security/cert-manager: 1.12.0 → 1.13.3
    Impact: Medium risk, multiple upgrades

CLUSTERS ON DIFFERENT BLUEPRINTS: 3
  - staging-us-east-1 (staging-standard) - Would switch blueprint families
  - dev-cluster-1 (dev-minimal) - Not comparable
  - dev-cluster-2 (dev-minimal) - Not comparable

Recommended Rollout Order:
  1. canary-us-east-1 (already in progress)
  2. staging-us-east-1 (staging tier)
  3. 12 clusters from v2.3.0 (low risk)
  4. 2 clusters from v2.2.0 (medium risk, needs review)

# Filter drift by plane
$ vela cluster drift production-us-east-1 --plane networking

Cluster: production-us-east-1
Plane: networking

  ┌─────────────────┬─────────────┬─────────────┬────────────────────────────────┐
  │ COMPONENT       │ CURRENT     │ EXPECTED    │ DRIFT                          │
  ├─────────────────┼─────────────┼─────────────┼────────────────────────────────┤
  │ ingress-nginx   │ 4.8.3       │ 4.8.3       │ ✓ Version match                │
  │                 │             │             │ ⚠ Config: replicas 5→3         │
  ├─────────────────┼─────────────┼─────────────┼────────────────────────────────┤
  │ cilium          │ 1.14.4      │ 1.14.4      │ ✓ No drift                     │
  ├─────────────────┼─────────────┼─────────────┼────────────────────────────────┤
  │ external-dns    │ 1.14.3      │ 1.14.3      │ ✓ No drift                     │
  └─────────────────┴─────────────┴─────────────┴────────────────────────────────┘

# Show what resources actually drifted
$ vela cluster drift production-eu-west-1 --show-resources

Cluster: production-eu-west-1
Blueprint: production-standard-v2.3.0
Status: Configuration Drift Detected

Drifted Resources:
  ┌────────────────────────────────────────────┬─────────────────────────┬─────────────────────┐
  │ RESOURCE                                   │ FIELD                   │ DRIFT               │
  ├────────────────────────────────────────────┼─────────────────────────┼─────────────────────┤
  │ Deployment/ingress-nginx-controller        │ spec.replicas           │ 3 → 2 (manual edit) │
  │ Deployment/ingress-nginx-controller        │ spec.template.spec.     │ 512Mi → 256Mi       │
  │                                            │ containers[0].resources │                     │
  │                                            │ .limits.memory          │                     │
  ├────────────────────────────────────────────┼─────────────────────────┼─────────────────────┤
  │ ConfigMap/ingress-nginx-controller         │ data.use-gzip           │ "true" → (deleted)  │
  └────────────────────────────────────────────┴─────────────────────────┴─────────────────────┘

Drift detected at: 2024-12-24T08:30:00Z
Likely cause: Manual kubectl edit or external controller

Actions:
  1. Remediate: vela cluster remediate production-eu-west-1
  2. Accept drift: vela cluster drift accept production-eu-west-1 --resource Deployment/ingress-nginx-controller
  3. Update blueprint: vela blueprint update production-standard --from-cluster production-eu-west-1

# Remediate drift
$ vela cluster remediate production-eu-west-1

Remediating drift on production-eu-west-1...
  ⟳ Deployment/ingress-nginx-controller: restoring spec.replicas to 3
  ⟳ Deployment/ingress-nginx-controller: restoring memory limit to 512Mi
  ⟳ ConfigMap/ingress-nginx-controller: restoring use-gzip setting
  ✓ All resources remediated

Verification:
  ✓ Deployment/ingress-nginx-controller: 3/3 replicas ready
  ✓ ConfigMap/ingress-nginx-controller: restored

# Remediate with dry-run
$ vela cluster remediate production-eu-west-1 --dry-run

DRY RUN - No changes will be made

Would remediate:
  - Deployment/ingress-nginx-controller:
      spec.replicas: 2 → 3
      spec.template.spec.containers[0].resources.limits.memory: 256Mi → 512Mi
  - ConfigMap/ingress-nginx-controller:
      data.use-gzip: (add) "true"

To apply these changes, run without --dry-run

# Accept intentional drift (exclude from future detection)
$ vela cluster drift accept production-eu-west-1 \
    --resource Deployment/ingress-nginx-controller \
    --field spec.replicas \
    --reason "Scaled down for cost optimization in EU region"

Drift exception created:
  Cluster:  production-eu-west-1
  Resource: Deployment/ingress-nginx-controller
  Field:    spec.replicas
  Reason:   Scaled down for cost optimization in EU region
  Created:  2024-12-24T10:20:00Z
  By:       admin@example.com

This field will be excluded from drift detection until the exception is removed.
To remove: vela cluster drift exceptions remove production-eu-west-1 --id exc-123

# List all drift exceptions
$ vela cluster drift exceptions --all

  ┌──────────────────────────┬─────────────────────────────────────┬────────────────────┬─────────────────────────────────────┐
  │ CLUSTER                  │ RESOURCE                            │ FIELD              │ REASON                              │
  ├──────────────────────────┼─────────────────────────────────────┼────────────────────┼─────────────────────────────────────┤
  │ production-eu-west-1     │ Deployment/ingress-nginx-controller │ spec.replicas      │ Cost optimization in EU             │
  │ staging-us-east-1        │ ConfigMap/prometheus-config         │ data.scrape_interval│ Faster scraping for testing        │
  └──────────────────────────┴─────────────────────────────────────┴────────────────────┴─────────────────────────────────────┘

# Export drift report for review/audit
$ vela cluster drift production-us-east-1 --blueprint production-standard-v2.4.0 --output json > drift-report.json

# Compare with Terraform state (for adopted clusters)
$ vela cluster drift production-legacy --include-terraform

Cluster: production-legacy
Mode: Adopted (with Terraform state tracking)

Kubernetes Drift:
  ✓ No drift from blueprint

Terraform Infrastructure Drift:
  ⚠ EKS cluster:
      - instance_types: ["m5.large"] → ["m5.xlarge"] (changed outside Terraform)
  ⚠ VPC:
      - No drift
  ⚠ Security Groups:
      - Ingress rule added: 0.0.0.0/0:443 (not in Terraform)

Actions:
  1. Import to Terraform: terraform import ...
  2. Accept infrastructure drift: vela cluster drift accept --terraform ...

# View cluster history
vela cluster history production-us-east-1
REVISION                       APPLIED AT            APPLIED BY                  STATUS
production-standard-v2.3.0     2024-12-24T08:00:00Z  rollout/ingress-upgrade     Succeeded
production-standard-v2.2.0     2024-12-20T08:00:00Z  rollout/security-patch      Succeeded
production-standard-v2.1.0     2024-12-15T08:00:00Z  manual                      Succeeded

# ============================================
# PLANE MANAGEMENT
# ============================================

vela plane list
NAME           CATEGORY        OWNER              VERSION     COMPONENTS   CLUSTERS
networking     networking      networking-team    v2.3.1      3            5
security       security        security-team      v1.8.0      2            5
observability  observability   platform-team      v3.1.0      2            5
storage        storage         storage-team       v1.2.0      2            3

vela plane show networking
Plane: networking
  Category: networking
  Owner:    networking-team
  Version:  networking-v2.3.1

Components:
  NAME            TYPE          VERSION   DESCRIPTION
  ingress-nginx   helm-release  4.8.3     NGINX Ingress Controller
  cilium          helm-release  1.14.4    eBPF-based CNI
  external-dns    helm-release  1.14.3    External DNS management

Policies:
  - health-check (health)
  - dependency-order (apply-order)

Outputs:
  - ingressClass: nginx
  - clusterDNS: cluster.example.com

Used By Blueprints:
  - production-standard (5 clusters)
  - staging-standard (2 clusters)

vela plane apply -f networking-plane.yaml
vela plane status networking
vela plane diff networking  # Show pending changes

# ============================================
# BLUEPRINT MANAGEMENT
# ============================================

vela blueprint list
NAME                 PLANES   CLUSTERS   LATEST REVISION              STATUS
production-standard  3        5          production-standard-v2.3.0   Running
staging-standard     2        2          staging-standard-v2.1.0      Running
dev-minimal          1        3          dev-minimal-v1.0.0           Running

vela blueprint show production-standard
Blueprint: production-standard
  Description: Standard production cluster configuration
  Revision:    production-standard-v2.3.0

Planes:
  NAME           REVISION             PATCHES
  networking     networking-v2.3.1    controller.replicaCount: 3
  security       security-v1.8.0      -
  observability  observability-v3.1.0 -

Clusters Using This Blueprint:
  NAME                    REVISION     STATUS
  production-us-east-1    v2.3.0       Synced
  production-us-west-2    v2.3.0       Synced
  production-eu-west-1    v2.3.0       Synced
  production-ap-south-1   v2.3.0       Synced
  production-ap-north-1   v2.3.0       Synced

vela blueprint apply -f blueprint.yaml
vela blueprint status production-standard
vela blueprint clusters production-standard

# ============================================
# ROLLOUT MANAGEMENT
# ============================================

vela rollout list
NAME                    BLUEPRINT            TARGET        STATUS        PROGRESS
ingress-upgrade-4.9.0   production-standard  v2.4.0        Progressing   10% (1/5 clusters)
security-patch-dec      production-standard  v2.3.1        Succeeded     100%

vela rollout create --blueprint production-standard --revision v2.4.0
vela rollout status ingress-upgrade-4.9.0
Rollout: ingress-upgrade-4.9.0
  Blueprint: production-standard
  Target:    production-standard-v2.4.0
  Source:    production-standard-v2.3.0
  Strategy:  Canary

Progress: Step 1/3 (10%)
  ┌─────────────────────┬───────────────────────┬──────────────────┐
  │ CLUSTER             │ STATUS                │ REVISION         │
  ├─────────────────────┼───────────────────────┼──────────────────┤
  │ production-canary   │ ✓ Updated             │ v2.4.0           │
  │ production-us-east  │ ○ Pending             │ v2.3.0           │
  │ production-us-west  │ ○ Pending             │ v2.3.0           │
  │ production-eu-west  │ ○ Pending             │ v2.3.0           │
  │ production-ap-south │ ○ Pending             │ v2.3.0           │
  └─────────────────────┴───────────────────────┴──────────────────┘

Analysis (last 5m):
  error-rate:   0.2%  ✓ (threshold: <1%)
  p99-latency:  120ms ✓ (threshold: <500ms)
  pod-restarts: 0     ✓ (threshold: <5)

Next Step: Waiting 25m before proceeding to 50%

vela rollout pause ingress-upgrade-4.9.0
vela rollout resume ingress-upgrade-4.9.0
vela rollout promote ingress-upgrade-4.9.0  # Skip to 100%
vela rollout rollback ingress-upgrade-4.9.0 --reason "Critical bug"
vela rollout history production-standard
```

### kubectl vela Plugin

The kubectl vela plugin provides seamless integration:

```bash
# All vela cluster commands work with kubectl
kubectl vela cluster list
kubectl vela cluster show production-us-east-1
kubectl vela plane list
kubectl vela blueprint show production-standard
kubectl vela rollout status ingress-upgrade-4.9.0

# Direct kubectl commands also work
kubectl get clusters -A
kubectl get clusterplanes -A
kubectl get clusterblueprints -A
kubectl get clusterrollouts -A
kubectl describe cluster production-us-east-1
```

---

## References

- [OAM Spec](https://github.com/oam-dev/spec)
- [KubeVela Application CRD](https://kubevela.io/docs/core-concepts/application)
- [Argo Rollouts](https://argoproj.github.io/argo-rollouts/)
- [Flux HelmRelease](https://fluxcd.io/flux/components/helm/)
- [Crossplane Compositions](https://docs.crossplane.io/latest/concepts/compositions/)
