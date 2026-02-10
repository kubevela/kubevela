# Policy CLI Command Output Examples

This document shows realistic output examples for the proposed `vela policy` commands.

## Command: `vela policy view my-app`

### Example 1: Application with Multiple Policies

```bash
$ vela policy view my-app
```

**Output:**

```
Applied Global Policies: 3 applied, 1 skipped

┌─────┬──────────────────────┬─────────────┬──────────┬──────────┬──────┬────────┬────────┬─────────┐
│ Seq │ Policy               │ Namespace   │ Priority │ Applied  │ Spec │ Labels │ Annot. │ Context │
├─────┼──────────────────────┼─────────────┼──────────┼──────────┼──────┼────────┼────────┼─────────┤
│ 1   │ inject-sidecar       │ vela-system │ 100      │ ✓ Yes    │ ✓ 1  │ ✓ 1    │ ✗ 0    │ ✗ 0     │
│ 2   │ resource-limits      │ vela-system │ 50       │ ✓ Yes    │ ✓ 1  │ ✗ 0    │ ✓ 1    │ ✓ Yes   │
│ 3   │ platform-labels      │ vela-system │ 10       │ ✓ Yes    │ ✗ 0  │ ✓ 2    │ ✗ 0    │ ✗ 0     │
│ -   │ tenant-isolation     │ vela-system │ 5        │ ✗ No     │ -    │ -      │ -      │ -       │
└─────┴──────────────────────┴─────────────┴──────────┴──────────┴──────┴────────┴────────┴─────────┘

Legend:
  Spec:    Number of spec changes (or ✗ if none)
  Labels:  Number of labels added (or ✗ if none)
  Annot.:  Number of annotations added (or ✗ if none)
  Context: ✓ if additional context provided

Skipped (1):
  • tenant-isolation: enabled=false (namespace does not match tenant-* pattern)

Summary:
  Total Policies: 4 (3 applied, 1 skipped)
  Spec Changes:   2 policies
  Labels Added:   3 total
  Annotations:    1 total
  Context Data:   1 policy

Press Enter to select a policy for details, or 'q' to quit
```

### Example 2: Interactive Policy Selection

When user presses Enter, show policy selection:

```
Select a policy to view details:

  ❯ inject-sidecar (vela-system)
    resource-limits (vela-system)
    platform-labels (vela-system)

  [↑↓ to move, Enter to select, q to quit]
```

After selecting `inject-sidecar`, show category submenu:

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Policy: inject-sidecar (vela-system)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Sequence:   1
Priority:   100
Applied:    Yes

What would you like to view?

  ❯ Spec Changes (1 modification)
    Labels (1 added)
    Annotations (none)
    Additional Context (none)
    [All Details]

  [↑↓ to move, Enter to select, b to go back, q to quit]
```

After selecting `Spec Changes` (default view: Unified Diff):

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Policy: inject-sidecar > Spec Changes [Unified Diff]
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

--- Original Spec
+++ After inject-sidecar

 spec:
   components:
   - name: backend
     type: webservice
     properties:
       image: myapp:v1.0.0
+  - name: monitoring-sidecar
+    type: webservice
+    properties:
+      image: monitoring-agent:v2.1.0
+      cpu: 100m
+      memory: 128Mi
+      env:
+      - name: MONITOR_TARGET
+        value: backend

ConfigMap: my-app-policy-diffs/001-inject-sidecar

Press 't' to toggle JSON view, 'b' to go back, 'n' for next policy, 'q' to quit
```

User presses 't' to toggle to JSON Merge Patch view:

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Policy: inject-sidecar > Spec Changes [JSON Merge Patch]
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

{
  "components": [
    null,
    {
      "name": "monitoring-sidecar",
      "type": "webservice",
      "properties": {
        "image": "monitoring-agent:v2.1.0",
        "cpu": "100m",
        "memory": "128Mi",
        "env": [
          {
            "name": "MONITOR_TARGET",
            "value": "backend"
          }
        ]
      }
    }
  ]
}

