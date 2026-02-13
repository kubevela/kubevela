# Post-Dispatch Trait Pending Status (Commit 4170f89e)

## Document scope

This document is a deep, commit-accurate walkthrough of the post-dispatch trait pending status feature introduced in commit `4170f89e298274850424a0e8edab5fcbd862ce9e` (Jan 21, 2026). It explains what the change solves, why it was introduced, how it works, and why specific design choices were made. It is intentionally focused on the code as it existed in that commit, even if later refactors have changed the implementation. If you are reading this long after the commit, treat this as historical context for the feature, not a verbatim description of the current codebase.

Target length: 5-6 pages of Markdown (roughly 2,500-3,500 words).

## Executive summary

KubeVela supports multi-stage component apply, where some traits can be marked as **post-dispatch** (i.e., applied only after the workload becomes healthy). Prior to this change, those post-dispatch traits would not surface any status while the workload was still unhealthy. This led to confusing user experiences: `vela status` would show an unhealthy component but no trait status at all for post-dispatch traits, and workflows might be stuck waiting for health without any visible explanation.

Commit `4170f89e` introduces a **pending status** for post-dispatch traits when the workload is not yet healthy. The change adds a `Pending` field to `ApplicationTraitStatus`, updates CRD schemas accordingly, and updates status collection to generate pending entries with a standard hourglass message (`"\u23f3"`) when a post-dispatch trait is not yet applied. It also rebuilds the trait status list via a map to prevent duplicate entries across reconciliations.

## Background and problem statement

### Multi-stage component apply and post-dispatch traits

In multi-stage component apply, a trait can declare a dispatch stage. A post-dispatch trait (also called "AfterWorkload") is applied only after the workload becomes healthy. This is useful for features that require a stable workload (for example, traffic shifting, or post-deploy configuration tasks).

### The status visibility gap

Before this commit, the controller collected trait health by iterating through component traits and immediately invoking `collectTraitHealthStatus`. But when a trait is post-dispatch and the workload is not yet healthy, the trait resource is not yet applied and therefore has no health status to collect. The result is that **no trait status is shown**, which makes it look like the trait does not exist.

This gap produces three user-facing problems:

1. **Lack of visibility**: Users do not see the configured post-dispatch traits during the period when they are pending.
2. **Confusing workflow states**: Workflows may be waiting for health, but the status view does not show any trait entries that are pending.
3. **Poor diagnosability**: Users can't differentiate between "trait is correctly waiting" vs "trait is misconfigured or missing."

### Goal of the change

Make post-dispatch traits visible immediately in status output when the workload is not healthy. Display a "pending" state and a simple message so the user knows the trait exists and is waiting to be applied.

## Goals and non-goals

### Goals

- Expose post-dispatch traits in component status even when the workload is not healthy.
- Provide a clear, minimal signal that the trait is pending.
- Prevent duplicate trait status entries across multiple reconciliations.
- Keep the behavior gated behind the existing multi-stage feature gate to avoid surprising users who have not opted in.

### Non-goals

- No CLI display changes are included in this commit.
- No changes to how traits are applied; only status collection is affected.
- No additional health checks or reconciliation logic changes beyond status collection.
- No per-trait custom pending messages (this commit uses a fixed hourglass symbol).

## High-level design

At a high level, the change does three things:

1. **Add a new status field**: `ApplicationTraitStatus.Pending` (boolean).
2. **Emit pending trait statuses**: When a trait is post-dispatch and the workload is not healthy, write a pending status with `Pending=true`, `Healthy=false`, and a standard message `"\u23f3"`.
3. **Deduplicate trait statuses**: Rebuild `status.Traits` from a map keyed by trait type to avoid duplicates across reconciliations.

All of this logic is enabled only when the `MultiStageComponentApply` feature gate is on. If that gate is off, the trait status logic remains the same as before.

## Code changes by file

### 1) `apis/core.oam.dev/common/types.go`

**What changed**

A new field is added to `ApplicationTraitStatus`:

```go
// ApplicationTraitStatus records the trait health status
 type ApplicationTraitStatus struct {
     Type    string            `json:"type"`
     Healthy bool              `json:"healthy"`
     Pending bool              `json:"pending,omitempty"`
     Details map[string]string `json:"details,omitempty"`
     Message string            `json:"message,omitempty"`
 }
```

**Why**

The controller needs a formal way to represent "pending" traits. Using a dedicated field avoids overloading `Healthy=false` (which could represent both "failed" and "not yet applied"), and makes it easier for clients to distinguish the pending state.

**Why not**

- **Just use a message**: A message alone is ambiguous for machines and more brittle for UI. A dedicated field is explicit and future-proof.
- **Use a new enum state**: That would require a broader refactor of status semantics and is not necessary for this feature.

### 2) CRD schema updates

**Files**

- `charts/vela-core/crds/core.oam.dev_applicationrevisions.yaml`
- `charts/vela-core/crds/core.oam.dev_applications.yaml`

**What changed**

Both schemas are updated to accept the `pending` boolean field for trait status entries.

**Why**

Without the CRD update, Kubernetes would drop or reject the field, and the status change would never persist. This is a standard required change when adding new status fields.

**Why not**

Skipping the CRD update would make the field unreliable and would break clients that expect it to exist. This change is mandatory for proper API behavior.

### 3) `pkg/controller/core.oam.dev/v1beta1/application/dispatcher.go`

**What changed**

A helper is added to build a consistent pending status:

```go
func createPendingTraitStatus(traitName string) common.ApplicationTraitStatus {
    return common.ApplicationTraitStatus{
        Type:    traitName,
        Healthy: false,
        Pending: true,
        Message: "\u23f3",
    }
}
```

**Why**

The helper encapsulates the pending trait shape so it is consistent wherever it is used. It keeps the main logic in `apply.go` cleaner and avoids duplicating the pending status fields.

**Why not**

- **Inline the struct in the status loop**: That would make the logic harder to read, and spread the "pending" semantics across multiple sites.
- **Use a more verbose message**: The choice of a single symbol keeps output compact and language-independent. A more detailed message would be a UI/UX decision best handled by the CLI or a richer UI layer.

### 4) `pkg/controller/core.oam.dev/v1beta1/application/apply.go`

This is the main logic change. It modifies `collectHealthStatus` so post-dispatch traits are shown as pending and so trait status entries are deduplicated.

#### Prior behavior (before this commit)

Before the change, the relevant part of the logic looked like this (simplified from the pre-commit code):

```go
status = h.getServiceStatus(status)
if !skipWorkload {
    isHealth, output, outputs, err = h.collectWorkloadHealthStatus(...)
}

var traitStatusList []common.ApplicationTraitStatus
for _, tr := range comp.Traits {
    // filter logic

    traitStatus, _outputs, err := h.collectTraitHealthStatus(...)
    // handle outputs, health aggregation
    traitStatusList = append(traitStatusList, traitStatus)

    // remove old status entries of the same type
    var oldStatus []common.ApplicationTraitStatus
    for _, _trait := range status.Traits {
        if _trait.Type != tr.Name {
            oldStatus = append(oldStatus, _trait)
        }
    }
    status.Traits = oldStatus
}
status.Traits = append(status.Traits, traitStatusList...)
```

**Observations**

- All traits are processed the same way. There is no special handling for post-dispatch traits.
- If a post-dispatch trait is not yet applied, `collectTraitHealthStatus` may return nothing or fail, and no status entry appears.
- `status.Traits` is modified by filtering out existing entries by type and then appending a new list. This reduces duplicates in many cases, but it does not handle all accumulation scenarios.

#### New behavior introduced in this commit

The updated logic introduces three key additions:

1. **Feature gate check** for multi-stage apply.
2. **Pending status generation** for post-dispatch traits when the workload is not healthy.
3. **Trait status de-duplication** using a map keyed by trait type.

Below is the updated flow, with commentary, as it appears in the commit.

##### Step 1: Track workload health separately

```go
status = h.getServiceStatus(status)
workloadHealthy := status.Healthy
if !skipWorkload {
    workloadHealthy, output, outputs, err = h.collectWorkloadHealthStatus(...)
    isHealth = workloadHealthy
}
```

