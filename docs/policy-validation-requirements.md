# PolicyDefinition Validation Requirements

## Overview

This document defines validation rules for PolicyDefinitions, especially global policies.

## Critical Validations (MUST)

### 1. Global Policy Parameter Validation

**Rule**: Global policies MUST have default values for ALL parameters.

**Rationale**: Global policies are auto-discovered and applied without user input. Even optional parameters without defaults cannot compile.

**Implementation**: Parse CUE AST and check that ALL parameter fields have default values (using `*` marker).

```cue
# ❌ INVALID - required parameter without default
parameter: {
  envName: string  // Can't compile without value!
}

# ❌ INVALID - optional but no default
parameter: {
  envName?: string  // Optional doesn't help - still can't compile!
}

# ✅ VALID - no parameters
parameter: {}

# ✅ VALID - all fields have defaults
parameter: {
  envName: *"production" | string  // Has default (*"production")
  replicas: *3 | int               // Has default (*3)
  logLevel: *"info" | string       // Has default (*"info")
}
```

**Error Message**: "Global policy '<name>' cannot have parameters without default values. Found parameters without defaults: [envName]. Global policies are auto-applied without user input, so all parameters must have default values using '*'. Example: envName: *\"production\" | string"

### 2. Global Policy Priority Validation

**Rule**: Global policies MUST have a priority field set.

**Rationale**: Without priority, execution order is purely alphabetical, making ordering unpredictable.

**Implementation**: Check `spec.priority` is set when `spec.global == true`.

```yaml
# ❌ INVALID - global without priority
spec:
  global: true
  # priority not set (defaults to 0, but should be explicit)

# ✅ VALID - explicit priority
spec:
  global: true
  priority: 100  # Explicit ordering
```

**Error Message**: "Global policy '<name>' must have an explicit priority field. This ensures predictable execution order."

**Alternative**: Make it a warning instead of error? Allow default 0?

### 3. CUE Schema Validation

**Rule**: CUE template MUST be syntactically valid and conform to expected schema.

**Schema**:
```cue
parameter: {[string]: _}
enabled: *true | bool
transforms?: {
  spec?: {
    type: "replace" | "merge"
    value: {...}
  }
  labels?: {
    type: "merge"  // Only merge allowed
    value: {[string]: string}
  }
  annotations?: {
    type: "merge"  // Only merge allowed
    value: {[string]: string}
  }
}
additionalContext?: {[string]: _}
```

**Validation**:
- CUE syntax is valid (can compile)
- `enabled` is boolean or defaults to true
- `transforms.labels.type` is only "merge" (not "replace")
- `transforms.annotations.type` is only "merge" (not "replace")
- `transforms.spec.type` is "merge" or "replace"

**Error Messages**:
- "CUE template syntax error: <details>"
- "transforms.labels.type must be 'merge', not 'replace'"
- "transforms.annotations.type must be 'merge', not 'replace'"

### 4. Scope Validation

**Rule**: Global policies MUST have scope="Application".

**Rationale**: Only Application-scoped policies can transform Applications.

```yaml
# ❌ INVALID - global but wrong scope
spec:
  global: true
  scope: WorkflowStep  # Wrong!

# ✅ VALID
spec:
  global: true
  scope: Application
```

**Error Message**: "Global policies must have scope='Application', found scope='<value>'"

## Important Validations (SHOULD)

### 5. Documentation Requirements

**Rule**: Global policies SHOULD have documentation annotations.

**Rationale**: Helps users understand what policies do and who owns them.

```yaml
# ⚠️ WARNING - missing documentation
metadata:
  name: my-global-policy

# ✅ GOOD - well documented
metadata:
  name: security-hardening
  annotations:
    policy.oam.dev/description: "Adds required security labels"
    policy.oam.dev/owner: "platform-team@company.com"
    policy.oam.dev/version: "v1.0.0"
```

**Warning Message**: "Global policy '<name>' should have documentation annotations: policy.oam.dev/description, policy.oam.dev/owner"

### 6. Naming Conventions

**Rule**: Global policies SHOULD follow naming conventions.

**Recommendations**:
- Use kebab-case: `security-hardening` not `securityHardening`
- Be descriptive: `add-compliance-labels` not `labels`
- Avoid generic names: `monitoring-config` not `config`

**Warning Message**: "Policy name '<name>' should use kebab-case and be descriptive"

### 7. Dangerous CUE Imports