Note: JSON Merge Patch (RFC 7386)
  • null values = no change at that position
  • New objects = additions
  • Replaced values = modifications

ConfigMap: my-app-policy-diffs/001-inject-sidecar

Press 't' to toggle Unified Diff, 'b' to go back, 'n' for next policy, 'q' to quit
```

After going back and selecting `Labels`:

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Policy: inject-sidecar > Labels
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Labels Added (1):

┌─────────────────────────┬─────────┐
│ Key                     │ Value   │
├─────────────────────────┼─────────┤
│ sidecar.io/injected     │ true    │
└─────────────────────────┴─────────┘

These labels are added to Application.metadata.labels

Press 'b' to go back, 'n' for next policy, 'q' to quit
```

If selecting `[All Details]` from the submenu:

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Policy: inject-sidecar (Complete Details)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Metadata:
  Namespace:  vela-system
  Sequence:   1
  Priority:   100
  Applied:    Yes

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Labels Added (1)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  sidecar.io/injected: "true"

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Annotations Added
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  (none)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Additional Context
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  (none)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Spec Changes [Unified Diff]
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

--- Original Spec
+++ After inject-sidecar

 spec:
   components:
   - name: backend
     type: webservice
     properties:
       image: myapp:v1.0.0
+  - name: monitoring-sidecar
+    type: webservice
+    properties:
+      image: monitoring-agent:v2.1.0
+      cpu: 100m
+      memory: 128Mi

ConfigMap: my-app-policy-diffs/001-inject-sidecar

Press 't' to toggle JSON view, 'b' to go back, 'n' for next policy, 'q' to quit
```

### Example 3: Viewing Policy with Multiple Labels

After selecting `platform-labels` from policy list, then selecting `Labels`:

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Policy: platform-labels > Labels
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Labels Added (2):

┌──────────────────────────┬─────────────┐
│ Key                      │ Value       │
├──────────────────────────┼─────────────┤
│ platform.io/managed-by   │ kubevela    │
│ platform.io/team         │ backend-team│
└──────────────────────────┴─────────────┘

These labels are added to Application.metadata.labels

Press 'b' to go back, 'n' for next policy, 'q' to quit
```

### Example 4: Viewing Additional Context

After selecting `resource-limits` from policy list, then selecting `Additional Context`:

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Policy: resource-limits > Additional Context
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Additional Context Data:

{
  "resourceProfile": {
    "tier": "standard",
    "burstable": true,
    "limits": {
      "cpu": "500m",
      "memory": "512Mi"
    }
  }
}

This context data is made available to workflows via:
  context.additionalContext.resourceProfile

Example usage in workflow:
  if context.additionalContext.resourceProfile.tier == "standard" {
    // Apply standard monitoring
  }

Press 'b' to go back, 'n' for next policy, 'q' to quit
```

### Example 5: No Policies Applied

```bash
$ vela policy view minimal-app
```

**Output:**

```
No global policies applied to Application 'minimal-app'

This could be because:
  • No global policies exist in vela-system or the application namespace
  • Application has annotation: policy.oam.dev/skip-global: "true"
  • Global policies feature is disabled (feature gate not enabled)

Application policies from spec: 0

To view available global policies:
  kubectl get policydefinitions -n vela-system -l 'policydefinition.oam.dev/global=true'
