# KEP: Cluster Infrastructure Abstraction

**Authors**: Anoop Gopalakrishnan
**Status**: Draft
**Created**: 2025-12-24
**Last Updated**: 2026-03-25

## Table of Contents

- [Introduction](#introduction)
  - [The Problem](#the-problem)
  - [The Orchestration Gap](#the-orchestration-gap)
  - [The Approach](#the-approach)
  - [CRDs Introduced](#crds-introduced)
- [Prerequisites and Architecture Overview](#prerequisites-and-architecture-overview)
  - [Prerequisites](#prerequisites)
  - [Architecture Overview](#architecture-overview)
  - [Bootstrap Sequence](#bootstrap-sequence)
  - [What Lives Where](#what-lives-where)
  - [Provisioning Controller Flexibility](#provisioning-controller-flexibility)
  - [VelaRuntime ClusterPlane](#velaruntime-clusterplane)
  - [Definition Distribution to Spoke Clusters](#definition-distribution-to-spoke-clusters)
- [Background](#background)
  - [The Problem with Application-Centric Only](#the-problem-with-application-centric-only)
  - [What Platform Teams Need](#what-platform-teams-need)
  - [Relationship to Existing Multi-Cluster Architecture](#relationship-to-existing-multi-cluster-architecture)
  - [Controller Ownership Model (Circular Reference Prevention)](#controller-ownership-model-circular-reference-prevention)
- [Goals](#goals)
- [Non-Goals](#non-goals)
- [Proposal](#proposal)
  - [Core CRDs](#core-crds)
    - [SpokeCluster (hub-side handle)](#spokecluster-hub-side-handle)
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
    - [Mode 2: Adopt](#mode-2-adopt---connect-to-existing-cluster)
    - [Mode 3: Connect](#mode-3-connect---manage-existing-cluster)
    - [Cluster Lifecycle Phases: infraProvisioning, clusterInit, planeProvisioning, healthValidation](#cluster-lifecycle-phases-infraprovisioning-clusterinit-planeprovisioning-healthvalidation)
    - [ClusterProviderDefinition](#clusterproviderdefinition)
  - [Definition Types](#definition-types)
    - [Definition Scope Model](#definition-scope-model)
    - [Definition Resolution for ClusterPlane](#definition-resolution-for-clusterplane)
    - [Infrastructure ComponentDefinition](#infrastructure-componentdefinition)
    - [Infrastructure TraitDefinition](#infrastructure-traitdefinition)
    - [Infrastructure PolicyDefinition](#infrastructure-policydefinition)
    - [Infrastructure WorkflowStepDefinition](#infrastructure-workflowstepdefinition)
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

### The Problem

KubeVela's `Application` CRD excels at deploying workloads, but it assumes clusters are already provisioned and configured. In practice, platform teams must manage a complex stack of infrastructure _beneath_ the application layer — networking (CNI, ingress, service mesh), security (OPA, cert-manager, network policies), observability (Prometheus, Grafana, logging), and the clusters themselves (VPC, EKS/GKE, node pools). Today, this is stitched together with a mix of Terraform, Helm charts, GitOps tools, and custom scripts, each solving one piece without a unifying model for:

1. **Composability** — Combining networking + security + observability into a coherent cluster specification
2. **Team ownership** — Letting the networking team own their layer independently from the security team
3. **Versioned rollouts** — Rolling out infrastructure changes progressively (canary, wave-based) across a fleet, not all-or-nothing
4. **Cross-cluster dependencies** — Sharing cloud resources (VPC, IAM) across clusters and wiring outputs between layers

> **Positioning:**
>
> 1. KubeVela does **not** provision clusters directly
> 2. KubeVela orchestrates **composition + ordering + rollout + ownership** across clusters and infrastructure layers
> 3. Provisioning is delegated to CAPI / Crossplane / Terraform / KRO / cloud-native operators (pluggable)

### The Orchestration Gap

Cluster provisioning tools exist. Cloud resource management tools exist. What's missing is the orchestration layer that composes them:

| Problem                                | Existing Solutions                             | This KEP's Role         |
| -------------------------------------- | ---------------------------------------------- | ----------------------- |
| **Cluster provisioning**               | CAPI, Crossplane, Terraform, KRO               | Delegate to these tools |
| **Cloud resource management**          | ACK (AWS), Config Connector (GCP), ASO (Azure) | Delegate to these tools |
| **Fleet blueprint composition**        | Gap (duct-taped together)                      | **Solve**               |
| **Team-owned infrastructure slices**   | Gap (no clear boundaries)                      | **Solve**               |
| **Rollout orchestration across fleet** | Gap (app-level only via Argo/Flagger)          | **Solve**               |
| **Cross-cluster dependencies**         | Gap (manual wiring)                            | **Solve**               |

KubeVela's job is to sequence components, gate rollouts, enforce ownership boundaries, and aggregate status across the fleet — not to replace the underlying infrastructure controllers:

| Example Definition     | Delegates To     | KubeVela Does NOT             |
| ---------------------- | ---------------- | ----------------------------- |
| Wraps tf-controller    | tf-controller    | Run Terraform or manage state |
| Wraps Crossplane XR    | Crossplane       | Call cloud APIs directly      |
| Wraps CAPI resources   | Cluster API      | Provision machines            |
| Wraps KRO instances    | KRO              | Manage resource graphs        |
| Wraps ACK resources    | ACK              | Call AWS APIs                 |
| Wraps Config Connector | Config Connector | Call GCP APIs                 |
| Wraps ASO resources    | ASO              | Call Azure APIs               |

### The Approach

This KEP extends OAM's abstraction model to cluster infrastructure by introducing a layered composition model:

- A **SpokeCluster** is the hub-side handle for a managed cluster: the fleet object an operator lists on the hub (`kubectl get spokeclusters`) and the dispatcher that hands a blueprint revision down to a spoke. The hub owns the SpokeCluster and learns spoke state by querying the spoke on demand, not by receiving pushes from it.

- A **Cluster** is the spoke-side, self-reconciling representation of that managed cluster. The spoke runs `vela-cluster-core`, which reconciles the dispatched `ClusterBlueprint` into a `Cluster` and keeps it converged locally. Reconciliation runs entirely on the spoke and never calls back to the hub, so it keeps going even while the hub is offline. Each cluster moves through lifecycle phases: shared cloud infrastructure is prepared on the hub before the cluster exists (`infraProvisioning`), the spoke installs the foundational controllers and charts the planes depend on (`clusterInit`), the spoke applies its blueprint by provisioning each `ClusterPlane` the blueprint declares (`planeProvisioning`), and an acceptance and health gate runs before the cluster is marked ready (`healthValidation`).

- A **ClusterPlane** is a composable infrastructure layer owned by a team — for example, a networking plane (Cilium, CoreDNS, ingress), a security plane (OPA, cert-manager), or an observability plane (Prometheus, Grafana). Each plane has its own components, versioning, health checks, and outputs. Planes can be scoped as `shared` (created once, consumed by many clusters) or `perCluster` (created for each cluster).

- A **ClusterBlueprint** composes multiple ClusterPlanes into a complete cluster specification. It defines which planes a cluster needs, their dependency ordering, and version constraints. Blueprints are immutable once published — changes create new revisions.

- A **ClusterRolloutStrategy** defines how blueprint changes propagate across a fleet of clusters — wave-based progression, maintenance windows, health gates, and automatic rollback on SLO breach.

Infrastructure definitions reuse the existing KubeVela definition CRDs (`ComponentDefinition`, `TraitDefinition`, etc.) with a scope annotation (`definition.oam.dev/scope: cluster`) to distinguish them from application definitions. Existing tooling, RBAC, and the definition revision system work unchanged.

### CRDs Introduced

**Core CRDs:**

| CRD                            | Description                                                                                |
| ------------------------------ | ------------------------------------------------------------------------------------------ |
| **`SpokeCluster`**             | Hub-side handle for a managed cluster: fleet listing and blueprint dispatcher, one per spoke |
| **`Cluster`**                  | Spoke-side, self-reconciling representation of a cluster, built from the dispatched blueprint |
| **`ClusterPlane`**             | A composable infrastructure layer owned by a team (e.g., networking plane, security plane) |
| **`ClusterPlaneRevision`**     | Immutable snapshot of a ClusterPlane at a specific version                                 |
| **`ClusterBlueprint`**         | A complete cluster specification composed of multiple ClusterPlanes                        |
| **`ClusterBlueprintRevision`** | Immutable snapshot of a ClusterBlueprint at a specific version                             |
| **`ClusterRolloutStrategy`**   | Shared rollout strategy that defines wave-based progression across cluster fleet           |
| **`ClusterRollout`**           | (Optional) Imperative rollout for emergency/manual overrides                               |
| **`ClusterRolloutCheckpoint`** | Checkpoint state for paused rollouts during maintenance window transitions                 |

**New Definition CRDs (Extensibility):**

| CRD                                   | Description                                                      |
| ------------------------------------- | ---------------------------------------------------------------- |
| **`ClusterProviderDefinition`**       | Defines cloud provider integration for cluster provisioning      |
| **`ObservabilityProviderDefinition`** | Defines observability provider types (Prometheus, Datadog, etc.) |

**Existing Definition CRDs Extended with Scope Annotation (`definition.oam.dev/scope: cluster`):**

Existing KubeVela definition CRDs are reused with a scope annotation (see [Definition Scope Model](#definition-scope-model)):

| Existing CRD                 | Infrastructure Role (with `scope: cluster`)                                                      |
| ---------------------------- | ------------------------------------------------------------------------------------------------ |
| **`ComponentDefinition`**    | Defines component types for ClusterPlanes (e.g., `helm-release`, `kustomization`, `k8s-objects`) |
| **`TraitDefinition`**        | Defines trait types for ClusterPlanes (e.g., `resource-quota`, `namespace-labels`)               |
| **`PolicyDefinition`**       | Defines policy types for ClusterPlanes (e.g., `apply-order`, `health-check`)                     |
| **`WorkflowStepDefinition`** | Defines workflow steps for cluster lifecycle (e.g., `apply-plane`, `validate-plane`, `suspend`)  |

**Runtime CRDs:**

| CRD                         | Description                                                       |
| --------------------------- | ----------------------------------------------------------------- |
| **`ObservabilityProvider`** | Instance of an observability provider with connection details     |
| **`ClusterDriftReport`**    | Report of detected drift between desired and actual cluster state |
| **`ClusterDriftException`** | Allowlist for expected drift that should not trigger alerts       |

---

## Prerequisites and Architecture Overview

### Prerequisites

Before using the CRDs defined in this KEP, the following must be in place:

| Prerequisite             | Description                                                                           | Responsibility                                                                           |
| ------------------------ | ------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- |
| **Hub Cluster**          | A Kubernetes cluster that serves as the management/control plane                      | Platform team bootstraps using Terraform, eksctl, cloud console, CAPI, or any other tool |
| **KubeVela Installed**   | vela-core and cluster-gateway installed on the hub cluster                            | `vela install` or Helm chart                                                             |
| **Cloud Credentials**    | Credentials for cloud providers where spoke clusters will be provisioned or connected | Stored as Secrets in hub cluster                                                         |
| **Network Connectivity** | Hub cluster must be able to reach spoke cluster API servers                           | VPC peering, transit gateway, or public endpoints                                        |

> **Important**: KubeVela does NOT bootstrap the hub cluster. The hub cluster is created and configured using your existing infrastructure-as-code tooling (Terraform, Pulumi, eksctl, gcloud, az cli, cloud console, etc.). Once the hub exists, KubeVela is installed on it, and then the CRDs in this KEP can be used to manage the fleet.

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           HUB-SPOKE ARCHITECTURE                                 │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  ┌─────────────────────────────────────────────────────────────────────────┐    │
│  │                     HUB / MANAGEMENT CLUSTER                             │    │
│  │                                                                          │    │
│  │   Bootstrapped externally via:                                           │    │
│  │   • Terraform / Pulumi / CloudFormation                                  │    │
│  │   • eksctl / gcloud / az aks create                                      │    │
│  │   • Cloud Console (AWS/GCP/Azure)                                        │    │
│  │                                                                          │    │
│  │   ┌──────────────────────────────────────────────────────────────────┐   │    │
│  │   │  KubeVela Components (installed on hub)                          │   │    │
│  │   │                                                                  │   │    │
│  │   │  • vela-core controller                                          │   │    │
│  │   │  • cluster-gateway (multi-cluster connectivity)                  │   │    │
│  │   │  • SpokeClusterController (this KEP)                             │   │    │
│  │   │  • ClusterPlaneController (this KEP)                             │   │    │
│  │   │  • ClusterBlueprintController (this KEP)                         │   │    │
│  │   │  • ClusterRolloutController (this KEP)                           │   │    │
│  │   └──────────────────────────────────────────────────────────────────┘   │    │
│  │                                                                          │    │
│  │   ┌──────────────────────────────────────────────────────────────────┐   │    │
│  │   │  CRDs on the hub (Cluster is spoke-resident)                     │   │    │
│  │   │                                                                  │   │    │
│  │   │  • SpokeCluster           (hub handle, one per spoke)            │   │    │
│  │   │  • ClusterPlane      (infrastructure layers)                     │   │    │
│  │   │  • ClusterBlueprint  (composed specifications)                   │   │    │
│  │   │  • ClusterRollout    (progressive rollout state)                 │   │    │
│  │   │  • *Definition CRDs  (extensibility)                             │   │    │
│  │   └──────────────────────────────────────────────────────────────────┘   │    │
│  │                                                                          │    │
│  │   ┌──────────────────────────────────────────────────────────────────┐   │    │
│  │   │  Optional: Provisioning Controllers (delegated)                  │   │    │
│  │   │                                                                  │   │    │
│  │   │  • Crossplane (for cloud resources)                              │   │    │
│  │   │  • Cluster API (for Kubernetes clusters)                         │   │    │
│  │   │  • tf-controller (for Terraform modules)                         │   │    │
│  │   │  • ACK / Config Connector / ASO (cloud-native)                   │   │    │
│  │   │  • KRO (Kube Resource Orchestrator)                              │   │    │
│  │   └──────────────────────────────────────────────────────────────────┘   │    │
│  │                                                                          │    │
│  └───────────────────────────────┬──────────────────────────────────────────┘    │
│                                  │                                               │
│                    Hub manages spoke clusters via                                │
│                    Kubernetes API (through cluster-gateway)                      │
│                                  │                                               │
│         ┌────────────────────────┼────────────────────────┐                     │
│         │                        │                        │                      │
│         ▼                        ▼                        ▼                      │
│  ┌─────────────┐          ┌─────────────┐          ┌─────────────┐              │
│  │   SPOKE 1   │          │   SPOKE 2   │          │   SPOKE N   │              │
│  │             │          │             │          │             │              │
│  │  prod-east  │          │  prod-west  │          │  staging    │              │
│  │             │          │             │          │             │              │
│  │ Created via:│          │ Created via:│          │ Created via:│              │
│  │ • Crossplane│          │ • CAPI      │          │ • Terraform │              │
│  │ • Terraform │          │ • Terraform │          │ • eksctl    │              │
│  │ • Manual    │          │ • KRO       │          │ • Manual    │              │
│  │             │          │             │          │             │              │
│  │ Receives:   │          │ Receives:   │          │ Receives:   │              │
│  │ • Blueprints│          │ • Blueprints│          │ • Blueprints│              │
│  │ • Planes    │          │ • Planes    │          │ • Planes    │              │
│  └─────────────┘          └─────────────┘          └─────────────┘              │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Bootstrap Sequence

The following sequence must be followed to set up the hub and begin managing spoke clusters:

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           BOOTSTRAP SEQUENCE                                     │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  STEP 1: Create Hub Cluster (EXTERNAL TO KUBEVELA)                              │
│  ─────────────────────────────────────────────────                              │
│  Platform team uses existing tooling:                                           │
│                                                                                  │
│    # AWS EKS example                                                            │
│    $ eksctl create cluster --name hub-cluster --region us-east-1                │
│                                                                                  │
│    # GKE example                                                                │
│    $ gcloud container clusters create hub-cluster --region us-central1          │
│                                                                                  │
│    # AKS example                                                                │
│    $ az aks create --name hub-cluster --resource-group platform-rg              │
│                                                                                  │
│    # Terraform example                                                          │
│    $ terraform apply -target=module.hub_cluster                                 │
│                                                                                  │
│  STEP 2: Install KubeVela on Hub                                                │
│  ───────────────────────────────                                                │
│    $ vela install                                                               │
│    # Or via Helm:                                                               │
│    $ helm install vela kubevela/vela-core -n vela-system                        │
│                                                                                  │
│  STEP 3: Install Cluster Infrastructure Controllers (This KEP)                  │
│  ─────────────────────────────────────────────────────────────                  │
│    $ vela addon enable cluster-infrastructure                                   │
│    # Installs: vela-cluster-core and its reconcilers                            │
│                                                                                  │
│  STEP 4: (Optional) Install Provisioning Controllers                            │
│  ───────────────────────────────────────────────────                            │
│    # If you want to provision spoke clusters from hub:                          │
│    $ helm install crossplane crossplane-stable/crossplane                       │
│    # Or:                                                                        │
│    $ clusterctl init --infrastructure aws                                       │
│    # Or:                                                                        │
│    $ helm install tf-controller tf-controller/tf-controller                     │
│                                                                                  │
│  STEP 5: Configure Cloud Credentials                                            │
│  ───────────────────────────────────                                            │
│    # Store credentials for spoke cluster access/provisioning                    │
│    $ kubectl create secret generic aws-credentials \                            │
│        --from-literal=AWS_ACCESS_KEY_ID=xxx \                                   │
│        --from-literal=AWS_SECRET_ACCESS_KEY=yyy \                               │
│        -n vela-system                                                           │
│                                                                                  │
│  STEP 6: Create/Connect Spoke Clusters (via this KEP's CRDs)                    │
│  ───────────────────────────────────────────────────────────                    │
│    # Now you can use SpokeCluster CRD to provision or connect spoke clusters    │
│    $ kubectl apply -f cluster-spoke-production.yaml                             │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### What Lives Where

| Resource                                       | Location                                                              | Notes                                                                                                                                                                                                                                         |
| ---------------------------------------------- | --------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| KubeVela cluster engine (`vela-cluster-core`)  | Every cluster in the fleet                                            | One engine, two roles, activated by the CRs a cluster holds. Hub-role reconciles `SpokeCluster` (blueprint dispatch and fleet) plus `ClusterPlane`, `ClusterBlueprint`, and `ClusterRolloutStrategy`. Spoke-role reconciles the dispatched `ClusterBlueprint` into the local `Cluster`, keeps it converged, and never pushes status to the hub. A cluster holding `SpokeCluster` objects acts as a hub for those children; one holding its own `Cluster` acts as a spoke; a mid-tier cluster does both. |
| KubeVela Core (`vela-core`)                    | Hub always; spokes if running Application workloads                   | The hub always runs vela-core. Spoke clusters that run KubeVela `Application` workloads also need vela-core and X-Definitions — see [VelaRuntime ClusterPlane](#velaruntime-clusterplane)                                                    |
| Provisioning controllers                                     | Hub always; spokes if Application workloads provision cloud resources | Crossplane, CAPI, tf-controller, KRO (optional, user's choice). The hub needs these for `infraProvisioning`. Spokes also need them if Application components provision cloud resources (e.g., a database via Crossplane or ACK) |
| Cloud credentials                              | Hub always; spokes if provisioning cloud resources                    | Secrets for provider access. Hub credentials are for cluster provisioning. Spoke credentials are for Application-scoped cloud resources                                                                                                       |
| ClusterPlane definitions                       | Hub cluster only                                                      | Existing `ComponentDefinition`, `TraitDefinition`, etc. with `definition.oam.dev/scope: cluster` in `vela-system`                                                                                                                             |
| X-Definitions for Application workloads        | Hub always; spokes if running Application workloads                   | `ComponentDefinition`, `TraitDefinition`, etc. needed wherever `Application` CRDs are processed — see [Definition Distribution](#definition-distribution-to-spoke-clusters)                                                                   |
| Deployed infrastructure                        | Spoke clusters                                                        | CNI, ingress, cert-manager, security policies, etc.                                                                                                                                                                                           |
| Workload Applications                          | Spoke clusters                                                        | Your actual applications (managed via KubeVela `Application` CRD)                                                                                                                                                                             |

> **The hub cluster is the single pane of glass** for managing your entire fleet. The hub holds the desired state (each `SpokeCluster` references an immutable `ClusterBlueprint` revision) and dispatches that blueprint down to the spoke. The spoke is the actor: `vela-cluster-core` reconciles the blueprint into a `Cluster` and keeps it converged. The hub reads spoke state by querying on demand, and the spoke never pushes status back, so hub downtime never stops a spoke from reconciling.
>
> **Important**: If spoke clusters run KubeVela `Application` workloads (not just raw Kubernetes resources), those spokes need the KubeVela application runtime (`vela-core` + X-Definitions) installed alongside `vela-cluster-core`. This is handled declaratively via a `vela-runtime` ClusterPlane (see [VelaRuntime ClusterPlane](#velaruntime-clusterplane)).

A `SpokeCluster` is a hub-role object, one per managed cluster. Because the topology is a tree rather than a star, a spoke that itself acts as a hub for downstream clusters holds `SpokeCluster` objects for those children. A cluster hosts `SpokeCluster` objects for whichever children it is a hub to, at any level of the tree.

### Provisioning Controller Flexibility

This KEP is **agnostic** to which provisioning controller you use. The `SpokeCluster`'s `infraProvisioning` can delegate cluster creation to any of:

| Controller                   | Use Case                            | How It Integrates                                     |
| ---------------------------- | ----------------------------------- | ----------------------------------------------------- |
| **Crossplane**               | Cloud-native resource management    | `ClusterProviderDefinition` wraps Crossplane XRs      |
| **Cluster API (CAPI)**       | Kubernetes-native cluster lifecycle | `ClusterProviderDefinition` wraps CAPI resources      |
| **tf-controller**            | Terraform modules in Kubernetes     | `ClusterProviderDefinition` wraps Terraform resources |
| **ACK/Config Connector/ASO** | Cloud-specific operators            | `ClusterProviderDefinition` wraps cloud CRDs          |
| **KRO**                      | Kube Resource Orchestrator          | `ClusterProviderDefinition` wraps KRO instances       |
| **None (Connect mode)**      | Pre-existing clusters               | No provisioning; just connect with kubeconfig         |

The platform team chooses which provisioning tools to install. KubeVela orchestrates **composition, ordering, rollout, and ownership** - it does NOT replace these tools.

### VelaRuntime ClusterPlane

If spoke clusters need to run KubeVela `Application` workloads (not just raw Kubernetes resources managed from the hub), they require the KubeVela runtime: `vela-core` controller and the relevant X-Definitions (`ComponentDefinition`, `TraitDefinition`, etc.).

Rather than requiring manual installation, this is expressed declaratively as a **ClusterPlane component** using the existing `helm` ComponentDefinition:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterPlane
metadata:
  name: vela-runtime
  namespace: vela-system
spec:
  description: "KubeVela application runtime for spoke clusters"
  components:
    - name: vela-core
      type: helm
      properties:
        chart: vela-core
        repoType: helm
        url: https://kubevela.github.io/charts
        version: "1.9.0"
        targetNamespace: vela-system
        releaseName: kubevela
        values:
          # Spoke-specific configuration: no cluster-gateway needed,
          # only the Application controller and definition loader
          multicluster:
            enabled: false
          clusterGateway:
            enabled: false
```

A blueprint for spoke clusters that run Application workloads includes this plane:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterBlueprint
metadata:
  name: production-standard
spec:
  planes:
    - name: vela-runtime # Install KubeVela runtime first
      ref:
        name: vela-runtime
        version: "1.0.0"
    - name: vela-definitions # Then push X-Definitions
      ref:
        name: vela-definitions
        version: "1.0.0"
    - name: networking
      ref:
        name: networking
        version: "2.3.1"
    # ... other planes
  workflow:
    steps:
      - name: deploy-vela-runtime
        type: deploy-plane
        properties:
          plane: vela-runtime
      - name: deploy-vela-definitions
        type: deploy-plane
        properties:
          plane: vela-definitions
        dependsOn:
          - deploy-vela-runtime # Definitions require vela-core to be running
      - name: deploy-networking
        type: deploy-plane
        properties:
          plane: networking
        dependsOn:
          - deploy-vela-definitions # Infrastructure planes can depend on runtime
```

> **Note**: Spoke clusters that only receive raw Kubernetes resources (Deployments, Services, ConfigMaps) dispatched from the hub do NOT need `vela-runtime`. The hub's vela-core renders the resources and applies them directly via cluster-gateway. The `vela-runtime` plane is only needed when spokes independently process `Application` CRDs.

### Definition Distribution to Spoke Clusters

When spoke clusters run their own vela-core (via the `vela-runtime` plane), they also need the X-Definitions (`ComponentDefinition`, `TraitDefinition`, etc.) that their Applications reference. There are two approaches:

**Approach 1: Definitions as a ClusterPlane**

Package the definitions as a Helm chart or Kustomize overlay and deploy them as a ClusterPlane component:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterPlane
metadata:
  name: vela-definitions
  namespace: vela-system
spec:
  description: "KubeVela X-Definitions for spoke Application workloads"
  components:
    - name: core-definitions
      type: helm
      properties:
        chart: vela-definitions
        repoType: helm
        url: https://kubevela.github.io/charts
        version: "1.9.0"
        targetNamespace: vela-system
        releaseName: vela-definitions
    - name: custom-definitions
      type: helm
      properties:
        chart: my-org-definitions
        repoType: helm
        url: https://charts.my-org.com
        version: "2.1.0"
        targetNamespace: vela-system
        releaseName: my-org-definitions
```

**Approach 2: Definition Module Push**

Use KubeVela's Go-based definition modules (`vela def apply-module`) to push definitions from the hub to spokes as part of a ClusterPlane workflow step:

```yaml
# A WorkflowStepDefinition (with scope: cluster) that pushes a definition module to the target cluster
- name: push-definitions
  type: push-definition-module
  properties:
    module: github.com/my-org/vela-definitions@v2.1.0
    targetNamespace: vela-system
```

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

#### Proposed Architecture: Spoke-Reconciled Cluster

**Key Architectural Decision:** the hub does not reconcile spoke infrastructure. The hub owns a `SpokeCluster` that wraps the desired `ClusterBlueprint` revision for a spoke and dispatches it. The spoke runs `vela-cluster-core`, which reconciles that blueprint into a self-sufficient `Cluster` and keeps it converged locally. The hub never receives a status push from the spoke; when it needs live state it queries the spoke on demand.

```
HUB
  SpokeCluster (core.oam.dev/v1beta1)
    spec.blueprintRef:         production-standard-v2.3.0   (desired)
    spec.credential:           how to reach the spoke
    status.dispatchedRevision: production-standard-v2.3.0   (last sent)
    status.connection:         Connected  (observed by on-demand probe)
        |
        |  dispatch blueprint revision  (push or pull)
        v
SPOKE  (runs vela-cluster-core)
    ClusterBlueprint (dispatched copy)
        |  reconciled locally
        v
    Cluster (core.oam.dev/v1beta1)  =  self-sufficient
        reconciles all planes from the blueprint
        keeps itself converged with or without the hub
        status.inventory / status.health  =  local truth

Status is read UP only on demand (the hub probes the spoke).
The spoke never pushes; hub downtime does not stop spoke reconciliation.
```

This keeps the connectivity self-sufficiency of the earlier design (no dependency on a separate `VirtualCluster` object) and extends it. The spoke is now self-sufficient for reconciliation as well, not just for connectivity: the `Cluster` is built and maintained where it runs.

#### Design Principles

1. **Self-Sufficient**: the spoke reconciles its own `Cluster` from the dispatched blueprint and stays converged independently of the hub.
2. **Central Intent, Local Truth**: the hub `SpokeCluster` carries the desired blueprint revision and dispatches it; live state is read from the spoke on demand.
3. **One-Way Reconciliation Boundary**: the spoke never writes to hub objects. Loose coupling holds in both directions, so hub downtime does not break the spoke and spoke downtime does not break the hub registry.
4. **Pluggable connectivity**: `SpokeCluster` authenticates to the spoke through a discriminated credential model keyed by auth method, with a directly supplied kubeconfig alongside cloud-native identity for each supported provider. Provider-specific settings, such as the auth mode, are scoped to their provider so invalid combinations cannot be expressed.
5. **Clear ownership boundaries**: controllers never modify `spec` fields they do not own, so no reconciliation cycle crosses the hub/spoke boundary.

**Connectivity substrate.** The hub reaches spoke API servers through cluster-gateway, the aggregated API server KubeVela already uses for multi-cluster. It is hub-initiated and agent-free, so the spoke runs nothing for connectivity and never pushes to the hub. Fleets where the hub cannot route to the spoke, for example across accounts or networks, are not addressed by cluster-gateway and would call for an agent-based substrate such as Open Cluster Management.

**Hub-to-spoke authentication.** Each cloud provider authenticates through its native workload identity and assumes a per-cluster scoped identity, so a credential reaches only the cluster it is meant for. For AWS this is EKS Pod Identity, with IRSA as an alternative; Azure and GCP use their workload-identity equivalents. Each provider's auth modes appear in the matching arm's `authMode`.

#### Controller Ownership Model (Circular Reference Prevention)

Ownership now spans two clusters. The hub owns desired state and dispatch; the spoke owns reconciliation and live status. No controller writes a `spec` it does not own, and nothing flows from spoke to hub except on an explicit hub-initiated read.

| Component                          | Owner                     | Who writes it                                   |
| ---------------------------------- | ------------------------- | ----------------------------------------------- |
| `ClusterBlueprint` (hub)           | User/GitOps               | Never modified after publish (immutable)        |
| `SpokeCluster.spec.blueprintRef`   | User/GitOps               | Never modified by controllers (desired state)   |
| `SpokeCluster.status` (dispatchedRevision, connection) | `SpokeClusterController` (hub) | Set on dispatch and on-demand probe |
| `Cluster.status` (inventory, health) | `vela-cluster-core` (spoke) | Set by local reconciliation, on the spoke     |

No edge ever writes from spoke to hub. The hub reads spoke state by querying it, which removes the cycle risk that a push-back path would create.

**The Correct Flow (No Cycle):**

1. User or GitOps sets `SpokeCluster.spec.blueprintRef` to a new revision on the hub.
2. `SpokeClusterController` asks `ClusterRolloutController` whether it may dispatch (wave ready, maintenance window open).
3. If denied, it waits and requeues. If approved, it dispatches the blueprint revision to the spoke.
4. The spoke's `vela-cluster-core` reconciles the blueprint into its `Cluster` and keeps it converged locally.
5. `SpokeClusterController` records `status.dispatchedRevision` and reads spoke health on demand for wave progression. It never writes the spoke's spec, and the spoke never writes the hub.

**Key Invariants:**

| Invariant                                | Description                                                                                                                |
| ---------------------------------------- | -------------------------------------------------------------------------------------------------------------------------- |
| **ClusterBlueprint is immutable**        | Once created, a blueprint revision never changes. New versions create new `ClusterBlueprintRevision` objects.              |
| **spec.blueprintRef is user-owned**      | Only users or GitOps set the desired revision on the hub `SpokeCluster`. Controllers never modify it.                      |
| **The spoke owns reconciliation**        | Only the spoke's `vela-cluster-core` reconciles the blueprint into the `Cluster`. The hub does not apply plane resources.  |
| **No status push-back**                  | The spoke never writes hub objects. The hub reads spoke state by querying on demand.                                       |
| **Rollout controls timing, not state**   | `ClusterRolloutController` gates WHEN the hub dispatches a new revision. It never modifies spec or blueprints.             |

#### Migration Path

| Stage                     | Hub object                            | Spoke                                  | Behavior                                                              |
| ------------------------- | ------------------------------------- | -------------------------------------- | -------------------------------------------------------------------- |
| **Stage 0** (current)     | None                                  | Manual kubeconfig                      | Current behavior, no change                                          |
| **Stage 1** (connect)     | `SpokeCluster` with `mode: connect`   | No spoke engine yet                    | Hub registers and probes the spoke; fleet listing works              |
| **Stage 2** (dispatch)    | `SpokeCluster` with `blueprintRef`    | `vela-cluster-core` installed          | Hub dispatches the blueprint; the spoke reconciles its `Cluster`     |
| **Stage 3** (provisioned) | `SpokeCluster` with `mode: provision` | `vela-cluster-core` installed at bootstrap | Hub provisions the cluster, then dispatches the blueprint to it  |

#### Controller Reconciliation

The model runs two controllers on two clusters.

**Hub `SpokeClusterController` reconcile algorithm:**

1. Establish connectivity to the spoke (using `spec.credential`).
2. If `spec.infraProvisioning.blueprintRef` is set, reconcile it on the hub against cloud APIs (VPC, IAM, DNS, and for `mode: provision` the cluster itself), consuming existing outputs when another `SpokeCluster` already reconciled the same shared blueprint. This runs before any dispatch and leaves `vela-cluster-core` on the spoke.
3. If `mode: connect` with no blueprint set, probe the spoke, record connection status, and stop.
4. If `spec.blueprintRef.revision` differs from `status.dispatchedRevision`:
   - Check rollout permission (read-only query to `ClusterRolloutStrategy`).
   - If denied: requeue and wait.
   - If approved: dispatch the spoke-reconciled blueprints (`clusterInit`, then the main blueprint) to the spoke, then record `status.dispatchedRevision`.
5. Probe the spoke on demand for `status.connection` and a health summary used for rollout progression.

**Spoke `vela-cluster-core` reconcile algorithm:**

1. Read the dispatched `ClusterBlueprint` and resolve it into planes.
2. Reconcile every plane into the local `Cluster`, in dependency order.
3. Keep the `Cluster` converged on every resync, independent of hub availability.
4. Maintain `Cluster.status.inventory` and `Cluster.status.health` as local truth.

**Key ownership boundaries:**

- The hub `SpokeClusterController` READS `spec.blueprintRef`, WRITES `SpokeCluster.status`, and dispatches the blueprint. It does not write spoke objects beyond delivering the blueprint.
- The spoke `vela-cluster-core` owns the `Cluster` and its status. It never writes hub objects.
- `ClusterRolloutController` gates dispatch timing via hub status fields only.

#### Controller Responsibilities Matrix

**Critical Design Principle:** a `ClusterPlane` is only reconciled on a spoke when a dispatched blueprint references it. Creating a `ClusterPlane` on the hub does not create infrastructure; the dispatched blueprint, reconciled by the spoke, is the trigger.

| Controller                          | Responsibilities                                                                                                                                                                                                  | Does NOT Do                                                                                          |
| ----------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| **SpokeClusterController** (hub)    | - Owns the `SpokeCluster` fleet object<br>- Reconciles `infraProvisioning` on the hub (cloud infra) before dispatch<br>- Dispatches the blueprint revision to the spoke when rollout permits<br>- Records `status.dispatchedRevision` and `status.connection`<br>- Probes the spoke on demand   | - Never reconciles spoke planes itself<br>- Never requires or receives a status push from the spoke  |
| **vela-cluster-core** (spoke)       | - Reconciles the dispatched `ClusterBlueprint` into the local `Cluster`<br>- Resolves Blueprint to Planes and applies them in order<br>- Keeps the cluster converged with or without the hub<br>- Owns `Cluster.status` as local truth | - Never writes hub objects<br>- Never pushes status to the hub                            |
| **ClusterPlaneController** (hub)    | - Creates `ClusterPlaneRevision` on publishVersion<br>- Validates inputs/outputs schema                                                                                                                            | - Does NOT create infrastructure resources<br>- Does NOT dispatch to clusters                        |
| **ClusterBlueprintController** (hub)| - Creates `ClusterBlueprintRevision` on publishVersion<br>- Validates plane composition                                                                                                                            | - Does NOT reconcile spoke infrastructure<br>- Does NOT modify SpokeCluster specs                    |
| **ClusterRolloutController** (hub)  | - Manages wave progression timing<br>- Enforces maintenance windows<br>- Gates blueprint dispatch                                                                                                                  | - Does NOT dispatch blueprints (only gates timing)<br>- Does NOT modify SpokeCluster specs           |

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

> **Hub and spoke objects.** The hub owns a `SpokeCluster` per managed cluster (the fleet handle and blueprint dispatcher). The spoke owns the `Cluster` (the self-reconciling representation built from the dispatched blueprint). The hub-side `SpokeCluster` is summarized first; the numbered CRDs below describe the spoke-side `Cluster` and the shared plane, blueprint, and rollout objects.

#### SpokeCluster (hub-side handle)

The `SpokeCluster` CRD lives on the hub, one per managed cluster. It carries the credential used to reach the spoke, the hub-reconciled `infraProvisioning` blueprint, the desired `blueprintRef` it dispatches, per-cluster `patches`, the rollout reference, and the `maintenance` windows that gate dispatch. It is what an operator lists with `kubectl get spokeclusters`. The hub `SpokeClusterController` reconciles `infraProvisioning` against cloud APIs, dispatches the referenced blueprint revision to the spoke, and records connection, dispatch, and pulled `health` on `status`; it never receives a status push from the spoke.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: SpokeCluster
metadata:
  name: production-us-east-1
  namespace: vela-system
  labels:
    environment: production
    region: us-east-1
    provider: aws
spec:
  # connect attaches to a cluster that already exists and never creates one.
  # provision creates the cluster when it is absent.
  mode: connect

  # How the hub authenticates to the spoke. A discriminated union keyed by auth
  # method; exactly one arm is set. The cloud-native arms store no static credentials.
  credential:
    type: aws                     # kubeconfig | aws | azure | gcp
    # kubeconfig: spoke kubeconfig supplied directly (k3s, kind, any existing cluster)
    # kubeconfig:
    #   secretRef: { name: production-us-east-1-kubeconfig, namespace: vela-system }
    aws:
      authMode: podIdentity       # podIdentity | irsa
      clusterName: production-us-east-1
      region: us-east-1
      roleArn: <per-cluster IAM role>
    # azure:
    #   authMode: workloadIdentity   # workloadIdentity | managedIdentity
    # gcp:
    #   authMode: workloadIdentity   # workloadIdentity

  # infraProvisioning (hub): shared cloud infrastructure the hub reconciles against
  # cloud APIs before the cluster exists, plus cluster creation on mode: provision.
  infraProvisioning:
    blueprintRef:
      name: shared-infrastructure-us-east # VPC, IAM, DNS

  # Desired blueprint revision to dispatch to the spoke (user/GitOps owned).
  blueprintRef:
    name: production-standard
    revision: production-standard-v2.3.0

  # Per-cluster overrides applied on top of the dispatched blueprint.
  patches:
    - plane: networking
      component: ingress-nginx
      properties:
        values:
          controller:
            replicaCount: 5

  # Rollout strategy that gates WHEN a new revision is dispatched.
  rolloutStrategyRef:
    name: production-rollout

  # Update windows that gate WHEN a new revision is dispatched to this spoke.
  maintenance:
    windows:
      - name: weekend-maintenance
        start: "02:00"
        end: "06:00"
        timezone: America/New_York
        days: [Sat, Sun]
    allowEmergencyUpdates: true
    enforceWindow: true

status:
  connection: Connected # observed by an on-demand probe, not pushed by the spoke
  dispatchedRevision: production-standard-v2.3.0
  clusterInfo: # summary pulled from the spoke on demand
    kubernetesVersion: v1.28.5
    nodeCount: 12
    platform: eks
    region: us-east-1
  health: # pulled from the spoke Cluster on demand while connected
    status: Healthy
    planesHealthy: 3
    planesTotal: 3
    lastPulledAt: "2024-12-24T10:00:00Z"
```

The `credential` field is a discriminated union keyed by `type`, the method the hub uses to authenticate to the spoke. `kubeconfig` is for clusters whose kubeconfig is supplied directly, which covers k3s, kind, and any cluster already reachable by a kubeconfig. The cloud-native arms (`aws`, `azure`, `gcp`) reach the cluster through the provider's workload identity and store no static credentials. Auth modes are scoped to their provider so an unrelated mode cannot attach to the wrong one: `aws` offers `podIdentity` and `irsa`, `azure` offers `workloadIdentity` and `managedIdentity`, `gcp` offers `workloadIdentity`. The cluster's flavour (eks, gke, aks, kind, k3s) is discovered and reported in status, it is not a credential type. New providers extend this set the same way the provisioning side extends `ClusterProviderDefinition`.

#### 1. Cluster

The `Cluster` CRD is the **spoke-side, self-reconciling representation** of a managed cluster. It lives on the spoke, where `vela-cluster-core` reconciles it from the dispatched `ClusterBlueprint`, and it is the local source of truth for cluster state, inventory, and applied infrastructure. The hub does not host a `Cluster`; it hosts the `SpokeCluster` summarized above.

> **Hub vs spoke fields.** Everything about how the hub reaches, schedules, or provisions a spoke (`mode`, `credential`, `infraProvisioning`, the dispatched `blueprintRef`, `patches`, `rolloutStrategyRef`, `maintenance`) lives on the hub `SpokeCluster` above. The spoke-side `Cluster` below carries only what the spoke reconciles locally: the `clusterInit`, `planeProvisioning`, and `healthValidation` phases (driven by the dispatched blueprint) plus its observed status. Cluster-level policies are part of the blueprint, not a separate `Cluster` field.

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
  # The spoke reconciles these phases locally with vela-cluster-core, from the
  # blueprint the hub dispatched (SpokeCluster.blueprintRef). The hub does not
  # reconcile them; it dispatches and then reads status by pull. Fields about
  # reaching, scheduling, or provisioning the spoke (credential, infraProvisioning,
  # patches, rolloutStrategyRef, maintenance) live on the hub SpokeCluster, not here.

  # clusterInit: the foundational layer the planes depend on.
  clusterInit:
    blueprintRef:
      name: cluster-foundation # CNI, base controllers/operators, Helm runtime, CRDs

  # planeProvisioning: the cluster planes, reconciled on top of clusterInit.
  planeProvisioning:
    blueprintRef:
      name: production-standard
      revision: production-standard-v2.3.0

  # healthValidation: acceptance and smoke checks. The verdict is read by the
  # hub on demand (pull); the spoke never pushes it up.
  healthValidation:
    blueprintRef:
      name: cluster-validation

status:
  # Local source of truth for this cluster. The hub reads it on demand; the
  # spoke never pushes status to the hub.

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

1. **Local source of truth** - The spoke `Cluster` holds the actual reconciled state of its own cluster
2. **Rich inventory** - Full component and resource inventory with versions
3. **Auto-discovery** - Cluster info, node count, versions are discovered automatically
4. **Blueprint-driven** - Phases are reconciled from the blueprint the hub dispatched; the desired `blueprintRef` lives on `SpokeCluster`
5. **Spoke-local reconciliation** - `vela-cluster-core` reconciles locally; per-cluster `patches` come from the hub `SpokeCluster`
6. **Health aggregation** - Roll-up health status from planes and components
7. **History tracking** - Full audit trail of what was applied when

#### 2. ClusterPlane

A `ClusterPlane` represents a composable infrastructure layer, typically owned by one team.

##### Reconciliation Trigger Model

**Critical:** A ClusterPlane is a **template**, not a self-reconciling resource. Creating a ClusterPlane CRD does NOT create infrastructure resources.

| Action                                        | What Happens                                                         | What Does NOT Happen           |
| --------------------------------------------- | -------------------------------------------------------------------- | ------------------------------ |
| Create ClusterPlane                           | Validates schema, creates ClusterPlaneRevision if publishVersion set | Does NOT create infrastructure |
| Update ClusterPlane                           | Re-validates, creates new revision if version bumped                 | Does NOT update infrastructure |
| Cluster references Blueprint containing Plane | **NOW infrastructure is created**                                    | —                              |
| Delete ClusterPlane                           | Blocked if consumers exist (scope=shared)                            | —                              |

**When is a ClusterPlane reconciled?**

```
ClusterPlane created → Nothing happens (it's just a template)
                       │
                       │  Later...
                       ▼
SpokeCluster created with blueprintRef → hub dispatches it; vela-cluster-core on the spoke resolves the Blueprint into planes
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
- No orphaned infrastructure (every resource tied to a SpokeCluster)
- Clear lifecycle (SpokeCluster deletion triggers cleanup)

##### GitOps Integration

The SpokeCluster CRD is designed to work seamlessly with GitOps. Since dispatch is triggered by **SpokeCluster CRD changes**, standard GitOps workflows apply:

```
Git repo (spokecluster.yaml, updated)
  → Flux/ArgoCD syncs the change to the hub API
  → SpokeClusterController detects the SpokeCluster change and dispatches the
    blueprint revision to the spoke; vela-cluster-core on the spoke reconciles.

Examples:
  • Update spec.blueprintRef.revision → hub dispatches the new revision
  • Create a SpokeCluster             → hub connects or provisions, then dispatches
  • Delete a SpokeCluster             → phased cleanup on the spoke, consumers decremented
```

**Supported GitOps Tools:**

| Tool              | Integration Pattern                                            |
| ----------------- | -------------------------------------------------------------- |
| **Flux CD**       | `Kustomization` or `HelmRelease` syncing Cluster CRDs from Git |
| **Argo CD**       | `Application` tracking Cluster manifests in Git repository     |
| **Rancher Fleet** | `GitRepo` with paths to Cluster definitions                    |
| **Jenkins X**     | Pipeline-driven `kubectl apply` of Cluster CRDs                |

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
  # Flux syncs SpokeCluster CRDs → the hub dispatches, the spoke reconciles
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
    # ArgoCD syncs SpokeCluster CRDs → the hub dispatches, the spoke reconciles
```

**Key Principle:** GitOps tools manage the **desired state** (SpokeCluster CRDs in Git); on the hub the SpokeClusterController dispatches the referenced blueprint, and the spoke's vela-cluster-core manages the **actual state** (local reconciliation). This separation lets teams use their existing GitOps workflows without modification.

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

  # Inputs consumed from other planes reconciled for the same cluster
  # (including shared planes from infraProvisioning blueprints)
  inputs:
    - name: vpcId
      fromPlane: shared-vpc-us-east-1 # Reference to a shared plane
      output: vpcId # Output name from that plane
      required: true # Fail if output not available

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
  # ClusterPlane is a template — its status reflects publishing state, not runtime.
  # Runtime state (phase, component health, outputs) lives on the spoke Cluster's
  # status.planes[], since vela-cluster-core reconciles the plane there.
  phase: Published # Draft, Published, Deprecated

  # Current published revision
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

    - name: networking-v2.3.0
      version: "2.3.0"
      created: "2024-12-20T14:30:00Z"
      createdBy: "bob@company.com"
      digest: "sha256:def456..."
      changelog: "Added external-dns component"

    - name: networking-v2.2.0
      version: "2.2.0"
      created: "2024-11-15T09:00:00Z"
      createdBy: "jane@company.com"
      digest: "sha256:ghi789..."
      changelog: "Upgraded Cilium to 1.14.x"

  # How many revisions to keep
  revisionHistoryLimit: 10

  # Which clusters are using this plane (for shared planes)
  consumers:
    count: 3
    clusters:
      - name: prod-us-east-1-a
      - name: prod-us-east-1-b
      - name: prod-us-east-1-c
    clusterDNS: "cluster.example.com"
  observedGeneration: 3
  lastUpdated: "2024-12-24T10:00:00Z"
```

##### Cloud Infrastructure as a ClusterPlane

All cluster infrastructure, including cloud resources like VPC, EKS, and node pools, is expressed as ClusterPlane components:

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
  # ClusterPlane is a template; runtime state (component health, outputs) lives on
  # each spoke Cluster's status.planes[] for the clusters using this plane.
  phase: Published
  currentRevision: aws-foundation-v1.2.0
  currentVersion: "1.2.0"
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

| Scenario                                | Result                            |
| --------------------------------------- | --------------------------------- |
| Same version + same content             | SUCCESS (idempotent, GitOps-safe) |
| Same version + different content        | REJECTED (must bump version)      |
| Delete revision referenced by blueprint | REJECTED                          |

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
  phase: Published # Draft, Published, Deprecated

  # Reference to current published revision (not embedded)
  currentRevision:
    name: networking-v2.3.1
    version: "2.3.1"
    digest: "sha256:abc123..."

  # Total revision count (for monitoring/alerting)
  revisionCount: 15

  # How many revisions to keep (GC deletes oldest beyond this)
  revisionHistoryLimit: 10

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
2. **Resolve**: For each input, use cluster-gateway connectivity (hub-initiated, managed by the hub-role `SpokeClusterController`) to reach the source cluster, read `status.outputs[output]`, cache with TTL
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

**The Critical Question:** Who "owns" a shared plane's resources? Since ClusterPlanes are only reconciled when a `SpokeCluster` references them for shared infrastructure (via `infraProvisioning`) or a spoke reconciles a dispatched blueprint, we need clear ownership semantics.

**Ownership Pattern: infraProvisioning Blueprint with Consumer Tracking**

Shared infrastructure is defined in a ClusterBlueprint and referenced via `infraProvisioning.blueprintRef` on each `SpokeCluster` that needs it. The first `SpokeCluster` to reference the blueprint triggers creation; subsequent SpokeClusters consume the existing outputs.

```
Hub cluster:
  ClusterBlueprint: shared-infrastructure-us-east
    - ClusterPlane: shared-vpc         (scope: shared)
    - ClusterPlane: shared-transit-gw  (scope: shared)
    - ClusterPlane: shared-dns         (scope: shared)

  Referenced as infraProvisioning by:
    SpokeCluster prod-us-east-1-a (mode: provision)  → first consumer, triggers creation
    SpokeCluster prod-us-east-1-b (mode: provision)  → consumes existing outputs

KEY INSIGHT: the first SpokeCluster's infraProvisioning triggers shared-plane
creation on the hub; later SpokeClusters consume the outputs without re-creating.
Shared planes are protected from deletion while any consumer exists.
```

**Example SpokeCluster with infraProvisioning:**

```yaml
apiVersion: core.oam.dev/v1beta1
kind: SpokeCluster
metadata:
  name: prod-us-east-1-a
  namespace: vela-system
  labels:
    region: us-east-1
    environment: production
spec:
  mode: provision
  credential:
    type: aws
    aws:
      authMode: podIdentity
      clusterName: prod-us-east-1-a
      region: us-east-1

  # infraProvisioning (hub): shared cloud infrastructure, reconciled once and
  # consumed by every SpokeCluster that references it.
  infraProvisioning:
    blueprintRef:
      name: shared-infrastructure-us-east

  # Blueprint dispatched to the spoke; the spoke reconciles it into its Cluster
  # (clusterInit, planeProvisioning) and runs healthValidation, read by the hub.
  blueprintRef:
    name: production-eks
```

**Why This Pattern?**

| Benefit                         | Explanation                                                                      |
| ------------------------------- | -------------------------------------------------------------------------------- |
| **Implicit Ownership**          | Consumer tracking on shared planes — no separate "owner" resource needed         |
| **Natural Deletion Protection** | Shared planes blocked from deletion while any Cluster references them            |
| **No Virtual Clusters**         | Every `SpokeCluster` represents a real cluster, no dual semantics                |
| **Explicit Lifecycle Phases**   | `infraProvisioning` → `clusterInit` → `planeProvisioning` → `healthValidation` makes ordering unambiguous |
| **Label-Based Access**          | `sharedWith.clusterSelector` on ClusterPlane controls which clusters can consume |

**Lifecycle Semantics:**

```
1. Platform team creates SpokeCluster prod-us-east-1-a with infraProvisioning: shared-infrastructure-us-east
   → SpokeClusterController reconciles the infraProvisioning blueprint on the hub
   → First consumer → creates shared planes (VPC, DNS, etc.)
   → Shared plane status.phase = Running, status.consumers.count = 1

2. Platform team creates SpokeCluster prod-us-east-1-b with the same infraProvisioning blueprint
   → SpokeClusterController sees the shared planes already reconciled
   → Consumes outputs (vpcId, subnetIds) without re-creating
   → Updates shared plane status.consumers.count = 2

3. Platform team deletes prod-us-east-1-b
   → SpokeClusterController decrements shared plane consumers
   → Shared plane resources REMAIN (still consumed by prod-us-east-1-a)

4. Platform team deletes prod-us-east-1-a (last consumer)
   → Shared plane consumers = 0 → eligible for cleanup
   → Cleanup policy determines whether shared resources are deleted or retained
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

| Resource                            | Deletion Behavior        | Blocked When                                                                                               |
| ----------------------------------- | ------------------------ | ---------------------------------------------------------------------------------------------------------- |
| **ClusterPlane (scope=shared)**     | BLOCKED if consumers > 0 | Any SpokeCluster's infraProvisioning (or a dispatched blueprint) consuming its outputs                     |
| **ClusterPlane (scope=perCluster)** | Allowed                  | Never blocked (per-cluster instances cleaned up)                                                           |
| **ClusterBlueprint**                | BLOCKED if referenced    | Any SpokeCluster references it via infraProvisioning or blueprintRef                                       |
| **ClusterBlueprintRevision**        | BLOCKED if active        | Any SpokeCluster using this specific revision                                                              |
| **SpokeCluster**                    | Allowed with cleanup     | Never blocked (phased cleanup: healthValidation, planeProvisioning, clusterInit, infraProvisioning)        |

##### ClusterPlane Deletion

Shared planes cannot be deleted while clusters are using them:

```bash
$ kubectl delete clusterplane shared-vpc-us-east-1

Error from server: admission webhook "clusterplane.validation.oam.dev" denied the request:
  Cannot delete shared ClusterPlane "shared-vpc-us-east-1"

  The following SpokeClusters reference this plane:
    - production-us-east-1-a (via blueprint: production-standard)
    - production-us-east-1-b (via blueprint: production-standard)
    - production-us-east-1-c (via blueprint: production-standard)

  To delete, first remove these SpokeClusters or update their blueprints.
  Use --force to delete anyway (DANGER: will orphan dependent infrastructure)
```

##### SpokeCluster Deletion Cascade

When a SpokeCluster is deleted, cleanup follows the reverse of the lifecycle phases:

```
SpokeCluster deletion triggered. Cleanup runs in reverse phase order:

1. healthValidation: remove validation apps and smoke tests from the spoke.
2. planeProvisioning: clean up the spoke's planes; for mode: provision, destroy
   the cloud infrastructure (Terraform/Crossplane cleanup for EKS, nodes) and
   remove perCluster plane instances and ResourceTrackers.
3. clusterInit: remove the foundational plane instances from the spoke.
4. infraProvisioning: decrement status.consumers.count on each shared plane; if
   consumers reach 0 the shared resources are eligible for cleanup (per policy),
   otherwise they remain.
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
  phase: Published
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

  # For shared planes, outputs are stored on the ClusterPlane itself since
  # there is only one materialized instance. Per-cluster planes store outputs
  # on the Cluster's status.planes[] instead.
  sharedInstance:
    phase: Running # Pending, Running, Failed
    outputs:
      vpcId: "vpc-0abc123def456"
      privateSubnets: '["subnet-1a","subnet-1b","subnet-1c"]'
      publicSubnets: '["subnet-2a","subnet-2b","subnet-2c"]'
    components:
      - name: vpc
        healthy: true
        message: "VPC created successfully"
```

**CLI Commands:**

```bash
# List shared planes and their consumers
vela plane list --scope shared

NAME                    SCOPE    CONSUMERS  VERSION
shared-vpc-us-east-1    shared   3          v2.1.0
shared-transit-gw       shared   5          v1.3.0

# Show details of a shared plane
vela plane status shared-vpc-us-east-1

SHARED PLANE: shared-vpc-us-east-1
VERSION: v2.1.0
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
| **Blueprint dispatch**            | Each hub `SpokeCluster` declares the blueprint it follows via `spec.blueprintRef`; the hub dispatches that revision to the spoke, which reconciles it locally. |
| **Never modified by controllers** | No controller (including `ClusterRolloutController`) ever modifies a `ClusterBlueprint`. Only users or GitOps automation create/update blueprints.       |

**Important**: The `spec.blueprintRef` on a `SpokeCluster` is the **desired state** owned by users/GitOps. The applied `status.blueprint` on the spoke `Cluster` is the **actual state**, reconciled by `vela-cluster-core`. This separation prevents circular references; see [Controller Ownership Model](#controller-ownership-model-circular-reference-prevention).

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
  # ClusterBlueprint is a template; runtime state lives on the spoke Cluster's status.
  phase: Published

  # Current published revision
  currentRevision: production-standard-v2.3.0
  currentVersion: "2.3.0"

  # Resolved plane revisions for this blueprint version
  resolvedPlanes:
    - name: aws-foundation
      revision: aws-foundation-v1.2.0
      version: "1.2.0"
    - name: networking
      revision: networking-v2.3.1
      version: "2.3.1"
    - name: security
      revision: security-v1.8.0
      version: "1.8.0"
    - name: observability
      revision: observability-v3.1.0
      version: "3.1.0"

  # Revision history
  revisions:
    - name: production-standard-v2.3.0
      version: "2.3.0"
      created: "2024-12-24T10:00:00Z"
      createdBy: "sre-team@company.com"
      digest: "sha256:abc123..."
      changelog: "Updated networking plane, added storage plane"
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

  # List of SpokeClusters using this blueprint (computed from SpokeCluster CRs)
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
kind: SpokeCluster
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
  phase: Published

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

**SpokeCluster ← Blueprint Relationship:**

Multiple SpokeClusters can reference the same ClusterBlueprint. The Blueprint defines WHAT (planes to deploy); each SpokeCluster declares WHICH blueprint the hub dispatches. On the spoke, vela-cluster-core reconciles the actual state.

#### 4. ClusterRollout (Optional - For Emergency/Manual Overrides)

> **Note**: With the introduction of `ClusterRolloutStrategy`, the `ClusterRollout` CRD becomes **optional** and is primarily used for:
>
> - **Emergency rollouts** that bypass normal wave progression
> - **Manual overrides** for specific clusters or cluster groups
> - **One-time operations** that don't follow the standard strategy
>
> For normal operations, SpokeClusters reference a `ClusterRolloutStrategy` via `rolloutStrategyRef`. The strategy controller automatically progresses through waves when **users or GitOps automation update `SpokeCluster.spec.blueprintRef`** to point to a new blueprint version. The `ClusterRolloutController` never modifies `spec.blueprintRef` itself; it only gates WHEN the `SpokeClusterController` dispatches the user-requested update.

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
| **WHEN** to deploy | `ClusterRolloutController` → gates `SpokeClusterController` dispatch | Wave progression, maintenance windows, health checks |
| **HOW** to deploy  | `ClusterRolloutStrategy.spec`                          | Waves, batching, pauses, approvals                   |

The `ClusterRolloutController` **never modifies** `SpokeCluster.spec.blueprintRef` or `ClusterBlueprint`. It only gates the timing of when the `SpokeClusterController` dispatches user-requested updates. See [Controller Ownership Model](#controller-ownership-model-circular-reference-prevention) for why this separation matters.

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
  # ClusterRolloutController checks SpokeCluster.status.maintenance.inWindow
  # (computed by SpokeClusterController) before permitting a dispatch
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

**Relationship: SpokeCluster → ClusterRolloutStrategy → ClusterBlueprint**

```
CLUSTER-DRIVEN ROLLOUT WITH SHARED STRATEGY

ClusterBlueprint "production-standard" (revision v2.4) = "what to deploy"
  planes: networking, security, observability

ClusterRolloutStrategy "production-rollout" = "how to roll out"
  waves: 1. canary, 2. staging, 3. non-critical, 4. critical
  analysis: error-rate < 1%, p99 < 500ms

SpokeClusters selected by the strategy (each references both):
  SpokeCluster cluster-canary  (tier: canary)    blueprintRef: production-standard,
    rolloutStrategyRef: production-rollout, maintenance: anytime
  SpokeCluster cluster-staging (tier: staging)   blueprintRef: production-standard,
    rolloutStrategyRef: production-rollout, maintenance: weekends
  SpokeCluster cluster-prod-1  (tier: critical)  blueprintRef: production-standard,
    rolloutStrategyRef: production-rollout, maintenance: Sat 2-6am

Wave order:
  WAVE 1 → cluster-canary  dispatched immediately
  WAVE 2 → cluster-staging waits 4h after canary is healthy
  WAVE 4 → cluster-prod-1  waits for approval + maintenance window
```

---

### Fleet Scale Controls

At scale, reconciling many clusters simultaneously can overwhelm the hub API server or downstream provisioning systems. This section provides guidance on scaling the controller architecture.

#### When Do You Need Scale Controls?

| Fleet Size       | Recommendation                                     |
| ---------------- | -------------------------------------------------- |
| <50 clusters     | Single controller instance is sufficient           |
| 50-200 clusters  | Add rate limiting to smooth API server load        |
| 200-500 clusters | Shard controllers (2-5 replicas)                   |
| 500+ clusters    | Sharding + status aggregation + consider multi-hub |
| 1000+ clusters   | Multi-hub architecture (regional GitOps instances) |

#### Primary Mechanism: Controller Sharding

**Sharding is the simplest and most effective approach.** Each controller instance watches a subset of clusters based on labels:

```yaml
# Deploy multiple controller replicas, each handling a partition
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cluster-controller
spec:
  replicas: 3 # 3 shards
  template:
    spec:
      containers:
        - name: controller
          args:
            - --shard-id=$(SHARD_ID)
            - --total-shards=3
          env:
            - name: SHARD_ID
              valueFrom:
                fieldRef:
                  fieldPath: metadata.labels['shard-id']
```

SpokeClusters are assigned to shards via consistent hashing or explicit labels:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: SpokeCluster
metadata:
  name: prod-us-east-1
  labels:
    cluster.oam.dev/shard: "1" # Explicit shard assignment
```

This pattern is proven at scale (ArgoCD uses `--application-controller-shard`).

#### Secondary Mechanism: Rate Limiting

For smoother API server load within each shard, configure rate limiting using standard controller-runtime options:

```yaml
# Controller configuration (ConfigMap or flags)
apiVersion: v1
kind: ConfigMap
metadata:
  name: cluster-controller-config
data:
  # Max concurrent reconciles per controller instance
  maxConcurrentReconciles: "10"

  # Rate limiter settings (token bucket)
  rateLimitQPS: "5"
  rateLimitBurst: "10"
```

#### Rollout-Level Controls

For ClusterRolloutStrategy, add scale controls to prevent thundering herd during fleet-wide updates:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterRolloutStrategy
metadata:
  name: gradual-fleet-rollout
spec:
  scaleControls:
    # Max clusters updating simultaneously across all waves
    maxConcurrentUpdates: 20

    # Rate limit: max clusters to start per minute
    maxUpdatesPerMinute: 5

    # Add jitter to spread reconciliation (reduces API spikes)
    jitterPercent: 10

  waves:
    - name: canary
      # ... wave config
```

#### Implementation Notes

These controls leverage **existing Kubernetes primitives**—no custom implementation required:

| Mechanism       | Implementation                                          |
| --------------- | ------------------------------------------------------- |
| Sharding        | Label selectors on controller watch + multiple replicas |
| Rate limiting   | client-go's `workqueue.RateLimiter` (built-in)          |
| Max concurrency | controller-runtime's `MaxConcurrentReconciles`          |
| Jitter          | Random delay before requeue (`time.Sleep` + jitter)     |

For fleets >500 clusters, consider **status aggregation** to reduce how much spoke state the hub pulls and records on each SpokeCluster:

```yaml
spec:
  statusAggregation:
    # Only write aggregated status every 30s instead of per-cluster
    aggregationInterval: 30s
    # Store detailed per-cluster status in external store
    detailedStatusRef:
      kind: ConfigMap
      name: fleet-detailed-status
```

---

### Maintenance Window Enforcement

Maintenance windows control **when** a SpokeCluster's updates are dispatched. The SpokeClusterController computes and exposes the window state (`SpokeCluster.status.maintenance.inWindow`), which the ClusterRolloutController checks before permitting a dispatch.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: SpokeCluster
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

The `Cluster` CRD supports the full cluster lifecycle, from provisioning new clusters to adopting existing ones, via three modes of operation.

#### Cluster Modes

```
CLUSTER LIFECYCLE MODES (set on SpokeCluster.spec.mode)

MODE 1: PROVISION  -  "create a new cluster from scratch"
  input: cloud credential + region + blueprint
  flow:  infraProvisioning (hub: VPC, IAM, DNS, cluster creation)
         → clusterInit + planeProvisioning (spoke reconciles the dispatched blueprint)
         → healthValidation (spoke; hub reads by pull)  → Cluster ready

MODE 2: ADOPT  -  "take over an existing cluster created elsewhere"
  input: kubeconfig or Terraform state + blueprint
  flow:  infraProvisioning (hub: IAM, RBAC, discovery / state import)
         → clusterInit + planeProvisioning (spoke)
         → healthValidation (spoke; hub reads by pull)

MODE 3: CONNECT  -  "just manage what is already in the cluster"
  input: kubeconfig (+ optional blueprint)
  flow:  healthValidation only (spoke; hub reads by pull)

infraProvisioning runs on the hub; clusterInit, planeProvisioning, and
healthValidation are reconciled by vela-cluster-core on the spoke. Shared
infraProvisioning blueprints are reconciled once and reused across
SpokeClusters (see Lifecycle Phases).
```

**Component Execution Model**

The lifecycle phase determines where each ClusterPlane's resources are reconciled. Only `infraProvisioning` runs on the hub; the rest are reconciled by `vela-cluster-core` on the spoke (see [Cluster Lifecycle Phases](#cluster-lifecycle-phases-infraprovisioning-clusterinit-planeprovisioning-healthvalidation)):

| Execution Context                         | Lifecycle Phase            | Reconciled On | Resources Land On            | Examples                                                                    |
| ----------------------------------------- | -------------------------- | ------------- | ---------------------------- | --------------------------------------------------------------------------- |
| **Shared cloud infrastructure**           | `infraProvisioning`        | Hub           | Cloud provider               | VPC, IAM roles, DNS zones                                                   |
| **Foundational layer**                    | `clusterInit`              | Spoke         | Spoke cluster                | CNI, base controllers/operators, Helm runtime, base CRDs                    |
| **Cluster planes**                        | `planeProvisioning`        | Spoke         | Spoke cluster                | Cilium, cert-manager, ingress controller, monitoring agents                 |
| **Acceptance and health**                 | `healthValidation`         | Spoke         | Spoke cluster (read by hub)  | Validation apps, smoke tests, readiness checks                              |
| **Application workloads**                 | N/A, uses Application CRD  | Spoke         | Spoke cluster                | Databases, message queues, application services                             |

- **Shared cloud infrastructure**: the `infraProvisioning` blueprint runs on the hub against cloud APIs (types like `terraform-module` or `crossplane-resource`). The spoke may not exist yet, so no spoke connectivity is needed. If another SpokeCluster references the same blueprint, its outputs are consumed without re-creation (shared plane semantics).
- **Foundational layer**: `vela-cluster-core` on the spoke reconciles `clusterInit`, the CNI, base operators, Helm runtime, and CRDs that the cluster planes depend on.
- **Cluster planes**: `vela-cluster-core` reconciles the dispatched blueprint's planes (networking, security, observability) into the local `Cluster`. For mode: provision the cluster was created during infraProvisioning on the hub. The hub only dispatches; it does not render or apply the planes.
- **Acceptance and health**: `healthValidation` runs on the spoke; the hub reads the verdict on demand (pull), never by push.
- **Application workloads**: not part of ClusterPlane. Application-scoped infrastructure (databases, caches, queues) is managed through the existing `Application` CRD using standard `ComponentDefinition` and `TraitDefinition` resources. These depend on cluster-level infrastructure being in place first.

#### Mode 1: Provision - Create New Cluster

Create a brand new cluster with minimal input. The Cluster CRD is intentionally simple - **all infrastructure configuration (VPC, EKS, node pools) is defined in ClusterPlanes within the blueprint**, not in the Cluster CRD itself.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: SpokeCluster
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
kind: SpokeCluster
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

Connect to an existing cluster and bring it under management. The SpokeClusterController manages connectivity through cluster-gateway (hub-initiated).

**Connectivity Options:**

| Option                     | Description                                | Use Case                     |
| -------------------------- | ------------------------------------------ | ---------------------------- |
| `credential` (inline)      | Credentials directly in spec               | Simple, self-contained       |
| `credential.secretRef`     | Reference to kubeconfig secret             | Standard Kubernetes pattern  |
| `credential.cloudProvider` | Cloud-native auth (IAM, workload identity) | Production cloud deployments |

**Option 1: Inline Credentials:**

```yaml
apiVersion: core.oam.dev/v1beta1
kind: SpokeCluster
metadata:
  name: new-partner-cluster
spec:
  mode: adopt

  # Inline credential - controller manages connectivity directly
  credential:
    type: X509 # X509, ServiceAccountToken, Bearer
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
kind: SpokeCluster
metadata:
  name: production-us-east-1
spec:
  mode: adopt

  # Reference any kubeconfig secret
  credential:
    secretRef:
      name: prod-cluster-kubeconfig
      namespace: vela-system
      key: kubeconfig # Optional, defaults to "kubeconfig"

  blueprintRef:
    name: production-standard
```

**Option 3: Cloud Provider Native Auth (Recommended for cloud):**

```yaml
apiVersion: core.oam.dev/v1beta1
kind: SpokeCluster
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
      # SpokeClusterController assumes the per-cluster IAM role (EKS Pod Identity)

  blueprintRef:
    name: production-standard
```

**Step-by-Step Adoption Workflow:**

```yaml
# Step 1: Create Cluster CRD with credentials (no CLI needed)
apiVersion: core.oam.dev/v1beta1
kind: SpokeCluster
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
kind: SpokeCluster
metadata:
  name: legacy-production
spec:
  mode: adopt
  credential:
    secretRef:
      name: legacy-production-kubeconfig
  adoption:
    existingResources:
      mode: reconcile # Now actually reconcile
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

Connect to an existing cluster without adopting infrastructure management. Uses the same connectivity options as Mode 2. connect asserts that the cluster already exists and attaches to it; it never creates a cluster. Creating a cluster when it is absent is the role of provision. This makes `mode` the seam between attaching and provisioning.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: SpokeCluster
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

#### Cluster Lifecycle Phases: infraProvisioning, clusterInit, planeProvisioning, healthValidation

A managed cluster moves through these lifecycle phases. `infraProvisioning` runs on the hub and prepares shared cloud infrastructure (VPC, IAM, DNS) before the cluster exists, and ensures `vela-cluster-core` is running on the spoke. `clusterInit` runs on the spoke: `vela-cluster-core` installs the foundational layer the planes depend on, such as Kubernetes controllers, operators, Helm charts, and CRDs. `planeProvisioning` also runs on the spoke: once the blueprint is dispatched, `vela-cluster-core` reconciles every plane into the local `Cluster` on top of that foundation and keeps it converged. `healthValidation` gates readiness with acceptance and smoke checks; the hub reads the verdict by probing the spoke on demand.

```yaml
# Hub: the SpokeCluster owns the hub-reconciled infraProvisioning and dispatches
# the blueprint. It does not carry the spoke-reconciled phases.
apiVersion: core.oam.dev/v1beta1
kind: SpokeCluster
metadata:
  name: prod-us-east-1-a
  namespace: vela-system
  labels:
    region: us-east-1
    environment: production
spec:
  mode: provision
  credential:
    type: aws
    aws:
      authMode: podIdentity
      clusterName: prod-us-east-1-a
      region: us-east-1

  # infraProvisioning (hub): shared cloud infrastructure and cluster creation,
  # reconciled on the hub (shared with other SpokeClusters via scope: shared).
  infraProvisioning:
    blueprintRef:
      name: shared-infrastructure-us-east # VPC, IAM, DNS

  # Blueprint dispatched to the spoke; the spoke reconciles it locally.
  blueprintRef:
    name: production-eks
---
# Spoke: vela-cluster-core reconciles these phases locally from the dispatched
# blueprint. The hub reads their status by pull; it is never pushed up.
apiVersion: core.oam.dev/v1beta1
kind: Cluster
metadata:
  name: prod-us-east-1-a
  namespace: vela-system
spec:
  clusterInit: # foundational layer the planes depend on
    blueprintRef:
      name: cluster-foundation # CNI, base controllers/operators, Helm runtime, CRDs
  planeProvisioning: # the cluster planes, on top of clusterInit
    blueprintRef:
      name: production-eks # control plane, node pools, Cilium, cert-manager, monitoring
  healthValidation: # acceptance and smoke checks; verdict read by the hub via pull
    blueprintRef:
      name: cluster-validation
```

**Lifecycle Phases:**

```
infraProvisioning  (Hub)
  - Prepares shared cloud infrastructure (VPC, IAM, DNS) against cloud APIs.
  - Runs on the hub; no spoke exists yet. Ends with vela-cluster-core running
    on the spoke so the spoke can reconcile the steps that follow.
  - If the blueprint is already reconciled by another SpokeCluster, it is a
    no-op and the existing outputs are consumed (scope: shared semantics).

clusterInit  (Spoke)
  - vela-cluster-core installs the foundational layer the planes depend on:
    Kubernetes controllers, operators, Helm charts, and base CRDs.
  - Runs on the spoke and is reconciled locally, like planeProvisioning.
  - Must converge before planeProvisioning, which builds on this foundation.

planeProvisioning  (Spoke)
  - The hub dispatches blueprintRef to the spoke.
  - vela-cluster-core reconciles every plane into the local Cluster and keeps
    it converged. Consumes infraProvisioning outputs (vpcId, subnetIds, ...).
  - For mode: provision the cluster was already created during infraProvisioning
    on the hub; planeProvisioning only reconciles the planes onto it.

healthValidation  (Spoke; hub reads the verdict by pull)
  - vela-cluster-core runs acceptance apps, smoke tests, and readiness checks on
    the spoke; the hub reads the health verdict on demand and never receives a push.

For mode: adopt / connect
  - infraProvisioning can prepare pre-adoption setup (IAM roles, RBAC) on the hub.
  - clusterInit and blueprintRef are dispatched for the spoke to reconcile.
  - healthValidation runs after the spoke converges.
  - Connectivity is available from the start (credentials provided).
```

**Shared infraProvisioning Blueprint Semantics:**

When multiple SpokeClusters reference the same `infraProvisioning.blueprintRef`, the shared plane semantics apply automatically:

```
spoke-a (infraProvisioning: shared-infrastructure-us-east)
  -> First consumer -> reconciles blueprint -> creates VPC -> stores outputs

spoke-b (infraProvisioning: shared-infrastructure-us-east)
  -> Blueprint already reconciled -> consumes outputs -> no-op

spoke-b deleted
  -> infraProvisioning blueprint still has consumer (spoke-a) -> no-op

spoke-a deleted
  -> infraProvisioning blueprint has no consumers -> eligible for cleanup
```

This uses the same `scope: shared` mechanism on ClusterPlanes within the blueprint. The hub tracks consumers via `status.consumers` on shared planes.

**Deletion Order:**

When a SpokeCluster is deleted, phases are cleaned up in reverse order:

1. **healthValidation** resources removed.
2. **planeProvisioning** resources cleaned up on the spoke (the cluster is deprovisioned if `mode: provision`).
3. **clusterInit** foundational resources removed from the spoke.
4. **infraProvisioning** consumer count decremented; shared resources remain if other consumers exist.

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
│  1. SpokeClusterController obtains kubeconfig                               │
│  2. Updates SpokeCluster status with connection info                        │
│  3. Dispatches the blueprint; the spoke reconciles it                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### Cluster Decommissioning Workflow

Safe, reversible cluster removal with rollback checkpoints. Decommissioning is a first-class lifecycle operation.

**Phase Progression:**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      DECOMMISSIONING PHASES                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   ACTIVE ──► CORDONED ──► DRAINING ──► SNAPSHOTTED ──► DEPROVISIONED       │
│                 │            │             │                │               │
│                 │            │             │                └─ Infra        │
│                 │            │             │                   deleted      │
│                 │            │             └─ State backed up               │
│                 │            └─ Workloads evicted                           │
│                 └─ New deployments blocked                                  │
│                                                                             │
│   ◄──────────── ROLLBACK POSSIBLE ─────────────►│◄───── NO ROLLBACK ──►    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

| Phase            | Actions                                                        | Rollback? | Gate                         |
| ---------------- | -------------------------------------------------------------- | --------- | ---------------------------- |
| `Cordoned`       | Block new Applications, mark cluster unhealthy for scheduling  | ✅ Yes    | Automatic or manual approval |
| `Draining`       | Evict workloads respecting PDBs, wait for graceful termination | ✅ Yes    | All pods evicted or timeout  |
| `Snapshotted`    | Create ClusterBlueprint snapshot, backup etcd/state            | ✅ Yes    | Snapshot verified            |
| `Deprovisioning` | Trigger infrastructure deletion via provisioning backend       | ❌ No     | Manual approval required     |
| `Decommissioned` | Remove from fleet inventory, cleanup cross-cluster references  | ❌ No     | Infrastructure deleted       |

**Cluster Decommission Spec:**

```yaml
apiVersion: core.oam.dev/v1beta1
kind: SpokeCluster
metadata:
  name: prod-us-east-1
spec:
  decommission:
    enabled: true
    strategy: graceful # graceful | force | drain-only

    # Drain configuration
    drainTimeout: 1h
    respectPodDisruptionBudgets: true
    deleteLocalData: false # Evict pods with emptyDir

    # Snapshot before destruction
    snapshotBefore: true
    snapshotStorageRef:
      name: cluster-snapshots-bucket

    # Cross-cluster dependency handling
    crossClusterDependencies:
      action: block # block | warn | ignore (default: block)
      # block = fail if other clusters have crossClusterInputs referencing this cluster
      # warn  = proceed but emit warning events
      # ignore = proceed without checking

    # Orphaned resource handling
    orphanedResourceCheck:
      enabled: true
      resourceTypes:
        - LoadBalancer
        - PersistentVolume
        - DNSRecord

status:
  decommission:
    phase: Draining
    startedAt: "2024-12-28T10:00:00Z"

    # Rollback checkpoint (can restore to this phase)
    checkpoint: Cordoned
    checkpointCreatedAt: "2024-12-28T10:05:00Z"

    # Drain progress
    drainProgress:
      total: 150
      evicted: 120
      pending: 30
      blocked: 0

    # Blockers preventing progress
    blockers: []
    # Example blockers:
    # - type: PDB
    #   name: budget-service
    #   namespace: production
    #   message: "PDB blocks eviction, minAvailable=3, current=3"
    # - type: CrossClusterDependency
    #   cluster: prod-us-west-2
    #   resource: "crossClusterInputs.networking.vpcPeeringId"

    # Orphaned resource warnings
    orphanedResources:
      - type: LoadBalancer
        name: ingress-nginx
        cloudId: "arn:aws:elasticloadbalancing:..."
        action: "Will be deleted with cluster"
```

**Decommission Strategies:**

| Strategy     | Behavior                                                | Use Case                                   |
| ------------ | ------------------------------------------------------- | ------------------------------------------ |
| `graceful`   | Full phase progression with gates                       | Normal planned decommission                |
| `force`      | Skip drain/snapshot, proceed directly to deprovisioning | Emergency (security breach, cost)          |
| `drain-only` | Cordon and drain but don't delete infrastructure        | Temporary maintenance, cluster replacement |

**Rollback:**

```yaml
# To rollback from Draining to Active:
spec:
  decommission:
    enabled: false # Disabling triggers rollback
    rollbackTo: Active # Or "Cordoned" to stay cordoned
```

**Cross-Cluster Dependency Check:**

Before decommissioning, the controller checks if other clusters reference this cluster:

```yaml
# Example: prod-us-west-2 depends on prod-us-east-1
apiVersion: core.oam.dev/v1beta1
kind: SpokeCluster
metadata:
  name: prod-us-west-2
spec:
  crossClusterInputs:
    vpcPeering:
      sourceCluster: prod-us-east-1 # ← Dependency
      outputPath: "planes.networking.outputs.vpcId"
```

With `crossClusterDependencies.action: block` (default), decommissioning prod-us-east-1 fails:

```bash
$ kubectl patch cluster prod-us-east-1 --type=merge -p '{"spec":{"decommission":{"enabled":true}}}'

Error: Cannot decommission cluster "prod-us-east-1"

Dependent clusters found:
  - prod-us-west-2 (crossClusterInputs.vpcPeering.sourceCluster)

To proceed:
  1. Migrate dependencies first, OR
  2. Set spec.decommission.crossClusterDependencies.action: warn
```

**CLI Commands:**

```bash
# Initiate graceful decommission
vela cluster decommission prod-us-east-1 --strategy graceful

# Check decommission status
vela cluster decommission-status prod-us-east-1
Phase: Draining (45m elapsed)
Progress: 120/150 pods evicted
Blockers: None
Checkpoint: Cordoned (rollback available)

# Rollback to active
vela cluster decommission-rollback prod-us-east-1 --to Active

# Force decommission (emergency)
vela cluster decommission prod-us-east-1 --strategy force --confirm
```

**Garbage Collection Policy:**

When a Cluster is deleted, ClusterPlanes and their components must be cleaned up in the correct order to respect dependencies. This mirrors KubeVela's existing garbage collection for Applications but adapted for infrastructure.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: SpokeCluster
metadata:
  name: prod-us-east-1
spec:
  garbageCollection:
    # Deletion policy
    policy: cascading # cascading | orphan | keep-snapshots
    # cascading      = Delete all planes and components (default)
    # orphan         = Delete Cluster CR but leave planes running
    # keep-snapshots = Delete everything except snapshots/backups

    # Plane deletion order
    planeDeletionOrder: dependency-aware # dependency-aware | parallel
    # dependency-aware = Delete in reverse order of dependsOn (safe, slower)
    # parallel         = Delete all planes simultaneously (fast, risky)

    # Component deletion within planes
    componentDeletionOrder: dependency-aware # dependency-aware | parallel

    # Timeout for each plane deletion
    planeGracePeriod: 10m

    # Cloud resource handling
    orphanedCloudResources:
      action: delete # delete | detach | warn-only
      # delete     = Delete cloud resources (LoadBalancers, EBS, etc.)
      # detach     = Remove from KubeVela tracking but leave in cloud
      # warn-only  = Emit events but don't block deletion

      # Resource-specific exemptions
      exemptions:
        - type: PersistentVolume
          action: detach # Keep PVs for data safety
          retentionDays: 30

        - type: Snapshot
          action: keep # Always keep snapshots
          retentionDays: 90

        - type: LoadBalancer
          action: delete # Clean up LBs to avoid cost

    # Finalizer behavior
    finalizers:
      - cluster.oam.dev/plane-cleanup
      - cluster.oam.dev/cross-cluster-ref-cleanup
      - cluster.oam.dev/orphaned-resource-check
```

**Deletion Order Example:**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         GARBAGE COLLECTION ORDER                            │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. VALIDATE DELETION                                                       │
│     ✓ Check cross-cluster dependencies (fail if consumers exist)            │
│     ✓ Check shared plane consumers (infraProvisioning blueprint references) │
│                                                                             │
│  2. DELETE PLANES (reverse dependency order)                                │
│     ┌──────────────────────────────────────────────────────────────┐        │
│     │  Plane: observability (no dependencies)                      │        │
│     │    ├─ Component: prometheus (deleted)                        │        │
│     │    └─ Component: loki (deleted)                              │        │
│     └──────────────────────────────────────────────────────────────┘        │
│                              ▼                                              │
│     ┌──────────────────────────────────────────────────────────────┐        │
│     │  Plane: security (depends on: aws-foundation)                │        │
│     │    ├─ Component: cert-manager (deleted)                      │        │
│     │    └─ Component: gatekeeper (deleted)                        │        │
│     └──────────────────────────────────────────────────────────────┘        │
│                              ▼                                              │
│     ┌──────────────────────────────────────────────────────────────┐        │
│     │  Plane: networking (depends on: aws-foundation)              │        │
│     │    ├─ Component: aws-load-balancer-controller (deleted)      │        │
│     │    ├─ Component: external-dns (deleted)                      │        │
│     │    └─ Component: cilium (deleted)                            │        │
│     └──────────────────────────────────────────────────────────────┘        │
│                              ▼                                              │
│     ┌──────────────────────────────────────────────────────────────┐        │
│     │  Plane: aws-foundation (last to delete)                      │        │
│     │    ├─ Component: node-pool-workload (deleted)                │        │
│     │    ├─ Component: node-pool-system (deleted)                  │        │
│     │    ├─ Component: eks-cluster (deleted)                       │        │
│     │    └─ Component: vpc (deleted last)                          │        │
│     └──────────────────────────────────────────────────────────────┘        │
│                                                                             │
│  3. CLEAN UP ORPHANED CLOUD RESOURCES                                       │
│     ✓ Scan for LoadBalancers, EBS volumes, DNS records                      │
│     ✓ Delete according to orphanedCloudResources policy                     │
│                                                                             │
│  4. REMOVE FINALIZERS AND DELETE SPOKECLUSTER CR                            │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Handling Cross-Cluster References:**

If another cluster references this cluster via `crossClusterInputs`, the finalizer blocks deletion:

```yaml
status:
  conditions:
    - type: ReadyToDelete
      status: "False"
      reason: CrossClusterDependencies
      message: |
        Cannot delete cluster prod-us-east-1.

        Dependent clusters:
          - prod-us-west-2 references planes.networking.outputs.vpcId

        Action required:
          1. Update prod-us-west-2 to remove crossClusterInputs, OR
          2. Delete prod-us-west-2 first, OR
          3. Set spec.garbageCollection.policy: orphan (leaves resources)
```

**Shared Plane Protection:**

Shared ClusterPlanes cannot be deleted while clusters consume their outputs:

```bash
$ kubectl delete clusterplane shared-vpc-us-east-1

Error from server (Forbidden): admission webhook denied the request:
  Cannot delete shared ClusterPlane "shared-vpc-us-east-1"

  3 SpokeClusters are consuming this plane's outputs:
    - prod-us-east-1-a (via infraProvisioning)
    - prod-us-east-1-b (via infraProvisioning)
    - prod-us-east-1-c (via infraProvisioning)

  This is protected by finalizer: cluster.oam.dev/shared-plane-consumer-check

  To delete:
    1. Delete or migrate consumer clusters first
    2. Or use spec.garbageCollection.policy: orphan (unsafe)
```

**Orphaned Resource Detection:**

After plane deletion, scan for orphaned cloud resources that weren't properly cleaned up:

```yaml
status:
  garbageCollection:
    phase: CleaningOrphanedResources
    orphanedResources:
      - type: LoadBalancer
        name: a1b2c3d4e5f6.elb.amazonaws.com
        cloudId: "arn:aws:elasticloadbalancing:us-east-1:..."
        action: deleting
        reason: "Created by ingress-nginx, not cleaned up by controller"

      - type: EBSVolume
        cloudId: "vol-0abc123def456"
        action: detached
        reason: "PersistentVolume exemption policy"

      - type: Snapshot
        cloudId: "snap-0xyz789"
        action: kept
        retentionUntil: "2025-03-28T10:00:00Z"
```

**CLI Support:**

```bash
# Check what will be deleted
vela cluster delete prod-us-east-1 --dry-run
Deletion Plan:
  Planes (reverse dependency order):
    1. observability (2 components)
    2. security (2 components)
    3. networking (3 components)
    4. aws-foundation (4 components)

  Orphaned Resources:
    - 2 LoadBalancers (will be deleted)
    - 5 PersistentVolumes (will be detached, kept for 30d)
    - 1 Snapshot (will be kept for 90d)

# Delete with custom policy
vela cluster delete prod-us-east-1 --gc-policy orphan
Warning: Cluster CR will be deleted but planes will remain running.
         You must clean up cloud resources manually.

# Force delete (skip finalizers - dangerous)
vela cluster delete prod-us-east-1 --force --confirm
```

**Cluster Replacement Pattern (Blue/Green):**

For in-place cluster upgrades, use decommissioning with a replacement cluster:

```yaml
# Step 1: Create replacement cluster with same blueprint
apiVersion: core.oam.dev/v1beta1
kind: SpokeCluster
metadata:
  name: prod-us-east-1-v2
spec:
  blueprintRef:
    name: production-standard
  # ... same config as original

---
# Step 2: After v2 is healthy, decommission original
# The rollout strategy can automate this via cluster selectors
apiVersion: core.oam.dev/v1beta1
kind: ClusterRolloutStrategy
metadata:
  name: blue-green-replacement
spec:
  waves:
    - name: activate-new
      clusterSelector:
        matchLabels:
          cluster.oam.dev/name: prod-us-east-1-v2
      # Wait for new cluster to be fully healthy

    - name: decommission-old
      clusterSelector:
        matchLabels:
          cluster.oam.dev/name: prod-us-east-1
      decommission:
        enabled: true
        strategy: graceful
      waitFor:
        previousWaveHealthy: true
```

---

### Definition Types

#### Definition Scope Model

The existing KubeVela definition CRDs are extended with a **scope annotation** to distinguish infrastructure definitions from application definitions. The definition system remains a single, unified type system — the scope annotation partitions it by usage context.

**Label and Annotation:** `definition.oam.dev/scope`

| Value         | Meaning                                                                                   |
| ------------- | ----------------------------------------------------------------------------------------- |
| `application` | (Default, implied if absent) Used by the Application controller for app-level composition |
| `cluster`     | Used by the ClusterPlane controller for infrastructure-level composition                  |
| `both`        | Available to both Application and ClusterPlane controllers                                |

The value is set as both a **label** (for efficient list/watch filtering via `labelSelector`) and an **annotation** (for human-readable display via `kubectl describe`):

```yaml
metadata:
  labels:
    definition.oam.dev/scope: cluster
  annotations:
    definition.oam.dev/scope: cluster
    definition.oam.dev/description: "Deploy a Helm chart as a ClusterPlane component"
```

**Why reuse existing definition CRDs:**

1. **Same rendering engine.** Infrastructure definitions use the same `spec.schematic.cue.template` and `spec.workload` fields as application definitions. The CUE evaluator, parameter schema generation, and `defkit` SDK all work unchanged.
2. **Existing tooling works.** `vela def list`, `vela def get`, `DefinitionRevision` tracking, and the `defkit` Go SDK all work without modification. A new CRD would require duplicating all of this.
3. **Consistent with existing conventions.** KubeVela already treats definitions in `vela-system` as globally available via the two-level namespace lookup (`GetDefinition()` in `pkg/oam/util/helper.go`). Infrastructure definitions follow this same pattern.
4. **Naming convention prevents collisions.** Infrastructure definitions use distinct names (e.g., `helm-release` for Flux HelmRelease rendering vs. the app-level `helm` for inline Helm chart rendering). The scope annotation adds a second layer of safety.

**App-specific fields on TraitDefinition:**

`TraitDefinitionSpec` includes fields that are application-specific (`RevisionEnabled`, `WorkloadRefPath`, `PodDisruptive`, `ManageWorkload`, `ControlPlaneOnly`, `Stage`). For `scope: cluster` traits:

- These fields are **ignored** by the ClusterPlane controller.
- A **validation webhook** rejects `scope: cluster` TraitDefinitions that set `PodDisruptive`, `ManageWorkload`, `ControlPlaneOnly`, or `Stage` to non-zero values, preventing confusion.
- `AppliesToWorkloads` and `ConflictsWith` remain meaningful and are respected by the ClusterPlane controller.

#### Definition Resolution for ClusterPlane

The existing `GetDefinition()` function resolves definitions with a two-level namespace fallback (application namespace → `vela-system`). This is **unchanged** for the Application controller.

The ClusterPlane controller uses a new helper, `GetInfraDefinition()`, which:

1. Searches **only** in `vela-system` (infrastructure definitions are always global).
2. Filters by label `definition.oam.dev/scope` matching `cluster` or `both`.
3. Returns an error if the definition exists but has `scope: application` (explicit rejection, not a silent miss).

```go
// GetInfraDefinition retrieves a definition scoped for cluster infrastructure.
// It only looks in vela-system and requires scope=cluster or scope=both.
func GetInfraDefinition(ctx context.Context, cli client.Reader, definition client.Object, definitionName string) error {
    if err := cli.Get(ctx, types.NamespacedName{
        Name:      definitionName,
        Namespace: oam.SystemDefinitionNamespace,
    }, definition); err != nil {
        return err
    }
    scope := definition.GetLabels()["definition.oam.dev/scope"]
    if scope != "cluster" && scope != "both" {
        return fmt.Errorf("definition %q exists but has scope=%q, not usable for ClusterPlane", definitionName, scope)
    }
    return nil
}
```

**DefinitionRevision:** The existing `DefinitionRevision` CRD (namespaced, in `vela-system`) is reused without changes. Revisions of `scope: cluster` definitions are tracked identically to application definitions.

The following subsections show the built-in infrastructure definitions that ship with the `cluster-infrastructure` addon:

#### Infrastructure ComponentDefinition

Defines component types available in ClusterPlanes. Uses the existing `ComponentDefinition` CRD with `scope: cluster`.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: helm-release
  namespace: vela-system
  labels:
    definition.oam.dev/scope: cluster
  annotations:
    definition.oam.dev/scope: cluster
    definition.oam.dev/description: "Deploy a Helm chart as a ClusterPlane component"
spec:
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

#### Infrastructure TraitDefinition

Defines traits that can be applied to ClusterPlane components. Uses the existing `TraitDefinition` CRD with `scope: cluster`.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: resource-quota
  namespace: vela-system
  labels:
    definition.oam.dev/scope: cluster
  annotations:
    definition.oam.dev/scope: cluster
    definition.oam.dev/description: "Apply resource quotas to plane component namespace"
spec:
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

#### Infrastructure PolicyDefinition

Defines policies applicable at the plane or blueprint level. Uses the existing `PolicyDefinition` CRD with `scope: cluster`.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: PolicyDefinition
metadata:
  name: apply-order
  namespace: vela-system
  labels:
    definition.oam.dev/scope: cluster
  annotations:
    definition.oam.dev/scope: cluster
    definition.oam.dev/description: "Define component apply order within a plane"
spec:
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

#### Infrastructure WorkflowStepDefinition

Defines workflow steps for cluster/plane operations. Uses the existing `WorkflowStepDefinition` CRD with `scope: cluster`.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: WorkflowStepDefinition
metadata:
  name: apply-plane
  namespace: vela-system
  labels:
    definition.oam.dev/scope: cluster
  annotations:
    definition.oam.dev/scope: cluster
    definition.oam.dev/description: "Apply a ClusterPlane to target clusters"
spec:
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
    resources: ["componentdefinitions", "traitdefinitions"] # scope: cluster definitions in vela-system
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

A critical requirement is understanding the health of clusters at multiple levels, from the overall cluster down to individual resources. Health is local truth on the spoke `Cluster`, aggregated there by `vela-cluster-core`; the hub reads it on demand (pull) and records a snapshot on `SpokeCluster.status.health`, so the spoke never pushes health to the hub. The health model must support:

1. **Hierarchical health aggregation** - the spoke `Cluster` rolls health up from planes, planes from components, components from resources
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

#### Health Status on the spoke Cluster CRD

The spoke `Cluster` status is the local source of truth for health at all levels. The hub reads it on demand and mirrors a summary onto `SpokeCluster.status.health`; the example below is the spoke-side `Cluster`:

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

The `ClusterRolloutStrategy` uses health for wave progression. It reads the health the hub pulled onto `SpokeCluster.status.health` (the spoke is never asked to push):

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

The `--blueprint` flag supports **what-if analysis**, comparing a cluster against any blueprint, not just its assigned one:

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
  # References the hub SpokeCluster; drift is detected on the spoke, read by the hub.
  clusterRef:
    kind: SpokeCluster
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

Drift detection results are persisted hub-side as `ClusterDriftReport` resources, derived from the spoke `Cluster`'s local `status.drift`:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterDriftReport
metadata:
  name: production-us-east-1-drift-2024-12-24
  namespace: platform-clusters
spec:
  # The hub SpokeCluster this report is about (drift observed on its spoke).
  clusterRef:
    kind: SpokeCluster
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
kind: SpokeCluster
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
# 2. The hub SpokeClusterController dispatches the blueprint; vela-cluster-core on the spoke reconciles it
# (the spoke Cluster's status.blueprint is the applied state; SpokeCluster.spec.blueprintRef is desired)

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

#### Rollback Granularity: Blueprint vs Plane

Rollout and rollback operate at the **blueprint level**, not individual planes. This is deliberate — a `ClusterBlueprintRevision` captures the exact combination of plane revisions that were tested and validated together.

**For rollback to a known-good state**, roll back the entire blueprint to a previous revision. This restores the exact combination of plane versions that was previously healthy:

```bash
# Rollback the entire blueprint to a known-good revision
$ vela blueprint rollback production-standard --to-revision production-standard-v2.2.0
```

**When only one plane is at fault** (e.g., networking team's update is bad but security plane is fine), the recommended approach is to **roll forward** with a new blueprint that pins only the problematic plane to its older revision:

```yaml
# Create a new blueprint version that reverts only the networking plane
apiVersion: core.oam.dev/v1beta1
kind: ClusterBlueprint
metadata:
  name: production-standard
  annotations:
    core.oam.dev/publishVersion: "2.4.1" # New version, not a rollback
spec:
  planes:
    - name: networking
      ref:
        name: networking
        version: "2.3.0" # Reverted to older version
    - name: security
      ref:
        name: security
        version: "1.8.0" # Unchanged — keep the good version
    - name: observability
      ref:
        name: observability
        version: "3.1.0" # Unchanged
```

This roll-forward approach is preferred because:

1. **Auditability** — every change is a new revision, not a destructive rollback
2. **Team responsibility** — each team is responsible for the health of their plane and can publish a fixed version independently
3. **No side effects** — rolling back an entire blueprint might undo good changes from other teams that happened to be in the same revision

Each team owns their plane's revision history and can publish fixes independently. The blueprint owner (typically SRE) composes the known-good versions into new blueprint revisions.

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

**Step 3: Blueprint for Shared VPC (infraProvisioning)**

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterBlueprint
metadata:
  name: shared-vpc-us-east-1-blueprint
spec:
  description: "Shared VPC infrastructure for us-east-1 production clusters"

  planes:
    - name: vpc
      ref:
        name: shared-vpc-us-east-1
```

**Step 4: Blueprint for Cluster Provisioning & Infrastructure**

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ClusterBlueprint
metadata:
  name: production-eks-standard
spec:
  description: "Standard production EKS cluster with networking and observability"

  planes:
    # Per-cluster EKS - created for each cluster
    - name: eks
      ref:
        name: eks-cluster

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

**Step 5: Create Clusters Using infraProvisioning + Blueprint**

```yaml
# Cluster A
apiVersion: core.oam.dev/v1beta1
kind: SpokeCluster
metadata:
  name: production-us-east-1-a
  labels:
    region: us-east-1
    environment: production
spec:
  mode: provision
  infraProvisioning:
    blueprintRef:
      name: shared-vpc-us-east-1-blueprint # Shared VPC, created once
  blueprintRef:
    name: production-eks-standard # Per-cluster EKS + infrastructure
---
# Cluster B
apiVersion: core.oam.dev/v1beta1
kind: SpokeCluster
metadata:
  name: production-us-east-1-b
  labels:
    region: us-east-1
    environment: production
spec:
  mode: provision
  infraProvisioning:
    blueprintRef:
      name: shared-vpc-us-east-1-blueprint
  blueprintRef:
    name: production-eks-standard
---
# Cluster C
apiVersion: core.oam.dev/v1beta1
kind: SpokeCluster
metadata:
  name: production-us-east-1-c
  labels:
    region: us-east-1
    environment: production
spec:
  mode: provision
  infraProvisioning:
    blueprintRef:
      name: shared-vpc-us-east-1-blueprint
  blueprintRef:
    name: production-eks-standard
```

**Result:**

When these clusters are created:

1. **Phase: infraProvisioning**. The first SpokeCluster to reconcile triggers creation of the `shared-vpc-us-east-1` plane (VPC, subnets, NAT Gateways). Subsequent SpokeClusters find it already reconciled and consume its outputs (no-op).

2. **Phase: blueprintRef** — Each Cluster gets its own `eks-cluster` plane instance, consuming VPC outputs via `{{ inputs.vpcId }}`. Each gets a unique EKS cluster name from `${context.cluster.name}` with its own node groups. Networking and observability planes are then dispatched to the spoke.

**Status After Provisioning:**

```bash
$ vela plane list --scope shared

NAME                    SCOPE    CONSUMERS  VERSION  OUTPUTS
shared-vpc-us-east-1    shared   3          v1       vpcId=vpc-0abc123

$ vela plane status shared-vpc-us-east-1

SHARED PLANE: shared-vpc-us-east-1
SCOPE: shared
CONSUMERS: 3

CONSUMERS:
  CLUSTER                      REFERENCED VIA             SINCE
  production-us-east-1-a       infraProvisioning                  2025-01-03T10:00:00Z
  production-us-east-1-b       infraProvisioning                  2025-01-03T10:15:00Z
  production-us-east-1-c       infraProvisioning                  2025-01-03T10:30:00Z

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
  2. Or update their infraProvisioning references to not use this plane

# Delete a cluster - per-cluster EKS is destroyed, shared VPC remains
$ kubectl delete cluster production-us-east-1-a

cluster.core.oam.dev "production-us-east-1-a" deleted

# Cluster A's EKS, networking, observability planes are cleaned up
# infraProvisioning consumer count decremented: shared-vpc-us-east-1 now shows 2 consumers
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

### SpokeCluster

The hub-side fleet object: how to reach the spoke, the hub-reconciled `infraProvisioning`, the dispatched blueprint, rollout, and maintenance windows. Listed with `kubectl get spokeclusters`.

| Field | Type | Description |
| --- | --- | --- |
| `spec.mode` | string | Lifecycle mode: `provision`, `adopt`, or `connect` |
| `spec.credential.type` | string | Auth method: `kubeconfig`, `aws`, `azure`, `gcp` |
| `spec.credential.<type>` | object | Provider-scoped auth: for `aws`, `authMode` (podIdentity or irsa), `clusterName`, `region`, `roleArn`; for a supplied kubeconfig, `kubeconfig.secretRef` |
| `spec.infraProvisioning.blueprintRef` | BlueprintRef | Hub-reconciled shared cloud infrastructure (VPC, IAM, DNS) and cluster creation, before the spoke reconciles. Shared across SpokeClusters; consumed without re-creation if already reconciled |
| `spec.blueprintRef` | BlueprintRef | Blueprint the hub dispatches to the spoke (desired state) |
| `spec.patches` | []PlanePatch | Per-cluster overrides applied on top of the dispatched blueprint |
| `spec.rolloutStrategyRef` | StrategyRef | ClusterRolloutStrategy that gates when a new revision is dispatched |
| `spec.rolloutStrategyRef.overrides` | OverrideSpec | Per-cluster rollout overrides |
| `spec.maintenance` | MaintenanceSpec | Windows that gate when a new revision is dispatched |
| `spec.maintenance.windows[]` | []MaintenanceWindow | Scheduled windows (`name`, `start`, `end`, `timezone`, `days`, `dstPolicy`) |
| `spec.maintenance.enforceWindow` | bool | Block dispatch outside the window |
| `spec.maintenance.allowEmergencyUpdates` | bool | Allow forced dispatch with `--force` |
| `status.connection` | string | Connection observed by an on-demand probe: `Connected`, `Disconnected` |
| `status.dispatchedRevision` | string | Blueprint revision last dispatched to the spoke |
| `status.provisioningStatus` | ProvisioningStatus | Infrastructure provisioning progress (provision mode) |
| `status.adoptionStatus` | AdoptionStatus | Adoption discovery and reconciliation status (adopt mode) |
| `status.clusterInfo` | ClusterInfo | Cluster info pulled from the spoke on demand |
| `status.health` | HealthStatus | Health snapshot pulled from the spoke `Cluster` while connected |
| `status.maintenance` | MaintenanceStatus | Computed window state (`inWindow`, `currentWindow`, `nextWindow`, `lastEvaluatedAt`, `timezoneInfo`) |

### Cluster

The spoke-side, self-reconciling representation. `vela-cluster-core` reconciles these phases locally from the dispatched blueprint; the hub reads status by pull, never by push.

| Field | Type | Description |
| --- | --- | --- |
| `spec.clusterInit.blueprintRef` | BlueprintRef | Foundational layer the planes depend on (CNI, base controllers/operators, Helm runtime, CRDs), reconciled on the spoke |
| `spec.planeProvisioning.blueprintRef` | BlueprintRef | The dispatched blueprint's cluster planes, reconciled on the spoke on top of clusterInit |
| `spec.healthValidation.blueprintRef` | BlueprintRef | Acceptance and smoke checks; the verdict is read by the hub on demand |
| `status.blueprint` | BlueprintStatus | Applied blueprint revision and sync state |
| `status.planes` | []PlaneInventory | Full inventory of planes and components |
| `status.health` | HealthStatus | Aggregated local health (planes, components, resources) |
| `status.drift` | DriftStatus | Local drift detection results |
| `status.conditions` | []Condition | Conditions such as BlueprintApplied, Healthy, DriftFree |
| `status.resources` | ResourceUsage | CPU, memory, pod usage |
| `status.history` | []HistoryEntry | Blueprint application history |

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
| `status.consumers`                      | ConsumersStatus   | SpokeClusters using this shared plane (scope=shared only)       |
| `status.consumers.count`                | int               | Number of SpokeClusters consuming this plane                    |
| `status.consumers.clusters`             | []ConsumerRef     | List of consuming SpokeClusters                                 |
| `status.consumers.clusters[].name`      | string            | SpokeCluster name                                               |
| `status.consumers.clusters[].blueprint` | string            | Blueprint the SpokeCluster uses                                 |
| `status.consumers.clusters[].since`     | Time              | When the SpokeCluster started consuming                         |
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

1. Add `definition.oam.dev/scope` label/annotation convention to existing definition CRDs
   - Scope values: `application` (default, backward-compatible), `cluster`, `both`
   - Validation webhook rejects invalid field combinations for `scope: cluster` (e.g., `PodDisruptive`, `ManageWorkload`, `Stage` on TraitDefinition)
   - Existing definitions without the annotation continue to work unchanged (implicit `scope: application`)

2. Implement scope-aware definition resolution for ClusterPlane controller
   - `GetInfraDefinition()` helper: searches only in `vela-system`, filters by `scope=cluster` or `scope=both`
   - Existing `GetDefinition()` for Application controller unchanged
   - Label-based list/watch filtering in ClusterPlane controller informers

3. Ship built-in infrastructure definitions as existing CRDs with `scope: cluster`
   - ComponentDefinition: `helm-release`, `kustomization`, `k8s-objects`, `terraform-module`, `crossplane-resource`
   - TraitDefinition: `resource-quota`, `namespace-labels`, `monitoring-annotations`
   - PolicyDefinition: `apply-order`, `health-check`
   - WorkflowStepDefinition: `apply-plane`, `validate-plane`, `wait`, `suspend`, `notification`, `health-check`, `script`, `http`, `webhook`, `step-group`

4. Built-in provider definitions (aws-eks, gcp-gke, azure-aks, kind) as `ClusterProviderDefinition`

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

| Current State                            | Migration Path                                                          | Effort                     |
| ---------------------------------------- | ----------------------------------------------------------------------- | -------------------------- |
| Clusters joined via `vela cluster join`  | Create SpokeCluster CRD with `clusterGatewayRef` pointing to existing secret | Minimal                    |
| cluster-gateway secrets in `vela-system` | Use `clusterGatewayRef`, or migrate to `credential.secretRef`           | Optional                   |
| Custom cluster-gateway configurations    | Reference existing secrets OR migrate to cloud-native auth              | Choose based on preference |

### Example: Referencing Existing cluster-gateway Secret

```yaml
apiVersion: core.oam.dev/v1beta1
kind: SpokeCluster
metadata:
  name: my-existing-cluster
spec:
  mode: connect
  # Reference existing cluster-gateway secret
  clusterGatewayRef:
    name: my-existing-cluster # Same name as vela cluster join created
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
