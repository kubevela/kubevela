# DevLog: Application-Scoped Policies Feature

**Date:** 2026-02-17
**Feature:** Application-scoped policy transformations for KubeVela
**Status:** Core feature complete, tested, and working

---

## Table of Contents
1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Key Files and Changes](#key-files-and-changes)
4. [Critical Bugs Fixed](#critical-bugs-fixed)
5. [Testing](#testing)
6. [Usage Guide](#usage-guide)
7. [Pending Work](#pending-work)

---

## Overview

### What This Feature Does

Application-scoped policies allow PolicyDefinitions to transform an Application CR **before** it's parsed and deployed. Unlike trait-level policies that apply to workloads, these policies can:

- **Transform the spec**: Add/modify/remove components, change workflow steps
- **Add metadata**: Inject labels and annotations
- **Provide context**: Share data between policies and make it available to workflows/components
- **Chain transformations**: Multiple policies execute in priority order, each seeing the previous policy's output

### Core Concepts

1. **Application-scoped PolicyDefinition**: A PolicyDefinition with `scope: application`
2. **Global vs Explicit Policies**:
   - **Global**: Discovered automatically in `vela-system` or Application's namespace, filtered by selectors
   - **Explicit**: Listed in `Application.spec.policies[]`
3. **In-memory transformations**: Policy changes to `Application.spec` happen in-memory during reconciliation
4. **ApplicationRevision**: Stores the transformed spec as the source of truth
5. **Caching**: 1-minute TTL cache for rendered policy results (invalidates on spec changes)

---

## Architecture

### Processing Flow

```
1. Application reconciliation starts
2. Check cache (1-min TTL + spec hash)
   ├─ HIT: Use cached rendered results
   └─ MISS: Render all policies (global + explicit)
3. Extract rendered outputs:
   ├─ metadata: labels, annotations, context
   └─ spec: components, workflow, policies
4. Apply metadata to Application (always)
5. Apply spec transformations to Application (in-memory)
6. Check autoRevision annotation:
   ├─ true: Keep transformed spec → creates new revisions
   └─ false: Restore from latest ApplicationRevision → stable spec
7. Store results in ConfigMap (observability)
8. Continue normal Application processing
```

### Policy Chaining

Policies execute in priority order (highest first):

```cue
// Policy 1 (priority: 200)
output: {
  components: [originalComponents + {name: "added-by-policy-1"}]
}

// Policy 2 (priority: 100) receives Policy 1's output as input
context: {
  previous: {
    components: [...] // Output from Policy 1
  }
}
output: {
  components: context.previous.components + [{name: "added-by-policy-2"}]
}
```

---

## Key Files and Changes

### Core Implementation Files

#### 1. `/pkg/controller/core.oam.dev/v1beta1/application/policy_transforms.go`
**Purpose**: Main policy rendering and transformation logic
**Key Functions**:
- `ApplyApplicationScopeTransforms()` - Entry point, orchestrates policy application
- `renderAllPolicies()` - Discovers and renders global + explicit policies
- `renderSinglePolicy()` - Renders one policy with chaining context
- `applySpecToApp()` - Applies transformed spec to Application (in-memory)
- `shouldAutoCreateRevision()` - Checks `app.oam.dev/autoRevision` annotation

**Critical Lines**:
- **Lines 93-117**: Cache check and policy rendering
- **Lines 135-179**: Apply transformations and ApplicationRevision restoration logic
- **Lines 146-170**: **BUG FIX** - Restore from ApplicationRevision when autoRevision=false
- **Lines 240-310**: Store results in ConfigMap for observability
- **Lines 1139-1147**: Check autoRevision annotation

#### 2. `/pkg/controller/core.oam.dev/v1beta1/application/application_policy_cache.go`
**Purpose**: In-memory cache for rendered policy results
**Key Features**:
- 1-minute TTL
- Invalidates on Application.Spec hash change
- Thread-safe (sync.RWMutex)
- Singleton instance: `applicationPolicyCache`

**Key Functions**:
- `Get()` / `GetWithReason()` - Retrieve cached results
- `Set()` - Store rendered results
- `InvalidateAll()` - Clear entire cache (used when global policies change)
- `InvalidateForNamespace()` - Clear namespace entries
- `computeAppSpecHash()` - Hash Application.Spec for invalidation

#### 3. `/pkg/controller/core.oam.dev/v1beta1/application/application_controller.go`
**Purpose**: Main Application controller
**Key Changes**:
- **Line ~180**: Call `ApplyApplicationScopeTransforms()` before parsing Application
- Policies apply **before** any parsing, component resolution, or workflow execution

#### 4. `/pkg/oam/labels.go`
**Purpose**: Label and annotation constants
**Key Addition**:
- **Lines 162-165**: `AnnotationAutoRevision = "app.oam.dev/autoRevision"`
  - Controls whether policy transformations create new ApplicationRevisions
  - Default: `false` (stable spec, restore from ApplicationRevision)
  - Set to `"true"` to enable revision creation on every transformation

#### 5. `/apis/core.oam.dev/v1beta1/application_types.go`
**Purpose**: Application status types
**Key Addition**:
- `Application.Status.AppliedApplicationPolicies` - List of applied policies with metadata
- `Application.Status.ApplicationPoliciesConfigMap` - ConfigMap name for observability

---

### Test Files

#### `/pkg/controller/core.oam.dev/v1beta1/application/policy_transforms_test.go`
**Key Tests Added**:

1. **Test Global PolicyDefinition Features** (line ~870)
   - Tests global policy discovery and filtering
   - Tests namespace-scoped vs vela-system global policies
   - Tests label/namespace selectors

2. **Test Policy Chaining** (line ~1020)
   - Tests `context.previous` propagation between policies
   - Verifies priority-based execution order

3. **Test ApplicationRevision Restoration** (line ~1199)
   - **NEW TEST** - Verifies double revision bug fix
   - Tests that subsequent reconciliations restore from ApplicationRevision
   - Ensures only ONE revision created on initial deployment

---

## Critical Bugs Fixed

### Bug #1: Double ApplicationRevision Creation

**Problem**: When deploying an Application with policies, TWO ApplicationRevisions were created immediately:
- v1: Had policy-transformed spec (correct)
- v2: Had original untransformed spec (wrong)

**Root Cause**:
1. First reconciliation: Cache MISS → policies render → transformations applied → v1 created ✅
2. Second reconciliation (triggered by status update): Cache HIT → transformations **not applied** to app.Spec
3. app.Spec still had original untransformed spec
4. This created v2 with different hash ❌

**Solution** (lines 146-170 in `policy_transforms.go`):
```go
// After applying transformed spec:
if !isFirstRevision && !autoRevision {
    // Load latest ApplicationRevision
    latestRev := &v1beta1.ApplicationRevision{}
    revName := app.Status.LatestRevision.Name
    err := h.Client.Get(ctx, client.ObjectKey{
        Name:      revName,
        Namespace: app.Namespace,
    }, latestRev)

    if err == nil && latestRev.Spec.Application.Spec.Components != nil {
        // Restore spec from revision (has policy transforms baked in)
        app.Spec.Components = latestRev.Spec.Application.Spec.Components
        if latestRev.Spec.Application.Spec.Workflow != nil {
            app.Spec.Workflow = latestRev.Spec.Application.Spec.Workflow
        }
        if len(latestRev.Spec.Application.Spec.Policies) > 0 {
            app.Spec.Policies = latestRev.Spec.Application.Spec.Policies
        }
    }
}
```

**Why It Works**:
- ApplicationRevision becomes the **source of truth** for the transformed spec
- On subsequent reconciliations with `autoRevision=false`, we restore from the revision
- This prevents creating new revisions from cached policy renders
- User changes to Application.Spec still invalidate cache and trigger re-rendering

**Test Added**: `"Test ApplicationRevision restoration prevents double revisions"` (line 1199)

---

## Testing

### Unit Tests

Run all policy transform tests:
```bash
go test -v ./pkg/controller/core.oam.dev/v1beta1/application -run "TestPolicyTransforms" -timeout 5m
```

Run specific test:
```bash
go test -v ./pkg/controller/core.oam.dev/v1beta1/application -run "TestPolicyTransforms.*double.*revision"
```

### Integration Testing

1. **Create a global policy**:
```yaml
apiVersion: core.oam.dev/v1beta1
kind: PolicyDefinition
metadata:
  name: atlas-context
  namespace: vela-system
spec:
  global: true
  priority: 100
  scope: application
  schematic:
    cue:
      template: |
        parameter: {}

        output: {
          labels: {
            "custom.guidewire.dev/service-id": "my-service"
          }
          context: {
            environment: "production"
          }
        }
```

2. **Create an Application**:
```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: my-app
  namespace: default
spec:
  components:
    - name: my-component
      type: webservice
      properties:
        image: nginx:latest
```

3. **Verify policies applied**:
```bash
# Check Application labels
kubectl get application my-app -o jsonpath='{.metadata.labels}'

# Check status
kubectl get application my-app -o jsonpath='{.status.appliedApplicationPolicies}'

# Check ConfigMap for observability
kubectl get configmap application-policies-default-my-app -o yaml
```

4. **Verify only ONE ApplicationRevision created**:
```bash
kubectl get applicationrevision | grep my-app
# Should only see: my-app-v1
```

### Debugging Tools

**View controller logs**:
```bash
kubectl logs -n vela-system deployment/kubevela-vela-core --tail=100 | grep -E "(Cache|policy|autoRevision|Restored)"
```

**Key log messages to look for**:
- `"Cache MISS - rendering all policies"` - First render
- `"Cache HIT - using cached policy results"` - Cache working
- `"Applied policy-rendered spec"` - Transformations applied
- `"Restored spec from ApplicationRevision"` - Restoration working (when autoRevision=false)
- `"Policy transforms completed" ... autoRevision=true/false` - Shows annotation value

**Check ConfigMap for rendered results**:
```bash
kubectl get configmap application-policies-default-<app-name> -o yaml
```

Contains:
- `info.yaml` - Metadata about rendering
- `rendered_<policy-name>.yaml` - Each policy's output
- `applied_spec.yaml` - Final transformed spec
- `original_spec.yaml` - Original Application spec

---

## Usage Guide

### Basic Usage: Explicit Policy

```yaml
apiVersion: core.oam.dev/v1beta1
kind: PolicyDefinition
metadata:
  name: add-sidecar
  namespace: default
spec:
  scope: application  # <-- Application-scoped
  schematic:
    cue:
      template: |
        parameter: {
          sidecarImage: string
        }

        output: {
          components: [
            for comp in context.appSpec.components {comp},
            {
              name: "sidecar"
              type: "webservice"
              properties: {
                image: parameter.sidecarImage
              }
            }
          ]
        }
---
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: my-app
spec:
  components:
    - name: main
      type: webservice
  policies:
    - name: add-sidecar
      type: add-sidecar
      properties:
        sidecarImage: "envoy:v1.0"
```

### Global Policy with Selectors

```yaml
apiVersion: core.oam.dev/v1beta1
kind: PolicyDefinition
metadata:
  name: production-labels
  namespace: vela-system
spec:
  global: true
  priority: 200
  scope: application
  # Only apply to Applications with this label
  labelSelector:
    "environment": "production"
  schematic:
    cue:
      template: |
        output: {
          labels: {
            "compliance.company.com/scanned": "true"
            "security.company.com/level": "high"
          }
        }
```

### Policy Chaining Example

```yaml
# Policy 1: Add monitoring component
apiVersion: core.oam.dev/v1beta1
kind: PolicyDefinition
metadata:
  name: add-monitoring
  namespace: vela-system
spec:
  global: true
  priority: 200
  scope: application
  schematic:
    cue:
      template: |
        output: {
          components: [
            for comp in context.appSpec.components {comp},
            {
              name: "prometheus"
              type: "webservice"
              properties: {image: "prometheus:latest"}
            }
          ]
        }
---
# Policy 2: Configure monitoring (sees Policy 1's output)
apiVersion: core.oam.dev/v1beta1
kind: PolicyDefinition
metadata:
  name: configure-monitoring
  namespace: vela-system
spec:
  global: true
  priority: 100  # Runs AFTER add-monitoring
  scope: application
  schematic:
    cue:
      template: |
        import "list"

        // Access previous policy's output
        _prevComponents: context.previous.components

        output: {
          components: [
            for comp in _prevComponents {
              if comp.name == "prometheus" {
                comp & {
                  properties: {
                    env: [{name: "SCRAPE_INTERVAL", value: "30s"}]
                  }
                }
              }
              if comp.name != "prometheus" {
                comp
              }
            }
          ]
        }
```

### Enabling autoRevision

To make policy transformations create new ApplicationRevisions on every change:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: my-app
  annotations:
    app.oam.dev/autoRevision: "true"  # <-- Enable revision creation
spec:
  components:
    - name: my-component
      type: webservice
```

**Important**: This is an **annotation**, not a label!

**Behavior**:
- `autoRevision: "true"` - Policy transformations create new revisions
- `autoRevision: "false"` (default) - Spec restored from ApplicationRevision, no new revisions
- Useful for GitOps workflows where you want stable revision numbers

---

## Bug #2: Component Dispatch Not Triggered with autoRevision=true

**Date Fixed:** 2026-02-17

**Problem**: When using `autoRevision=true` and policies retrigger (e.g., global PolicyDefinition changes), a new ApplicationRevision was created but components were NOT redeployed. Only direct changes to Application.spec triggered redeployment.

**Root Cause**: The dispatcher's `componentPropertiesChanged()` function compared component properties against the WRONG ApplicationRevision:
- Compared against `currentAppRev` (the NEW revision being created, which already had policy transforms)
- Both `comp.Params` and `currentAppRev` had the same transformed values
- Result: No difference detected → No dispatch

**The Fix** (commit SHA: 06e1d20a7):

Modified 2 files to conditionally use `latestAppRev` (previous revision) for comparison when `autoRevision=true`:

1. **generator.go:330** - Pass `h.latestAppRev` as additional parameter to `generateDispatcher()`
2. **dispatcher.go:117** - Updated signature to accept `previousAppRev` parameter
3. **dispatcher.go:149-171** - Added conditional comparison logic:
   ```go
   comparisonRev := appRev  // Default: use currentAppRev (existing behavior)

   // If autoRevision=true and we have a previous revision, compare against it
   if annotations[oam.AnnotationAutoRevision] == "true" && previousAppRev != nil {
       comparisonRev = previousAppRev  // Use previous revision for comparison
   }

   propertiesChanged = componentPropertiesChanged(comp, comparisonRev)
   ```

**Why It Works**:
- With `autoRevision=true`: Compares transformed component (from workflow) vs previous revision (before transform) → Detects change
- With `autoRevision=false` (default): Uses existing logic (compare vs currentAppRev) for backward compatibility
- First deployment: Falls back to `currentAppRev` when `previousAppRev == nil`

**Documentation Added**:
- Added extensive comment in `dispatcher.go:151-158` noting that the default comparison logic "seems unclear and may have become over-complicated over time"
- Left breadcrumb for future developers to consider simplifying this logic unconditionally
- Updated `componentPropertiesChanged()` function documentation

**Files Changed**:
- `/pkg/controller/core.oam.dev/v1beta1/application/generator.go`
- `/pkg/controller/core.oam.dev/v1beta1/application/dispatcher.go`

---

## Pending Work

### High Priority

1. **Implement `context.previous` support** ✅ (Partially done - needs verification)
   - Currently: Policies can access previous policy's output
   - TODO: Verify chaining works correctly in all scenarios
   - Test file: Already has test at line ~1020

2. **Remove cascade invalidation code** (if not needed)
   - Location: `application_policy_cache.go:142-151`
   - Current: `InvalidateForNamespace()` deletes ALL entries
   - Should only delete entries for affected namespace
   - Need to evaluate if this is correct behavior

### Medium Priority

3. **Add metrics/telemetry**
   - Cache hit/miss rates
   - Policy rendering duration
   - Number of policies applied per Application

4. **Improve error handling**
   - Better error messages when policy CUE template fails
   - Distinguish between policy errors vs Application errors

5. **Documentation**
   - User-facing docs for writing Application-scoped policies
   - Migration guide from trait-level policies
   - Best practices for policy chaining

### Low Priority

6. **Performance optimizations**
   - Consider longer cache TTL with smarter invalidation
   - Parallel policy rendering (if policies don't depend on each other)

7. **Enhanced ConfigMap output**
   - Add diff between original and transformed spec
   - Include policy execution order and timing

---

## CUE Template Context Reference

Application-scoped policies have access to:

```cue
// Current Application spec (original, before transformations)
context: {
  appSpec: {
    components: [...]
    policies: [...]
    workflow: {...}
  }

  // Application metadata
  appName: string
  appNamespace: string
  appLabels: {...}
  appAnnotations: {...}

  // Previous policy's output (for chaining)
  previous: {
    components: [...]
    workflow: {...}
    policies: [...]
    labels: {...}
    annotations: {...}
    context: {...}
  }
}

// Policy parameters
parameter: {
  // User-provided parameters from Application.spec.policies[].properties
}

// Policy output
output: {
  // Spec transformations (optional)
  components?: [...]
  workflow?: {...}
  policies?: [...]

  // Metadata additions (optional)
  labels?: {...}
  annotations?: {...}

  // Additional context for other policies/workflow (optional)
  context?: {...}
}
```

---

## Key Learnings

1. **ApplicationRevision as source of truth**: Using ApplicationRevision to restore the transformed spec was the key insight to fixing the double revision bug. It provides a stable "memory" of what the spec should be.

2. **In-memory transformations**: Keeping transformations in-memory (not persisting to etcd) ensures policies don't cause infinite reconciliation loops.

3. **Cache invalidation is hard**: The spec hash approach works well, but requires careful consideration of what should trigger invalidation.

4. **Annotations vs Labels**: The `autoRevision` control MUST be an annotation (not label) because that's where the code checks. This tripped up initial testing.

5. **Policy chaining complexity**: Providing `context.previous` requires careful ordering and state management during rendering.

---

## Related Files

### Configuration
- `/apis/core.oam.dev/v1beta1/application_types.go` - Application CRD types
- `/apis/core.oam.dev/v1beta1/policy_types.go` - PolicyDefinition CRD types
- `/pkg/oam/labels.go` - Label/annotation constants

### Controllers
- `/pkg/controller/core.oam.dev/v1beta1/application/application_controller.go` - Main controller
- `/pkg/controller/core.oam.dev/v1beta1/application/parser.go` - Application parsing
- `/pkg/controller/core.oam.dev/v1beta1/application/revision.go` - ApplicationRevision handling

### Utilities
- `/pkg/utils/apply/apply.go` - ComputeSpecHash function
- `/pkg/monitor/context/context.go` - Monitoring context utilities

---

## Commit History

### Main Commits
1. Initial implementation of Application-scoped policies
2. Added policy caching with TTL and spec hash invalidation
3. **[CRITICAL FIX]** Fixed double ApplicationRevision bug by restoring from ApplicationRevision
4. Added test case for ApplicationRevision restoration
5. Added `app.oam.dev/autoRevision` annotation support

### Test Commits
- Added global policy discovery tests
- Added policy chaining tests
- Added ApplicationRevision restoration tests

---

## Contact / Questions

For questions about this feature:
- Check logs: Look for "policy" or "Cache" messages in controller logs
- Check ConfigMap: `application-policies-<namespace>-<app-name>` for debugging
- Check status: `kubectl get app <name> -o jsonpath='{.status.appliedApplicationPolicies}'`

---

**Last Updated:** 2026-02-17
**Status:** Feature complete and tested
**Next Steps:** Address pending work items above
