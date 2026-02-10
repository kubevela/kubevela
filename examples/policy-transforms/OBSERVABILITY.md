# Global Policy Observability

One of the key challenges with runtime manipulation is understanding **what changed** and **which policy changed it**. This guide shows how to debug and trace policy effects.

## Viewing Applied Policies

### Basic Status Check

```bash
# View which global policies were applied
kubectl get app my-app -o jsonpath='{.status.appliedGlobalPolicies}' | jq
```

Example output:
```json
[
  {
    "name": "security-hardening",
    "namespace": "vela-system",
    "applied": true,
    "addedLabels": {
      "security.platform.io/scanned": "true",
      "security.platform.io/minimum-tls": "1.2"
    },
    "addedAnnotations": {
      "security.platform.io/scan-date": "2024-01-01",
      "security.platform.io/scan-tool": "trivy"
    },
    "additionalContext": {
      "securityPolicyVersion": "v2.1.0"
    },
    "specModified": false
  },
  {
    "name": "platform-labels",
    "namespace": "vela-system",
    "applied": true,
    "addedLabels": {
      "platform.io/managed-by": "kubevela",
      "platform.io/region": "us-west-2"
    },
    "specModified": false
  },
  {
    "name": "tenant-config",
    "namespace": "vela-system",
    "applied": false,
    "reason": "enabled=false"
  }
]
```

## Understanding Status Fields

### Field Reference

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Policy name |
| `namespace` | string | Policy namespace (vela-system or app namespace) |
| `applied` | bool | Whether policy was applied |
| `reason` | string | Why policy was skipped (if `applied=false`) |
| `addedLabels` | map | **NEW**: Labels this policy added |
| `addedAnnotations` | map | **NEW**: Annotations this policy added |
| `additionalContext` | map | **NEW**: Context data this policy provided |
| `specModified` | bool | **NEW**: Whether policy modified Application spec |

## Debugging Scenarios

### "Where did this label come from?"

```bash
# Find which policy added a specific label
kubectl get app my-app -o json | \
  jq '.status.appliedGlobalPolicies[] | select(.addedLabels["team"] != null) | {policy: .name, namespace: .namespace, value: .addedLabels["team"]}'
```

Output:
```json
{
  "policy": "platform-labels",
  "namespace": "vela-system",
  "value": "platform-team"
}
```

### "Which policies modified my spec?"

```bash
# List all policies that modified the spec
kubectl get app my-app -o json | \
  jq '.status.appliedGlobalPolicies[] | select(.specModified == true) | {policy: .name, namespace: .namespace}'
```

Output:
```json
{
  "policy": "inject-sidecar",
  "namespace": "vela-system"
}
{
  "policy": "resource-limits",
  "namespace": "production"
}
```

### "What context data is available?"

```bash
# View all additionalContext from policies
kubectl get app my-app -o json | \
  jq '.status.appliedGlobalPolicies[] | select(.additionalContext != null) | {policy: .name, context: .additionalContext}'
```

Output:
```json
{
  "policy": "security-hardening",
  "context": {
    "securityPolicyVersion": "v2.1.0",
    "complianceLevel": "pci-dss"
  }
}
```

### "Why didn't a policy apply?"

```bash
# Find skipped policies and reasons
kubectl get app my-app -o json | \
  jq '.status.appliedGlobalPolicies[] | select(.applied == false) | {policy: .name, reason: .reason}'
```

Output:
```json
{
  "policy": "tenant-config",
  "reason": "enabled=false"
}
{
  "policy": "dev-environment",
  "reason": "enabled=false"
}
```

## Complete Audit Trail

Create a shell function for comprehensive policy audit:

```bash
# Add to ~/.bashrc or ~/.zshrc
vela-policy-audit() {
    local app=$1
    local namespace=${2:-default}

    echo "=== Policy Audit for $app (namespace: $namespace) ==="
    echo

    echo "📋 Applied Policies:"
    kubectl get app $app -n $namespace -o json | \
        jq -r '.status.appliedGlobalPolicies[] | select(.applied == true) | "  ✅ \(.name) (from \(.namespace))"'

    echo
    echo "⏭️  Skipped Policies:"
    kubectl get app $app -n $namespace -o json | \
        jq -r '.status.appliedGlobalPolicies[] | select(.applied == false) | "  ⏭️  \(.name): \(.reason)"'

    echo
    echo "🏷️  Labels Added by Policies:"
    kubectl get app $app -n $namespace -o json | \
        jq -r '.status.appliedGlobalPolicies[] | select(.addedLabels != null) | "  Policy: \(.name)\n    Labels: \(.addedLabels | to_entries | map("      \(.key)=\(.value)") | join("\n"))"'

    echo
    echo "📝 Annotations Added by Policies:"
    kubectl get app $app -n $namespace -o json | \
        jq -r '.status.appliedGlobalPolicies[] | select(.addedAnnotations != null) | "  Policy: \(.name)\n    Annotations: \(.addedAnnotations | to_entries | map("      \(.key)=\(.value)") | join("\n"))"'

    echo
    echo "🔧 Spec Modifications:"
    kubectl get app $app -n $namespace -o json | \
        jq -r '.status.appliedGlobalPolicies[] | select(.specModified == true) | "  ⚠️  \(.name) modified Application spec"'

    echo
    echo "📦 Additional Context:"
    kubectl get app $app -n $namespace -o json | \
        jq -r '.status.appliedGlobalPolicies[] | select(.additionalContext != null) | "  Policy: \(.name)\n    Context: \(.additionalContext | to_entries | map("      \(.key)=\(.value)") | join("\n"))"'
}

# Usage:
# vela-policy-audit my-app
# vela-policy-audit my-app production
```

Example output:
```
=== Policy Audit for my-app (namespace: default) ===

📋 Applied Policies:
  ✅ security-hardening (from vela-system)
  ✅ platform-labels (from vela-system)

⏭️  Skipped Policies:
  ⏭️  tenant-config: enabled=false

🏷️  Labels Added by Policies:
  Policy: security-hardening
    Labels:
      security.platform.io/scanned=true
      security.platform.io/minimum-tls=1.2
  Policy: platform-labels
    Labels:
      platform.io/managed-by=kubevela
      platform.io/region=us-west-2

📝 Annotations Added by Policies:
  Policy: security-hardening
    Annotations:
      security.platform.io/scan-date=2024-01-01

🔧 Spec Modifications:
  (none)

📦 Additional Context:
  Policy: security-hardening
    Context:
      securityPolicyVersion=v2.1.0
```

## Kubernetes Events

In addition to status fields, policies also emit Kubernetes Events:

```bash
# View policy-related events
kubectl get events --field-selector involvedObject.name=my-app

# Filter for global policy events
kubectl get events --field-selector involvedObject.name=my-app | grep -i "globalpolicy"
```

Example events:
```
LAST SEEN   TYPE     REASON                 MESSAGE
2m          Normal   GlobalPolicyApplied    Applied global policy security-hardening from namespace vela-system
2m          Normal   GlobalPolicyApplied    Applied global policy platform-labels from namespace vela-system
2m          Normal   GlobalPolicySkipped    Skipped global policy tenant-config: enabled=false
```

## Troubleshooting Workflows

### 1. Unexpected Label/Annotation

**Problem**: "My Application has a label I didn't add"

**Solution**:
```bash
# Find the culprit
kubectl get app my-app -o json | \
  jq '.status.appliedGlobalPolicies[] | select(.addedLabels["mysterious-label"] != null) | {policy: .name, namespace: .namespace}'

# View the policy definition
kubectl get policydefinition <policy-name> -n <namespace> -o yaml
```

### 2. Policy Not Applying

**Problem**: "My global policy isn't being applied"