The `workloadHealthy` variable captures the workload's health status to decide whether post-dispatch traits should be marked as pending.

##### Step 2: Initialize trait status maps

```go
pendingEnabled := utilfeature.DefaultMutableFeatureGate.Enabled(features.MultiStageComponentApply)
traitStatusByType := make(map[string]common.ApplicationTraitStatus, len(status.Traits))
traitOrder := make([]string, 0, len(status.Traits))
for _, ts := range status.Traits {
    if _, exists := traitStatusByType[ts.Type]; exists {
        continue
    }
    traitStatusByType[ts.Type] = ts
    traitOrder = append(traitOrder, ts.Type)
}
addTraitStatus := func(ts common.ApplicationTraitStatus) {
    if _, exists := traitStatusByType[ts.Type]; !exists {
        traitOrder = append(traitOrder, ts.Type)
    }
    traitStatusByType[ts.Type] = ts
}
processedTraits := make(map[string]struct{})
```

**Why this matters**

- `traitStatusByType` provides de-duplication by type, ensuring only one status per trait type is retained.
- `traitOrder` preserves the display order of traits across reconciliations.
- The helper `addTraitStatus` makes it easy to update or insert a status while keeping order.
- `processedTraits` records which traits were evaluated in the main loop.

##### Step 3: Handle post-dispatch traits as pending

Inside the trait loop:

```go
processedTraits[tr.Name] = struct{}{}
if pendingEnabled {
    traitStage, err := getTraitDispatchStage(...)
    isPostDispatch := err == nil && traitStage == PostDispatch
    if isPostDispatch && !workloadHealthy {
        addTraitStatus(createPendingTraitStatus(tr.Name))
        continue collectNext
    }
}

traitStatus, _outputs, err := h.collectTraitHealthStatus(...)
addTraitStatus(traitStatus)
```

**Why this matters**

- If the trait is post-dispatch and the workload is not healthy, the controller does **not** try to fetch trait health. It records a pending status instead.
- The gate `pendingEnabled` ensures this behavior only happens when multi-stage apply is turned on.
- The controller still collects trait health normally for pre-dispatch traits or if the workload is healthy.

##### Step 4: Add missing post-dispatch traits from the ApplicationRevision

After the main loop, another block adds pending statuses for post-dispatch traits that were not processed in the loop but still exist in the ApplicationRevision:

```go
if pendingEnabled && !workloadHealthy {
    for _, component := range h.currentAppRev.Spec.Application.Spec.Components {
        if component.Name != comp.Name {
            continue
        }
        for _, trait := range component.Traits {
            if _, ok := processedTraits[trait.Type]; ok {
                continue
            }
            if _, ok := traitStatusByType[trait.Type]; ok {
                continue
            }
            traitStage, err := getTraitDispatchStage(...)
            isPostDispatch := err == nil && traitStage == PostDispatch
            if isPostDispatch {
                addTraitStatus(createPendingTraitStatus(trait.Type))
            }
        }
        break
    }
}
```

**Why this matters**

- This handles traits that may have been skipped in the loop (for example, due to filtering).
- It also ensures that trait statuses reflect the ApplicationRevision spec, not only the in-memory component representation.

##### Step 5: Rebuild `status.Traits` deterministically

```go
status.Traits = make([]common.ApplicationTraitStatus, 0, len(traitStatusByType))
for _, traitType := range traitOrder {
    status.Traits = append(status.Traits, traitStatusByType[traitType])
}
```

This replaces the previous append-based approach and ensures a clean, de-duplicated list each reconciliation.

## Behavior: before vs after

### Before

- Post-dispatch traits were absent from status output until the workload became healthy.
- Status list could accumulate or reorder unpredictably across reconciliations.
- Users had no immediate indication that a post-dispatch trait was waiting.

### After (feature gate enabled)

- Post-dispatch traits appear immediately with:
  - `Pending = true`
  - `Healthy = false`
  - `Message = "\u23f3"`