```

### Example 6: JSON Output

```bash
$ vela policy view my-app --output json
```

**Output:**

```json
{
  "application": "my-app",
  "namespace": "default",
  "appliedPolicies": [
    {
      "sequence": 1,
      "name": "inject-sidecar",
      "namespace": "vela-system",
      "priority": 100,
      "applied": true,
      "specModified": true,
      "addedLabels": {
        "sidecar.io/injected": "true"
      },
      "addedAnnotations": {},
      "additionalContext": null,
      "reason": ""
    },
    {
      "sequence": 2,
      "name": "resource-limits",
      "namespace": "vela-system",
      "priority": 50,
      "applied": true,
      "specModified": true,
      "addedLabels": {},
      "addedAnnotations": {
        "policy.io/resource-profile": "standard"
      },
      "additionalContext": {
        "resourceProfile": {
          "tier": "standard",
          "burstable": true
        }
      },
      "reason": ""
    },
    {
      "sequence": 3,
      "name": "platform-labels",
      "namespace": "vela-system",
      "priority": 10,
      "applied": true,
      "specModified": false,
      "addedLabels": {
        "platform.io/managed-by": "kubevela",
        "platform.io/team": "backend-team"
      },
      "addedAnnotations": {},
      "additionalContext": null,
      "reason": ""
    },
    {
      "sequence": 0,
      "name": "tenant-isolation",
      "namespace": "vela-system",
      "priority": 5,
      "applied": false,
      "specModified": false,
      "addedLabels": {},
      "addedAnnotations": {},
      "additionalContext": null,
      "reason": "enabled=false (namespace does not match tenant-* pattern)"
    }
  ],
  "policyDiffsConfigMap": "my-app-policy-diffs",
  "summary": {
    "totalDiscovered": 4,
    "applied": 3,
    "skipped": 1,
    "specModifications": 2,
    "labelsAdded": 3,
    "annotationsAdded": 1
  }
}
```

---

## Command: `vela policy dry-run my-app`

### Example 1: Full Simulation (Default Mode)

```bash
$ vela policy dry-run my-app
```

**Output:**

```
Dry-run Simulation
Application: my-app (namespace: default)
Mode: Full simulation (all policies that would apply)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Execution Plan
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Discovered policies that will be applied:

  1. inject-sidecar          (vela-system, priority: 100) [global]
  2. resource-limits         (vela-system, priority: 50)  [global]
  3. platform-labels         (vela-system, priority: 10)  [global]
  4. my-custom-policy        (from Application spec)      [app-spec]

Skipped policies:
  • tenant-isolation         (vela-system) - enabled=false

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Applying Policies...
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

[1/4] inject-sidecar
  ✓ Policy enabled
  ✓ CUE template compiled successfully

  Changes:
    Spec Modified:     Yes (1 change)
    Labels Added:      1
      • sidecar.io/injected: "true"
    Annotations Added: 0
    Context Data:      None

[2/4] resource-limits
  ✓ Policy enabled
  ✓ CUE template compiled successfully

  Changes:
    Spec Modified:     Yes (2 changes)
    Labels Added:      0
    Annotations Added: 1
      • policy.io/resource-profile: "standard"
    Context Data:      resourceProfile

[3/4] platform-labels
  ✓ Policy enabled
  ✓ CUE template compiled successfully

  Changes:
    Spec Modified:     No
    Labels Added:      2
      • platform.io/managed-by: "kubevela"
      • platform.io/team: "backend-team"
    Annotations Added: 0
    Context Data:      None

[4/4] my-custom-policy
  ✓ Policy enabled
  ✓ CUE template compiled successfully

  Changes:
    Spec Modified:     Yes (1 change)
    Labels Added:      0
    Annotations Added: 0
    Context Data:      None

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Summary
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Policies Applied:       4
Policies Skipped:       1
Spec Modifications:     3
Labels Added:           3
Annotations Added:      1

⚠ Warnings:
  • resource-limits modified the same field as my-custom-policy (components[0].properties)
    Both policies should be reviewed for conflicts

✓ No errors detected

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Final Application State
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Labels (3 total):
  platform.io/managed-by:  kubevela          (platform-labels)
  platform.io/team:        backend-team      (platform-labels)
  sidecar.io/injected:     true              (inject-sidecar)

Annotations (1 total):
  policy.io/resource-profile: standard       (resource-limits)

Additional Context:
  resourceProfile:                           (resource-limits)
    tier: standard
    burstable: true

Application Spec:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: my-app
  namespace: default
  labels:
    platform.io/managed-by: kubevela
    platform.io/team: backend-team
    sidecar.io/injected: "true"
  annotations:
    policy.io/resource-profile: standard
