# Application-Scoped Policy Transforms

This feature enables PolicyDefinitions to transform Applications at runtime before they are parsed into resources.

## Overview

Application-scoped policies are a new type of PolicyDefinition that can modify the Application CR in-memory before it goes through the normal parsing and deployment flow. This enables powerful use cases like:

- Injecting sidecars, environment variables, or configuration into all components
- Adding labels, annotations, or metadata based on policy rules
- Conditionally modifying Application specs based on parameters
- Replacing entire Application specs for advanced deployment strategies (canary, blue-green)
- Passing additional context to workflow steps

## How It Works

1. **Policy Execution Point**: Application-scoped policies run immediately after Application validation but before parsing (after `handleFinalizers` and `handleWorkflowRestartAnnotation`, before `GenerateAppFile`)

2. **In-Memory Modifications**: All transforms are applied to the in-memory Application object only - changes are NOT persisted to the cluster

3. **Context Availability**: The policy CUE template has access to:
   - `parameter`: Policy parameters from the Application spec
   - `context.application`: The full Application object being processed

4. **Output Fields**:
   - `enabled` (bool, default true): Whether to apply this policy
   - `transforms`: Object containing transform operations
   - `additionalContext`: Data to pass to workflow steps (available as `context.custom`)

## PolicyDefinition Structure

```yaml
apiVersion: core.oam.dev/v1beta1
kind: PolicyDefinition
metadata:
  name: my-transform-policy
spec:
  # REQUIRED: Set scope to Application
  scope: Application
  schematic:
    cue:
      template: |
        parameter: {
          # Define your policy parameters
        }

        # Optional: Conditional application (default: true)
        enabled: true | bool

        # Optional: Transform operations
        transforms: {
          # Spec transform (supports "replace" or "merge")
          spec?: {
            type: "replace" | "merge"
            value: {...}
          }

          # Labels transform (only supports "merge" for safety)
          labels?: {
            type: "merge"
            value: {[string]: string}
          }

          # Annotations transform (only supports "merge" for safety)
          annotations?: {
            type: "merge"
            value: {[string]: string}
          }
        }

        # Optional: Additional context for workflow steps
        additionalContext: {
          # Any data you want to pass to workflow
        }
```

## Transform Operations

### Spec Transforms

**Merge** (default): Deep merges the provided value into the existing spec
```cue
transforms: spec: {
  type: "merge"
  value: {
    components: [{
      properties: {
        env: [{name: "NEW_VAR", value: "value"}]
      }
    }]
  }
}
```

**Replace**: Completely replaces the Application spec
```cue
transforms: spec: {
  type: "replace"
  value: {
    components: [/* new components */]
  }
}
```

### Labels and Annotations

Only **merge** operation is supported for safety (prevents removing platform-critical metadata):

```cue
transforms: labels: {
  type: "merge"
  value: {
    "team": "platform"
    "environment": "production"
  }
}
```

## Using additionalContext in Workflows

Data from `additionalContext` is stored in the Go context and can be accessed in workflow steps:

```yaml
workflow:
  steps:
    - name: my-step
      type: apply-object
      properties:
        value:
          # Access via context.custom (if workflow supports it)
          metadata:
            annotations:
              policy-data: context.custom.policyApplied
```

Note: The exact mechanism for accessing `context.custom` in workflow steps may depend on workflow step implementation.

## Global PolicyDefinitions

Global policies automatically apply to Applications without being explicitly referenced in the Application spec. This feature requires the `EnableGlobalPolicies` feature gate.

**Enable the feature**:
```bash
--feature-gates=EnableGlobalPolicies=true
```

### Marking a Policy as Global

```yaml
spec:
  global: true  # Mark as global
  priority: 100  # Control execution order (higher runs first)
  scope: Application  # Must be Application-scoped
```

### Scope Rules

1. **vela-system namespace**: Applies to ALL Applications in ALL namespaces (cluster-wide standards)
2. **Other namespaces**: Applies only to Applications in that namespace (namespace standards)

### Execution Order

Global policies execute BEFORE explicit policies. Within each tier, policies are ordered by:
1. **Priority** (descending): Higher priority values run first
2. **Name** (alphabetical): Policies with the same priority are ordered alphabetically

**Full execution order**:
1. Global policies from Application's namespace (namespace-specific, sorted by priority+name)
2. Global policies from vela-system (cluster-wide, sorted by priority+name)
3. Explicit policies from Application spec (user-defined order)

**Rationale**: Namespace policies run first so they can override cluster-wide defaults. Explicit policies run last so users have the final say.

### Priority Field

Use the `Priority` field to control execution order:

```yaml
spec:
  global: true
  priority: 100  # Higher values run first (default: 0)
```

**Example use cases**:
- Priority 1000: Critical security policies (must run first)
- Priority 100: Standard platform labels/annotations
- Priority 50: Optional monitoring/observability
- Priority 0: Low-priority defaults

See `global-policy-priority-example.yaml` for a complete example.

### Deduplication

If a global policy with the same name exists in both vela-system and a namespace:
- **Namespace version WINS**: The namespace version completely replaces the vela-system version
- **Use case**: Namespaces can override cluster-wide policies with their own implementation
- **Example**: `auto-add-labels` in vela-system adds `env=prod`, but namespace `dev` overrides with `env=dev`

See `global-policy-namespace-override-example.yaml` for a complete example.

### Opting Out

Applications can opt-out of ALL global policies:

```yaml
metadata:
  annotations:
    policy.oam.dev/skip-global: "true"
```

See `application-opt-out-example.yaml` for a complete example.

### Conditional Application

Use the `enabled` field to conditionally apply global policies:

```cue
import "strings"

# Only apply to tenant namespaces
enabled: strings.HasPrefix(context.application.metadata.namespace, "tenant-")
```

See `global-policy-tenant-config.yaml` for a complete example.

### Observability

Check which global policies were applied:

```bash
kubectl get app my-app -o jsonpath='{.status.appliedGlobalPolicies}'
```

Output shows:
- Which global policies were discovered
- Whether they were applied or skipped
- Reason for skipping (e.g., `enabled=false`)

Kubernetes Events are also emitted for each global policy application or skip.

### Important Constraints

1. **Mutual Exclusivity**: Global policies CANNOT be explicitly referenced in Application specs
   - ❌ Invalid: Referencing a global policy in `spec.policies`
   - ✅ Valid: Let global policies auto-apply, or opt-out with annotation

2. **Parameters Must Have Defaults**: Global policies can have parameters, but ALL must have default values
   - ❌ Invalid: `parameter: { env: string }` (no default)
   - ❌ Invalid: `parameter: { env?: string }` (optional but no default)
   - ✅ Valid: `parameter: { env: *"prod" | string }` (has default)
   - ✅ Valid: `parameter: {}` (empty)
   - Use `context.application` to access application data dynamically

3. **Feature Gate Required**: Must enable `EnableGlobalPolicies` feature gate

### Use Cases

- **Platform Standards**: Add labels, annotations, or metadata to all Applications
- **Governance**: Enforce backup policies, monitoring, security standards
- **Multi-tenancy**: Apply tenant-specific configuration based on namespace
- **Environment Standards**: Production apps get different policies than dev/staging
- **Security Compliance**: Automatically enforce security policies across all applications

## Examples

### Example 1: Add Environment Labels

See `policy-definition-add-labels.yaml` - Adds environment and team labels to Applications.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
spec:
  policies:
    - name: env-labels
      type: add-environment-labels
      properties:
        environment: production
        team: platform-team
```

Result: Application will have labels `environment=production`, `team=platform-team`, and `managed-by=kubevela` added.

### Example 2: Inject Monitoring Sidecar

See `policy-definition-inject-sidecar.yaml` - Conditionally injects a monitoring sidecar into all components.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
spec:
  policies:
    - name: monitoring
      type: inject-monitoring-sidecar
      properties:
        enableMonitoring: true
        sidecarImage: "my-monitoring-agent:v2"
```

Result: Each component will have a monitoring sidecar added if enableMonitoring=true.

### Example 3: Canary Deployment Override

See `policy-definition-replace-spec.yaml` - Replaces the entire spec to create a canary deployment.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
spec:
  policies:
    - name: canary
      type: canary-deployment-override
      properties:
        canaryEnabled: true
        canaryReplicas: 1
```

Result: Application spec is replaced with a canary configuration.

## Testing

To test this feature:

1. Create a PolicyDefinition with `scope: Application`
2. Create an Application that references the policy
3. Apply both to your cluster
4. Check the Application controller logs for policy application messages
5. Verify the transforms were applied by checking deployed resources

## Safety Constraints

1. **Labels/Annotations**: Only support `merge` operation to prevent accidentally removing platform-critical metadata

2. **Spec Transforms**: Support both `replace` and `merge`, but use with caution:
   - `replace` completely overwrites the spec
   - `merge` is safer for incremental modifications

3. **Type Safety**: The CUE template enforces proper structure for transforms

4. **Conditional Execution**: Use `enabled: false` to skip policy application

## Debugging & Observability

### Viewing Applied Policies

Check which global policies were applied:

```bash
kubectl get app my-app -o jsonpath='{.status.appliedGlobalPolicies}' | jq
```

This shows:
- Which policies were discovered and applied
- Labels, annotations, and context added by each policy
- Whether each policy modified the Application spec
- Why policies were skipped (if `applied: false`)

### Viewing Spec Changes (✅ New Feature)

When global policies modify the Application spec, KubeVela stores detailed diffs in a ConfigMap for auditing:

```bash
# Check if spec diffs exist
kubectl get app my-app -o jsonpath='{.status.policyDiffsConfigMap}'
# Output: my-app-policy-diffs

# View diff for first policy
kubectl get configmap my-app-policy-diffs -o jsonpath='{.data.001-policy-name}' | jq
```

The ConfigMap contains:
- **Sequence-prefixed keys**: `001-policy-name`, `002-policy-name` (preserves execution order)
- **JSON Merge Patch diffs**: Shows exactly what each policy changed
- **Standard labels**: Discoverable with `kubectl get cm -l "app.oam.dev/policy-diffs=true"`

### Future: CLI Tools

Planned CLI commands for better UX:
- `vela policy view <app>` - Interactive viewer for policy changes with before/after comparison
- `vela policy dry-run <app> --policies <p1> <p2>` - Preview policy effects before applying

See [OBSERVABILITY.md](./OBSERVABILITY.md) for detailed debugging examples and use cases.

## Implementation Details

- **File**: `pkg/controller/core.oam.dev/v1beta1/application/policy_transforms.go`
- **Integration Point**: `application_controller.go:161-167` (after finalizers, before GenerateAppFile)
- **Context Key**: `PolicyAdditionalContextKey` stores additionalContext in Go context
- **CUE Compiler**: Uses `cuex.DefaultCompiler` for template rendering
- **Deep Merge**: Recursive merge for maps in spec and additionalContext

## References

- Original Application Controller: `pkg/controller/core.oam.dev/v1beta1/application/application_controller.go`
- PolicyDefinition API: `apis/core.oam.dev/v1beta1/policy_definition.go`
- Tests: `pkg/controller/core.oam.dev/v1beta1/application/policy_transforms_test.go`