**Rule**: SHOULD warn on dangerous CUE imports.

**Potentially dangerous imports**:
- `net`: Network operations
- `exec`: Execute commands
- `file`: File system access
- `http`: HTTP requests (may be legitimate for lookups)

**Legitimate imports**:
- `strings`: String manipulation
- `list`: List operations
- `math`: Math operations
- `encoding/json`: JSON parsing
- `encoding/yaml`: YAML parsing

```cue
# ⚠️ WARNING - potentially dangerous
import "net"

# ✅ OK
import "strings"
import "encoding/json"
```

**Warning Message**: "Policy uses potentially dangerous import: <import>. Review security implications."

### 8. Critical Label Protection

**Rule**: SHOULD prevent removal of critical platform labels.

**Protected labels/annotations**:
- `app.kubernetes.io/*`
- `app.oam.dev/*`
- `oam.dev/*`
- Any labels added by KubeVela itself

**Implementation**: Check if policy uses "replace" on labels/annotations (already blocked by rule #3).

### 9. Priority Ranges

**Rule**: SHOULD use priority ranges by category.

**Recommended ranges**:
- 1000-1999: Critical security/compliance
- 500-999: Standard platform policies
- 100-499: Optional enhancements
- 0-99: Low priority defaults

```yaml
# ⚠️ WARNING - unusual priority
spec:
  global: true
  priority: 50000  # Unusually high

# ✅ GOOD
spec:
  global: true
  priority: 1000  # Security policy
```

**Warning Message**: "Priority <value> is outside recommended ranges. Consider using: 1000-1999 (security), 500-999 (standard), 100-499 (optional), 0-99 (low priority)."

## Advanced Validations (NICE-TO-HAVE)

### 10. CUE Complexity Analysis

**Rule**: MAY warn on complex CUE templates.

**Metrics**:
- Nesting depth > 5
- Template length > 200 lines
- Large loops (processing > 100 items)
- Recursive definitions

**Warning Message**: "CUE template is complex (nesting depth: 8). Consider simplifying."

### 11. Transform Size Limits

**Rule**: MAY limit size of transform values.

**Rationale**: Prevent accidentally adding huge specs.

```cue
# ⚠️ WARNING - large transform
transforms: spec: {
  type: "merge"
  value: {
    // 10MB of spec changes
  }
}
```

**Warning Message**: "Transform value is large (<size>KB). Consider if this is intentional."

### 12. Conditional Complexity

**Rule**: MAY warn on complex `enabled` conditions.

**Rationale**: Simple conditions are easier to understand and debug.

```cue
# ⚠️ WARNING - complex condition
import "strings"
import "list"

enabled: strings.HasPrefix(context.application.metadata.namespace, "tenant-") &&
         len(context.application.spec.components) > 5 &&
         list.Contains(context.application.metadata.labels, "requires-policy")

# ✅ SIMPLE
enabled: strings.HasPrefix(context.application.metadata.namespace, "tenant-")
```

**Warning Message**: "Enabled condition is complex. Consider simplifying for easier debugging."

### 13. Breaking Change Detection

**Rule**: MAY warn on breaking changes to existing policies.

**Breaking changes**:
- Changing priority significantly (>100 difference)
- Changing from non-global to global
- Changing scope
- Adding required parameters (for non-global)

**Warning Message**: "Priority changed from 100 to 900 (delta: 800). This may affect execution order."

### 14. Conflict Detection

**Rule**: MAY detect potential conflicts between policies.

**Examples**:
- Two policies setting same label to different values
- Priority collision (same priority, similar names)

**Warning Message**: "Policy 'my-policy-a' and 'my-policy-b' both set label 'team'. Last one wins."

### 15. Performance Impact

**Rule**: MAY estimate performance impact.

**Factors**:
- Number of API calls in CUE (if detectable)
- Complexity of transforms
- Size of merge operations

**Warning Message**: "Policy may have performance impact. Consider caching expensive operations."

## Implementation Strategy

### Phase 1: Critical Validations (Blocking)

Implement these as **ValidatingWebhook** that blocks invalid policies:

1. ✅ No required parameters for global policies
2. ✅ Priority field required for global policies
3. ✅ CUE schema validation
4. ✅ Scope must be Application

### Phase 2: Important Validations (Warnings)

Implement as warnings in webhook response:

5. ⚠️ Documentation annotations
6. ⚠️ Naming conventions
7. ⚠️ Dangerous imports
8. ⚠️ Critical label protection (covered by #3)
9. ⚠️ Priority ranges

### Phase 3: Advanced Validations (Future)

Implement in CLI tool or separate linter:

10. Complexity analysis
11. Transform size limits
12. Conditional complexity
13. Breaking change detection
14. Conflict detection
15. Performance impact estimation

## Validation Webhook Design

```go
// ValidatePolicyDefinition validates a PolicyDefinition
func ValidatePolicyDefinition(old, new *v1beta1.PolicyDefinition) error {
    var allErrors []error
    var warnings []string

    // Critical validations (blocking)
    if new.Spec.Global {
        // Rule 1: No required parameters
        if err := validateNoRequiredParameters(new); err != nil {
            allErrors = append(allErrors, err)
        }

        // Rule 2: Priority must be set
        if !hasPriority(new) {
            allErrors = append(allErrors, errors.New("global policies must have explicit priority"))
        }

        // Rule 4: Scope must be Application
        if new.Spec.Scope != v1beta1.ApplicationScope {
            allErrors = append(allErrors, errors.New("global policies must have scope='Application'"))
        }
    }

    // Rule 3: CUE schema validation
    if err := validateCUESchema(new); err != nil {
        allErrors = append(allErrors, err)
    }

    // Important validations (warnings)
    if new.Spec.Global {
        // Rule 5: Documentation
        if !hasDocumentation(new) {
            warnings = append(warnings, "missing documentation annotations")
        }

        // Rule 7: Dangerous imports
        if dangerousImports := checkDangerousImports(new); len(dangerousImports) > 0 {
            warnings = append(warnings, fmt.Sprintf("dangerous imports: %v", dangerousImports))
        }

        // Rule 9: Priority ranges
        if !inRecommendedRange(new.Spec.Priority) {
            warnings = append(warnings, "priority outside recommended ranges")
        }
    }

    if len(allErrors) > 0 {
        return utilerrors.NewAggregate(allErrors)
    }

    // Return warnings as annotations or in response
    if len(warnings) > 0 {
        logWarnings(warnings)
    }

    return nil
}
```

## Testing Strategy

Each validation rule needs:
1. Positive test (valid policy passes)
2. Negative test (invalid policy fails)
3. Edge case tests

Example:
```go
It("rejects global policy with required parameters", func() {
    policy := &v1beta1.PolicyDefinition{
        Spec: v1beta1.PolicyDefinitionSpec{
            Global: true,
            Schematic: &common.Schematic{
                CUE: &common.CUE{
                    Template: `
parameter: {
    envName: string  // Required!
}
`,
                },
            },
        },
    }

    err := ValidatePolicyDefinition(nil, policy)
    Expect(err).ShouldNot(BeNil())
    Expect(err.Error()).Should(ContainSubstring("required parameters"))
})
```

## User Experience

### CLI Command

```bash
# Validate before applying
kubectl vela policy validate -f my-policy.yaml

# Output:
❌ Error: Global policy 'my-policy' has required parameters: [envName]
⚠️  Warning: Missing documentation annotations
⚠️  Warning: Priority 50 is outside recommended ranges

# With --strict flag
kubectl vela policy validate -f my-policy.yaml --strict
# Treats warnings as errors
```

### Admission Webhook Feedback

```bash
kubectl apply -f bad-policy.yaml

Error from server (Forbidden): error when creating "bad-policy.yaml": admission webhook "validate.policydefinition.core.oam.dev" denied the request:
- Global policy 'security-hardening' has required parameters: [envName]
- Global policies must have explicit priority field
```

## Open Questions

1. **Should priority be required or just recommended?**
   - Required: More strict, enforces best practices
   - Recommended: More flexible, defaults to 0

2. **Should we allow dangerous imports at all?**
   - Block completely: Very strict, may limit legitimate use cases
   - Warn only: More flexible, but risk of abuse
   - Allowlist: Platform admin can configure allowed imports

3. **How strict should parameter validation be?**
   - Block any parameters: Strictest
   - Block required parameters: Balanced (current proposal)
   - Warn only: Most flexible

4. **Should we validate update operations differently?**
   - More lenient on updates to avoid breaking existing policies
   - Same strict validation to maintain quality

5. **Should validation be opt-in or opt-out?**
   - Opt-in: More flexible, may miss validation
   - Opt-out: Stricter, but can disable for advanced users