spec:
  components:
  - name: backend
    type: webservice
    properties:
      image: myapp:v1.0.0
      resources:
        limits:
          cpu: 500m
          memory: 512Mi
    traits:
    - type: scaler
      properties:
        replicas: 3
  - name: monitoring-sidecar
    type: webservice
    properties:
      image: monitoring-agent:v2.1.0
      cpu: 100m
      memory: 128Mi
      resources:
        limits:
          cpu: 100m
          memory: 128Mi
      env:
      - name: MONITOR_TARGET
        value: backend
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

This is a dry-run. No changes were applied to the cluster.

To view detailed diffs for each policy:
  vela policy dry-run my-app --output diff

To get JSON output for scripting:
  vela policy dry-run my-app --output json
```

### Example 2: Isolated Testing (Single Policy)

```bash
$ vela policy dry-run my-app --policies inject-sidecar
```

**Output:**

```
Dry-run Simulation
Application: my-app (namespace: default)
Mode: Isolated (testing specified policies only)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Execution Plan
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Testing policies in isolation (ignoring existing global and app policies):

  1. inject-sidecar          (vela-system) [specified]

Note: This is an isolated test. In production, this policy would run alongside:
  • 2 other global policies
  • 1 application spec policy

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Applying Policies...
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

[1/1] inject-sidecar
  ✓ Policy enabled
  ✓ CUE template compiled successfully

  Changes:
    Spec Modified:     Yes (1 change)
    Labels Added:      1
      • sidecar.io/injected: "true"
    Annotations Added: 0
    Context Data:      None

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Summary
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Policies Applied:       1
Spec Modifications:     1
Labels Added:           1
Annotations Added:      0

✓ No warnings
✓ No errors

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Final Application State
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Impact Summary:
  • +1 component (monitoring-sidecar)
  • +1 label

Labels (1 total):
  sidecar.io/injected: true                  (inject-sidecar)

Annotations:
  (none)

Additional Context:
  (none)

Spec Changes:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
--- Original Application Spec
+++ After inject-sidecar Policy

 spec:
   components:
   - name: backend
     type: webservice
     properties:
       image: myapp:v1.0.0
+  - name: monitoring-sidecar
+    type: webservice
+    properties:
+      image: monitoring-agent:v2.1.0
+      cpu: 100m
+      memory: 128Mi
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

### Example 3: Additive Testing (With Global Policies)

```bash
$ vela policy dry-run my-app --policies new-security-policy --include-global-policies
```

**Output:**

```
Dry-run Simulation
Application: my-app (namespace: default)
Mode: Additive (specified policies + existing global policies)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Execution Plan
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  1. inject-sidecar          (vela-system, priority: 100) [global]
  2. resource-limits         (vela-system, priority: 50)  [global]
  3. platform-labels         (vela-system, priority: 10)  [global]
  4. new-security-policy     (vela-system)                [specified - TESTING]

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Applying Policies...
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

[1/4] inject-sidecar [global]
  ✓ Applied (see full simulation mode for details)

[2/4] resource-limits [global]
  ✓ Applied (see full simulation mode for details)

[3/4] platform-labels [global]
  ✓ Applied (see full simulation mode for details)

[4/4] new-security-policy [TESTING]
  ✓ Policy enabled
  ✓ CUE template compiled successfully

  Changes:
    Spec Modified:     Yes (2 changes)
    Labels Added:      1
      • security.io/hardened: "true"
    Annotations Added: 2
      • security.io/scan-date: "2024-02-09"
      • security.io/scan-tool: "trivy"
    Context Data:      None

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Summary
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Policies Applied:       4 (3 global + 1 test)
Spec Modifications:     4
Labels Added:           4
Annotations Added:      3

✓ No conflicts detected with existing policies
✓ new-security-policy integrates cleanly

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Final Application State
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Labels (4 total):
  platform.io/managed-by:  kubevela          (platform-labels)
  platform.io/team:        backend-team      (platform-labels)
  sidecar.io/injected:     true              (inject-sidecar)
  security.io/hardened:    true              (new-security-policy)

Annotations (3 total):
  policy.io/resource-profile: standard       (resource-limits)
  security.io/scan-date:      2024-02-09     (new-security-policy)
  security.io/scan-tool:      trivy          (new-security-policy)

Additional Context:
  resourceProfile:                           (resource-limits)
    tier: standard
    burstable: true

Note: new-security-policy added 2 spec changes, 1 label, and 2 annotations
      All changes integrate cleanly with existing global policies

To deploy this policy:
  kubectl apply -f new-security-policy.yaml
```