- Trait statuses are rebuilt per reconciliation and de-duplicated.
- Users can tell that the trait is waiting on workload health.

## Why the design choices were made

### Why use a `Pending` field instead of just message text

- It provides a machine-readable signal for UIs and automation.
- It cleanly separates "not healthy because pending" from "not healthy because failed."
- It is extensible; future fields or UI logic can pivot on it.

### Why use a simple hourglass symbol

- It is concise and language-independent.
- It keeps output compact in CLI display.
- It avoids implying a specific failure or error.

### Why gate the behavior behind `MultiStageComponentApply`

- Post-dispatch trait semantics are only meaningful when multi-stage apply is enabled.
- It avoids changing behavior for users who are not using that feature.
- It gives operators a clear switch to enable or disable the pending status feature.

### Why skip health checks for post-dispatch traits when workload is unhealthy

- The traits are not applied yet; collecting health would require querying resources that do not exist.
- It prevents noisy errors and wasted API calls.
- It aligns with the actual state of the system: the trait is pending, not failed.

### Why rebuild `status.Traits` from a map

- It ensures a stable, de-duplicated list every reconcile.
- It eliminates subtle accumulation of trait status entries across reconcile loops.
- It makes trait ordering predictable when combined with `traitOrder`.

## Compatibility and API considerations

### Backward compatibility

- The new `pending` field is optional (`omitempty`) and does not break clients that ignore it.
- The CRD schemas are updated, so stored status data remains consistent.
- Existing clients that only check `Healthy` can continue to work, but may treat pending as unhealthy until they adopt the new field.

### Upgrade behavior

- After applying the CRD changes, controllers built with this commit will start emitting `pending` in Application and ApplicationRevision statuses.
- No user-facing configuration change is required beyond enabling the feature gate.

## Performance and operational impact

- **Reduced API churn**: pending traits do not trigger a health check when the workload is unhealthy.
- **Predictable reconciliation**: de-duplication avoids ever-growing status lists.
- **Minimal memory overhead**: maps are short-lived and sized based on current trait count.

## Edge cases handled

1. **Traits filtered out by traitFilters**
   - Traits skipped in the main loop may still appear as pending if they exist in the ApplicationRevision and are post-dispatch.

2. **Trait definition fetch failures**
   - If `getTraitDispatchStage` errors, `isPostDispatch` is false and no pending status is added. This is a "fail closed" behavior that avoids mislabeling traits.

3. **Repeated reconciles**
   - Trait list is rebuilt each time; duplicates do not accumulate.

4. **Mixed pre- and post-dispatch traits**
   - Pre-dispatch traits are collected normally.
   - Post-dispatch traits are marked pending until the workload is healthy.

## What this commit does NOT do

- It does not change the CLI output logic directly. Any CLI rendering of pending is a separate concern.
- It does not modify trait application behavior.
- It does not add tests for the new logic in this commit.

## Validation guidance

To validate the feature introduced in this commit:

1. **Enable the feature gate** `MultiStageComponentApply`.
2. Apply an Application with at least one post-dispatch trait (a trait whose definition stage is AfterWorkload).
3. Ensure the workload is unhealthy.
4. Check Application status and confirm the trait appears with `pending: true` and `message: "\u23f3"`.
5. Once the workload becomes healthy, verify the post-dispatch trait transitions to normal health reporting.

## Future improvements (outside this commit)

- Add explicit CLI UI treatment for `pending` (distinct from `healthy=false`).
- Provide a richer pending message (for example, "waiting for workload to become healthy").
- Add unit tests for pending trait status behavior.
- Support per-trait custom pending messages via annotations or trait definition metadata.

## Summary

Commit `4170f89e` introduces a focused but meaningful improvement to status visibility for post-dispatch traits under the multi-stage apply feature gate. By adding a `Pending` field to trait status, generating pending entries when the workload is unhealthy, and rebuilding trait status lists through a map, the controller now shows traits immediately and avoids status list duplication. The change is backward-compatible, API-correct (via CRD updates), and operationally cheap.

The result is a clearer, more predictable status experience that removes a common source of confusion for users when post-dispatch traits are configured but not yet applied.