**Checklist**:
```bash
# 1. Is policy marked as global?
kubectl get policydefinition my-policy -n vela-system -o jsonpath='{.spec.global}'

# 2. Is Application opting out?
kubectl get app my-app -o jsonpath='{.metadata.annotations.policy\.oam\.dev/skip-global}'

# 3. Check applied policies status
kubectl get app my-app -o json | jq '.status.appliedGlobalPolicies[] | select(.name == "my-policy")'

# 4. Check feature gate
kubectl get deployment vela-core -n vela-system -o yaml | grep feature-gates
```

### 3. Conflicting Policies

**Problem**: "Two policies are setting the same label to different values"

**Solution**:
```bash
# Find all policies setting a specific label
kubectl get app my-app -o json | \
  jq '.status.appliedGlobalPolicies[] | select(.addedLabels["team"] != null) | {policy: .name, priority: .priority, value: .addedLabels["team"]}'

# The LAST policy in the list wins (highest priority + alphabetical order)
```

### 4. Context Data Not Available in Workflow

**Problem**: "Workflow can't access policy's additionalContext"

**Solution**:
```bash
# Check if policy provided the context
kubectl get app my-app -o json | \
  jq '.status.appliedGlobalPolicies[] | select(.additionalContext != null)'

# Context is stored in Go context and passed to workflow
# Not all workflow steps may support context.custom access
# Check workflow step implementation
```

## Best Practices

### 1. Use Descriptive Names

**Good**:
```yaml
addedLabels:
  security.platform.io/scanned: "true"
  platform.io/managed-by: "kubevela"
```

**Bad**:
```yaml
addedLabels:
  x: "true"
  mgr: "vela"
```

### 2. Document Your Policies

Add annotations to PolicyDefinitions:
```yaml
metadata:
  annotations:
    policy.oam.dev/description: "Adds required security labels for compliance"
    policy.oam.dev/owner: "security-team@company.com"
    policy.oam.dev/adds-labels: "security.platform.io/*"
```

### 3. Monitor Policy Impact

Set up monitoring for:
- Number of Applications affected by each policy
- Policies that frequently fail (enabled=false)
- Policies that modify specs (higher risk)

### 4. Use Namespaced Policies for Testing

Before deploying to vela-system:
```bash
# Test in specific namespace first
kubectl apply -f my-policy.yaml -n test-namespace

# Check Applications in that namespace
kubectl get app -n test-namespace -o json | jq '.items[].status.appliedGlobalPolicies'

# If working well, promote to vela-system
kubectl apply -f my-policy.yaml -n vela-system
```

## Integration with GitOps

### ArgoCD/Flux Drift Detection

The status fields show what policies changed, but ArgoCD/Flux will see drift since changes aren't in Git.

**Option 1: Ignore policy-added labels/annotations**
```yaml
# ArgoCD Application
spec:
  ignoreDifferences:
  - group: core.oam.dev
    kind: Application
    jsonPointers:
    - /metadata/labels/security.platform.io
    - /metadata/annotations/policy.oam.dev
```

**Option 2: Document in Git (recommended)**
```yaml
# my-app.yaml (in Git)
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: my-app
  labels:
    # Added by policy: security-hardening (vela-system)
    security.platform.io/scanned: "true"
```

## Viewing Spec Diffs (✅ Implemented)

When global policies modify the Application spec, KubeVela stores detailed JSON Merge Patch diffs in a ConfigMap for debugging and auditing.

### ConfigMap Structure

Policy diffs are stored in a ConfigMap named `{app-name}-policy-diffs` with:
- **Labels**: Standard KubeVela labels (`app.oam.dev/name`, `app.oam.dev/namespace`, `app.oam.dev/uid`)
- **Keys**: Sequence-prefixed format `001-policy-name`, `002-policy-name` to preserve execution order
- **Values**: JSON Merge Patch (RFC 7386) showing what changed

### Basic Diff Viewing