### Example 4: Viewing Only Labels/Annotations/Context

If you only care about metadata changes (not spec):

```bash
$ vela policy dry-run my-app --output summary
```

**Output:**

```
Dry-run Simulation
Application: my-app (namespace: default)
Mode: Full simulation (all policies that would apply)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Summary
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Policies Applied:       3
Policies Skipped:       1
Spec Modifications:     2
Labels Added:           3
Annotations Added:      1

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Labels
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

┌───────────────────────────┬──────────────┬──────────────────────┐
│ Key                       │ Value        │ Added By             │
├───────────────────────────┼──────────────┼──────────────────────┤
│ platform.io/managed-by    │ kubevela     │ platform-labels      │
│ platform.io/team          │ backend-team │ platform-labels      │
│ sidecar.io/injected       │ true         │ inject-sidecar       │
└───────────────────────────┴──────────────┴──────────────────────┘

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Annotations
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

┌──────────────────────────────┬──────────┬──────────────────────┐
│ Key                          │ Value    │ Added By             │
├──────────────────────────────┼──────────┼──────────────────────┤
│ policy.io/resource-profile   │ standard │ resource-limits      │
└──────────────────────────────┴──────────┴──────────────────────┘

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Additional Context
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

resourceProfile (from resource-limits):
{
  "tier": "standard",
  "burstable": true,
  "limits": {
    "cpu": "500m",
    "memory": "512Mi"
  }
}

Available in workflows as:
  context.additionalContext.resourceProfile

To see spec changes:
  vela policy dry-run my-app
  vela policy dry-run my-app --output diff
```

### Example 5: Diff Output Format

```bash
$ vela policy dry-run my-app --policies inject-sidecar --output diff
```

**Output:**

```diff
--- Original Application Spec
+++ After Policy: inject-sidecar

 apiVersion: core.oam.dev/v1beta1
 kind: Application
 metadata:
   name: my-app
   namespace: default
+  labels:
+    sidecar.io/injected: "true"
 spec:
   components:
   - name: backend
     type: webservice
     properties:
       image: myapp:v1.0.0
+  - name: monitoring-sidecar
+    type: webservice
+    properties:
+      image: monitoring-agent:v2.1.0
+      cpu: 100m
+      memory: 128Mi
+      env:
+      - name: MONITOR_TARGET
+        value: backend
```

### Example 6: Error Cases

#### Policy Not Found

```bash
$ vela policy dry-run my-app --policies non-existent-policy
```

**Output:**

```
Error: Policy not found

Could not find PolicyDefinition: non-existent-policy

Searched in:
  • Namespace: vela-system
  • Namespace: default

Available policies in vela-system:
  • inject-sidecar (global, priority: 100)
  • resource-limits (global, priority: 50)
  • platform-labels (global, priority: 10)

Available policies in default:
  • none

Did you mean one of these?
  • inject-sidecar
  • resource-limits

To list all policies:
  kubectl get policydefinitions -A
```

#### CUE Compilation Error

```bash
$ vela policy dry-run my-app --policies broken-policy
```

**Output:**

```
Dry-run Simulation
Application: my-app (namespace: default)
Mode: Isolated (testing specified policies only)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Execution Plan
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  1. broken-policy           (vela-system) [specified]

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Applying Policies...
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

[1/1] broken-policy
  ✗ CUE compilation failed

Error: failed to compile policy template

  transforms.spec.value.components[0]: reference "nonExistentVariable" not found:
      template.cue:15:9

  15 |         image: nonExistentVariable
     |                ^

  Available context variables:
    • context.application
    • context.name
    • context.namespace
    • parameter

Policy template location:
  PolicyDefinition: broken-policy
  Namespace: vela-system

To fix: Edit the PolicyDefinition and correct the CUE template

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Summary
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Policies Applied:       0
Errors:                 1

✗ Dry-run failed - fix policy template errors before deploying
```

