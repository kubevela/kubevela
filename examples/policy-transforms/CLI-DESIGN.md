# Policy CLI Commands - Design Specification

## Overview

Two new CLI commands for debugging and testing global policies:
1. `vela policy view` - View applied policies and their effects
2. `vela policy dry-run` - Preview policy effects before applying

## Command 1: `vela policy view`

### Purpose
Interactive viewer for policy changes applied to an Application.

### Usage
```bash
vela policy view <app-name> [flags]
```

### Flags
- `-n, --namespace <namespace>` - Application namespace (default: current context)
- `--output <format>` - Output format: `table` (default), `json`, `yaml`

### Behavior

#### 1. Display Summary Table
```
Applied Global Policies (3):
┌──────────────────────┬─────────────┬──────────┬──────────────┬─────────────┐
│ Policy               │ Namespace   │ Sequence │ Priority     │ Spec Changed│
├──────────────────────┼─────────────┼──────────┼──────────────┼─────────────┤
│ inject-sidecar       │ vela-system │ 1        │ 100          │ Yes         │
│ resource-limits      │ vela-system │ 2        │ 50           │ Yes         │
│ platform-labels      │ vela-system │ 3        │ 0            │ No          │
└──────────────────────┴─────────────┴──────────┴──────────────┴─────────────┘

Labels Added:
  platform.io/managed-by: kubevela (platform-labels)
  sidecar.io/injected: true (inject-sidecar)

Annotations Added:
  config.io/resource-profile: standard (resource-limits)

Spec Changes: 2 policies modified the spec
View detailed diffs: kubectl get configmap my-app-policy-diffs
```

#### 2. Interactive Mode (if terminal supports)
```
Select a policy to view changes: [Use arrows to move, type to filter, Enter to view]
> inject-sidecar (added monitoring-sidecar component)
  resource-limits (modified resource constraints)
  platform-labels (metadata only)
```

When selected, show:
```
Policy: inject-sidecar (vela-system)
Sequence: 1, Priority: 100

Changes Made:
  ✓ Spec Modified: Yes
  ✓ Added Labels: sidecar.io/injected=true

Spec Diff (JSON Merge Patch):
{
  "components": [
    null,
    {
      "name": "monitoring-sidecar",
      "type": "webservice",
      "properties": {
        "image": "monitoring:latest"
      }
    }
  ]
}

Interpretation:
  • Added component at index 1: monitoring-sidecar
```

#### 3. Non-interactive Mode (CI/scripting)
```bash
# JSON output
vela policy view my-app --output json

# YAML output
vela policy view my-app --output yaml
```

### Implementation Notes
- Read from `app.Status.AppliedGlobalPolicies`
- Read diffs from ConfigMap if `PolicyDiffsConfigMap` is set
- Use `github.com/AlecAivazis/survey/v2` for interactive selection (like `vela debug`)
- Use `github.com/FogDong/uitable` for table display (like other vela commands)
- Parse JSON Merge Patch to provide human-readable interpretation

---

## Command 2: `vela policy dry-run`

### Purpose
Preview policy effects before applying to an Application.

### Usage
```bash
vela policy dry-run <app-name> [flags]
```

### Flags
- `-n, --namespace <namespace>` - Application namespace (default: current context)
- `--policies <p1,p2,pN>` - Specific policies to test (comma-separated)
- `--include-global-policies` - Include existing global policies
- `--include-app-policies` - Include policies from Application spec
- `--output <format>` - Output format: `table` (default), `summary`, `json`, `yaml`, `diff`
  - `table`: Full output with policy details and final Application spec
  - `summary`: Show only labels, annotations, and context (no spec)
  - `json`: Machine-readable JSON output
  - `yaml`: Machine-readable YAML output
  - `diff`: Unified diff format showing changes

### Behavior Modes

#### Mode 1: Isolated Testing (default when --policies specified)
Test ONLY specified policies, ignore all existing policies.

```bash
vela policy dry-run my-app --policies inject-monitoring
```

**Use case**: Testing a new policy in isolation before deploying it.

**Execution order**:
1. Specified policies (in CLI order)

#### Mode 2: Additive Testing (--include-global-policies)
Test specified policies WITH existing global policies.

```bash
vela policy dry-run my-app --policies new-security-policy --include-global-policies
```

**Use case**: Test how a new policy interacts with existing globals, detect conflicts.

**Execution order**:
1. Existing global policies (sorted by priority + name)
2. Specified policies (in CLI order)

#### Mode 3: Full Simulation (no --policies flag)
Simulate complete policy chain that would apply to the Application.

```bash
vela policy dry-run my-app
```

**Use case**: Debug unexpected behavior, understand current state.

**Execution order**:
1. Existing global policies (sorted by priority + name)
2. App spec policies (in spec order)

#### Mode 4: Full + Additional Policies
Full simulation plus extra test policies.

```bash
vela policy dry-run my-app --policies test-policy --include-global-policies --include-app-policies
```

**Execution order**:
1. Existing global policies (sorted by priority + name)
2. Specified policies (in CLI order)
3. App spec policies (in spec order)

### Output Format

