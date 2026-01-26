# KEP: Cluster Infrastructure Abstraction

**Authors**: Anoop Gopalakrishnan
**Status**: Draft
**Created**: 2025-12-24
**Last Updated**: 2026-01-03

## Table of Contents

- [Introduction](#introduction)
- [Background](#background)
  - [The Problem with Application-Centric Only](#the-problem-with-application-centric-only)
  - [What Platform Teams Need](#what-platform-teams-need)
  - [Relationship to Existing Multi-Cluster Architecture](#relationship-to-existing-multi-cluster-architecture)
  - [Controller Ownership Model (Circular Reference Prevention)](#controller-ownership-model-circular-reference-prevention)
- [Goals](#goals)
- [Non-Goals](#non-goals)
- [Proposal](#proposal)
  - [Core CRDs](#core-crds)
    - [Cluster](#1-cluster)
    - [ClusterPlane](#2-clusterplane)
    - [ClusterPlane Versioning Strategy](#clusterplane-versioning-strategy)
    - [ClusterPlaneRevision CRD](#clusterplanerevision-crd)
    - [Cross-Cluster Dependency Handling](#cross-cluster-dependency-handling)
    - [Shared Infrastructure Planes](#shared-infrastructure-planes)
    - [ClusterPlane Workflow and Deployment Order](#clusterplane-workflow-and-deployment-order)
    - [ClusterBlueprint](#3-clusterblueprint)
    - [ClusterBlueprint Versioning Strategy](#clusterblueprint-versioning-strategy)
    - [Version Constraint Resolution](#version-constraint-resolution)
    - [ClusterBlueprintRevision CRD](#clusterblueprintrevision-crd)
    - [ClusterRollout (Optional)](#4-clusterrollout-optional---for-emergencymanual-overrides)
    - [ClusterRolloutStrategy](#5-clusterrolloutstrategy)
    - [Maintenance Window Enforcement](#maintenance-window-enforcement)
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

We introduce the following CRDs:

**Core CRDs:**

| CRD                            | Description                                                                                |
| ------------------------------ | ------------------------------------------------------------------------------------------ |
| **`Cluster`**                  | First-class representation of a managed cluster with full inventory, health, and status    |
| **`ClusterPlane`**             | A composable infrastructure layer owned by a team (e.g., networking plane, security plane) |
| **`ClusterPlaneRevision`**     | Immutable snapshot of a ClusterPlane at a specific version                                 |
| **`ClusterBlueprint`**         | A complete cluster specification composed of multiple ClusterPlanes                        |
| **`ClusterBlueprintRevision`** | Immutable snapshot of a ClusterBlueprint at a specific version                             |
| **`ClusterRolloutStrategy`**   | Shared rollout strategy that defines wave-based progression across cluster fleet           |
| **`ClusterRollout`**           | (Optional) Imperative rollout for emergency/manual overrides                               |
| **`ClusterRolloutCheckpoint`** | Checkpoint state for paused rollouts during maintenance window transitions                 |

**Definition CRDs (Extensibility):**

| CRD                                   | Description                                                                |
| ------------------------------------- | -------------------------------------------------------------------------- |
| **`ClusterProviderDefinition`**       | Defines cloud provider integration for cluster provisioning                |
| **`PlaneComponentDefinition`**        | Defines component types for ClusterPlanes (similar to ComponentDefinition) |
| **`PlaneTraitDefinition`**            | Defines trait types for ClusterPlanes (similar to TraitDefinition)         |
| **`PlanePolicyDefinition`**           | Defines policy types for ClusterPlanes                                     |
| **`ClusterWorkflowStepDefinition`**   | Defines workflow steps for cluster lifecycle operations                    |
| **`ObservabilityProviderDefinition`** | Defines observability provider types (Prometheus, Datadog, etc.)           |

**Runtime CRDs:**

| CRD                         | Description                                                       |
| --------------------------- | ----------------------------------------------------------------- |
| **`ObservabilityProvider`** | Instance of an observability provider with connection details     |
| **`ClusterDriftReport`**    | Report of detected drift between desired and actual cluster state |
| **`ClusterDriftException`** | Allowlist for expected drift that should not trigger alerts       |

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
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐              │
│  │  Networking     │  │    Security     │  │  Observability  │              │
│  │     Team        │  │      Team       │  │      Team       │              │
│  │                 │  │                 │  │                 │              │
│  │  - Ingress      │  │  - OPA/Gatekeeper│ │  - Prometheus   │              │
│  │  - CNI          │  │  - Cert-manager │  │  - Grafana      │              │
│  │  - Service Mesh │  │  - Secrets mgmt │  │  - Logging      │              │
│  │  - DNS          │  │  - Network Pol  │  │  - Tracing      │              │
│  └────────┬────────┘  └────────┬────────┘  └────────┬────────┘              │
│           │                    │                    │                       │
│           ▼                    ▼                    ▼                       │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                        ClusterBlueprint                             │    │
│  │                                                                     │    │
│  │   Composes: NetworkingPlane + SecurityPlane + ObservabilityPlane    │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                    │                                        │
│                                    ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                         ClusterRollout                              │    │
│  │                                                                     │    │
│  │   Strategy: Canary 10% → 50% → 100%                                 │    │
│  │   Monitoring: Error rate < 1%, Latency p99 < 100ms                  │    │
│  │   Rollback: Automatic on SLO breach                                 │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                    │                                        │
│                                    ▼                                        │
│           ┌────────────┬────────────┬────────────┬────────────┐             │
│           │ cluster-1  │ cluster-2  │ cluster-3  │ cluster-N  │             │
│           └────────────┴────────────┴────────────┴────────────┘             │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

### Relationship to Existing Multi-Cluster Architecture

KubeVela currently uses **cluster-gateway** (`github.com/oam-dev/cluster-gateway`) for multi-cluster connectivity. It's important to understand how the proposed `Cluster` CRD relates to the existing architecture:

#### Current Architecture: VirtualCluster

```
┌────────────────────────────────────────────────────────────────────────┐
│                    CURRENT: cluster-gateway                            │
├────────────────────────────────────────────────────────────────────────┤
│                                                                        │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ Secret (vela-system)                                            │   │
│  │   name: cluster-production-us-east-1                            │   │
│  │   labels:                                                       │   │
│  │     cluster.core.oam.dev/cluster-credential-type: X509          │   │
│  │   data:                                                         │   │
│  │     endpoint: <base64>                                          │   │
│  │     ca.crt: <base64>                                            │   │
│  │     tls.crt: <base64>                                           │   │
│  │     tls.key: <base64>                                           │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                              │                                         │
│                              ▼                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ VirtualCluster (cluster-gateway CRD)                            │   │
│  │   - Provides API proxy to remote cluster                        │   │
│  │   - Handles authentication/authorization                        │   │
│  │   - No infrastructure state, just connectivity                  │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                        │
└────────────────────────────────────────────────────────────────────────┘
```

**Limitations of current approach:**

- No declarative "what should be on this cluster"
- No versioned infrastructure specification
- No progressive rollout for cluster changes
- No composition or team ownership boundaries
- Clusters are just connection endpoints, not managed resources

#### Proposed Architecture: Self-Sufficient Cluster CRD

**Key Architectural Decision:** The Cluster CRD manages connectivity **directly**. This ensures the core CRD is self-sufficient.

```
┌────────────────────────────────────────────────────────────────────────┐
│           PROPOSED: Cluster CRD (Self-Sufficient Connectivity)         │
├────────────────────────────────────────────────────────────────────────┤
│                                                                        │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ Cluster CRD (core.oam.dev/v1beta1)                              │   │
│  │                                                                 │   │
│  │   INTENT & STATE:                                               │   │
│  │   - blueprintRef: production-standard-v2.3.0                    │   │
│  │   - patches: cluster-specific overrides                         │   │
│  │   - inventory: what's deployed                                  │   │
│  │   - health: aggregated status                                   │   │
│  │   - lifecycle: provision/adopt/connect/infrastructure mode      │   │
│  │                                                                 │   │
│  │   CONNECTIVITY (self-managed by ClusterController):             │   │
│  │   ┌───────────────────────────────────────────────────────────┐ │   │
│  │   │ Option 1: Inline Credentials                              │ │   │
│  │   │   credential:                                             │ │   │
│  │   │     type: X509 | ServiceAccountToken | Bearer             │ │   │
│  │   │     endpoint: "https://..."                               │ │   │
│  │   │     caData: "..."                                         │ │   │
│  │   ├───────────────────────────────────────────────────────────┤ │   │
│  │   │ Option 2: Secret Reference (kubeconfig)                   │ │   │
│  │   │   credential:                                             │ │   │
│  │   │     secretRef:                                            │ │   │
│  │   │       name: my-kubeconfig                                 │ │   │
│  │   │       key: kubeconfig                                     │ │   │
│  │   ├───────────────────────────────────────────────────────────┤ │   │
│  │   │ Option 3: Cloud Provider Native (IAM, workload identity)  │ │   │
│  │   │   credential:                                             │ │   │
│  │   │     cloudProvider:                                        │ │   │
│  │   │       type: aws-eks | gcp-gke | azure-aks                 │ │   │
│  │   └───────────────────────────────────────────────────────────┘ │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                        │
│  ClusterController handles connectivity directly:                      │
│  - Creates internal client for remote cluster                          │
│  - Self-contained connectivity management                              │
│                                                                        │
└────────────────────────────────────────────────────────────────────────┘
```

#### Design Principles

1. **Self-Sufficient**: Cluster CRD functions independently
2. **Pluggable Connectivity**: Multiple credential options supported (inline, secretRef, cloudProvider)
3. **Layered Abstraction**: `Cluster` CRD adds intent/state layer on top of connectivity
4. **Single Source of Truth**: `Cluster` CRD becomes the authoritative record for managed clusters
5. **Clear Ownership Boundaries**: Controllers NEVER modify `spec` fields they don't own (prevents circular references)

#### Controller Ownership Model (Circular Reference Prevention)

A critical design principle is **preventing circular references** between controllers. Each controller has explicit ownership boundaries:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    CONTROLLER OWNERSHIP MODEL (NO CYCLES)                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  COMPONENT                  OWNER                 CONTROLLERS CAN MODIFY?   │
│  ─────────                 ─────                 ────────────────────────   │
│                                                                             │
│  ClusterBlueprint          User/GitOps           NEVER (immutable template) │
│                                                                             │
│  Cluster.spec.blueprintRef User/GitOps           NEVER by controllers       │
│                                                  (this is desired state)    │
│                                                                             │
│  Cluster.status.blueprint  ClusterController     YES (actual applied state) │
│  Cluster.status.health     ClusterController     YES                        │
│  Cluster.status.inventory  ClusterController     YES                        │
│  Cluster.status.maintenance ClusterController    YES (window computation)   │
│                                                                             │
│  RolloutStrategy.status    ClusterRolloutCtrl    YES (wave/progress status) │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Why This Matters:**

Without clear ownership, the following cycle could occur:

1. `Cluster.spec.blueprintRef` → references `ClusterBlueprint`
2. `ClusterBlueprint` → defines planes to deploy
3. `ClusterPlane` → deploys resources that affect cluster health
4. `Cluster.status.health` → affects rollout progression
5. **BAD**: Rollout controller updates `Cluster.spec.blueprintRef` → cycle!

**The Correct Flow (No Cycle):**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         CORRECT UPDATE FLOW                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. USER/GITOPS updates Cluster.spec.blueprintRef to new version            │
│     │                                                                       │
│     ▼                                                                       │
│  2. ClusterController detects spec.blueprintRef != status.blueprint         │
│     │                                                                       │
│     ▼                                                                       │
│  3. ClusterController queries ClusterRolloutController: "Can I update?"     │
│     │                                                                       │
│     ├─── NO: Rollout says "wait" (wave not ready, window closed, etc.)      │
│     │         → ClusterController waits, requeues                           │
│     │                                                                       │
│     └─── YES: Rollout says "proceed"                                        │
│           │                                                                 │
│           ▼                                                                 │
│  4. ClusterController applies blueprint to cluster                          │
│     │                                                                       │
│     ▼                                                                       │
│  5. ClusterController updates status.blueprint, status.health               │
│     │                                                                       │
│     ▼                                                                       │
│  6. ClusterRolloutController reads status.health for wave progression       │
│     (but NEVER modifies spec.blueprintRef - that's User/GitOps's job)       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Key Invariants:**

| Invariant                                    | Description                                                                                                                    |
| -------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| **ClusterBlueprint is immutable**            | Once created, a blueprint version never changes. New versions create new `ClusterBlueprintRevision` objects.                   |
| **spec.blueprintRef is user-owned**          | Only users or external automation (GitOps) modify `Cluster.spec.blueprintRef`. Controllers NEVER touch it.                     |
| **status.blueprint is controller-owned**     | Only `ClusterController` modifies `status.blueprint` after successful application.                                             |
| **Rollout controls timing, not state**       | `ClusterRolloutController` gates WHEN updates happen via a "can proceed" signal. It never modifies cluster spec or blueprints. |
| **Health affects progression, not triggers** | Cluster health affects rollout wave progression decisions, but health changes don't trigger spec modifications.                |

#### Migration Path

| Stage                     | Cluster CRD                  | Connectivity                                 | Behavior                                                         |
| ------------------------- | ---------------------------- | -------------------------------------------- | ---------------------------------------------------------------- |
| **Stage 0** (current)     | Not used                     | Manual kubeconfig management                 | Current behavior, no change                                      |
| **Stage 1** (adoption)    | Created with `mode: connect` | Any supported method (inline, secretRef, cloudProvider) | Cluster CRD tracks state                       |
| **Stage 2** (managed)     | Full spec with blueprint     | ClusterController manages connectivity       | Controller ensures connectivity, applies infrastructure          |
| **Stage 3** (provisioned) | `mode: provision`            | Auto-created from provisioning outputs       | Cluster CRD provisions cluster AND creates connectivity          |

#### Controller Reconciliation

**ClusterController reconcile algorithm:**

1. Establish connectivity to target cluster (using spec.credential)
2. Create internal client for remote cluster
3. **Always** update `status.maintenance` (compute window state)
4. If `spec.blueprintRef.revision` ≠ `status.blueprint.revision`:
   - Check rollout permission (read-only query to ClusterRolloutStrategy)
   - If denied: requeue and wait
   - If approved: apply blueprint, update `status.blueprint`
5. Update inventory and health status

**Rollout permission check:**

- No strategy → allow immediately
- Strategy with `respectClusterWindows: true` → check `status.maintenance.inWindow`
- Check wave progression status → allow if wave permits

**Key ownership boundaries:**

- `ClusterController` READS `spec.blueprintRef`, WRITES `status.blueprint`
- `ClusterController` NEVER modifies `spec` or `ClusterBlueprint`
- `ClusterRolloutController` gates timing via status fields

#### Controller Responsibilities Matrix

**Critical Design Principle:** ClusterPlane is ONLY reconciled when referenced by a Cluster. Creating a ClusterPlane CRD does NOT create infrastructure resources—the Cluster CRD is the reconciliation trigger.

| Controller | Responsibilities | Does NOT Do |
|------------|------------------|-------------|
| **ClusterController** | • Triggers ALL resource reconciliation<br>• Resolves Blueprint → Planes<br>• For shared planes: first Cluster triggers creation, others consume outputs<br>• For perCluster planes: creates instance per cluster<br>• Manages connectivity directly<br>• Aggregates health/inventory | • Never modifies ClusterPlane or ClusterBlueprint specs<br>• Never creates resources without a Cluster trigger |
| **ClusterPlaneController** | • Creates ClusterPlaneRevision on publishVersion<br>• Validates inputs/outputs schema<br>• Tracks consumers (updates status.consumers)<br>• Computes status based on Cluster reports | • Does NOT create infrastructure resources<br>• Does NOT dispatch to clusters |
| **ClusterBlueprintController** | • Creates ClusterBlueprintRevision on publishVersion<br>• Validates plane composition<br>• Resolves version constraints | • Does NOT dispatch to clusters (pull model)<br>• Does NOT modify Cluster specs |
| **ClusterRolloutController** | • Manages wave progression timing<br>• Enforces maintenance windows<br>• Gates blueprint transitions | • Does NOT apply blueprints (only gates timing)<br>• Does NOT modify Cluster specs |

**Reconciliation Flow:**

```
User creates/updates Cluster with blueprintRef
           │
           ▼
┌─────────────────────────────────────────────────────────────┐
│ ClusterController                                           │
│   1. Check rollout permission (via ClusterRolloutController)│
│   2. Resolve Blueprint → list of ClusterPlanes              │
│   3. For each Plane:                                        │
│      ├─ scope=shared AND already reconciled?                │
│      │    → Consume outputs from existing instance          │
│      ├─ scope=shared AND first consumer?                    │
│      │    → Reconcile shared plane, store outputs           │
│      └─ scope=perCluster?                                   │
│           → Reconcile new instance for this cluster         │
│   4. Update Cluster status with plane statuses              │
└─────────────────────────────────────────────────────────────┘
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
4. **Implementing cloud provider APIs** - We integrate with existing providers (Crossplane, KRO, ACK, Terraform etc) rather than reimplementing

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
            replicaCount: 5 # This cluster needs more replicas

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
              - condition: "< 0.5%" # Stricter than strategy default
      # Skip certain waves (useful for canary clusters)
      # skipWaves: [non-critical, critical]

  # Maintenance windows for this cluster
  # See "Maintenance Window Enforcement" section for details
  maintenance:
    windows:
      - name: weekend-maintenance
        start: "02:00"
        end: "06:00"
        timezone: "America/New_York" # IANA timezone name
        days: [Sat, Sun]
        dstPolicy: extend # extend | shrink | skip (DST handling)
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
  connectionStatus: Connected # Connected, Disconnected, Unknown
  lastProbeTime: "2024-12-24T10:00:00Z"
  latency: "45ms"

  # Cluster information (auto-discovered)
  clusterInfo:
    kubernetesVersion: "v1.28.5"
    platform: "eks" # eks, gke, aks, kind, k3s, etc.
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
    status: Synced # Synced, OutOfSync, Updating, Failed

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
    status: Healthy # Healthy, Degraded, Unhealthy, Unknown
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

  # Maintenance window state (computed by ClusterController)
  # See "Maintenance Window Enforcement" section for details
  maintenance:
    # Is the cluster currently in a maintenance window?
    inWindow: true
    # Current active window (populated when inWindow is true)
    currentWindow:
      name: weekend-maintenance
      startedAt: "2024-12-24T07:00:00Z"
      endsAt: "2024-12-24T11:00:00Z"
      remainingMinutes: 120
    # Next scheduled window
    nextWindow:
      name: weeknight-maintenance
      startsAt: "2024-12-25T08:00:00Z"
      startsInMinutes: 1320
    # Last time windows were evaluated
    lastEvaluatedAt: "2024-12-24T09:00:00Z"
    # Timezone information
    timezoneInfo:
      name: "America/New_York"
      currentOffset: "-05:00"
      isDST: false

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

##### Reconciliation Trigger Model

**Critical:** A ClusterPlane is a **template**, not a self-reconciling resource. Creating a ClusterPlane CRD does NOT create infrastructure resources.

| Action | What Happens | What Does NOT Happen |
|--------|--------------|----------------------|
| Create ClusterPlane | Validates schema, creates ClusterPlaneRevision if publishVersion set | Does NOT create infrastructure |
| Update ClusterPlane | Re-validates, creates new revision if version bumped | Does NOT update infrastructure |
| Cluster references Blueprint containing Plane | **NOW infrastructure is created** | — |
| Delete ClusterPlane | Blocked if consumers exist (scope=shared) | — |

**When is a ClusterPlane reconciled?**

```
ClusterPlane created → Nothing happens (it's just a template)
                       │
                       │  Later...
                       ▼
Cluster created with blueprintRef → ClusterController resolves Blueprint
                       │
                       ▼
               For each Plane in Blueprint:
               ┌─────────────────────────────────────────────┐
               │ scope=shared?                               │
               │   ├─ Already reconciled? → Consume outputs  │
               │   └─ First time? → Reconcile NOW            │
               │                                             │
               │ scope=perCluster?                           │
               │   └─ Reconcile for THIS cluster             │
               └─────────────────────────────────────────────┘
```

This **pull model** ensures:
- ClusterPlanes are reusable templates
- No orphaned infrastructure (every resource tied to a Cluster)
- Clear lifecycle (Cluster deletion triggers cleanup)

##### GitOps Integration

The Cluster CRD is designed to work seamlessly with GitOps tools. Since reconciliation is triggered by **Cluster CRD changes**, standard GitOps workflows apply:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         GITOPS RECONCILIATION FLOW                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────────────────────┐ │
│  │  Git Repo    │     │  GitOps Tool │     │     KubeVela Controllers     │ │
│  │              │     │              │     │                              │ │
│  │ cluster.yaml │────▶│ Flux/ArgoCD  │────▶│  ClusterController detects   │ │
│  │ (updated)    │     │ syncs change │     │  Cluster spec change and     │ │
│  │              │     │ to K8s API   │     │  reconciles infrastructure   │ │
│  └──────────────┘     └──────────────┘     └──────────────────────────────┘ │
│                                                                             │
│  Examples:                                                                  │
│  • Update spec.blueprintRef.version → triggers blueprint upgrade            │
│  • Create new Cluster CRD → triggers plane reconciliation                   │
│  • Delete Cluster CRD → triggers infrastructure cleanup                     │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Supported GitOps Tools:**

| Tool | Integration Pattern |
|------|---------------------|
| **Flux CD** | `Kustomization` or `HelmRelease` syncing Cluster CRDs from Git |
| **Argo CD** | `Application` tracking Cluster manifests in Git repository |
| **Rancher Fleet** | `GitRepo` with paths to Cluster definitions |
| **Jenkins X** | Pipeline-driven `kubectl apply` of Cluster CRDs |

**Example: Flux CD Integration**

```yaml
# flux-system/cluster-infrastructure.yaml
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: cluster-infrastructure
  namespace: flux-system
spec:
  interval: 5m
  sourceRef:
    kind: GitRepository
    name: infrastructure-repo
  path: ./clusters/production
  prune: true
  # Flux syncs Cluster CRDs → KubeVela ClusterController reconciles
```

**Example: Argo CD Integration**

```yaml
# argocd/cluster-app.yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: production-clusters
  namespace: argocd
spec:
  project: infrastructure
  source:
    repoURL: https://github.com/org/cluster-definitions
    targetRevision: main
    path: clusters/production
  destination:
    server: https://kubernetes.default.svc
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    # ArgoCD syncs Cluster CRDs → KubeVela ClusterController reconciles
```

**Key Principle:** GitOps tools manage the **desired state** (Cluster CRDs in Git), KubeVela's ClusterController manages the **actual state** (infrastructure reconciliation). This separation allows teams to use their existing GitOps workflows without modification.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterPlane
metadata:
  name: networking
  namespace: vela-system
  labels:
    plane.oam.dev/owner: networking-team
    plane.oam.dev/category: networking
  annotations:
    # Publishing follows Application's publishVersion pattern
    # No annotation = draft (mutable), with annotation = creates immutable ClusterPlaneRevision
    plane.oam.dev/publishVersion: "2.3.1"
spec:
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
  # Follows the same model as Application: components can have dependsOn
  components:
    - name: cilium
      type: helm-release
      # No dependsOn - deploys first (or in parallel with others that have no deps)
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

    - name: ingress-nginx
      type: helm-release
      dependsOn: [cilium] # <-- Wait for CNI to be ready before deploying ingress
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

    - name: external-dns
      type: helm-release
      dependsOn: [cilium] # <-- Also waits for CNI; deploys in parallel with ingress-nginx
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

  # Optional: Explicit workflow for advanced orchestration
  # If not specified, auto-generates deploy steps using component dependsOn (same as Application)
  # workflow:
  #   steps:
  #     - name: deploy-cni
  #       type: deploy
  #       properties:
  #         components: [cilium]
  #     - name: validate-cni
  #       type: script
  #       dependsOn: [deploy-cni]
  #       properties:
  #         image: cilium/cilium-cli:latest
  #         command: ["cilium", "status", "--wait"]
  #     - name: deploy-rest
  #       type: deploy
  #       dependsOn: [validate-cni]
  #       properties:
  #         components: [ingress-nginx, external-dns]

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

  # Inputs consumed from other planes (same-cluster shared planes)
  # Used to consume outputs from shared planes defined in the same blueprint
  inputs:
    - name: vpcId
      fromPlane: shared-vpc-us-east-1   # Reference to a shared plane
      output: vpcId                      # Output name from that plane
      required: true                     # Fail if output not available

    - name: privateSubnets
      fromPlane: shared-vpc-us-east-1
      output: privateSubnets

  # Cross-cluster inputs (from planes in OTHER clusters) - NEW
  crossClusterInputs:
    - name: centralVaultEndpoint
      fromCluster: management-cluster
      fromPlane: security
      output: vaultEndpoint

    - name: centralPrometheusEndpoint
      fromCluster: observability-hub
      fromPlane: observability
      output: prometheusEndpoint

status:
  phase: Running # Pending, Provisioning, Running, Failed, Updating

  # Current active revision
  currentRevision: networking-v2.3.1
  currentVersion: "2.3.1"

  # Revision history (immutable snapshots)
  revisions:
    - name: networking-v2.3.1
      version: "2.3.1"
      created: "2024-12-24T10:00:00Z"
      createdBy: "jane@company.com"
      digest: "sha256:abc123..." # Hash of spec for integrity
      changelog: "Updated ingress-nginx to 4.8.3 (security patch)"
      active: true # Currently deployed

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

##### Cloud Infrastructure as a ClusterPlane

A key design principle is that **all cluster infrastructure, including cloud resources like VPC, EKS, and node pools, should be expressed as ClusterPlane components**. This ensures:

1. **Composability**: Cloud infrastructure uses the same model as application infrastructure
2. **Versioning**: VPC/cluster changes are versioned and can be rolled back
3. **Separation of concerns**: Cloud provisioning is a plane owned by the platform team
4. **Blueprint integration**: Everything is in the blueprint, not scattered across CRDs

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterPlane
metadata:
  name: aws-foundation
  namespace: vela-system
  labels:
    plane.oam.dev/owner: platform-team
    plane.oam.dev/category: cloud-infrastructure
  annotations:
    plane.oam.dev/publishVersion: "1.2.0"
spec:
  description: "AWS cloud infrastructure foundation - VPC, EKS cluster, and node pools"

  changelog: |
    ## 1.2.0
    - Upgraded to Kubernetes 1.28
    - Added GPU node pool for ML workloads

  # Cloud infrastructure expressed as components
  # Uses terraform-module or crossplane-resource component types
  components:
    - name: vpc
      type: terraform-module
      properties:
        source: "terraform-aws-modules/vpc/aws"
        version: "5.0.0"
        values:
          name: "${cluster.name}-vpc"
          cidr: "10.0.0.0/16"
          azs:
            [
              "${provider.region}a",
              "${provider.region}b",
              "${provider.region}c",
            ]
          private_subnets: ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]
          public_subnets: ["10.0.101.0/24", "10.0.102.0/24", "10.0.103.0/24"]
          enable_nat_gateway: true
          single_nat_gateway: false
          enable_dns_hostnames: true
          tags:
            "kubernetes.io/cluster/${cluster.name}": "shared"

    - name: eks-cluster
      type: terraform-module
      dependsOn: [vpc] # Wait for VPC to be ready
      properties:
        source: "terraform-aws-modules/eks/aws"
        version: "19.0.0"
        values:
          cluster_name: "${cluster.name}"
          cluster_version: "1.28"
          vpc_id: "${vpc.outputs.vpc_id}"
          subnet_ids: "${vpc.outputs.private_subnets}"
          cluster_endpoint_public_access: false
          cluster_endpoint_private_access: true
          enable_irsa: true
          cluster_addons:
            coredns:
              most_recent: true
            kube-proxy:
              most_recent: true
            vpc-cni:
              most_recent: true

    - name: node-pool-system
      type: terraform-module
      dependsOn: [eks-cluster]
      properties:
        source: "terraform-aws-modules/eks/aws//modules/eks-managed-node-group"
        values:
          name: "system"
          cluster_name: "${eks-cluster.outputs.cluster_name}"
          subnet_ids: "${vpc.outputs.private_subnets}"
          instance_types: ["m5.large"]
          min_size: 3
          max_size: 6
          desired_size: 3
          labels:
            role: system
          taints:
            - key: CriticalAddonsOnly
              value: "true"
              effect: NO_SCHEDULE

    - name: node-pool-workload
      type: terraform-module
      dependsOn: [eks-cluster]
      properties:
        source: "terraform-aws-modules/eks/aws//modules/eks-managed-node-group"
        values:
          name: "workload"
          cluster_name: "${eks-cluster.outputs.cluster_name}"
          subnet_ids: "${vpc.outputs.private_subnets}"
          instance_types: ["m5.xlarge", "m5.2xlarge"]
          min_size: 2
          max_size: 20
          desired_size: 3
          capacity_type: "SPOT" # Cost optimization
          labels:
            role: workload

  # Outputs used by other planes and for connectivity setup
  outputs:
    - name: vpcId
      valueFrom:
        component: vpc
        fieldPath: outputs.vpc_id

    - name: clusterEndpoint
      valueFrom:
        component: eks-cluster
        fieldPath: outputs.cluster_endpoint

    - name: clusterCertificateAuthority
      valueFrom:
        component: eks-cluster
        fieldPath: outputs.cluster_certificate_authority_data

    - name: clusterName
      valueFrom:
        component: eks-cluster
        fieldPath: outputs.cluster_name

status:
  phase: Running
  currentRevision: aws-foundation-v1.2.0
  components:
    - name: vpc
      healthy: true
      message: "VPC created: vpc-0abc123"
    - name: eks-cluster
      healthy: true
      message: "EKS cluster ready: 1.28"
    - name: node-pool-system
      healthy: true
      message: "3/3 nodes ready"
    - name: node-pool-workload
      healthy: true
      message: "3/3 nodes ready"
  outputs:
    vpcId: "vpc-0abc123def456"
    clusterEndpoint: "https://ABC123.gr7.us-east-1.eks.amazonaws.com"
    clusterName: "production-us-east-1"
```

**Why Cloud Infrastructure as a ClusterPlane?**

| Concern        | Without (clusterSpec)            | With (ClusterPlane)                         |
| -------------- | -------------------------------- | ------------------------------------------- |
| Versioning     | Embedded in Cluster CRD          | Independent versioning, immutable revisions |
| Reusability    | Copy-paste across clusters       | Reference same plane revision               |
| Team ownership | Platform owns entire Cluster CRD | Networking team owns VPC, platform owns EKS |
| Testing        | Test entire cluster provisioning | Test VPC plane independently                |
| Rollback       | Rollback entire cluster          | Rollback just the component that failed     |
| GitOps         | Large Cluster CRDs in git        | Modular plane definitions                   |

##### ClusterPlane Versioning Strategy

ClusterPlane uses semantic versioning with immutable revisions, following the same pattern as KubeVela's Application CRD. Version publishing is controlled via the `plane.oam.dev/publishVersion` annotation.

**Version Semantics (SemVer):** MAJOR (breaking changes), MINOR (new features), PATCH (bug fixes)

**Publishing Flow:**

1. **Draft mode**: No annotation → iterate freely, no revision created
2. **Publish**: Add `plane.oam.dev/publishVersion: "2.3.1"` → creates immutable `ClusterPlaneRevision/networking-v2.3.1`
3. **Continue**: Bump version to "2.4.0" → new revision, previous remains available

**Version Collision Rules:**
| Scenario | Result |
|----------|--------|
| Same version + same content | SUCCESS (idempotent, GitOps-safe) |
| Same version + different content | REJECTED (must bump version) |
| Delete revision referenced by blueprint | REJECTED |

**Admission Webhook for Version Validation:**

```yaml
# Admission webhook validates version changes and collision prevention
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: clusterplane-version-validation
webhooks:
  - name: version.plane.oam.dev
    rules:
      - apiGroups: ["core.oam.dev"]
        resources: ["clusterplanes"]
        operations: ["CREATE", "UPDATE"]
    # Validates:
    # 1. If publishVersion annotation exists, check for collision
    # 2. Same version + different content = REJECT
    # 3. Same version + same content = ALLOW (idempotent)
  - name: revision.plane.oam.dev
    rules:
      - apiGroups: ["core.oam.dev"]
        resources: ["clusterplanerevisions"]
        operations: ["DELETE"]
    # Prevents deletion of revisions referenced by blueprints
```

**How Teams Publish Versions:**

```yaml
# STEP 1: Draft mode - iterate on the plane without publishing
apiVersion: core.oam.dev/v1beta1
kind: ClusterPlane
metadata:
  name: networking
  # No publishVersion annotation = draft mode
spec:
  owner:
    team: platform-networking
    contacts: ["netops@company.com"]

  components:
    - name: ingress-nginx
      type: helm-release
      properties:
        chart: ingress-nginx
        version: "4.9.0"
    # ... rest of components
---
# STEP 2: Ready to publish - add the annotation
apiVersion: core.oam.dev/v1beta1
kind: ClusterPlane
metadata:
  name: networking
  annotations:
    # Publish version using resource-specific annotation
    plane.oam.dev/publishVersion: "2.4.0"
spec:
  owner:
    team: platform-networking
    contacts: ["netops@company.com"]

  # Changelog documents what changed (recommended but optional)
  changelog: |
    ## 2.4.0
    - Added Gateway API support
    - Upgraded ingress-nginx to 4.9.0
    - BREAKING: Removed legacy annotation support

  components:
    - name: ingress-nginx
      type: helm-release
      properties:
        chart: ingress-nginx
        version: "4.9.0"
    # ... rest of components
```

**Publishing with kubectl apply (GitOps-compatible):**

```bash
# Draft mode: Apply without publishVersion annotation
$ kubectl apply -f clusterplane-networking.yaml
clusterplane.core.oam.dev/networking created

# Make changes, iterate...
$ kubectl apply -f clusterplane-networking.yaml
clusterplane.core.oam.dev/networking configured

# Ready to publish: Add annotation and apply
$ kubectl apply -f clusterplane-networking.yaml  # now has publishVersion: "2.4.0"
clusterplane.core.oam.dev/networking configured
clusterplanerevision.core.oam.dev/networking-v2.4.0 created

# Verify the revision was created
$ kubectl get clusterplanerevision -l core.oam.dev/plane-name=networking
NAME                 VERSION   AGE
networking-v2.4.0    2.4.0     5s
networking-v2.3.1    2.3.1     2d

# Try to republish same version with different content = ERROR
$ kubectl apply -f clusterplane-networking-modified.yaml  # has publishVersion: "2.4.0"
Error from server: admission webhook "version.plane.oam.dev" denied the request:
version "2.4.0" already published with different content. Use a new version (e.g., 2.4.1).
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
        revision: networking-v2.3.1 # Explicit revision

    # Option 2: Pin to version (resolves to revision)
    - name: security
      ref:
        name: security
        version: "1.8.0" # Resolves to security-v1.8.0

    # Option 3: Use latest (for dev/staging, auto-updates)
    - name: observability
      ref:
        name: observability
        # No revision or version = latest

    # Option 4: Version constraint (auto-upgrade within range)
    - name: storage
      ref:
        name: storage
        versionConstraint: ">=1.0.0 <2.0.0" # Any 1.x version
```

**CLI Commands for Versioning:**

```bash
# Publish a new version (sets publishVersion annotation)
# This is equivalent to kubectl apply with plane.oam.dev/publishVersion annotation
$ vela plane publish networking --version 2.4.0 --changelog "Added Gateway API support"

Publishing networking v2.4.0...
  → Setting annotation: plane.oam.dev/publishVersion: "2.4.0"
  → Creating ClusterPlaneRevision/networking-v2.4.0

✓ Published networking-v2.4.0

# List all revisions of a plane
$ vela plane revisions networking

REVISION              VERSION   CREATED                 BY                  ACTIVE
networking-v2.4.0     2.4.0     2024-12-25 09:00:00    jane@company.com    ✓
networking-v2.3.1     2.3.1     2024-12-24 10:00:00    jane@company.com
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

# Rollback to previous version (creates new revision by setting new publishVersion)
$ vela plane rollback networking --to-revision networking-v2.3.0

Rolling back networking to v2.3.0...
  → Resetting spec to v2.3.0 configuration
  → Setting annotation: plane.oam.dev/publishVersion: "2.3.2"
  → Creating new revision networking-v2.3.2 (based on v2.3.0)

Proceed? [y/N]: y
✓ Rollback complete. New revision: networking-v2.3.2

# Promote a plane version to blueprints (updates blueprint's plane reference)
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

##### ClusterPlaneRevision CRD

As cluster fleets scale, storing revision history directly in `status.revisions` encounters Kubernetes etcd size limits (~1MB per object). To address this, we introduce `ClusterPlaneRevision` as a separate CRD—following the same pattern as `ApplicationRevision`.

**Why a Separate CRD?**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    CLUSTERPLANEREVISION RATIONALE                           │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  PROBLEM: Status.revisions grows unbounded                                  │
│  ─────────────────────────────────────────────                              │
│  • Each revision stores: spec snapshot, component versions, outputs         │
│  • 10 revisions × 100KB each = 1MB (etcd limit)                             │
│  • Fleet of 100+ clusters amplifies this issue                              │
│                                                                             │
│  SOLUTION: Separate ClusterPlaneRevision CRDs                               │
│  ─────────────────────────────────────────────                              │
│  • ClusterPlane status stores only: currentRevision, revisionCount          │
│  • Full history stored in ClusterPlaneRevision objects                      │
│  • Enables compression (like ApplicationRevision)                           │
│  • Garbage collection via revisionHistoryLimit                              │
│                                                                             │
│  RELATIONSHIP:                                                              │
│                                                                             │
│  ClusterPlane (networking)                                                  │
│    │                                                                        │
│    ├── ClusterPlaneRevision (networking-v2.3.1) ◄── currentRevision         │
│    ├── ClusterPlaneRevision (networking-v2.3.0)                             │
│    ├── ClusterPlaneRevision (networking-v2.2.0)                             │
│    └── ... (up to revisionHistoryLimit)                                     │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

**ClusterPlaneRevision Spec:**

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterPlaneRevision
metadata:
  name: networking-v2.3.1
  namespace: vela-system
  labels:
    core.oam.dev/plane-name: networking
    core.oam.dev/plane-version: "2.3.1"
  ownerReferences:
    - apiVersion: core.oam.dev/v1beta1
      kind: ClusterPlane
      name: networking
      uid: abc-123-def
spec:
  # Immutable snapshot of the ClusterPlane spec at this version
  planeSnapshot:
    version: "2.3.1"
    owner:
      team: platform-networking
      contacts: ["netops@company.com"]

    components:
      - name: ingress-nginx
        type: helm-release
        properties:
          chart: ingress-nginx
          repo: https://kubernetes.github.io/ingress-nginx
          version: "4.8.3"
          values:
            controller:
              replicaCount: 2

      - name: cilium
        type: helm-release
        properties:
          chart: cilium
          repo: https://helm.cilium.io
          version: "1.14.4"

    outputs:
      - name: ingressClass
        valueFrom:
          component: ingress-nginx
          fieldPath: status.ingressClassName

  # Metadata about this revision
  revisionMeta:
    created: "2024-12-24T10:00:00Z"
    createdBy: "jane@company.com"
    changelog: "Updated ingress-nginx to 4.8.3 (security patch CVE-2024-1234)"
    digest: "sha256:abc123def456..." # Hash of spec for integrity verification
    parentRevision: "networking-v2.3.0" # Previous revision (for diff)

  # Compression settings (optional, for large specs)
  compression:
    type: gzip # or zstd, none
    # When enabled, planeSnapshot is compressed in storage

status:
  # Whether this revision was successfully applied
  succeeded: true

  # Which clusters are currently using this revision
  activeInClusters:
    - name: production-us-east-1
      syncedAt: "2024-12-24T10:05:00Z"
    - name: production-us-west-2
      syncedAt: "2024-12-24T10:06:00Z"

  # Outputs produced by this revision (cached for cross-plane references)
  outputs:
    ingressClass: nginx
    clusterDNS: "cluster.example.com"

  # ResourceTracker reference for garbage collection
  resourceTrackerRef:
    name: clusterplane-networking-v2.3.1-root
    uid: xyz-789-abc
```

**Updated ClusterPlane Status (Lightweight):**

With `ClusterPlaneRevision` CRDs, the ClusterPlane status becomes lightweight:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterPlane
metadata:
  name: networking
spec:
  # ... (unchanged)
status:
  phase: Running

  # Reference to current active revision (not embedded)
  currentRevision:
    name: networking-v2.3.1
    version: "2.3.1"
    digest: "sha256:abc123..."

  # Total revision count (for monitoring/alerting)
  revisionCount: 15

  # How many revisions to keep (GC deletes oldest beyond this)
  revisionHistoryLimit: 10

  # Quick health summary (details in ClusterPlaneRevision)
  healthy: true
  message: "All components running"

  # Outputs (cached from current revision for fast access)
  outputs:
    ingressClass: nginx
    clusterDNS: "cluster.example.com"

  observedGeneration: 3
  lastUpdated: "2024-12-24T10:00:00Z"
```

**Revision Lifecycle:**

1. **Create**: On publishVersion annotation, controller creates immutable ClusterPlaneRevision with spec snapshot, content digest, and OwnerReference
2. **Deploy**: For each target cluster, create ResourceTracker, deploy components, update `activeInClusters`
3. **GC**: When `revisionCount > revisionHistoryLimit`, delete oldest revisions where `activeInClusters` is empty and not referenced by blueprints

**CLI Commands:**

```bash
vela plane revisions <name>                      # List revisions
vela plane revision <rev> --show-spec            # Show details
vela plane diff <name> --from v1 --to v2         # Compare revisions
vela plane gc <name> --keep 5                    # Force GC

Garbage collecting old revisions...
  → Keeping: networking-v2.3.1, networking-v2.3.0, networking-v2.2.0,
             networking-v2.1.0, networking-v2.0.0
  → Deleting: networking-v1.9.0, networking-v1.8.0
  → Cleaning up ResourceTrackers

✓ Deleted 2 old revisions
```

##### Cross-Cluster Dependency Handling

In large-scale platform deployments, infrastructure components often need to reference outputs from other clusters. For example:

- **Spoke clusters** need the Vault endpoint from a **management cluster**
- **Edge clusters** need Prometheus remote-write endpoints from a **central observability hub**
- **Regional clusters** need registry mirrors from a **central artifact cluster**

The `crossClusterInputs` field enables declarative cross-cluster dependencies:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    CROSS-CLUSTER DEPENDENCY MODEL                           │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  PROBLEM: Components need config from other clusters                        │
│  ─────────────────────────────────────────────────────                      │
│                                                                             │
│    Management Cluster          Spoke Cluster (production-us-east-1)         │
│    ┌──────────────────┐         ┌─────────────────────────────────────┐     │
│    │ security plane   │         │ security plane                      │     │
│    │   └─ vault       │ ◄────── │   └─ vault-agent                    │     │
│    │      ↓           │  needs  │       needs: vaultEndpoint          │     │
│    │   outputs:       │         │                                     │     │
│    │     vaultEndpoint│         │ How does spoke get this value?      │     │
│    └──────────────────┘         └─────────────────────────────────────┘     │
│                                                                             │
│  SOLUTION: crossClusterInputs with automatic resolution                     │
│  ─────────────────────────────────────────────────────                      │
│                                                                             │
│    Spoke Cluster (production-us-east-1):                                    │
│    ┌────────────────────────────────────────────────────────────────────┐   │
│    │ apiVersion: core.oam.dev/v1beta1                                   │   │
│    │ kind: ClusterPlane                                                 │   │
│    │ metadata:                                                          │   │
│    │   name: security                                                   │   │
│    │ spec:                                                              │   │
│    │   crossClusterInputs:                                              │   │
│    │     - name: vaultEndpoint                                          │   │
│    │       fromCluster: management-cluster     # Source cluster         │   │
│    │       fromPlane: security                 # Source plane           │   │
│    │       output: vaultEndpoint               # Output name            │   │
│    │       required: true                      # Fail if unavailable    │   │
│    │       cacheTTL: 5m                        # Cache for resilience   │   │
│    │                                                                    │   │
│    │   components:                                                      │   │
│    │     - name: vault-agent                                            │   │
│    │       properties:                                                  │   │
│    │         # Reference the cross-cluster input                        │   │
│    │         vaultAddr: "{{ inputs.vaultEndpoint }}"                    │   │
│    └────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

**CrossClusterInput Spec:**

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterPlane
metadata:
  name: observability
  namespace: vela-system
  annotations:
    plane.oam.dev/publishVersion: "1.5.0"
spec:
  # Cross-cluster dependencies
  crossClusterInputs:
    # Get Prometheus endpoint from central observability hub
    - name: remoteWriteEndpoint
      fromCluster: observability-hub # Source cluster name
      fromPlane: observability # ClusterPlane in source cluster
      output: prometheusRemoteWrite # Output name from source plane
      required: true # Block deployment if unavailable
      cacheTTL: 5m # Cache value to survive transient failures
      fallback: "" # Optional fallback if not required

    # Get container registry from artifact cluster
    - name: registryMirror
      fromCluster: artifact-cluster
      fromPlane: registry
      output: mirrorEndpoint
      required: false
      fallback: "docker.io" # Use public registry if mirror unavailable

    # Get secrets encryption key from management cluster
    - name: sealingKey
      fromCluster: management-cluster
      fromPlane: security
      output: clusterSealingKey
      required: true
      # Secrets are automatically handled securely

  components:
    - name: prometheus-agent
      type: helm-release
      properties:
        values:
          remoteWrite:
            - url: "{{ inputs.remoteWriteEndpoint }}"

    - name: containerd-config
      type: k8s-objects
      properties:
        objects:
          - apiVersion: v1
            kind: ConfigMap
            metadata:
              name: containerd-hosts
            data:
              hosts.toml: |
                [host."{{ inputs.registryMirror }}"]
                  capabilities = ["pull", "resolve"]
```

**Resolution Flow:**

1. **Discover**: List all `crossClusterInputs` from spec
2. **Resolve**: For each input, use cluster connectivity (managed by ClusterController) to access source cluster, read `status.outputs[output]`, cache with TTL
3. **Validate**: Required inputs must resolve (→ phase=Blocked if not), optional use fallback
4. **Inject**: Template substitution `{{ inputs.{name} }}`
5. **Watch**: Re-reconcile when source outputs change

**CLI Commands:**

```bash
vela plane deps <name>                    # Show dependencies
vela plane deps --all --graph             # Fleet-wide dependency graph
vela plane validate <name> --check-deps   # Validate before deploy
vela plane deps refresh <name>            # Force refresh cache
```

**Resilience:** Uses caching with TTL, fallback values for optional deps, circuit breaker (opens after 5 failures, half-open after 30s).

##### Shared Infrastructure Planes

In enterprise deployments, infrastructure resources like VPCs, NAT Gateways, and subnets are often **shared across multiple Kubernetes clusters**. Rather than complex component-level sharing semantics, ClusterPlane uses a simple **plane-level scope** model.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      PLANE SCOPE MODEL                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Management Cluster (runs KubeVela)                                         │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                                                                       │  │
│  │  ┌─────────────────────────┐    ┌─────────────────────────┐           │  │
│  │  │ ClusterPlane            │    │ ClusterPlane            │           │  │
│  │  │ name: shared-vpc        │    │ name: eks-cluster       │           │  │
│  │  │ scope: shared           │    │ scope: perCluster       │           │  │
│  │  │                         │    │                         │           │  │
│  │  │ Creates ONE VPC in AWS  │    │ Creates EKS per cluster │           │  │
│  │  │ used by all clusters    │    │ that uses the blueprint │           │  │
│  │  └───────────┬─────────────┘    └───────────┬─────────────┘           │  │
│  │              │                              │                         │  │
│  │              │ outputs.vpcId                │ inputs.vpcId            │  │
│  │              └──────────────────────────────┘                         │  │
│  │                                                                       │  │
│  │  ┌─────────────────────────────────────────────────────────────────┐  │  │
│  │  │ ClusterBlueprint: production-standard                           │  │  │
│  │  │   planes:                                                       │  │  │
│  │  │     - ref: shared-vpc        ← Created once                     │  │  │
│  │  │     - ref: eks-cluster       ← Created per cluster              │  │  │
│  │  │     - ref: networking        ← Deployed to each cluster         │  │  │
│  │  └─────────────────────────────────────────────────────────────────┘  │  │
│  │                                                                       │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
│  KEY INSIGHT: All ClusterPlane CRDs live on the management cluster.         │
│  The 'scope' field determines whether resources are created once (shared)   │
│  or per-cluster (perCluster).                                               │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Scope Types:**

| Scope        | Behavior                                                           | Use Case                                            |
| ------------ | ------------------------------------------------------------------ | --------------------------------------------------- |
| `perCluster` | Resources created for each cluster using the blueprint (default)   | EKS clusters, node groups, cluster-specific IAM     |
| `shared`     | Resources created once, outputs available to all clusters in scope | VPCs, NAT Gateways, shared subnets, Transit Gateway |

##### Shared Plane Ownership Model

**The Critical Question:** Who "owns" a shared plane's resources? Since ClusterPlanes are only reconciled when referenced by a Cluster, we need clear ownership semantics.

**Ownership Pattern: Infrastructure Preparer Cluster**

The recommended pattern uses a dedicated **infrastructure mode Cluster** to own shared planes:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    INFRASTRUCTURE PREPARER PATTERN                           │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Hub/Management Cluster                                                     │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                                                                       │  │
│  │  Cluster: us-east-1-infrastructure (mode: infrastructure)             │  │
│  │  ┌─────────────────────────────────────────────────────────────────┐  │  │
│  │  │ Blueprint: shared-infrastructure-us-east                        │  │  │
│  │  │   └─ ClusterPlane: shared-vpc (scope: shared)                   │  │  │
│  │  │   └─ ClusterPlane: shared-transit-gw (scope: shared)            │  │  │
│  │  │   └─ ClusterPlane: shared-dns (scope: shared)                   │  │  │
│  │  └─────────────────────────────────────────────────────────────────┘  │  │
│  │                          │ outputs consumed by                        │  │
│  │                          ▼                                            │  │
│  │  ┌─────────────────────────────────────────────────────────────────┐  │  │
│  │  │ Cluster: prod-us-east-1-a (mode: provision)                     │  │  │
│  │  │   └─ Blueprint: production-eks                                  │  │  │
│  │  │        └─ ClusterPlane: shared-vpc (consumes outputs)           │  │  │
│  │  │        └─ ClusterPlane: eks-cluster (scope: perCluster)         │  │  │
│  │  └─────────────────────────────────────────────────────────────────┘  │  │
│  │                                                                       │  │
│  │  ┌─────────────────────────────────────────────────────────────────┐  │  │
│  │  │ Cluster: prod-us-east-1-b (mode: provision)                     │  │  │
│  │  │   └─ Blueprint: production-eks                                  │  │  │
│  │  │        └─ ClusterPlane: shared-vpc (consumes outputs)           │  │  │
│  │  │        └─ ClusterPlane: eks-cluster (scope: perCluster)         │  │  │
│  │  └─────────────────────────────────────────────────────────────────┘  │  │
│  │                                                                       │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
│  KEY INSIGHT: The infrastructure preparer cluster OWNS the shared planes.   │
│  Workload clusters CONSUME outputs but don't trigger shared plane creation. │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Infrastructure Preparer Cluster:**

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Cluster
metadata:
  name: us-east-1-infrastructure
  namespace: vela-system
  labels:
    cluster.oam.dev/role: infrastructure-preparer
    region: us-east-1
spec:
  # Infrastructure mode - not a real K8s cluster, just owns shared planes
  mode: infrastructure

  # No credentials needed - this is a virtual/logical cluster
  # Resources are created in cloud (AWS/GCP) or hub cluster namespace

  # Where shared plane resources are created
  infrastructureTarget:
    type: cloud      # cloud | hub
    providerRef:
      name: aws-production
    # OR for hub-local shared resources:
    # type: hub
    # namespace: shared-infrastructure

  # Blueprint containing shared planes
  blueprintRef:
    name: shared-infrastructure-us-east
```

**Why This Pattern?**

| Benefit | Explanation |
|---------|-------------|
| **Clear Ownership** | Platform team owns the preparer, shared planes have explicit owner |
| **Natural Deletion Protection** | Preparer blocked from deletion while consumers exist |
| **Clean RBAC** | Platform team manages preparer, app teams manage workload clusters |
| **Independent Lifecycle** | Shared infra lifecycle separate from workload cluster lifecycle |
| **Label-Based Access** | `sharedWith.clusterSelector` controls which clusters can consume |

**Lifecycle Semantics:**

```
1. Platform team creates us-east-1-infrastructure (mode: infrastructure)
   → ClusterController reconciles shared planes (VPC, DNS, etc.)
   → Shared plane status.phase = Running

2. App team creates prod-us-east-1-a with production-eks blueprint
   → Blueprint references shared-vpc
   → ClusterController sees shared-vpc already reconciled
   → Consumes outputs (vpcId, subnetIds) without re-creating
   → Updates shared plane status.consumers

3. App team deletes prod-us-east-1-a
   → ClusterController removes from shared plane consumers
   → Shared plane resources REMAIN (owned by preparer)

4. Platform team tries to delete us-east-1-infrastructure
   → BLOCKED: "2 workload clusters consuming shared planes"
   → Must remove consumers first (or force delete with warning)
```

**Shared Plane Definition:**

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterPlane
metadata:
  name: shared-vpc-us-east-1
  namespace: vela-system
spec:
  scope: shared # Created once, not per-cluster

  # Which clusters can consume this plane's outputs
  sharedWith:
    clusterSelector:
      matchLabels:
        region: us-east-1
        environment: production

  components:
    - name: vpc
      type: terraform-module
      properties:
        source: "terraform-aws-modules/vpc/aws"
        version: "5.1.0"
        values:
          name: "production-us-east-1-vpc"
          cidr: "10.0.0.0/16"
          azs: ["us-east-1a", "us-east-1b", "us-east-1c"]
          private_subnets: ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]
          public_subnets: ["10.0.101.0/24", "10.0.102.0/24", "10.0.103.0/24"]
          enable_nat_gateway: true
          tags:
            "shared-infrastructure": "true"
            "managed-by": "kubevela-clusterplane"

  outputs:
    - name: vpcId
      valueFrom:
        component: vpc
        fieldPath: outputs.vpc_id
    - name: privateSubnets
      valueFrom:
        component: vpc
        fieldPath: outputs.private_subnets
    - name: publicSubnets
      valueFrom:
        component: vpc
        fieldPath: outputs.public_subnets
```

**Per-Cluster Plane Consuming Shared Outputs:**

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterPlane
metadata:
  name: eks-cluster
  namespace: vela-system
spec:
  scope: perCluster # Default - created for each cluster

  # Import outputs from shared plane
  inputs:
    - name: vpcId
      fromPlane: shared-vpc-us-east-1
      output: vpcId
    - name: privateSubnets
      fromPlane: shared-vpc-us-east-1
      output: privateSubnets

  components:
    - name: eks
      type: terraform-module
      properties:
        source: "terraform-aws-modules/eks/aws"
        version: "19.21.0"
        values:
          cluster_name: "${context.cluster.name}"
          cluster_version: "1.29"
          vpc_id: "{{ inputs.vpcId }}"
          subnet_ids: "{{ inputs.privateSubnets }}"
          enable_irsa: true

    - name: node-group
      type: terraform-module
      properties:
        source: "terraform-aws-modules/eks/aws//modules/eks-managed-node-group"
        values:
          cluster_name: "{{ outputs.eks.cluster_name }}"
          subnet_ids: "{{ inputs.privateSubnets }}"
          instance_types: ["m5.large"]
          min_size: 3
          max_size: 10

  outputs:
    - name: clusterEndpoint
      valueFrom:
        component: eks
        fieldPath: outputs.cluster_endpoint
    - name: clusterName
      valueFrom:
        component: eks
        fieldPath: outputs.cluster_name
```

**Blueprint Composition:**

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterBlueprint
metadata:
  name: production-standard
spec:
  planes:
    # Shared plane - created once for all clusters
    - name: shared-vpc
      ref:
        name: shared-vpc-us-east-1

    # Per-cluster planes - created for each cluster
    - name: eks
      ref:
        name: eks-cluster
      dependsOn: [shared-vpc]

    - name: networking
      ref:
        name: networking
      dependsOn: [eks]
```

The blueprint doesn't need special sharing configuration—the plane's `scope` field determines the behavior.

**Deletion Protection and Lifecycle Semantics:**

##### Deletion Semantics Matrix

| Resource | Deletion Behavior | Blocked When |
|----------|-------------------|--------------|
| **ClusterPlane (scope=shared)** | BLOCKED if consumers > 0 | Any Cluster/Blueprint consuming outputs |
| **ClusterPlane (scope=perCluster)** | Allowed | Never blocked (per-cluster instances cleaned up) |
| **ClusterBlueprint** | BLOCKED if referenced | Any Cluster has `blueprintRef` pointing to it |
| **ClusterBlueprintRevision** | BLOCKED if active | Any Cluster using this specific revision |
| **Cluster (mode=infrastructure)** | BLOCKED if consumers | Workload clusters consuming its shared planes |
| **Cluster (mode=provision/adopt/connect)** | Allowed with cleanup | Never blocked (triggers resource cleanup) |

##### ClusterPlane Deletion

Shared planes cannot be deleted while clusters are using them:

```bash
$ kubectl delete clusterplane shared-vpc-us-east-1

Error from server: admission webhook "clusterplane.validation.oam.dev" denied the request:
  Cannot delete shared ClusterPlane "shared-vpc-us-east-1"

  The following clusters are using this plane:
    - production-us-east-1-a (via blueprint: production-standard)
    - production-us-east-1-b (via blueprint: production-standard)
    - production-us-east-1-c (via blueprint: production-standard)

  To delete, first remove these clusters or update their blueprints.
  Use --force to delete anyway (DANGER: will orphan dependent infrastructure)
```

##### Cluster Deletion Cascade

When a Cluster is deleted, the following cleanup occurs:

```
Cluster deletion triggered
           │
           ▼
┌─────────────────────────────────────────────────────────────┐
│ 1. Remove from shared plane consumers                       │
│    → Decrements status.consumers.count on each shared plane │
│    → Updates status.consumers.clusters list                 │
│                                                             │
│ 2. Clean up perCluster plane instances                      │
│    → Deletes resources created for this cluster             │
│    → Removes ResourceTrackers                               │
│                                                             │
│ 3. For mode=provision: Destroy cloud infrastructure         │
│    → Triggers Terraform/Crossplane cleanup                  │
│    → VPC, EKS, nodes are deleted                            │
│                                                             │
│ 4. Remove connectivity credentials (if controller-created)  │
└─────────────────────────────────────────────────────────────┘
```

##### Force Deletion

Force deletion bypasses protection but requires explicit acknowledgment:

```bash
# Force delete with explicit confirmation
$ kubectl delete clusterplane shared-vpc-us-east-1 \
    --cascade=orphan \
    --force

⚠️  WARNING: Force deletion will orphan dependent infrastructure!
    3 clusters depend on this shared plane.
    Their resources will NOT be automatically cleaned up.

Type 'DELETE shared-vpc-us-east-1' to confirm:
```

**Status Tracking:**

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterPlane
metadata:
  name: shared-vpc-us-east-1
status:
  phase: Running
  scope: shared

  # Clusters currently using this shared plane
  consumers:
    count: 3
    clusters:
      - name: production-us-east-1-a
        blueprint: production-standard
        since: "2025-01-03T10:00:00Z"
      - name: production-us-east-1-b
        blueprint: production-standard
        since: "2025-01-03T10:30:00Z"
      - name: production-us-east-1-c
        blueprint: production-standard
        since: "2025-01-03T11:00:00Z"

  components:
    - name: vpc
      healthy: true
      message: "VPC created successfully"

  outputs:
    vpcId: "vpc-0abc123def456"
    privateSubnets: '["subnet-1a","subnet-1b","subnet-1c"]'
    publicSubnets: '["subnet-2a","subnet-2b","subnet-2c"]'
```

**CLI Commands:**

```bash
# List shared planes and their consumers
vela plane list --scope shared

NAME                    SCOPE    CONSUMERS  STATUS
shared-vpc-us-east-1    shared   3          Running
shared-transit-gw       shared   5          Running

# Show details of a shared plane
vela plane status shared-vpc-us-east-1

SHARED PLANE: shared-vpc-us-east-1
STATUS: Running
SCOPE: shared (3 consumers)

CONSUMERS:
  Cluster                      Blueprint            Since
  ─────────────────────────────────────────────────────────
  production-us-east-1-a       production-standard  2025-01-03
  production-us-east-1-b       production-standard  2025-01-03
  production-us-east-1-c       production-standard  2025-01-03

OUTPUTS:
  vpcId: vpc-0abc123def456
  privateSubnets: ["subnet-1a","subnet-1b","subnet-1c"]

# Check what would happen if a shared plane is deleted
vela plane delete shared-vpc-us-east-1 --dry-run

⚠️  BLOCKED: 3 clusters depend on this shared plane
    Cannot delete without --force flag
```

**Why This Model is Clean:**

| Aspect                    | Benefit                                                                 |
| ------------------------- | ----------------------------------------------------------------------- |
| **Simple mental model**   | Shared infra = shared plane, per-cluster infra = per-cluster plane      |
| **No ownership transfer** | Shared planes live on management cluster, not tied to workload clusters |
| **Clear boundaries**      | Forces good design - separate shared vs per-cluster concerns            |
| **Easy implementation**   | Just `scope` field + validation webhook for deletion                    |
| **Familiar pattern**      | Similar to Terraform workspaces or Helm release scopes                  |

##### ClusterPlane Workflow and Deployment Order

ClusterPlane follows the **same workflow model as Application** for consistency. This ensures platform engineers familiar with KubeVela's Application CRD can immediately understand ClusterPlane behavior.

**Default Behavior (No Workflow Specified):**

When `spec.workflow` is not defined, the controller auto-generates a workflow:

1. Creates one `deploy` step per component
2. Uses each component's `dependsOn` field to establish ordering
3. Components without `dependsOn` deploy **in parallel**
4. Components with `dependsOn` wait for their dependencies

**Example:** Given components A (no deps), B → A, C → B, D → B, the workflow executes: A → B → (C, D in parallel).

**Explicit Workflow (Optional):**

For advanced use cases, define an explicit `workflow` to:

- Run validation scripts between deployments
- Add approval gates for production changes
- Execute conditional logic based on cluster properties
- Send notifications on success/failure
- Implement custom rollback strategies

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterPlane
metadata:
  name: networking
  annotations:
    plane.oam.dev/publishVersion: "2.4.0"
spec:
  components:
    - name: gateway-api-crds
      type: helm-release
      properties: { ... }
    - name: cilium
      type: helm-release
      properties: { ... }
    - name: ingress-nginx
      type: helm-release
      properties: { ... }

  # Explicit workflow overrides default behavior
  workflow:
    steps:
      # Step 1: Deploy CRDs
      - name: deploy-crds
        type: deploy
        properties:
          components: [gateway-api-crds]

      # Step 2: Wait for CRDs to be established
      - name: wait-crds
        type: wait
        dependsOn: [deploy-crds]
        properties:
          resources:
            - apiVersion: apiextensions.k8s.io/v1
              kind: CustomResourceDefinition
              name: gateways.gateway.networking.k8s.io
          condition:
            type: Established
            status: "True"
          timeout: 2m

      # Step 3: Deploy CNI
      - name: deploy-cni
        type: deploy
        dependsOn: [wait-crds]
        properties:
          components: [cilium]

      # Step 4: Validate CNI connectivity
      - name: validate-cni
        type: script
        dependsOn: [deploy-cni]
        properties:
          image: cilium/cilium-cli:latest
          command: ["cilium", "status", "--wait"]
          timeout: 5m

      # Step 5: Approval gate (for production)
      - name: approval-gate
        type: suspend
        dependsOn: [validate-cni]
        if: "context.cluster.labels.environment == 'production'"
        properties:
          message: "CNI validated. Approve to continue with ingress deployment."
          timeout: 24h

      # Step 6: Deploy ingress
      - name: deploy-ingress
        type: deploy
        dependsOn: [approval-gate]
        properties:
          components: [ingress-nginx]

      # Step 7: Smoke test
      - name: smoke-test
        type: http
        dependsOn: [deploy-ingress]
        properties:
          url: "http://ingress-nginx-controller.ingress-nginx.svc/healthz"
          expectedStatus: 200
          retries: 5
          retryInterval: 10s

    # Failure handling
    onFailure:
      - name: notify-failure
        type: notification
        properties:
          slack:
            channel: "#platform-alerts"
            message: "Networking plane deployment failed at step: {{workflow.failedStep}}"
```

**Available Workflow Step Types:**

| Step Type      | Purpose                       | Example Use Case                       |
| -------------- | ----------------------------- | -------------------------------------- |
| `deploy`       | Deploy one or more components | Deploy CRDs before controllers         |
| `wait`         | Wait for resource condition   | CRD established, Deployment ready      |
| `health-check` | Verify component health       | Ensure CNI is fully operational        |
| `script`       | Run container with command    | Connectivity tests, validation scripts |
| `http`         | HTTP request check            | Smoke test endpoints                   |
| `webhook`      | Call external service         | Trigger CI/CD, external validation     |
| `suspend`      | Pause for manual approval     | Production deployment gates            |
| `notification` | Send alert/message            | Slack, email, PagerDuty                |

**Workflow Inputs and Outputs:**

Components and workflow steps can pass data between each other:

```yaml
spec:
  components:
    - name: cert-manager
      type: helm-release
      outputs:
        - name: issuerReady
          valueFrom:
            fieldPath: status.conditions[?(@.type=="Ready")].status

    - name: ingress-nginx
      type: helm-release
      dependsOn: [cert-manager]
      inputs:
        - from: cert-manager
          parameterKey: values.controller.extraArgs.default-ssl-certificate
          # Use output from cert-manager
```

#### 3. ClusterBlueprint

A `ClusterBlueprint` composes multiple `ClusterPlanes` into a complete cluster specification. **ClusterBlueprints are immutable templates**—once a version is created, it never changes. New versions create new `ClusterBlueprintRevision` objects.

**Key Design Points:**

| Principle                         | Description                                                                                                                                              |
| --------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Template, not live state**      | ClusterBlueprint defines _what_ a cluster should look like. It is a template, not a live configuration.                                                  |
| **Immutable versioning**          | Once a blueprint version is created, it never changes. Modifications create new versions.                                                                |
| **Pull model**                    | Individual `Cluster` resources declare which blueprint they follow via `spec.blueprintRef`. Clusters pull blueprints; blueprints don't push to clusters. |
| **Never modified by controllers** | No controller (including `ClusterRolloutController`) ever modifies a `ClusterBlueprint`. Only users or GitOps automation create/update blueprints.       |

**Important**: The `spec.blueprintRef` in a `Cluster` is the **desired state** owned by users/GitOps. The `status.blueprint` is the **actual state** owned by `ClusterController`. This separation prevents circular references—see [Controller Ownership Model](#controller-ownership-model-circular-reference-prevention).

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterBlueprint
metadata:
  name: production-standard
  namespace: vela-system
  labels:
    tier: production
  annotations:
    # Publishing follows Application's publishVersion pattern
    # No annotation = draft (mutable), with annotation = creates immutable ClusterBlueprintRevision
    blueprint.oam.dev/publishVersion: "2.3.0"
spec:
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
  # IMPORTANT: Cloud infrastructure (VPC, EKS, nodes) is a plane, not in Cluster CRD
  planes:
    # Cloud foundation plane - provisions VPC, EKS cluster, node pools
    # This is what creates the actual Kubernetes cluster
    - name: aws-foundation
      ref:
        name: aws-foundation
        revision: aws-foundation-v1.2.0
      # Cloud-specific overrides for this blueprint
      patches:
        - component: node-pool-workload
          properties:
            values:
              instance_types: ["m5.2xlarge", "m5.4xlarge"] # Larger for production
              min_size: 5
              max_size: 50

    # Networking plane - CNI, ingress, DNS (depends on cluster existing)
    - name: networking
      ref:
        name: networking
        revision: networking-v2.3.1
      dependsOn: [aws-foundation] # Wait for cluster to exist
      patches:
        - component: ingress-nginx
          properties:
            values:
              controller:
                replicaCount: 3 # Override for production

    # Security plane - cert-manager, policies
    - name: security
      ref:
        name: security
        revision: security-v1.8.0
      dependsOn: [aws-foundation]

    # Observability plane
    - name: observability
      ref:
        name: observability
      dependsOn: [networking, security]

    # Storage plane - conditional for AWS clusters
    - name: storage
      ref:
        name: storage
      dependsOn: [aws-foundation]
      when: "context.cluster.labels.provider == 'aws'"

  # Blueprint-level policies
  policies:
    - name: resource-governance
      type: resource-limits
      properties:
        maxTotalCPU: "100"
        maxTotalMemory: "200Gi"

  # Blueprint-level workflow (orchestrates plane deployment)
  # Note: If not specified, workflow is auto-generated from plane dependsOn
  workflow:
    steps:
      # Step 1: Provision cloud infrastructure (VPC, EKS, nodes)
      - name: provision-cloud
        type: apply-plane
        properties:
          plane: aws-foundation
          # For provisioning mode, this creates the actual K8s cluster
          # Outputs are used to create connectivity credentials automatically

      - name: wait-for-cluster
        type: suspend
        properties:
          duration: "5m"
          message: "Waiting for Kubernetes cluster to be ready"
        dependsOn: [provision-cloud]

      # Step 2: Deploy core infrastructure planes (networking + security in parallel)
      - name: deploy-networking
        type: apply-plane
        properties:
          plane: networking
        dependsOn: [wait-for-cluster]

      - name: deploy-security
        type: apply-plane
        properties:
          plane: security
        dependsOn: [wait-for-cluster]

      - name: wait-for-core
        type: suspend
        properties:
          duration: "60s"
          message: "Waiting for core infrastructure to stabilize"
        dependsOn: [deploy-networking, deploy-security]

      # Step 3: Deploy observability
      - name: deploy-observability
        type: apply-plane
        properties:
          plane: observability
        dependsOn: [wait-for-core]

      # Step 4: Validation
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
        dependsOn: [deploy-observability]

status:
  phase: Running

  # Current active revision
  currentRevision: production-standard-v2.3.0
  currentVersion: "2.3.0"

  # Resolved plane revisions for this blueprint version
  planes:
    - name: aws-foundation
      revision: aws-foundation-v1.2.0
      version: "1.2.0"
      status: Running
      outputs:
        clusterEndpoint: "https://ABC123.gr7.us-east-1.eks.amazonaws.com"
        vpcId: "vpc-0abc123"
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
      planeRevisions: # Snapshot of which plane versions were used
        aws-foundation: aws-foundation-v1.2.0
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
        aws-foundation: aws-foundation-v1.1.0
        networking: networking-v2.2.0
        security: security-v1.8.0
        observability: observability-v3.1.0

  revisionHistoryLimit: 10

  # List of clusters using this blueprint (computed from Cluster CRs)
  clusters:
    total: 5
    byRevision:
      production-standard-v2.3.0: 3 # Already on latest
      production-standard-v2.2.0: 2 # Still updating
    synced: 3
    updating: 2
    failed: 0
  observedGeneration: 5
```

##### ClusterBlueprint Versioning Strategy

ClusterBlueprint versioning follows the same annotation-based pattern as ClusterPlane, using `blueprint.oam.dev/publishVersion` for explicit version publishing. This aligns with KubeVela's Application `app.oam.dev/publishVersion` pattern while using a resource-specific annotation namespace.

**Annotation:** `blueprint.oam.dev/publishVersion: "2.3.0"` → creates immutable `ClusterBlueprintRevision/production-standard-v2.3.0`

**Blueprint version captures:** Composition of plane revisions + patches + policies. Example: production-standard v2.3.0 includes networking-v2.3.1, security-v1.8.0, observability-v3.1.0.

**When to bump:** Change plane references, add/remove planes, modify patches/policies, change workflow. **Not needed:** Unpinned plane updates (tracked in status), metadata changes.

**Publishing Flow:** Same as ClusterPlane - draft mode (no annotation) → publish (add annotation) → new versions (bump annotation).

**Version Collision Handling (same as ClusterPlane):**

- Same version + Same content → SUCCESS (idempotent, GitOps-safe)
- Same version + Different content → REJECTED
- Delete revision referenced by Cluster → REJECTED

**How Teams Publish Blueprint Versions:**

```yaml
# STEP 1: Draft mode - iterate on the blueprint without publishing
apiVersion: core.oam.dev/v1beta1
kind: ClusterBlueprint
metadata:
  name: production-standard
  # No publishVersion annotation = draft mode
spec:
  planes:
    - name: networking
      ref:
        name: networking
        revision: networking-v2.3.1

    - name: security
      ref:
        name: security
        revision: security-v1.8.0
---
# STEP 2: Ready to publish - add the annotation
apiVersion: core.oam.dev/v1beta1
kind: ClusterBlueprint
metadata:
  name: production-standard
  annotations:
    # Publish version using resource-specific annotation
    blueprint.oam.dev/publishVersion: "2.3.0"
spec:
  # Changelog documents what changed (recommended but optional)
  changelog: |
    ## 2.3.0
    - Upgraded networking plane to v2.3.1
    - Added conditional storage plane for AWS clusters

  planes:
    - name: networking
      ref:
        name: networking
        revision: networking-v2.3.1

    - name: security
      ref:
        name: security
        revision: security-v1.8.0

    # New conditional plane
    - name: storage
      ref:
        name: storage
        revision: storage-v1.2.0
      condition: "cluster.labels.cloud == 'aws'"
```

**Publishing with kubectl apply (GitOps-compatible):**

```bash
# Draft mode: Apply without publishVersion annotation
$ kubectl apply -f clusterblueprint-production.yaml
clusterblueprint.core.oam.dev/production-standard created

# Make changes, iterate...
$ kubectl apply -f clusterblueprint-production.yaml
clusterblueprint.core.oam.dev/production-standard configured

# Ready to publish: Add annotation and apply
$ kubectl apply -f clusterblueprint-production.yaml  # now has publishVersion: "2.3.0"
clusterblueprint.core.oam.dev/production-standard configured
clusterblueprintrevision.core.oam.dev/production-standard-v2.3.0 created

# Verify the revision was created
$ kubectl get clusterblueprintrevision -l core.oam.dev/blueprint-name=production-standard
NAME                            VERSION   CLUSTERS   AGE
production-standard-v2.3.0      2.3.0     0          5s
production-standard-v2.2.0      2.2.0     2          2d

# Try to republish same version with different content = ERROR
$ kubectl apply -f clusterblueprint-production-modified.yaml  # has publishVersion: "2.3.0"
Error from server: admission webhook "version.blueprint.oam.dev" denied the request:
version "2.3.0" already published with different content. Use a new version (e.g., 2.3.1).
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

This CLI command will update each clusters spec.blueprintRef (user-owned).
ClusterRolloutStrategy 'production-canary' will gate when each update is applied:
  Wave 1: production-us-west-2 (canary)
  Wave 2: production-us-east-1 (after validation)

Proceed? [y/N]:

# Create new blueprint version from current state (CLI method)
# This is equivalent to kubectl apply with publishVersion annotation
$ vela blueprint publish production-standard --version 2.4.0 \
    --changelog "Upgraded observability to v4.0.0"

Publishing production-standard v2.4.0...
  → Setting annotation: blueprint.oam.dev/publishVersion: "2.4.0"
  → Snapshotting current plane references
  → Recording changelog

✓ Created production-standard-v2.4.0

# Alternative: Use kubectl apply with annotation (GitOps-compatible)
$ cat production-standard.yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterBlueprint
metadata:
  name: production-standard
  annotations:
    blueprint.oam.dev/publishVersion: "2.4.0"
spec:
  planes: [...]

$ kubectl apply -f production-standard.yaml
clusterblueprint.core.oam.dev/production-standard configured
clusterblueprintrevision.core.oam.dev/production-standard-v2.4.0 created
```

##### Version Constraint Resolution

When using `versionConstraint` instead of pinning to a specific revision, the system resolves which version to use using semver constraints (e.g., `>=2.0.0 <3.0.0`). Resolution occurs on blueprint apply, when new plane versions are published, or via manual trigger.

**Resolution Spec:**

```yaml
spec:
  planes:
    - name: networking
      ref:
        name: networking
        versionConstraint: ">=2.0.0 <3.0.0"
        resolution:
          strategy: highest # highest | lowest | latest-created | oldest-created
          fallback: fail # fail | use-current | use-latest
          autoUpgrade:
            enabled: true
            allowedBumps: [patch]
            requireApproval:
              minor: true
              major: true
```

Supported constraint operators: `=`, `>`, `>=`, `<`, `<=`, `~` (patch), `^` (minor), `||` (or), `*` (any).

##### ClusterBlueprintRevision CRD

Just as `ClusterPlane` requires a separate `ClusterPlaneRevision` CRD to handle scaling concerns, `ClusterBlueprint` needs `ClusterBlueprintRevision` to store immutable snapshots of complete infrastructure compositions. This is critical because:

1. **Composition Complexity**: A blueprint references multiple planes, each with their own versions
2. **Audit Trail**: Enterprise environments require complete history of what was deployed to which clusters
3. **Rollback Precision**: Rollbacks must restore the exact combination of plane versions

**Key Insight:** Each ClusterBlueprintRevision captures the EXACT ClusterPlaneRevision names (not just version strings), enabling precise rollbacks to the exact combination of plane states.

**ClusterBlueprintRevision Spec:**

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterBlueprintRevision
metadata:
  name: production-standard-v2.3.0
  namespace: vela-system
  labels:
    core.oam.dev/blueprint-name: production-standard
    core.oam.dev/blueprint-version: "2.3.0"
  ownerReferences:
    - apiVersion: core.oam.dev/v1beta1
      kind: ClusterBlueprint
      name: production-standard
      uid: blueprint-123-abc
spec:
  # Immutable snapshot of the ClusterBlueprint at this version
  blueprintSnapshot:
    version: "2.3.0"

    # Exact plane revisions used in this blueprint version
    # These are ClusterPlaneRevision names, not version strings
    planeRevisions:
      - name: networking
        revision: networking-v2.3.1
        version: "2.3.1"
        digest: "sha256:abc123..."
        pinned: true # Was this explicitly pinned?

      - name: security
        revision: security-v1.8.0
        version: "1.8.0"
        digest: "sha256:def456..."
        pinned: true

      - name: observability
        revision: observability-v3.1.0
        version: "3.1.0"
        digest: "sha256:ghi789..."
        pinned: false # Used latest at time of creation

      - name: storage
        revision: storage-v1.2.0
        version: "1.2.0"
        digest: "sha256:jkl012..."
        conditional: true
        condition:
          matchLabels:
            cloud-provider: aws

    # Blueprint-level patches captured at this version
    patches:
      - plane: networking
        component: ingress-nginx
        patch:
          values:
            controller:
              replicaCount: 3

    # Policies active in this version
    policies:
      - name: topology-spread
        type: topology
        properties:
          clusters: ["production-*"]
          constraints:
            maxSkew: 1

  # Metadata about this revision
  revisionMeta:
    created: "2024-12-24T10:00:00Z"
    createdBy: "sre-team@company.com"
    changelog: |
      - Upgraded networking plane to v2.3.1 (security patch)
      - Added observability plane v3.1.0
      - Increased ingress replicas to 3
    digest: "sha256:blueprint-hash-xyz..."
    parentRevision: "production-standard-v2.2.0"

  # Compression for large blueprints
  compression:
    type: gzip

status:
  # Deployment status
  succeeded: true

  # Clusters using this specific blueprint revision
  activeInClusters:
    - name: production-us-east-1
      syncedAt: "2024-12-24T10:10:00Z"
      planeStatus:
        networking: Synced
        security: Synced
        observability: Synced

    - name: production-us-west-2
      syncedAt: "2024-12-24T10:12:00Z"
      planeStatus:
        networking: Synced
        security: Synced
        observability: Synced

    - name: production-eu-west-1
      syncedAt: "2024-12-24T10:15:00Z"
      planeStatus:
        networking: Synced
        security: Synced
        observability: Updating

  # ResourceTrackers for this blueprint revision (per cluster)
  resourceTrackers:
    - cluster: production-us-east-1
      name: clusterblueprint-production-standard-v2.3.0-root
      managedResources: 47

    - cluster: production-us-west-2
      name: clusterblueprint-production-standard-v2.3.0-root
      managedResources: 47

    - cluster: production-eu-west-1
      name: clusterblueprint-production-standard-v2.3.0-root
      managedResources: 45 # Storage plane not applied
```

**Updated ClusterBlueprint Status (Lightweight):**

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterBlueprint
metadata:
  name: production-standard
spec:
  # ... (unchanged)
status:
  phase: Active

  # Reference to current active revision
  currentRevision:
    name: production-standard-v2.3.0
    version: "2.3.0"
    digest: "sha256:blueprint-hash-xyz..."

  # Quick reference to plane versions in current revision
  currentPlaneVersions:
    networking: "2.3.1"
    security: "1.8.0"
    observability: "3.1.0"

  # Total revision count
  revisionCount: 12

  # Cluster deployment summary
  clusters:
    total: 5
    synced: 4
    updating: 1
    failed: 0

  observedGeneration: 8
  lastUpdated: "2024-12-24T10:00:00Z"
```

**Blueprint Revision Lifecycle:**

1. **Trigger**: Revision created when `blueprint.oam.dev/publishVersion` annotation is set via kubectl apply, `vela blueprint publish`, or GitOps sync
2. **Snapshot**: Controller captures current plane revisions, patches, policies, and computes content digest
3. **Deploy**: Clusters referencing this revision get exact plane versions applied with ResourceTracker updates
4. **GC**: Revisions older than `revisionHistoryLimit` are deleted if not referenced by any cluster

**CLI Commands:**

```bash
vela blueprint revisions <name>                    # List revisions
vela blueprint revision <rev> --show-planes        # Show details
vela blueprint diff <name> --from v1 --to v2       # Compare revisions
vela blueprint rollback-plan <name> --to-revision <rev>  # Preview rollback
```

**Cluster ← Blueprint Relationship:**

Multiple Clusters can reference the same ClusterBlueprint. The Blueprint defines WHAT (planes to deploy), Clusters declare WHICH blueprint to use. The Cluster controller reconciles the actual state.

#### 4. ClusterRollout (Optional - For Emergency/Manual Overrides)

> **Note**: With the introduction of `ClusterRolloutStrategy`, the `ClusterRollout` CRD becomes **optional** and is primarily used for:
>
> - **Emergency rollouts** that bypass normal wave progression
> - **Manual overrides** for specific clusters or cluster groups
> - **One-time operations** that don't follow the standard strategy
>
> For normal operations, clusters reference a `ClusterRolloutStrategy` via `rolloutStrategyRef`. The strategy controller automatically progresses through waves when **users or GitOps automation update `Cluster.spec.blueprintRef`** to point to a new blueprint version. The `ClusterRolloutController` never modifies `spec.blueprintRef` itself—it only gates WHEN the `ClusterController` can apply the user-requested update.

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
    type: canary # canary, blueGreen, rolling
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
          - condition: "< 1" # Must be less than 1%
            failureLimit: 3 # Allow 3 failures before rollback

      - name: p99-latency
        provider: prometheus
        query: |
          histogram_quantile(0.99,
            sum(rate(nginx_ingress_controller_request_duration_seconds_bucket[5m]))
            by (le))
        thresholds:
          - condition: "< 0.5" # p99 < 500ms
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
    strategy: immediate # immediate, gradual

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
      autoApproveAfter: "48h" # Optional: auto-approve if no response

status:
  phase: Progressing # Pending, Progressing, Paused, Succeeded, Failed, RolledBack

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

A `ClusterRolloutStrategy` defines **when and how blueprint updates are rolled out across a fleet** of clusters. Clusters reference this strategy via `rolloutStrategyRef`, enabling coordinated updates where Cluster B only updates after Cluster A succeeds.

**Critical Distinction:**

| Aspect             | Who Controls                                           | Description                                          |
| ------------------ | ------------------------------------------------------ | ---------------------------------------------------- |
| **WHAT** to deploy | User/GitOps → `Cluster.spec.blueprintRef`              | Desired blueprint version                            |
| **WHEN** to deploy | `ClusterRolloutController` → gates `ClusterController` | Wave progression, maintenance windows, health checks |
| **HOW** to deploy  | `ClusterRolloutStrategy.spec`                          | Waves, batching, pauses, approvals                   |

The `ClusterRolloutController` **never modifies** `Cluster.spec.blueprintRef` or `ClusterBlueprint`. It only gates the timing of when `ClusterController` can apply user-requested updates. This prevents circular references—see [Controller Ownership Model](#controller-ownership-model-circular-reference-prevention).

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
        size: 5 # Update 5 clusters at a time
        interval: "30m" # Wait 30m between batches

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
        size: 1 # One cluster at a time
        interval: "2h"

  # Maintenance window behavior
  # ClusterRolloutController checks cluster.status.maintenance.inWindow
  # (computed by ClusterController) before proceeding with updates
  maintenanceWindows:
    # Respect individual cluster maintenance windows
    respectClusterWindows: true
    # If true, skip clusters outside their window (proceed with others)
    # If false, wait for all clusters in wave to be in their window
    skipIfOutsideWindow: true
    # Maximum time to wait for a maintenance window
    maxWaitTime: "168h" # 1 week

    # What to do when window ends during an active update
    # See "Maintenance Window Enforcement" section for details
    inProgressUpdateStrategy: continue # continue | graceful | checkpoint

    # Alert configuration for window events
    alerts:
      onWindowEndDuringUpdate: true
      channels:
        - type: slack
          target: "#platform-alerts"
        - type: pagerduty
          target: "platform-oncall"
          severity: warning

  # Per-cluster rollout behavior (within each cluster)
  clusterUpdateBehavior:
    # How to update components within a single cluster
    strategy: canary # canary, rolling, blueGreen, allAtOnce
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
    strategy: immediate # immediate, gradual
    # Scope of rollback
    scope: wave # wave, cluster, fleet
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
  phase: Active # Active, Paused, Superseded

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
│              CLUSTER-DRIVEN ROLLOUT WITH SHARED STRATEGY                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ClusterBlueprint                ClusterRolloutStrategy                     │
│  ─────────────────                ───────────────────────                   │
│  "What to deploy"                 "How to roll out"                         │
│                                                                             │
│  ┌─────────────────┐              ┌─────────────────────┐                   │
│  │ production-     │              │ production-rollout  │                   │
│  │ standard        │              │                     │                   │
│  │                 │              │ waves:              │                   │
│  │ revision: v2.4  │              │  1. canary          │                   │
│  │                 │              │  2. staging         │                   │
│  │ planes:         │              │  3. non-critical    │                   │
│  │  - networking   │              │  4. critical        │                   │
│  │  - security     │              │                     │                   │
│  │  - observability│              │ analysis:           │                   │
│  └────────┬────────┘              │  - error-rate < 1%  │                   │
│           │                       │  - p99 < 500ms      │                   │
│           │                       └──────────┬──────────┘                   │
│           │                                  │                              │
│           │              ┌───────────────────┼───────────────────┐          │
│           │              │                   │                   │          │
│           │              ▼                   ▼                   ▼          │
│           │     ┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐ │
│           │     │ cluster-canary  │ │ cluster-staging │ │ cluster-prod-1  │ │
│           │     │                 │ │                 │ │                 │ │
│           │     │ tier: canary    │ │ tier: staging   │ │ tier: critical  │ │
│           │     │                 │ │                 │ │                 │ │
│           └────►│ blueprintRef:   │ │ blueprintRef:   │ │ blueprintRef:   │ │
│                 │   production-   │ │   production-   │ │   production-   │ │
│                 │   standard      │ │   standard      │ │   standard      │ │
│                 │                 │ │                 │ │                 │ │
│                 │ rolloutStrategy │ │ rolloutStrategy │ │ rolloutStrategy │ │
│                 │ Ref: production │ │ Ref: production │ │ Ref: production │ │
│                 │ -rollout        │ │ -rollout        │ │ -rollout        │ │
│                 │                 │ │                 │ │                 │ │
│                 │ maintenance:    │ │ maintenance:    │ │ maintenance:    │ │
│                 │  anytime        │ │  weekends       │ │  Sat 2-6am      │ │
│                 └─────────────────┘ └─────────────────┘ └─────────────────┘ │
│                         │                   │                   │           │
│                         │                   │                   │           │
│  WAVE 1 ───────────────►│                   │                   │           │
│  Updates immediately    │                   │                   │           │
│                         │                   │                   │           │
│  WAVE 2 ────────────────┼──────────────────►│                   │           │
│  Waits 4h after canary  │                   │                   │           │
│  is healthy             │                   │                   │           │
│                         │                   │                   │           │
│  WAVE 4 ────────────────┼───────────────────┼──────────────────►│           │
│  Waits for approval     │                   │                   │           │
│  + maintenance window   │                   │                   │           │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

### Maintenance Window Enforcement

Maintenance windows control **when** cluster updates can occur. The ClusterController computes and exposes window state (`status.maintenance.inWindow`), while ClusterRolloutController checks this before updates.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Cluster
metadata:
  name: production-us-east-1
spec:
  maintenance:
    windows:
      - name: weekend-maintenance
        start: "02:00"
        end: "06:00"
        timezone: "America/New_York" # IANA timezone
        days: [Sat, Sun]
        dstPolicy: extend # extend | shrink | skip
    enforceWindow: true
    allowEmergencyUpdates: true
status:
  maintenance:
    inWindow: true
    currentWindow: { name: weekend-maintenance, endsAt: "2024-12-28T11:00:00Z" }
    nextWindow:
      { name: weeknight-maintenance, startsAt: "2024-12-30T08:00:00Z" }
```

**In-Progress Update Strategies** (when window ends during update):

| Strategy     | Behavior                                           |
| ------------ | -------------------------------------------------- |
| `continue`   | Complete the update (default)                      |
| `graceful`   | Complete current step, pause before next           |
| `checkpoint` | Pause immediately, create checkpoint, resume later |

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterRolloutStrategy
spec:
  maintenanceWindows:
    respectClusterWindows: true
    skipIfOutsideWindow: true
    maxWaitTime: "168h"
    inProgressUpdateStrategy: graceful
```

---

### Cluster Lifecycle Management

A key design goal is supporting the **full cluster lifecycle** - from provisioning brand new clusters to adopting existing ones. The `Cluster` CRD supports three modes of operation.

#### Cluster Modes

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         CLUSTER LIFECYCLE MODES                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  MODE 1: PROVISION                MODE 2: ADOPT                             │
│  ─────────────────                ────────────────                          │
│  "Create a new cluster            "Take over an existing                    │
│   from scratch"                    cluster created elsewhere"               │
│                                                                             │
│  ┌─────────────────┐              ┌─────────────────┐                       │
│  │ Cloud Creds     │              │ Kubeconfig OR   │                       │
│  │ + Region        │              │ Terraform State │                       │
│  │ + Blueprint     │              │ + Blueprint     │                       │
│  └────────┬────────┘              └────────┬────────┘                       │
│           │                                │                                │
│           ▼                                ▼                                │
│  ┌─────────────────┐              ┌─────────────────┐                       │
│  │ VPC Created     │              │ Discovery &     │                       │
│  │ EKS Provisioned │              │ Inventory Scan  │                       │
│  │ Nodes Launched  │              │ State Import    │                       │
│  └────────┬────────┘              └────────┬────────┘                       │
│           │                                │                                │
│           ▼                                ▼                                │
│  ┌─────────────────┐              ┌─────────────────┐                       │
│  │ Blueprint       │              │ Blueprint       │                       │
│  │ Applied         │              │ Reconciled      │                       │
│  └─────────────────┘              └─────────────────┘                       │
│                                                                             │
│  MODE 3: CONNECT                 MODE 4: INFRASTRUCTURE                     │
│  ───────────────                 ─────────────────────                      │
│  "Just manage what's in the      "Virtual cluster to own shared            │
│   cluster, no provisioning"       infrastructure planes"                    │
│                                                                             │
│  ┌─────────────────┐              ┌─────────────────┐                       │
│  │ Kubeconfig      │              │ Blueprint with  │                       │
│  │ + Blueprint     │              │ shared planes   │                       │
│  │ (optional)      │              │ (no kubeconfig) │                       │
│  └────────┬────────┘              └────────┬────────┘                       │
│           │                                │                                │
│           ▼                                ▼                                │
│  ┌─────────────────┐              ┌─────────────────┐                       │
│  │ Inventory Scan  │              │ Shared Planes   │                       │
│  │ Blueprint Apply │              │ Reconciled      │                       │
│  └─────────────────┘              │ Outputs Exposed │                       │
│                                   └─────────────────┘                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### Mode 1: Provision - Create New Cluster

Create a brand new cluster with minimal input. The Cluster CRD is intentionally simple - **all infrastructure configuration (VPC, EKS, node pools) is defined in ClusterPlanes within the blueprint**, not in the Cluster CRD itself.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Cluster
metadata:
  name: production-us-east-1
  namespace: vela-system
spec:
  # MODE: Provision new cluster
  mode: provision

  # Cloud provider configuration - just credentials and region
  provider:
    type: aws # aws, gcp, azure, kind, k3s

    # Reference to cloud credentials secret
    credentialRef:
      name: aws-platform-credentials
      namespace: vela-system

    # Region for cloud resources
    region: us-east-1

  # Blueprint defines ALL infrastructure via ClusterPlanes
  # The aws-foundation plane in this blueprint handles VPC, EKS, node pools
  blueprintRef:
    name: production-standard
    # Optional: override plane parameters for this specific cluster
    # patches:
    #   - plane: aws-foundation
    #     component: vpc
    #     properties:
    #       values:
    #         cidr: "10.100.0.0/16"  # Different CIDR for this cluster

status:
  mode: provision
  phase: Provisioning # Pending, Provisioning, Ready, Failed

  # Connection info (populated by aws-foundation plane outputs)
  connection:
    endpoint: "" # Populated when EKS is ready
    certificateAuthority: ""
    # Connectivity credentials are auto-created from plane outputs

  # Plane provisioning progress (from blueprint)
  planes:
    - name: aws-foundation
      phase: Provisioning
      components:
        - name: vpc
          status: Created
          outputs:
            vpc_id: "vpc-0123456789"
        - name: eks-cluster
          status: Creating
          message: "EKS cluster provisioning..."
        - name: node-pool-system
          status: Pending
        - name: node-pool-workload
          status: Pending

    - name: networking
      phase: Pending # Waiting for aws-foundation

    - name: security
      phase: Pending

  # Timeline
  startedAt: "2024-12-24T10:00:00Z"
  estimatedCompletion: "2024-12-24T10:25:00Z"
```

**Why No `clusterSpec`?**

Infrastructure configuration belongs in ClusterPlanes, not the Cluster CRD:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    INFRASTRUCTURE IN BLUEPRINT, NOT CLUSTER                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  CLUSTER CRD (minimal):                                                     │
│  ──────────────────────                                                     │
│  • provider.credentialRef  → Cloud credentials                              │
│  • provider.region         → Where to deploy                                │
│  • blueprintRef            → What to deploy (everything else)               │
│                                                                             │
│  BLUEPRINT contains:                                                        │
│  ───────────────────                                                        │
│  planes:                                                                    │
│    - name: aws-foundation        ← VPC, EKS, node pools                     │
│      revisionName: aws-foundation-v1.2.0                                    │
│                                                                             │
│    - name: networking            ← CNI, ingress, DNS                        │
│      revisionName: networking-v2.3.1                                        │
│      dependsOn: [aws-foundation]                                            │
│                                                                             │
│    - name: security              ← Cert-manager, policies                   │
│      revisionName: security-v1.8.0                                          │
│      dependsOn: [aws-foundation]                                            │
│                                                                             │
│  BENEFITS:                                                                  │
│  ─────────                                                                  │
│  ✓ VPC/EKS is versioned like any other plane                                │
│  ✓ Platform team owns aws-foundation, can update independently              │
│  ✓ Same blueprint works across clusters (credentials differ)                │
│  ✓ Can test infrastructure changes via plane revision                       │
│  ✓ Rollback VPC changes just like app changes                               │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Minimal Provision Example:**

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
    name: dev-minimal # Blueprint includes dev-sized aws-foundation plane
# That's it! All infrastructure (VPC, EKS, nodes) comes from the blueprint.
# The aws-foundation plane in dev-minimal uses smaller instance types.
```

#### Mode 2: Adopt - Connect to Existing Cluster

Connect to an existing cluster and bring it under management. The ClusterController manages connectivity directly.

**Connectivity Options:**

| Option | Description | Use Case |
|--------|-------------|----------|
| `credential` (inline) | Credentials directly in spec | Simple, self-contained |
| `credential.secretRef` | Reference to kubeconfig secret | Standard Kubernetes pattern |
| `credential.cloudProvider` | Cloud-native auth (IAM, workload identity) | Production cloud deployments |

**Option 1: Inline Credentials:**

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Cluster
metadata:
  name: new-partner-cluster
spec:
  mode: adopt

  # Inline credential - controller manages connectivity directly
  credential:
    type: X509  # X509, ServiceAccountToken, Bearer
    endpoint: "https://partner.k8s.example.com:6443"
    caData: "LS0tLS1CRUdJTi..."
    certData: "LS0tLS1CRUdJTi..."
    keyData: "LS0tLS1CRUdJTi..."

  blueprintRef:
    name: partner-minimal
```

**Option 2: Secret Reference (Standard Kubernetes pattern):**

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Cluster
metadata:
  name: production-us-east-1
spec:
  mode: adopt

  # Reference any kubeconfig secret
  credential:
    secretRef:
      name: prod-cluster-kubeconfig
      namespace: vela-system
      key: kubeconfig  # Optional, defaults to "kubeconfig"

  blueprintRef:
    name: production-standard
```

**Option 3: Cloud Provider Native Auth (Recommended for cloud):**

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Cluster
metadata:
  name: eks-production
spec:
  mode: adopt

  # Use cloud-native authentication (no static credentials)
  credential:
    cloudProvider:
      type: aws-eks
      clusterName: my-eks-cluster
      region: us-east-1
      # Uses workload identity / IRSA - no credentials stored
      # ClusterController assumes IAM role to get temporary credentials

  blueprintRef:
    name: production-standard
```

**Step-by-Step Adoption Workflow:**

```yaml
# Step 1: Create Cluster CRD with credentials (no CLI needed)
apiVersion: core.oam.dev/v1beta1
kind: Cluster
metadata:
  name: legacy-production
  namespace: vela-system
spec:
  mode: adopt
  credential:
    secretRef:
      name: legacy-production-kubeconfig
  adoption:
    existingResources:
      mode: discover
  blueprintRef:
    name: production-standard
    reconcileMode: dryRun

---
# Step 2: After reviewing status.adoptionStatus.discoveredComponents:

apiVersion: core.oam.dev/v1beta1
kind: Cluster
metadata:
  name: legacy-production
spec:
  mode: adopt
  credential:
    secretRef:
      name: legacy-production-kubeconfig
  adoption:
    existingResources:
      mode: reconcile  # Now actually reconcile
  blueprintRef:
    name: production-standard
    reconcileMode: gradual
    gradualReconcile:
      order:
        - action: upgrade
          components: [ingress-nginx, cert-manager]
        - action: install
          components: [gatekeeper]
        - action: install
          components: [prometheus-stack]
      pauseBetweenPhases: "1h"
      progression: manual
```

#### Mode 3: Connect - Manage Existing Cluster

Simply connect to an existing cluster without adopting infrastructure management. Uses the same connectivity options as Mode 2.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Cluster
metadata:
  name: partner-cluster
  namespace: vela-system
spec:
  # MODE: Just connect and manage Kubernetes resources
  mode: connect

  # Any connectivity option works (see Mode 2 for all options)
  credential:
    secretRef:
      name: partner-cluster-kubeconfig

  # Or use cloud provider auth:
  # credential:
  #   cloudProvider:
  #     type: gcp-gke
  #     projectId: my-project
  #     clusterName: partner-cluster
  #     region: us-central1

  # What to manage (optional - limits scope)
  managementScope:
    namespaces:
      include:
        - vela-managed-*
        - platform-*
      exclude:
        - kube-system
        - kube-public
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

#### Mode 4: Infrastructure - Own Shared Planes

A **virtual cluster** that owns shared infrastructure planes. This is NOT a real Kubernetes cluster—it's a logical grouping for shared infrastructure with clear ownership semantics.

**Key Characteristics:**
- No kubeconfig or credentials needed (not a real K8s cluster)
- Owns shared planes (VPC, Transit Gateway, shared DNS, etc.)
- Workload clusters consume outputs from these shared planes
- Deletion blocked while consumers exist

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Cluster
metadata:
  name: us-east-1-infrastructure
  namespace: vela-system
  labels:
    cluster.oam.dev/role: infrastructure-preparer
    region: us-east-1
spec:
  # MODE: Infrastructure preparer - owns shared planes
  mode: infrastructure

  # Where shared plane resources are created
  infrastructureTarget:
    # Option 1: Cloud resources (VPC, NAT Gateway, etc.)
    type: cloud
    providerRef:
      name: aws-production

    # Option 2: Hub cluster namespace (for K8s-native shared resources)
    # type: hub
    # namespace: shared-infrastructure-us-east

  # Blueprint containing shared planes
  blueprintRef:
    name: shared-infrastructure-us-east

status:
  mode: infrastructure
  phase: Ready

  # Shared plane status
  planes:
    - name: shared-vpc
      scope: shared
      phase: Running
      outputs:
        vpcId: "vpc-0abc123def456"
        privateSubnets: '["subnet-1a","subnet-1b","subnet-1c"]'

  # Workload clusters consuming these shared planes
  consumers:
    count: 3
    clusters:
      - name: prod-us-east-1-a
        consumedPlanes: [shared-vpc]
      - name: prod-us-east-1-b
        consumedPlanes: [shared-vpc]
      - name: prod-us-east-1-c
        consumedPlanes: [shared-vpc]
```

**Why Infrastructure Mode?**

| Without Infrastructure Mode | With Infrastructure Mode |
|-----------------------------|--------------------------|
| Shared plane ownership unclear | Platform team owns infrastructure cluster |
| First workload cluster triggers creation | Infrastructure cluster triggers creation |
| Deletion semantics confusing | Natural deletion protection via consumers |
| No clear RBAC boundary | Platform team manages infra, app teams manage workloads |

**Deletion Protection:**

```bash
$ kubectl delete cluster us-east-1-infrastructure

Error from server: admission webhook "cluster.validation.oam.dev" denied the request:
  Cannot delete infrastructure cluster "us-east-1-infrastructure"

  3 workload clusters are consuming shared planes:
    - prod-us-east-1-a (consuming: shared-vpc)
    - prod-us-east-1-b (consuming: shared-vpc)
    - prod-us-east-1-c (consuming: shared-vpc)

  To delete:
  1. Remove or migrate consumers first
  2. Or use --force (DANGER: will orphan shared infrastructure)
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
      natGateway: single # single, perAz, none

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
│                    CLUSTER PROVISIONING ARCHITECTURE                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌───────────────┐                                                          │
│  │    Cluster    │                                                          │
│  │   Controller  │                                                          │
│  └───────┬───────┘                                                          │
│          │                                                                  │
│          │ Reads ClusterProviderDefinition                                  │
│          │ to determine provisioning method                                 │
│          │                                                                  │
│          ▼                                                                  │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                    PROVISIONING BACKENDS                            │    │
│  ├─────────────────────────────────────────────────────────────────────┤    │
│  │                                                                     │    │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐               │    │
│  │  │  Crossplane  │  │  Terraform   │  │    Native    │               │    │
│  │  │              │  │  Controller  │  │   Provider   │               │    │
│  │  │  - AWS EKS   │  │              │  │              │               │    │
│  │  │  - GCP GKE   │  │  - Any TF    │  │  - kind      │               │    │
│  │  │  - Azure AKS │  │    module    │  │  - k3s       │               │    │
│  │  │              │  │              │  │  - custom    │               │    │
│  │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘               │    │
│  │         │                 │                 │                       │    │
│  └─────────┼─────────────────┼─────────────────┼───────────────────────┘    │
│            │                 │                 │                            │
│            ▼                 ▼                 ▼                            │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                    CLOUD PROVIDERS                                  │    │
│  │                                                                     │    │
│  │   ┌─────────┐    ┌─────────┐    ┌─────────┐    ┌─────────┐          │    │
│  │   │   AWS   │    │   GCP   │    │  Azure  │    │  Local  │          │    │
│  │   │   EKS   │    │   GKE   │    │   AKS   │    │  kind   │          │    │
│  │   └─────────┘    └─────────┘    └─────────┘    └─────────┘          │    │
│  │                                                                     │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  After Provisioning:                                                        │
│  ──────────────────                                                         │
│  1. ClusterController obtains kubeconfig                                    │
│  2. Updates Cluster status with connection info                             │
│  3. ClusterController applies planes from referenced Blueprint              │
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
    type: autodetect # The Helm chart determines the workload type

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
│                         CLUSTER ROLLOUT STATE MACHINE                       │
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
│     ┌───────────────────────────────────────────────────────────────┐       │
│     │                      BATCH LOOP                               │       │
│     │  ┌─────────────┐     ┌─────────────┐     ┌─────────────┐      │       │
│     │  │   Updating  │────▶│  Analyzing  │────▶│   Paused    │      │       │
│     │  │   Cluster   │     │   Metrics   │     │ (optional)  │      │       │
│     │  └─────────────┘     └──────┬──────┘     └──────┬──────┘      │       │
│     │                             │                    │            │       │
│     │         ┌───────────────────┼────────────────────┘            │       │
│     │         │                   │                                 │       │
│     │         │    SLO Pass       │    SLO Fail                     │       │
│     │         ▼                   ▼                                 │       │
│     │  ┌─────────────┐     ┌─────────────┐                          │       │
│     │  │ Next Batch  │     │ RollingBack │                          │       │
│     │  └──────┬──────┘     └──────┬──────┘                          │       │
│     │         │                   │                                 │       │
│     └─────────┼───────────────────┼─────────────────────────────────┘       │
│               │                   │                                         │
│               │ All batches       │                                         │
│               │ complete          │                                         │
│               ▼                   ▼                                         │
│        ┌──────────┐        ┌────────────┐                                   │
│        │Succeeded │        │ RolledBack │                                   │
│        └──────────┘        └────────────┘                                   │
│                                                                             │
│  Manual Controls:                                                           │
│  - Pause: Enter Paused state at any batch                                   │
│  - Resume: Continue from Paused state                                       │
│  - Abort: Cancel rollout, remain at current state                           │
│  - Rollback: Manually trigger rollback                                      │
│  - Promote: Skip remaining batches, apply to all                            │
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
  namespace: platform-networking # Team's namespace
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
    verbs: ["get", "list", "watch"] # Can reference but not modify
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
│                           HEALTH HIERARCHY                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  CLUSTER LEVEL                                                              │
│  ─────────────                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ Cluster: production-us-east-1                                       │    │
│  │ Health: Degraded (1 of 3 planes unhealthy)                          │    │
│  │                                                                     │    │
│  │ Aggregated from:                                                    │    │
│  │   ✓ networking: Healthy                                             │    │
│  │   ✗ security: Degraded (cert-manager unhealthy)                     │    │
│  │   ✓ observability: Healthy                                          │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│           │                                                                 │
│           ▼                                                                 │
│  PLANE LEVEL (drill down into security plane)                               │
│  ───────────                                                                │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ Plane: security                                                     │    │
│  │ Health: Degraded (1 of 3 components unhealthy)                      │    │
│  │                                                                     │    │
│  │ Aggregated from:                                                    │    │
│  │   ✓ gatekeeper: Healthy                                             │    │
│  │   ✗ cert-manager: Unhealthy (certificate renewal failing)           │    │
│  │   ✓ external-secrets: Healthy                                       │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│           │                                                                 │
│           ▼                                                                 │
│  COMPONENT LEVEL (drill down into cert-manager)                             │
│  ───────────────                                                            │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │ Component: cert-manager                                             │    │
│  │ Health: Unhealthy                                                   │    │
│  │                                                                     │    │
│  │ Health Checks:                                                      │    │
│  │   ✓ Deployment ready: 3/3 replicas                                  │    │
│  │   ✓ Pod health: All pods running                                    │    │
│  │   ✗ Functional: Certificate renewal error rate > 5%                 │    │
│  │   ✗ SLO: ACME challenge success rate < 99%                          │    │
│  │                                                                     │    │
│  │ Resources:                                                          │    │
│  │   ✓ Deployment/cert-manager: 3/3 ready                              │    │
│  │   ✓ Deployment/cert-manager-webhook: 1/1 ready                      │    │
│  │   ✓ Deployment/cert-manager-cainjector: 1/1 ready                   │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### ObservabilityProviderDefinition

To support multiple observability backends, we introduce `ObservabilityProviderDefinition`. Each definition specifies connection schema, query templates, and built-in metrics.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ObservabilityProviderDefinition
metadata:
  name: prometheus
  namespace: vela-system
spec:
  type: prometheus
  connectionSpec:
    properties:
      endpoint: { type: string, required: true }
      auth:
        {
          type: object,
          properties:
            {
              type: { enum: [none, basic, bearer] },
              secretRef: { type: object },
            },
        }
  queryTemplate: |
    query: { type: "instant" | "range", promql: string }
  builtinMetrics:
    - name: error-rate
      query: 'sum(rate(http_requests_total{status=~"5.."}[5m])) / sum(rate(http_requests_total[5m])) * 100'
```

Additional providers (Datadog, New Relic, CloudWatch, custom-webhook) follow the same pattern with provider-specific query languages.

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
              value: 1 # Error rate < 1%
            for: "5m" # Must be true for 5 minutes

        # Datadog APM check (alternative provider)
        - name: latency-slo
          type: metrics
          metrics:
            providerRef:
              name: datadog-prod
            query: "p99:nginx.http.request.duration{service:ingress-nginx}"
            threshold:
              operator: "<"
              value: 0.5 # p99 < 500ms

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
      strategy: all # all, any, majority, weighted

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
    status: Degraded # Healthy, Degraded, Unhealthy, Unknown, Progressing
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
  ┌─────────────────┬────────────┬─────────────────────────────────────────┐
  │ COMPONENT       │ STATUS     │ HEALTH CHECKS                           │
  ├─────────────────┼────────────┼─────────────────────────────────────────┤
  │ gatekeeper      │ ✓ Healthy  │ deployment-ready: ✓                     │
  │                 │            │ policy-violations: ✓ (< 10)             │
  ├─────────────────┼────────────┼─────────────────────────────────────────┤
  │ cert-manager    │ ✗ Unhealthy│ deployment-ready: ✓ (3/3)               │
  │                 │            │ certificate-renewal: ✗ (8.5% > 5%)      │
  │                 │            │ acme-success-rate: ✗ (91% < 99%)        │
  ├─────────────────┼────────────┼─────────────────────────────────────────┤
  │ external-secrets│ ✓ Healthy  │ deployment-ready: ✓                     │
  │                 │            │ sync-success-rate: ✓ (99.9%)            │
  └─────────────────┴────────────┴─────────────────────────────────────────┘

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
          expiresAt: "2025-03-01T00:00:00Z" # Optional expiration
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
  comparisonType: assigned # or "what-if"
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
        version: "4.9.0" # Updated from 4.8.3
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
# 1. Register new cluster using the Cluster CRD (connect mode)
apiVersion: core.oam.dev/v1beta1
kind: Cluster
metadata:
  name: production-ap-south-1
  labels:
    environment: production
    region: ap-south-1
    tier: standard
spec:
  mode: connect
  credential:
    secretRef:
      name: prod-ap-south-1-kubeconfig
      namespace: vela-system
  # spec.blueprintRef is user-owned - user specifies desired blueprint
  blueprintRef:
    name: production-standard

---
# 2. ClusterController detects new cluster and applies the blueprint via workflow
# (ClusterController owns status.blueprint, not spec.blueprintRef)

# 3. Status shows onboarding progress
status:
  blueprint:
    name: production-standard
    revision: production-standard-v2.3.0
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
        type: dns # or loadBalancer
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

### Use Case 5: Multi-Cluster Shared VPC Infrastructure

This use case demonstrates how multiple EKS clusters in the same AWS region can **share foundational infrastructure** (VPC, NAT Gateways, subnets) while maintaining their own cluster-specific resources using the plane-level `scope` model.

**Scenario:**

- 3 production EKS clusters in `us-east-1` sharing a single VPC
- Shared: VPC, subnets, NAT Gateways, Internet Gateway
- Per-cluster: EKS control plane, node groups, IAM OIDC provider

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          SHARED VPC ARCHITECTURE                            │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  AWS Region: us-east-1                                                      │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                     VPC: 10.0.0.0/16 (SHARED)                         │  │
│  │                     Created by: shared-vpc-us-east-1 plane            │  │
│  │                                                                       │  │
│  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐        │  │
│  │  │ Private Subnet  │  │ Private Subnet  │  │ Private Subnet  │        │  │
│  │  │    (AZ-a)       │  │    (AZ-b)       │  │    (AZ-c)       │        │  │
│  │  │                 │  │                 │  │                 │        │  │
│  │  │ ┌─────────────┐ │  │ ┌─────────────┐ │  │ ┌─────────────┐ │        │  │
│  │  │ │ EKS-A Nodes │ │  │ │ EKS-B Nodes │ │  │ │ EKS-C Nodes │ │        │  │
│  │  │ │ (perCluster)│ │  │ │ (perCluster)│ │  │ │ (perCluster)│ │        │  │
│  │  │ └─────────────┘ │  │ └─────────────┘ │  │ └─────────────┘ │        │  │
│  │  └────────┬────────┘  └────────┬────────┘  └────────┬────────┘        │  │
│  │           │                    │                    │                 │  │
│  │           └────────────────────┴────────────────────┘                 │  │
│  │                                │                                      │  │
│  │                    ┌───────────▼───────────┐                          │  │
│  │                    │  NAT Gateways (SHARED) │                         │  │
│  │                    └───────────┬───────────┘                          │  │
│  │                                │                                      │  │
│  │                    ┌───────────▼───────────┐                          │  │
│  │                    │ Internet GW (SHARED)  │                          │  │
│  │                    └───────────────────────┘                          │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Step 1: Define Shared VPC Plane**

```yaml
# Shared plane - created once, used by all clusters in scope
apiVersion: core.oam.dev/v1beta1
kind: ClusterPlane
metadata:
  name: shared-vpc-us-east-1
  namespace: vela-system
  labels:
    plane.oam.dev/category: infrastructure
spec:
  description: "Shared VPC infrastructure for us-east-1 production clusters"
  scope: shared

  # Restrict which clusters can use this plane
  sharedWith:
    clusterSelector:
      matchLabels:
        region: us-east-1
        environment: production

  components:
    - name: vpc
      type: terraform-module
      properties:
        source: "terraform-aws-modules/vpc/aws"
        version: "5.1.0"
        values:
          name: "production-us-east-1-vpc"
          cidr: "10.0.0.0/16"
          azs: ["us-east-1a", "us-east-1b", "us-east-1c"]
          private_subnets: ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]
          public_subnets: ["10.0.101.0/24", "10.0.102.0/24", "10.0.103.0/24"]
          enable_nat_gateway: true
          single_nat_gateway: false # HA - one per AZ
          enable_dns_hostnames: true
          enable_dns_support: true
          # Tag for Kubernetes integration
          private_subnet_tags:
            "kubernetes.io/role/internal-elb": "1"
          public_subnet_tags:
            "kubernetes.io/role/elb": "1"
          tags:
            "shared-infrastructure": "true"
            "managed-by": "kubevela"

  outputs:
    - name: vpcId
      valueFrom:
        component: vpc
        fieldPath: outputs.vpc_id
    - name: privateSubnets
      valueFrom:
        component: vpc
        fieldPath: outputs.private_subnets
    - name: publicSubnets
      valueFrom:
        component: vpc
        fieldPath: outputs.public_subnets
    - name: natGatewayIds
      valueFrom:
        component: vpc
        fieldPath: outputs.natgw_ids
```

**Step 2: Define Per-Cluster EKS Plane**

```yaml
# Per-cluster plane - created for each cluster using the blueprint
apiVersion: core.oam.dev/v1beta1
kind: ClusterPlane
metadata:
  name: eks-cluster
  namespace: vela-system
spec:
  description: "EKS cluster with node groups"
  scope: perCluster # Default - each cluster gets its own

  # Import outputs from shared VPC plane
  inputs:
    - name: vpcId
      fromPlane: shared-vpc-us-east-1
      output: vpcId
    - name: privateSubnets
      fromPlane: shared-vpc-us-east-1
      output: privateSubnets

  components:
    - name: eks
      type: terraform-module
      properties:
        source: "terraform-aws-modules/eks/aws"
        version: "19.21.0"
        values:
          cluster_name: "${context.cluster.name}"
          cluster_version: "1.29"
          vpc_id: "{{ inputs.vpcId }}"
          subnet_ids: "{{ inputs.privateSubnets }}"
          cluster_endpoint_public_access: false
          cluster_endpoint_private_access: true
          enable_irsa: true
          # Cluster-specific tags
          tags:
            "kubernetes.io/cluster/${context.cluster.name}": "owned"

    - name: node-group-system
      type: terraform-module
      dependsOn: [eks]
      properties:
        source: "terraform-aws-modules/eks/aws//modules/eks-managed-node-group"
        values:
          name: "system"
          cluster_name: "{{ outputs.eks.cluster_name }}"
          subnet_ids: "{{ inputs.privateSubnets }}"
          instance_types: ["m5.large"]
          min_size: 3
          max_size: 5
          labels:
            role: system

    - name: node-group-workload
      type: terraform-module
      dependsOn: [eks]
      properties:
        source: "terraform-aws-modules/eks/aws//modules/eks-managed-node-group"
        values:
          name: "workload"
          cluster_name: "{{ outputs.eks.cluster_name }}"
          subnet_ids: "{{ inputs.privateSubnets }}"
          instance_types: ["m5.xlarge", "m5.2xlarge"]
          min_size: 2
          max_size: 20
          labels:
            role: workload

  outputs:
    - name: clusterEndpoint
      valueFrom:
        component: eks
        fieldPath: outputs.cluster_endpoint
    - name: clusterName
      valueFrom:
        component: eks
        fieldPath: outputs.cluster_name
    - name: oidcProviderArn
      valueFrom:
        component: eks
        fieldPath: outputs.oidc_provider_arn
```

**Step 3: Blueprint Composes Both**

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterBlueprint
metadata:
  name: production-eks-standard
spec:
  description: "Standard production EKS cluster with shared VPC"

  planes:
    # Shared VPC - created once
    - name: vpc
      ref:
        name: shared-vpc-us-east-1

    # Per-cluster EKS - created for each cluster
    - name: eks
      ref:
        name: eks-cluster
      dependsOn: [vpc]

    # Per-cluster networking (Cilium, ingress, etc.)
    - name: networking
      ref:
        name: networking
      dependsOn: [eks]

    # Per-cluster observability
    - name: observability
      ref:
        name: observability
      dependsOn: [networking]
```

**Step 4: Create Clusters Using Blueprint**

```yaml
# Cluster A
apiVersion: core.oam.dev/v1beta1
kind: Cluster
metadata:
  name: production-us-east-1-a
  labels:
    region: us-east-1
    environment: production
spec:
  mode: provision
  blueprintRef:
    name: production-eks-standard
---
# Cluster B
apiVersion: core.oam.dev/v1beta1
kind: Cluster
metadata:
  name: production-us-east-1-b
  labels:
    region: us-east-1
    environment: production
spec:
  mode: provision
  blueprintRef:
    name: production-eks-standard
---
# Cluster C
apiVersion: core.oam.dev/v1beta1
kind: Cluster
metadata:
  name: production-us-east-1-c
  labels:
    region: us-east-1
    environment: production
spec:
  mode: provision
  blueprintRef:
    name: production-eks-standard
```

**Result:**

When these clusters are created:

1. **shared-vpc-us-east-1** plane **is reconciled once** (by the first Cluster referencing it, or by an infrastructure-mode preparer cluster):

   - VPC, subnets, NAT Gateways are created
   - Outputs become available for consumption

2. **eks-cluster** plane **is reconciled three times** (once per Cluster that references it):
   - Each Cluster triggers its own reconciliation
   - Each gets unique EKS cluster name from `${context.cluster.name}`
   - All consume the same VPC outputs via `{{ inputs.vpcId }}`
   - Each has its own node groups

**Status After Provisioning:**

```bash
$ vela plane list --scope shared

NAME                    SCOPE    CONSUMERS  STATUS   OUTPUTS
shared-vpc-us-east-1    shared   3          Running  vpcId=vpc-0abc123

$ vela plane status shared-vpc-us-east-1

SHARED PLANE: shared-vpc-us-east-1
SCOPE: shared
STATUS: Running

CONSUMERS (3):
  CLUSTER                      BLUEPRINT                  SINCE
  production-us-east-1-a       production-eks-standard    2025-01-03T10:00:00Z
  production-us-east-1-b       production-eks-standard    2025-01-03T10:15:00Z
  production-us-east-1-c       production-eks-standard    2025-01-03T10:30:00Z

OUTPUTS:
  vpcId: vpc-0abc123def456
  privateSubnets: ["subnet-priv-1a", "subnet-priv-1b", "subnet-priv-1c"]
  publicSubnets: ["subnet-pub-1a", "subnet-pub-1b", "subnet-pub-1c"]
  natGatewayIds: ["nat-1a", "nat-1b", "nat-1c"]
```

**Deletion Safety:**

```bash
# Try to delete the shared VPC plane
$ kubectl delete clusterplane shared-vpc-us-east-1

Error from server: Cannot delete shared ClusterPlane "shared-vpc-us-east-1"
  3 clusters are consuming this plane's outputs.

  To delete safely:
  1. Delete or migrate clusters: production-us-east-1-a, production-us-east-1-b, production-us-east-1-c
  2. Or update their blueprints to not use this plane

# Delete a cluster - VPC remains intact
$ kubectl delete cluster production-us-east-1-a

cluster.core.oam.dev "production-us-east-1-a" deleted

# EKS for cluster A is destroyed, but shared VPC remains
# shared-vpc-us-east-1 now shows 2 consumers
```

**Key Benefits of This Approach:**

| Benefit                   | Description                                                                        |
| ------------------------- | ---------------------------------------------------------------------------------- |
| **Cost Efficiency**       | Single VPC with NAT Gateways shared across clusters (NAT can cost $30+/month each) |
| **Simplified Networking** | All clusters in same VPC can communicate directly                                  |
| **Clear Ownership**       | VPC is owned by the `shared-vpc-us-east-1` plane, not any cluster                  |
| **Safe Deletion**         | Can't accidentally delete VPC while clusters depend on it                          |
| **Easy to Understand**    | Shared plane = shared resources, per-cluster plane = cluster-specific resources    |

---

## Edge Cases and Considerations

| #   | Problem                                                       | Solution                                                                                     |
| --- | ------------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| 1   | **Circular Dependencies**: Plane A → B → A                    | Validation webhook rejects; topological sort detects cycles                                  |
| 2   | **Partial Plane Failure**: 2/3 components fail                | `failurePolicy: failFast\|continueOnError\|rollbackOnError`; per-component `critical: false` |
| 3   | **Version Conflicts**: Plane needs K8s 1.28+, cluster is 1.27 | `spec.requirements.kubernetes.minVersion`; validation fails if unmet                         |
| 4   | **Rollout During Incident**: Metrics skewed                   | `incidentIntegration.pauseOnIncident: [P1, P2]`; baseline comparison mode                    |
| 5   | **Disruptive Upgrades**: CNI requires node restart            | `upgradeStrategy: nodeByNode` with `maxUnavailable`, PDB awareness                           |
| 6   | **Orphaned Resources**: Component removal leaves resources    | `garbageCollection.onComponentRemoval: delete\|orphan\|warn`                                 |
| 7   | **State Drift**: Cluster 3/10 drifts                          | `driftDetection.enabled: true`; auto-remediation with rate limiting                          |
| 8   | **Multi-Timezone Rollout**: Avoid business hours per region   | Per-cluster `maintenance.windows[]` with IANA timezones; rollout respects windows            |
| 9   | **Secrets Distribution**: Components need credentials         | `secrets[].distribution.type: perCluster` with templates                                     |
| 10  | **Provisioning Failure**: VPC created, EKS fails              | `provisioningPolicy.onFailure: retry\|cleanup\|pause`; `partialInfrastructure.retain: true`  |
| 11  | **Conflicting Components**: Cluster has old ingress-nginx     | `adoption.conflictResolution.strategy: prompt\|upgrade\|replace`                             |
| 12  | **Credential Rotation**: Credentials expire mid-provision     | `credentialPolicy.refresh.enabled: true`; `onFailure: pause`                                 |
| 13  | **Unknown Components**: Custom deployments discovered         | `adoption.unknownComponents.action: discover\|import\|ignore`                                |
| 14  | **Air-gapped Provisioning**: No internet access               | `networkPolicy.airgapped: true`; private registry/helm repo mirrors                          |
| 15  | **Cluster Deletion**: Clean up but preserve audit logs        | `deletionPolicy.clusterDeletion: delete`; `retain: [cloudwatchLogs, s3Backups]`              |
| 16  | **Shared Plane Deletion**: Delete shared plane with consumers | Validation webhook blocks; must delete consumers first or use `--force`                      |
| 17  | **Unauthorized Consumer**: Cluster doesn't match sharedWith   | Validation rejects; cluster labels must match `sharedWith.clusterSelector`                   |
| 18  | **Shared Plane Not Ready**: Per-cluster plane needs outputs   | Per-cluster plane blocks with `phase=Waiting`; retries until shared plane outputs available  |
| 19  | **Scope Change**: Change plane from perCluster to shared      | Validation rejects scope changes on existing planes; create new plane instead                |

---

## API Reference

### Cluster

| Field                                               | Type                | Description                                                   |
| --------------------------------------------------- | ------------------- | ------------------------------------------------------------- |
| `spec.mode`                                         | string              | Cluster mode: `provision`, `adopt`, `connect`, or `infrastructure` |
| `spec.provider.type`                                | string              | Cloud provider: `aws`, `gcp`, `azure`, `kind`, `k3s`          |
| `spec.provider.credentialRef`                       | SecretRef           | Reference to cloud credentials secret                         |
| `spec.provider.region`                              | string              | Cloud region for provisioning                                 |
| `spec.credential.secretRef`                         | SecretRef           | Kubeconfig secret reference (for adopt/connect)               |
| `spec.clusterSpec`                                  | ClusterSpec         | Kubernetes version, node pools, networking (provision mode)   |
| `spec.blueprintRef`                                 | BlueprintRef        | Blueprint to apply                                            |
| `spec.adoption`                                     | AdoptionSpec        | Adoption configuration (adopt mode)                           |
| `spec.patches`                                      | []PlanePatch        | Cluster-specific blueprint overrides                          |
| `spec.rolloutStrategyRef`                           | StrategyRef         | Reference to ClusterRolloutStrategy                           |
| `spec.rolloutStrategyRef.overrides`                 | OverrideSpec        | Cluster-specific rollout overrides                            |
| `spec.maintenance`                                  | MaintenanceSpec     | Maintenance windows                                           |
| `spec.maintenance.windows`                          | []MaintenanceWindow | Scheduled maintenance windows                                 |
| `spec.maintenance.windows[].name`                   | string              | Window identifier                                             |
| `spec.maintenance.windows[].start`                  | string              | Start time (HH:MM format)                                     |
| `spec.maintenance.windows[].end`                    | string              | End time (HH:MM format)                                       |
| `spec.maintenance.windows[].timezone`               | string              | IANA timezone name (e.g., `America/New_York`)                 |
| `spec.maintenance.windows[].days`                   | []string            | Days of week: `Mon`, `Tue`, `Wed`, `Thu`, `Fri`, `Sat`, `Sun` |
| `spec.maintenance.windows[].dstPolicy`              | string              | DST handling: `extend` (default), `shrink`, `skip`            |
| `spec.maintenance.enforceWindow`                    | bool                | Block updates outside maintenance window                      |
| `spec.maintenance.allowEmergencyUpdates`            | bool                | Allow forced updates with `--force` flag                      |
| `status.mode`                                       | string              | Active mode                                                   |
| `status.connectionStatus`                           | string              | Connection status: `Connected`, `Disconnected`                |
| `status.provisioningStatus`                         | ProvisioningStatus  | Infrastructure provisioning progress                          |
| `status.adoptionStatus`                             | AdoptionStatus      | Adoption discovery and reconciliation status                  |
| `status.clusterInfo`                                | ClusterInfo         | Discovered cluster information                                |
| `status.blueprint`                                  | BlueprintStatus     | Applied blueprint status                                      |
| `status.planes`                                     | []PlaneInventory    | Full inventory of planes and components                       |
| `status.health`                                     | HealthStatus        | Aggregated health status                                      |
| `status.drift`                                      | DriftStatus         | Drift detection results                                       |
| `status.maintenance`                                | MaintenanceStatus   | Computed maintenance window state                             |
| `status.maintenance.inWindow`                       | bool                | Currently within a maintenance window                         |
| `status.maintenance.currentWindow`                  | WindowInfo          | Current active window details (if inWindow)                   |
| `status.maintenance.currentWindow.name`             | string              | Window name                                                   |
| `status.maintenance.currentWindow.endsAt`           | Time                | When current window ends (UTC)                                |
| `status.maintenance.currentWindow.remainingMinutes` | int                 | Minutes remaining in window                                   |
| `status.maintenance.nextWindow`                     | WindowInfo          | Next scheduled window                                         |
| `status.maintenance.nextWindow.startsAt`            | Time                | When next window starts (UTC)                                 |
| `status.maintenance.nextWindow.startsInMinutes`     | int                 | Minutes until next window                                     |
| `status.maintenance.lastEvaluatedAt`                | Time                | Last window evaluation time                                   |
| `status.maintenance.timezoneInfo`                   | TimezoneInfo        | Timezone details with DST info                                |
| `status.resources`                                  | ResourceUsage       | CPU, memory, pod usage                                        |
| `status.history`                                    | []HistoryEntry      | Blueprint application history                                 |

### ClusterPlane

| Field                                   | Type              | Description                                                |
| --------------------------------------- | ----------------- | ---------------------------------------------------------- |
| `spec.description`                      | string            | Human-readable description                                 |
| `spec.scope`                            | string            | `perCluster` (default) or `shared`                         |
| `spec.sharedWith`                       | SharedWithSpec    | Constraints on which clusters can use shared plane         |
| `spec.sharedWith.clusterSelector`       | LabelSelector     | Labels clusters must have to consume this shared plane     |
| `spec.inputs`                           | []PlaneInput      | Inputs from other planes (simpler than crossClusterInputs) |
| `spec.inputs[].name`                    | string            | Input name for templating                                  |
| `spec.inputs[].fromPlane`               | string            | Source plane name                                          |
| `spec.inputs[].output`                  | string            | Output name from source plane                              |
| `spec.components`                       | []PlaneComponent  | Components in this plane                                   |
| `spec.policies`                         | []PlanePolicy     | Plane-level policies                                       |
| `spec.outputs`                          | []PlaneOutput     | Values exposed to other planes                             |
| `spec.requirements`                     | Requirements      | Compatibility requirements                                 |
| `spec.failurePolicy`                    | FailurePolicy     | How to handle component failures                           |
| `spec.garbageCollection`                | GCPolicy          | Resource cleanup policy                                    |
| `status.phase`                          | string            | Current phase                                              |
| `status.scope`                          | string            | Effective scope (`perCluster` or `shared`)                 |
| `status.consumers`                      | ConsumersStatus   | Clusters using this shared plane (scope=shared only)       |
| `status.consumers.count`                | int               | Number of clusters consuming this plane                    |
| `status.consumers.clusters`             | []ConsumerRef     | List of consuming clusters                                 |
| `status.consumers.clusters[].name`      | string            | Cluster name                                               |
| `status.consumers.clusters[].blueprint` | string            | Blueprint the cluster uses                                 |
| `status.consumers.clusters[].since`     | Time              | When the cluster started consuming                         |
| `status.components`                     | []ComponentStatus | Per-component status                                       |
| `status.outputs`                        | map[string]string | Resolved output values                                     |

### ClusterBlueprint

| Field                    | Type              | Description                    |
| ------------------------ | ----------------- | ------------------------------ |
| `spec.planes`            | []PlaneRef        | Referenced planes with patches |
| `spec.policies`          | []BlueprintPolicy | Blueprint-level policies       |
| `spec.workflow`          | Workflow          | Deployment workflow            |
| `status.planes`          | []PlaneStatus     | Per-plane status               |
| `status.appliedClusters` | []ClusterStatus   | Per-cluster status             |

### ClusterRolloutStrategy

| Field                                                    | Type            | Description                                                                      |
| -------------------------------------------------------- | --------------- | -------------------------------------------------------------------------------- |
| `spec.description`                                       | string          | Human-readable description                                                       |
| `spec.waves`                                             | []Wave          | Wave definitions with ordering and selectors                                     |
| `spec.waves[].name`                                      | string          | Wave identifier                                                                  |
| `spec.waves[].order`                                     | int             | Wave execution order                                                             |
| `spec.waves[].clusterSelector`                           | LabelSelector   | Which clusters belong to this wave                                               |
| `spec.waves[].waitFor`                                   | WaitCondition   | Previous wave dependency                                                         |
| `spec.waves[].waitFor.wave`                              | string          | Name of wave to wait for                                                         |
| `spec.waves[].waitFor.healthyDuration`                   | Duration        | How long wave must be healthy                                                    |
| `spec.waves[].pause`                                     | PauseSpec       | Pause duration after wave                                                        |
| `spec.waves[].approval`                                  | ApprovalSpec    | Manual approval requirement                                                      |
| `spec.waves[].batching`                                  | BatchSpec       | Batch size and interval within wave                                              |
| `spec.maintenanceWindows.respectClusterWindows`          | bool            | Respect per-cluster maintenance windows                                          |
| `spec.maintenanceWindows.skipIfOutsideWindow`            | bool            | Skip clusters outside their window                                               |
| `spec.maintenanceWindows.maxWaitTime`                    | Duration        | Maximum time to wait for window                                                  |
| `spec.maintenanceWindows.inProgressUpdateStrategy`       | string          | Strategy for in-progress updates: `continue` (default), `graceful`, `checkpoint` |
| `spec.maintenanceWindows.alerts`                         | AlertConfig     | Alert configuration for window events                                            |
| `spec.maintenanceWindows.alerts.onWindowEndDuringUpdate` | bool            | Send alert when window ends during update                                        |
| `spec.maintenanceWindows.alerts.channels`                | []AlertChannel  | Alert destinations (slack, pagerduty, email)                                     |
| `spec.clusterUpdateBehavior`                             | UpdateBehavior  | Per-cluster rollout strategy                                                     |
| `spec.analysis`                                          | AnalysisSpec    | Metrics and thresholds                                                           |
| `spec.rollback`                                          | RollbackSpec    | Automatic rollback configuration                                                 |
| `status.phase`                                           | string          | `Active`, `Paused`, `Superseded`                                                 |
| `status.currentRollout`                                  | RolloutProgress | Current rollout progress                                                         |
| `status.currentRollout.currentWave`                      | string          | Currently updating wave                                                          |
| `status.currentRollout.waveProgress`                     | []WaveStatus    | Per-wave status                                                                  |
| `status.clusters`                                        | ClusterCounts   | Cluster counts by wave                                                           |
| `status.analysis`                                        | AnalysisStatus  | Current analysis results                                                         |

### ClusterRollout (Optional - Emergency/Manual Overrides)

| Field                   | Type                   | Description                           |
| ----------------------- | ---------------------- | ------------------------------------- |
| `spec.targetBlueprint`  | BlueprintRef           | Target blueprint/revision             |
| `spec.sourceBlueprint`  | BlueprintRef           | Source blueprint (optional)           |
| `spec.strategy`         | RolloutStrategy        | Canary/BlueGreen/Rolling              |
| `spec.analysis`         | AnalysisSpec           | Metrics and thresholds                |
| `spec.rollback`         | RollbackSpec           | Rollback configuration                |
| `spec.approvals`        | []ApprovalGate         | Manual approval gates                 |
| `spec.overrideStrategy` | bool                   | Override cluster's rolloutStrategyRef |
| `status.phase`          | string                 | Current phase                         |
| `status.currentStep`    | int                    | Current rollout step                  |
| `status.clusters`       | []ClusterRolloutStatus | Per-cluster status                    |
| `status.analysis`       | AnalysisStatus         | Current analysis results              |

### ClusterRolloutCheckpoint

Created when `inProgressUpdateStrategy: checkpoint` is used and maintenance window ends during an update.

| Field                                 | Type          | Description                                            |
| ------------------------------------- | ------------- | ------------------------------------------------------ |
| `spec.clusterRef`                     | ClusterRef    | Reference to the cluster being updated                 |
| `spec.rolloutState`                   | RolloutState  | State at time of checkpoint                            |
| `spec.rolloutState.blueprintRevision` | string        | Target blueprint revision                              |
| `spec.rolloutState.previousRevision`  | string        | Previous blueprint revision                            |
| `spec.rolloutState.currentWave`       | string        | Wave being processed                                   |
| `spec.rolloutState.currentStep`       | int           | Current step number                                    |
| `spec.rolloutState.stepProgress`      | int           | Percentage of current step completed                   |
| `spec.appliedResources`               | []ResourceRef | Resources already applied                              |
| `spec.pendingResources`               | []ResourceRef | Resources pending application                          |
| `spec.createdAt`                      | Time          | When checkpoint was created                            |
| `spec.reason`                         | string        | Reason for checkpoint (e.g., `MaintenanceWindowEnded`) |
| `spec.windowDetails`                  | WindowDetails | Details about the ended maintenance window             |
| `status.phase`                        | string        | `Pending`, `Resuming`, `Resumed`, `Expired`, `Failed`  |
| `status.resumable`                    | bool          | Whether checkpoint can still be resumed                |
| `status.expiresAt`                    | Time          | When checkpoint expires (default: 3 days)              |

### ObservabilityProviderDefinition

| Field                         | Type             | Description                                                                 |
| ----------------------------- | ---------------- | --------------------------------------------------------------------------- |
| `spec.description`            | string           | Human-readable description                                                  |
| `spec.type`                   | string           | Provider type: `prometheus`, `datadog`, `newrelic`, `cloudwatch`, `webhook` |
| `spec.connectionSpec`         | ConnectionSchema | JSON schema for connection configuration                                    |
| `spec.queryTemplate`          | string           | CUE template for query execution                                            |
| `spec.responseTemplate`       | string           | CUE template for response parsing                                           |
| `spec.builtinMetrics`         | []MetricTemplate | Pre-defined metric queries                                                  |
| `spec.builtinMetrics[].name`  | string           | Metric name                                                                 |
| `spec.builtinMetrics[].query` | string           | Query in provider's query language                                          |
| `spec.builtinMetrics[].unit`  | string           | Unit of measurement                                                         |

### ObservabilityProvider

| Field                       | Type          | Description                                  |
| --------------------------- | ------------- | -------------------------------------------- |
| `spec.definitionRef`        | DefinitionRef | Reference to ObservabilityProviderDefinition |
| `spec.connection`           | Connection    | Provider-specific connection configuration   |
| `spec.connection.endpoint`  | string        | Provider endpoint URL                        |
| `spec.connection.auth`      | AuthConfig    | Authentication configuration                 |
| `spec.healthCheck.interval` | Duration      | How often to check provider health           |
| `spec.healthCheck.timeout`  | Duration      | Timeout for health checks                    |
| `status.phase`              | string        | `Ready`, `Unhealthy`, `Unknown`              |
| `status.lastCheckTime`      | Time          | Last successful connection time              |

### HealthCheck (Component-level)

| Field                    | Type        | Description                                        |
| ------------------------ | ----------- | -------------------------------------------------- |
| `name`                   | string      | Health check identifier                            |
| `type`                   | string      | Check type: `kubernetes`, `metrics`, `http`, `cue` |
| `kubernetes.resourceRef` | ResourceRef | Reference to Kubernetes resource                   |
| `kubernetes.condition`   | Condition   | Expected condition                                 |
| `metrics.providerRef`    | ProviderRef | Reference to ObservabilityProvider                 |
| `metrics.query`          | string      | Query in provider's query language                 |
| `metrics.threshold`      | Threshold   | Expected value threshold                           |
| `metrics.for`            | Duration    | Duration threshold must hold                       |
| `http.url`               | string      | HTTP endpoint to check                             |
| `http.expectedStatus`    | int         | Expected HTTP status code                          |
| `cue.healthPolicy`       | string      | CUE expression returning `isHealth: bool`          |

### HealthStatus (Status structures)

| Field                | Type          | Description                                                  |
| -------------------- | ------------- | ------------------------------------------------------------ |
| `status`             | string        | `Healthy`, `Degraded`, `Unhealthy`, `Unknown`, `Progressing` |
| `reason`             | string        | Machine-readable reason code                                 |
| `message`            | string        | Human-readable message                                       |
| `score`              | int           | Health score 0-100 (for weighted aggregation)                |
| `lastCheckTime`      | Time          | Last health evaluation time                                  |
| `checks`             | []CheckResult | Individual health check results                              |
| `checks[].name`      | string        | Check name                                                   |
| `checks[].status`    | string        | `Passing`, `Failing`, `Unknown`                              |
| `checks[].value`     | string        | Current value                                                |
| `checks[].threshold` | string        | Expected threshold                                           |
| `checks[].since`     | Time          | When current status began                                    |

### ClusterDriftReport

| Field                                  | Type             | Description                             |
| -------------------------------------- | ---------------- | --------------------------------------- |
| `spec.clusterRef`                      | ClusterRef       | Reference to the cluster being analyzed |
| `spec.blueprintRef.name`               | string           | Blueprint name for comparison           |
| `spec.blueprintRef.revision`           | string           | Blueprint revision for comparison       |
| `spec.comparisonType`                  | string           | `assigned` (default) or `what-if`       |
| `status.driftDetected`                 | bool             | Whether any drift was detected          |
| `status.lastChecked`                   | Time             | When drift was last evaluated           |
| `status.summary.totalPlanes`           | int              | Total number of planes in blueprint     |
| `status.summary.driftedPlanes`         | int              | Number of planes with drift             |
| `status.summary.totalComponents`       | int              | Total number of components              |
| `status.summary.driftedComponents`     | int              | Number of components with drift         |
| `status.planeDrifts`                   | []PlaneDrift     | Per-plane drift details                 |
| `status.planeDrifts[].planeName`       | string           | Name of the plane                       |
| `status.planeDrifts[].status`          | string           | `synced`, `drifted`, `missing`, `extra` |
| `status.planeDrifts[].componentDrifts` | []ComponentDrift | Per-component drift details             |

### ClusterDriftException

| Field                                   | Type             | Description                           |
| --------------------------------------- | ---------------- | ------------------------------------- |
| `spec.clusterRef`                       | ClusterRef       | Reference to the cluster              |
| `spec.exceptions`                       | []Exception      | List of accepted drift exceptions     |
| `spec.exceptions[].resource`            | ResourceRef      | Reference to the drifted resource     |
| `spec.exceptions[].fields`              | []FieldException | Specific fields to exclude from drift |
| `spec.exceptions[].fields[].path`       | string           | JSONPath to the field                 |
| `spec.exceptions[].fields[].reason`     | string           | Reason for accepting this drift       |
| `spec.exceptions[].fields[].approvedBy` | string           | Who approved the exception            |
| `spec.exceptions[].fields[].expiresAt`  | Time             | Optional expiration for the exception |

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

## Appendix A: Backward Compatibility with cluster-gateway

For organizations currently using `vela cluster join` and cluster-gateway secrets, backward compatibility is provided through the optional `clusterGatewayRef` field.

### Migration Options

| Current State | Migration Path | Effort |
|--------------|----------------|--------|
| Clusters joined via `vela cluster join` | Create Cluster CRD with `clusterGatewayRef` pointing to existing secret | Minimal |
| cluster-gateway secrets in `vela-system` | Use `clusterGatewayRef`, or migrate to `credential.secretRef` | Optional |
| Custom cluster-gateway configurations | Reference existing secrets OR migrate to cloud-native auth | Choose based on preference |

### Example: Referencing Existing cluster-gateway Secret

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Cluster
metadata:
  name: my-existing-cluster
spec:
  mode: connect
  # Reference existing cluster-gateway secret
  clusterGatewayRef:
    name: my-existing-cluster  # Same name as vela cluster join created
    namespace: vela-system
```

### Key Points

- **No forced migration**: Existing cluster-gateway secrets continue to work
- **Incremental adoption**: Teams can migrate clusters at their own pace
- **Future-proof**: New clusters should use `credential` options (inline, secretRef, cloudProvider)

---

## References

- [OAM Spec](https://github.com/oam-dev/spec)
- [KubeVela Application CRD](https://kubevela.io/docs/core-concepts/application)
- [Argo Rollouts](https://argoproj.github.io/argo-rollouts/)
- [Flux HelmRelease](https://fluxcd.io/flux/components/helm/)
- [Crossplane Compositions](https://docs.crossplane.io/latest/concepts/compositions/)
- [Platform Engineering](https://platformengineering.org/platform-tooling)