### Example 7: JSON Output (for CI/CD)

```bash
$ vela policy dry-run my-app --policies inject-sidecar --output json
```

**Output:**

```json
{
  "application": "my-app",
  "namespace": "default",
  "mode": "isolated",
  "executionPlan": [
    {
      "sequence": 1,
      "policyName": "inject-sidecar",
      "policyNamespace": "vela-system",
      "priority": 100,
      "source": "specified"
    }
  ],
  "results": [
    {
      "sequence": 1,
      "policyName": "inject-sidecar",
      "policyNamespace": "vela-system",
      "enabled": true,
      "applied": true,
      "error": null,
      "specModified": true,
      "addedLabels": {
        "sidecar.io/injected": "true"
      },
      "addedAnnotations": {},
      "additionalContext": null,
      "diff": {
        "components": [
          null,
          {
            "name": "monitoring-sidecar",
            "type": "webservice",
            "properties": {
              "image": "monitoring-agent:v2.1.0",
              "cpu": "100m",
              "memory": "128Mi"
            }
          }
        ]
      }
    }
  ],
  "summary": {
    "policiesApplied": 1,
    "policiesSkipped": 0,
    "specModifications": 1,
    "labelsAdded": 1,
    "annotationsAdded": 0,
    "errors": 0,
    "warnings": 0
  },
  "warnings": [],
  "errors": [],
  "finalSpec": {
    "components": [
      {
        "name": "backend",
        "type": "webservice",
        "properties": {
          "image": "myapp:v1.0.0"
        }
      },
      {
        "name": "monitoring-sidecar",
        "type": "webservice",
        "properties": {
          "image": "monitoring-agent:v2.1.0",
          "cpu": "100m",
          "memory": "128Mi"
        }
      }
    ]
  }
}
```

---

## Color Coding (Terminal with Color Support)

When running in a terminal with color support:

- **Green (✓)**: Success indicators, applied policies
- **Red (✗)**: Errors, failed policies
- **Yellow (⚠)**: Warnings, skipped policies
- **Blue**: Headers, section dividers
- **Cyan**: Policy names
- **Magenta**: Field names (labels, annotations)
- **Gray**: Metadata (sequence numbers, priorities)

---

## Progress Indicators (for Slow Operations)

When applying many policies or large Applications:

```
Dry-run Simulation
Application: large-app (namespace: production)

Loading Application...         [████████████████████] 100% (1.2s)
Discovering policies...        [████████████████████] 100% (0.8s)
Applying 15 policies...        [████████████░░░░░░░░]  65% (3.4s)
  • inject-sidecar             ✓
  • resource-limits            ✓
  • platform-labels            ✓
  • security-hardening         ✓
  • backup-policy              ✓
  • monitoring-config          ✓
  • logging-policy             ✓
  • network-policy             ✓
  • compliance-check           ✓
  • data-classification        ⏳ (running...)
```

---

## Comparison: Side-by-Side View

Future enhancement for `--show-diffs` flag:

```bash
$ vela policy dry-run my-app --policies inject-sidecar --show-diffs
```

**Output:**

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Side-by-Side Comparison: inject-sidecar
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Before Policy                           After Policy
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
spec:                                   spec:
  components:                             components:
  - name: backend                         - name: backend
    type: webservice                        type: webservice
    properties:                             properties:
      image: myapp:v1.0.0                     image: myapp:v1.0.0
                                            - name: monitoring-sidecar        [+]
                                              type: webservice                [+]
                                              properties:                     [+]
                                                image: monitoring-agent:v2    [+]

Labels:                                 Labels:
  <none>                                  sidecar.io/injected: "true"       [+]
```
