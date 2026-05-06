# KEP-2.7: WorkflowRun Controller Bundling

**Status:** Drafting (Not ready for consumption)
**Parent:** [vNext Roadmap](../README.md)

---

## Overview

KEP-2.7 addresses the WorkflowRun controller: an independent controller for running arbitrary workflows outside Component/Application scope. The core decision is that the WorkflowRun controller is **bundled in the KubeVela Helm chart by default**, eliminating the current pain point where nothing can presume its presence.

---

## Motivation

In KubeVela, the workflow controller is a separate installation that nothing can safely presume the presence of. This creates friction for:

- Platform engineers building `WorkflowStepDefinition`-based tooling who cannot rely on the WorkflowRun controller being available
- Definition authors who want to offer standalone operational runbooks that are not bound to a Component or Application lifecycle
- Teams who only need workflow capabilities and not the full KubeVela stack

The current situation where `WorkflowRun` is a separate, optional addon means that platform teams and Definition authors cannot build on it confidently.

---

## Design

### Embedded Workflow Engine vs. WorkflowRun Controller

There are **two distinct workflow execution contexts** in KubeVela 2.0:

1. **Embedded workflow engine** — a Go library embedded directly in the component-controller. It is not a separate controller. It executes `create`/`upgrade`/`delete` lifecycle workflows for `Component` CRs, with native access to Component context, outputs, status, and exports. This engine is always present on the spoke — if a component-controller is running, the workflow engine is available.

2. **Standalone WorkflowRun controller** — an independent controller with its own binary for running arbitrary `WorkflowRun` CRs outside Component/Application scope. This is the controller addressed by this KEP.

These are separate concerns. The embedded workflow engine is not optional and is not the subject of this KEP.

### Standalone WorkflowRun Controller

The WorkflowRun controller:

- Is **optional** — can be installed standalone by teams who only need that capability
- Is **bundled** — included in the KubeVela Helm chart by default, enabled unless explicitly disabled

This eliminates the current KubeVela pain point where the workflow controller is a separate installation that nothing can presume the presence of. A standard KubeVela installation always includes the WorkflowRun controller — Definition authors and platform teams can build on it confidently.

### Why Bundle by Default

Bundling the WorkflowRun controller by default means:

- **`WorkflowStepDefinition` becomes a reliable primitive** — Definition authors can reference `WorkflowStepDefinition` types in standalone `WorkflowRun` contexts without per-cluster capability detection
- **Platform tooling has a stable foundation** — teams building operational tooling (OperationDefinition phases, CI pipelines, administrative workflows) can rely on the WorkflowRun controller being present in any standard KubeVela installation
- **Opt-out is still possible** — teams with strict resource constraints or security requirements can disable it via `KubeVela.spec.components.workflowRunController.enabled: false`

---

## Deployment Summary

| Component | Standalone | KubeVela bundle |
|---|---|---|
| component-controller + workflow engine | ✓ (spoke installation) | ✓ |
| application-controller | — | ✓ |
| WorkflowRun controller | ✓ (optional) | ✓ (bundled, default on) |

### KubeVela CR Configuration

```yaml
apiVersion: core.oam.dev/v1alpha1
kind: KubeVela
metadata:
  name: kubevela
  namespace: vela-system
spec:
  role: hub
  components:
    workflowRunController:     # all roles, default on
      enabled: true            # set to false to disable
```

---

## Relationship to Other KEPs

- **KEP-2.2 (Spoke component-controller)** — the embedded workflow engine inside the component-controller is a separate concern from the standalone WorkflowRun controller. KEP-2.2 covers the embedded engine. This KEP covers standalone bundling.
- **KEP-2.6 (KubeVela Operator)** — the `KubeVela` CR's `components.workflowRunController` field governs whether the WorkflowRun controller is deployed. The operator installs, configures, and drift-corrects it.
- **KEP-2.15 (OperationDefinition)** — `WorkflowStepDefinition` primitives are shared across Operations and WorkflowRuns. Bundling the WorkflowRun controller ensures the same step types are available in both execution contexts.
- **KEP-2.2 (Spoke controller)** — `WorkflowStepDefinition` scope field controls which execution contexts a step may be used in (`Application`, `Operation`, `WorkflowRun`).