```bash
# Check if policy diffs exist
kubectl get app my-app -o jsonpath='{.status.policyDiffsConfigMap}'
# Output: my-app-policy-diffs

# List all policy diffs in execution order
kubectl get configmap my-app-policy-diffs -o jsonpath='{.data}' | jq 'keys'
# Output: ["001-inject-sidecar", "002-resource-limits"]

# View specific policy diff
kubectl get configmap my-app-policy-diffs -o jsonpath='{.data.001-inject-sidecar}' | jq
```

### Example Diff Output

```json
{
  "components": [
    null,
    {
      "name": "monitoring-sidecar",
      "type": "webservice",
      "properties": {
        "image": "monitoring:latest",
        "cpu": "100m"
      }
    }
  ]
}
```

This shows the `inject-sidecar` policy added a new component at index 1.

### Finding All Applications with Policy Diffs

```bash
# List all policy-diffs ConfigMaps
kubectl get configmaps -l "app.oam.dev/policy-diffs=true"

# List ConfigMaps for a specific app
kubectl get configmaps -l "app.oam.dev/name=my-app"
```

### Diff Interpretation

JSON Merge Patch format:
- **New fields**: Added to object (e.g., `"components": [null, {...}]` adds component at index 1)
- **Modified fields**: Replaced with new value (e.g., `"replicas": 3` changes replicas)
- **`null` values**: Indicate no change at that position

### Combining Status + Diffs

Get complete picture of policy effects:

```bash
# Get metadata about policies
kubectl get app my-app -o json | jq '.status.appliedGlobalPolicies[]'

# Get actual spec changes
kubectl get configmap my-app-policy-diffs -o json | jq '.data'
```

## Future Enhancements

Planned improvements to observability:

### 1. CLI Tools (High Priority)

**`vela policy view <app>`**
- Interactive viewer for policy changes
- Shows before/after comparison
- Highlights which policies made which changes
- Similar UX to `vela debug`

Example usage:
```bash
$ vela policy view my-app

Applied Global Policies (3):
┌──────────────────────┬─────────────┬──────────┬──────────────┐
│ Policy               │ Namespace   │ Sequence │ Spec Changed │
├──────────────────────┼─────────────┼──────────┼──────────────┤
│ inject-sidecar       │ vela-system │ 1        │ Yes          │
│ resource-limits      │ vela-system │ 2        │ Yes          │
│ platform-labels      │ vela-system │ 3        │ No           │
└──────────────────────┴─────────────┴──────────┴──────────────┘

Select a policy to view changes: [Use arrows to move, type to filter]
> inject-sidecar (added monitoring-sidecar component)
  resource-limits (modified resource constraints)
```

**`vela policy dry-run <app> --policies <policy1> <policy2> <policyN>`**
- Preview policy effects before applying
- Test policy changes without creating Application
- Validate policy templates and detect conflicts

Example usage:
```bash
$ vela policy dry-run my-app --policies inject-sidecar resource-limits

Dry-run simulation:
✓ Policy: inject-sidecar (priority: 100)
  - Added component: monitoring-sidecar
  - Added label: sidecar.io/injected=true

✓ Policy: resource-limits (priority: 50)
  - Modified: components[0].properties.resources.limits.cpu → 500m
  - Modified: components[0].properties.resources.limits.memory → 512Mi

⚠ Warning: No conflicts detected
```

### 2. Additional Tools

1. **Metrics**: Prometheus metrics for policy application
2. **Web UI**: Visual policy impact dashboard in VelaUX
3. **Policy Audit**: `vela policy audit <app>` - complete audit trail

## Summary

With the enhanced status fields, you can now:

✅ **Trace** every label/annotation to its source policy
✅ **Debug** why policies were skipped
✅ **Audit** what context data policies provided
✅ **Monitor** which policies modified specs
✅ **Understand** the complete policy chain for any Application

This makes runtime manipulation **observable and debuggable**, addressing one of the key concerns with implicit transformations.