#### Default Output
```
Dry-run simulation for Application: my-app (namespace: default)

Execution Plan:
  1. inject-sidecar (vela-system, priority: 100) [global]
  2. resource-limits (vela-system, priority: 50) [global]
  3. security-hardening (specified) [test]

Applying policies...

✓ Policy 1/3: inject-sidecar
  Sequence: 1
  Enabled: true
  Changes:
    • Spec Modified: Yes
    • Added Labels: sidecar.io/injected=true
    • Added component: monitoring-sidecar

✓ Policy 2/3: resource-limits
  Sequence: 2
  Enabled: true
  Changes:
    • Spec Modified: Yes
    • Modified: components[0].properties.resources.limits.cpu → 500m
    • Modified: components[0].properties.resources.limits.memory → 512Mi

✓ Policy 3/3: security-hardening
  Sequence: 3
  Enabled: true
  Changes:
    • Spec Modified: Yes
    • Added: components[0].properties.securityContext.runAsNonRoot=true

Summary:
  Total Policies: 3
  Applied: 3
  Skipped: 0
  Spec Modifications: 3
  Labels Added: 1
  Annotations Added: 0

⚠ Warnings:
  • No conflicts detected

Final Application State:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Labels (1 total):
  sidecar.io/injected: "true"                (inject-sidecar)

Annotations:
  (none)

Additional Context:
  (none)

Application Spec:
---
<YAML of final spec>
---
```

#### Summary Output (--output summary)
Show only labels, annotations, and context (no spec):

```bash
vela policy dry-run my-app --output summary
```

Displays:
- Summary counts (policies applied/skipped, changes)
- Final labels table with attribution
- Final annotations table with attribution
- Final additional context with attribution

**Use case**: When you only care about metadata changes (labels/annotations) and don't need to see spec modifications. Useful for quickly checking if policies are adding the correct platform labels.

#### Diff Output (--output diff)
```bash
vela policy dry-run my-app --output diff
```

Shows unified diff of original spec vs final spec:
```diff
--- Original Spec
+++ After Policies
@@ -1,5 +1,10 @@
 components:
   - name: backend
     type: webservice
     properties:
       image: myapp:v1
+      resources:
+        limits:
+          cpu: 500m
+          memory: 512Mi
+  - name: monitoring-sidecar
+    type: webservice
```

#### JSON/YAML Output
Machine-readable output for scripting:
```json
{
  "application": "my-app",
  "namespace": "default",
  "executionPlan": [
    {
      "sequence": 1,
      "policyName": "inject-sidecar",
      "policyNamespace": "vela-system",
      "priority": 100,
      "source": "global"
    }
  ],
  "results": [
    {
      "sequence": 1,
      "policyName": "inject-sidecar",
      "enabled": true,
      "applied": true,
      "specModified": true,
      "addedLabels": {"sidecar.io/injected": "true"},
      "addedAnnotations": {},
      "additionalContext": null,
      "diff": { ... }
    }
  ],
  "finalState": {
    "labels": {
      "sidecar.io/injected": "true"
    },
    "annotations": {},
    "additionalContext": {},
    "spec": { ... }
  }
}
```

### Implementation Notes

#### Core Logic
1. **Load Application**: Get Application from cluster (or from file with `-f`)
2. **Discover Policies**: Based on flags, build execution plan
3. **Create Temporary AppHandler**: Use existing `ApplyApplicationScopeTransforms` logic
4. **Apply Policies in Dry-Run Mode**:
   - Execute transforms on in-memory copy
   - Track diffs for each policy
   - Don't persist to cluster
5. **Display Results**: Format based on `--output` flag

#### Code Reuse
- Reuse `ApplyApplicationScopeTransforms` from `policy_transforms.go`
- Reuse `discoverGlobalPolicies` for global policy discovery
- Reuse diff computation logic (`deepCopyAppSpec`, `computeJSONPatch`)
- Similar pattern to existing `vela dry-run` command

#### Key Differences from Real Reconciliation
- No persistence to cluster
- No status updates
- No events emitted
- No ConfigMap creation
- Returns results to CLI instead

#### Error Handling
- Policy not found: Clear error message
- Invalid policy template: Show CUE compilation error
- Policy conflicts: Detect and warn
- Application not found: Offer to use `-f` flag for file input

---

## Implementation Plan

### Phase 1: `vela policy view` (simpler, no dry-run logic)
**Files to create/modify**:
- `/workspaces/development/kubevela/references/cli/policy.go` (new)
- `/workspaces/development/kubevela/references/cli/cli.go` (register command)

**Dependencies**:
- Application status reading
- ConfigMap reading
- Table formatting (existing utilities)
- Interactive selection (survey/v2)

**Estimated complexity**: Low (mostly UI/formatting)

### Phase 2: `vela policy dry-run` (complex, requires simulation logic)
**Files to create/modify**:
- `/workspaces/development/kubevela/references/cli/policy.go` (extend)
- Factor out reusable logic from `policy_transforms.go` if needed

**Dependencies**:
- Application loading
- Policy discovery
- Transform application (reuse controller logic)
- Diff computation
- Output formatting

**Estimated complexity**: Medium-High (simulation logic, multiple modes)

### Phase 3: Integration & Testing
- Add tests in `policy_test.go`
- Update CLI documentation
- Add examples to README

---

## Open Questions

1. **Application source**: Should dry-run support `-f <file>` for Applications not yet in cluster?
2. **Policy source**: Should we support testing policies from files before deploying them?
3. **Conflict detection**: How aggressive should we be in detecting policy conflicts?
4. **Performance**: For large Applications with many policies, should we add progress indicators?

---

## Future Enhancements

1. **Watch Mode**: `vela policy view my-app --watch` - Live updates as policies change
2. **Compare Mode**: `vela policy diff app1 app2` - Compare policy effects across apps
3. **Export**: `vela policy view my-app --export > report.html` - Generate HTML report
4. **Validation**: `vela policy validate <policy-file>` - Validate policy syntax before applying
