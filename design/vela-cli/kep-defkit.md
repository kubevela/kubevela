# KEP: Definition Kit (defkit) - Go SDK for X-Definition Authoring

## Summary

Definition Kit (defkit) is a Go SDK that enables platform engineers to author KubeVela X-Definitions using native Go code instead of CUE. The SDK compiles Go to CUE transparently, providing full IDE support while maintaining compatibility with the KubeVela controller.

> **Status**: This document is a design proposal. While the core architecture and goals are established, specific API names, method signatures, and implementation details may evolve as we proceed with implementation and incorporate community feedback.

## Motivation

### Goals

1. **Author X-Definitions in Go** - Write ComponentDefinition, TraitDefinition, PolicyDefinition, and WorkflowStepDefinition using fluent Go APIs
2. **Transparent CUE compilation** - Go code compiles to CUE automatically; developers never see or write CUE
3. **Full IDE support** - Autocomplete, type checking, and inline documentation
4. **Testable definitions** - Unit test definitions using standard Go testing frameworks
5. **Schema-agnostic resource construction** - Support any Kubernetes resource without coupling to K8s versions
6. **Easy distribution via Go modules** - Share and version X-Definitions as standard Go packages, enabling `go get` for platform capabilities
7. **Secure code execution model** - Go code executes only at compile-time (CLI), not at runtime in the controller, preventing code injection risks

### Non-Goals

1. Multi-language support in initial release (Go only)
2. Replacing CUE as the controller's internal engine
3. Runtime CUE evaluation in the SDK

## Proposal

### Why Go for X-Definitions?

**1. Familiar Tooling** - Platform engineers already use Go for controllers, operators, and CLI tools. Writing definitions in the same language eliminates context switching and leverages existing skills.

**2. IDE Experience** - Full autocomplete, type checking, go-to-definition, and inline documentation. No special CUE plugins required.

**3. Standard Distribution** - Definitions become Go packages that can be versioned, shared via `go get`, and composed like any other library. No custom registries or tooling needed.

**4. Testability** - Use standard Go testing frameworks (go test, testify, gomega) to unit test definitions before deployment. Mock contexts enable testing without a cluster.

**5. Compile-Time Safety** - Catch errors at compile time rather than at deployment time. Invalid field names, type mismatches, and missing required parameters are caught immediately.

### Fluent API Design

defkit provides a fluent builder API where parameters are defined inline and bound to local variables. All types are in a single `defkit` package for simplicity:

```go
func WebserviceDefinition() *defkit.ComponentDefinition {
    // Parameters defined as local variables within the function
    image := defkit.String("image").Required()
    replicas := defkit.Int("replicas").Default(3).Min(1).Max(100)
    cpu := defkit.String("cpu")

    return defkit.NewComponent("webservice").
        Description("A production-ready web service").
        Workload("apps/v1", "Deployment").
        Params(image, replicas, cpu).
        Template(func(tpl *defkit.Template) {
            vela := defkit.VelaCtx()

            deploy := defkit.NewResource("apps/v1", "Deployment").
                Set("spec.replicas", replicas).
                Set("spec.selector.matchLabels[app.oam.dev/component]", vela.Name()).
                Set("spec.template.spec.containers[0].name", vela.Name()).
                Set("spec.template.spec.containers[0].image", image).
                // Optional parameters use fluent conditional
                SetIf(cpu.IsSet(), "spec.template.spec.containers[0].resources.limits.cpu", cpu)

            tpl.Output(deploy)
        })
}
```

**Key design insight**: Parameters are Go variables that can be used directly in both definition and template. No string lookups, no dual references. The variable `image` carries both its schema definition AND serves as the accessor.

This reads naturally: "Define image, replicas, and cpu as parameters. Create a webservice component using them."

**Note:** Parameters are defined as local variables within the function (not package-level) to ensure proper encapsulation and test isolation. The template function receives a `Template` context (named `tpl` to avoid confusion with Go's `context` package). Runtime context like component name and namespace is accessed via `defkit.VelaCtx()`.

### Common Patterns

The SDK provides typed helpers for common definition patterns:

| Pattern | Go API | Example |
|---------|--------|---------|
| Required parameter | `defkit.String("image").Required()` | `image` must be provided |
| Default value | `defkit.Int("replicas").Default(3)` | `replicas` defaults to 3 |
| Validation | `defkit.Int("replicas").Min(1).Max(100)` | `replicas` between 1-100 |
| Enums | `defkit.Enum("policy").Values("Always", "Never")` | `policy` with allowed values |
| Optional object | `defkit.Object("persistence").WithFields(...)` | `persistence` block |
| Lists | `defkit.StringList("args")` or `defkit.List("args").WithFields(...)` | `args` list |

### Runtime Context Access

CUE templates access runtime values (like deployment status) through well-known paths. The Go SDK provides fluent methods via `defkit.VelaCtx()` that generate these paths:

```go
vela := defkit.VelaCtx()
vela.Name()                  // → context.name
vela.Namespace()             // → context.namespace
vela.AppName()               // → context.appName
vela.AppRevision()           // → context.appRevision
vela.ClusterVersion().Minor() // → context.clusterVersion.minor
```

Runtime context is accessed separately from template context, which focuses on output generation. Health policies and custom status use the built-in helpers:

```go
// Health check using deployment helper
def.HealthPolicy(defkit.DeploymentHealth().Build())

// Custom status using deployment helper
def.CustomStatus(defkit.DeploymentStatus().Build())
```

### When to Use What

```
┌─────────────────────────────────────────────────────────────────┐
│                     DECISION FRAMEWORK                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│   Can I express it with defkit Go API?                          │
│                                                                 │
│       YES ──────────► Use Go (95% of cases)                     │
│                       • Static values: Set("image", "nginx")    │
│                       • Parameters: use the variable directly   │
│                       • Custom status: defkit.Status().Field()  │
│                       • Cluster info: vela.ClusterVersion()     │
│                                                                 │
│       NO ───────────► Use RawCUE() (rare escape hatch)          │
│                       • True unification semantics (a & b)      │
│                       • Complex nested comprehensions (3+ levels)│
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Target Developer Experience

```go
package myplatform

import (
    "github.com/oam-dev/kubevela/pkg/definition/defkit"
)

func init() {
    defkit.Register(WebserviceComponent())
}

func WebserviceComponent() *defkit.ComponentDefinition {
    // Parameters scoped to definition function
    image := defkit.String("image").Required()
    replicas := defkit.Int("replicas").Default(3).Min(1).Max(100)
    cpu := defkit.String("cpu").Optional()

    return defkit.NewComponent("webservice").
        Description("A production-ready web service").
        Workload("apps/v1", "Deployment").
        Params(image, replicas, cpu).
        Template(func(tpl *defkit.Template) {
            vela := defkit.VelaCtx()

            deploy := defkit.NewResource("apps/v1", "Deployment").
                Set("metadata.name", vela.Name()).
                Set("spec.replicas", replicas).
                Set("spec.selector.matchLabels[app.oam.dev/component]", vela.Name()).
                Set("spec.template.spec.containers", []map[string]any{{
                    "name":  vela.Name(),
                    "image": image,
                }}).
                SetIf(cpu.IsSet(), "spec.template.spec.containers[0].resources.limits.cpu", cpu)

            tpl.Output(deploy)
        }).
        // Health policy using deployment helper
        HealthPolicy(defkit.DeploymentHealth().Build()).
        // Custom status using deployment helper
        CustomStatus(defkit.DeploymentStatus().Build())
}
```

---

## VelaContext API Reference

### Runtime Context Accessors

The `defkit.VelaCtx()` function provides access to runtime context values:

| Go Method | Generated CUE | Description |
|-----------|---------------|-------------|
| `vela.Name()` | `context.name` | Component name |
| `vela.Namespace()` | `context.namespace` | Target namespace |
| `vela.AppName()` | `context.appName` | Application name |
| `vela.AppRevision()` | `context.appRevision` | Application revision |
| `vela.Revision()` | `context.revision` | Component revision |
| `vela.ClusterVersion().Minor()` | `context.clusterVersion.minor` | K8s minor version |
| `vela.ClusterVersion().Major()` | `context.clusterVersion.major` | K8s major version |

### Parameter Variables

Parameters are typed variables that generate CUE paths automatically:

| Go Usage | Generated CUE | Description |
|----------|---------------|-------------|
| `image` (in template) | `parameter.image` | Parameter value (from variable name) |
| `cpu.IsSet()` | `parameter.cpu != _\|_` | Check if optional parameter is set |
| `persistence.Field("size")` | `parameter.persistence.size` | Nested field access |

**How it works**: Each parameter variable knows its name and type. When used in a template expression, it compiles to the corresponding `parameter.X` CUE path.

### Comparison and Conditional Helpers

| Go Method | Generated CUE | Description |
|-----------|---------------|-------------|
| `defkit.Eq(a, b)` | `a == b` | Equality comparison |
| `defkit.Ne(a, b)` | `a != b` | Inequality comparison |
| `defkit.Lt(a, b)` | `a < b` | Less than |
| `defkit.Le(a, b)` | `a <= b` | Less than or equal |
| `defkit.Gt(a, b)` | `a > b` | Greater than |
| `defkit.Ge(a, b)` | `a >= b` | Greater than or equal |
| `defkit.And(a, b)` | `a && b` | Logical AND |
| `defkit.Or(a, b)` | `a \|\| b` | Logical OR |
| `defkit.Not(cond)` | `!cond` | Logical NOT |
| `defkit.Lit(value)` | `value` | Literal value (for comparisons) |
| `param.IsSet()` | `param != _\|_` | Check if optional param is set |

---

## Examples

### Health Policy

#### Composable Health Expressions

defkit provides a fully composable expression-based API for defining health policies. This enables health checks for **any** resource type, not just Kubernetes workloads.

**Core Primitives:**

All health expression methods are accessed via the `Health()` builder, providing a unified API:

| Primitive | Purpose | Example |
|-----------|---------|---------|
| `Health().Condition(type)` | Check `status.conditions[]` array | `Health().Condition("Ready").IsTrue()` |
| `Health().Phase(phases...)` | Check `status.phase` field | `Health().Phase("Running", "Succeeded")` |
| `Health().Field(path)` | Compare any status field | `Health().Field("status.state").Eq("active")` |
| `Health().Exists(path)` | Check field existence | `Health().Exists("status.endpoint")` |
| `Health().And(...)` | All expressions must be true | `Health().And(expr1, expr2)` |
| `Health().Or(...)` | Any expression must be true | `Health().Or(expr1, expr2)` |
| `Health().Not(...)` | Negate expression | `Health().Not(h.Condition("Stalled").IsTrue())` |
| `Health().AllTrue(conds...)` | All conditions are True | `Health().AllTrue("Ready", "Synced")` |
| `Health().AnyTrue(conds...)` | Any condition is True | `Health().AnyTrue("Ready", "Available")` |
| `Health().Always()` | Always healthy (existence) | `Health().Always()` |

**Condition Expressions:**

```go
h := defkit.Health()

// Single condition check (most CRDs, Crossplane, cert-manager, etc.)
HealthPolicyExpr(h.Condition("Ready").IsTrue())

// Multiple conditions - all must be true
HealthPolicyExpr(h.AllTrue("Ready", "Synced"))

// Multiple conditions - any is sufficient
HealthPolicyExpr(h.AnyTrue("Ready", "Available"))

// Check condition exists (regardless of status)
HealthPolicyExpr(h.Condition("Initialized").Exists())

// Check condition reason
HealthPolicyExpr(h.Condition("Ready").ReasonIs("Available"))
```

**Generated CUE for `h.Condition("Ready").IsTrue()`:**
```cue
_readyCond: [ for c in context.output.status.conditions if c.type == "Ready" { c } ]
isHealth: len(_readyCond) > 0 && _readyCond[0].status == "True"
```

**Generated CUE for `h.AllTrue("Ready", "Synced")`:**
```cue
_readyCond: [ for c in context.output.status.conditions if c.type == "Ready" { c } ]
_syncedCond: [ for c in context.output.status.conditions if c.type == "Synced" { c } ]
isHealth: (len(_readyCond) > 0 && _readyCond[0].status == "True") &&
          (len(_syncedCond) > 0 && _syncedCond[0].status == "True")
```

**Phase Expressions:**

```go
h := defkit.Health()

// Pod-style phase check
HealthPolicyExpr(h.Phase("Running", "Succeeded"))

// Custom phase field path
HealthPolicyExpr(h.PhaseField("status.currentPhase", "Active", "Ready"))
```

**Generated CUE for `h.Phase("Running", "Succeeded")`:**
```cue
isHealth: context.output.status.phase == "Running" ||
          context.output.status.phase == "Succeeded"
```

**Field Expressions:**

```go
h := defkit.Health()

// Field equality
HealthPolicyExpr(h.Field("status.state").Eq("active"))

// Numeric comparisons
HealthPolicyExpr(h.Field("status.replicas").Gt(0))
HealthPolicyExpr(h.Field("status.availableReplicas").Gte(1))

// Field existence
HealthPolicyExpr(h.Exists("status.loadBalancer.ingress"))

// Field in set of values
HealthPolicyExpr(h.Field("status.phase").In("Running", "Succeeded", "Complete"))

// Compare field to another field
HealthPolicyExpr(h.Field("status.readyReplicas").Eq(h.FieldRef("spec.replicas")))
```

**Generated CUE for `h.Field("status.state").Eq("active")`:**
```cue
isHealth: context.output.status.state == "active"
```

**Generated CUE for `h.Field("status.readyReplicas").Eq(h.FieldRef("spec.replicas"))`:**
```cue
isHealth: context.output.status.readyReplicas == context.output.spec.replicas
```

**Composite Expressions:**

```go
h := defkit.Health()

// Complex composition with And/Or/Not
HealthPolicyExpr(h.And(
    h.Condition("Ready").IsTrue(),
    h.Not(h.Condition("Stalled").IsTrue()),
))

// Mixed patterns
HealthPolicyExpr(h.And(
    h.Condition("Ready").IsTrue(),
    h.Or(
        h.Field("status.replicas").Gte(1),
        h.Exists("status.endpoint"),
    ),
))

// Real-world: Custom CRD with arbitrary status structure
HealthPolicyExpr(h.And(
    h.Field("status.state").Eq("active"),
    h.Exists("status.connectionString"),
    h.Field("status.lastSyncTime").Exists(),
))
```

**Generated CUE for nested And/Or:**
```cue
_readyCond: [ for c in context.output.status.conditions if c.type == "Ready" { c } ]
isHealth: (len(_readyCond) > 0 && _readyCond[0].status == "True") &&
          ((context.output.status.replicas >= 1) || (context.output.status.endpoint != _|_))
```

**Field Expression Methods:**

| Method | CUE Output | Description |
|--------|------------|-------------|
| `.Eq(value)` | `field == value` | Equal to |
| `.Ne(value)` | `field != value` | Not equal to |
| `.Gt(value)` | `field > value` | Greater than |
| `.Gte(value)` | `field >= value` | Greater than or equal |
| `.Lt(value)` | `field < value` | Less than |
| `.Lte(value)` | `field <= value` | Less than or equal |
| `.In(values...)` | `field == v1 \|\| field == v2` | In set of values |
| `.Contains(substr)` | String contains | For string fields |
| `.Eq(h.FieldRef(path))` | `field == otherField` | Compare to another field |

#### Built-in Workload Helpers

For common Kubernetes workloads, defkit provides pre-configured helpers that implement the standard health patterns:

```go
// For Deployments - use the built-in helper
HealthPolicy(defkit.DeploymentHealth().Build())
```

**Generated CUE:**
```cue
ready: {
    updatedReplicas:    *0 | int
    readyReplicas:      *0 | int
    replicas:           *0 | int
    observedGeneration: *0 | int
} & {
    if context.output.status.updatedReplicas != _|_ {
        updatedReplicas: context.output.status.updatedReplicas
    }
    if context.output.status.readyReplicas != _|_ {
        readyReplicas: context.output.status.readyReplicas
    }
    if context.output.status.replicas != _|_ {
        replicas: context.output.status.replicas
    }
    if context.output.status.observedGeneration != _|_ {
        observedGeneration: context.output.status.observedGeneration
    }
}
_isHealth: (context.output.spec.replicas == ready.readyReplicas) &&
           (context.output.spec.replicas == ready.updatedReplicas) &&
           (context.output.spec.replicas == ready.replicas) &&
           (ready.observedGeneration == context.output.metadata.generation ||
            ready.observedGeneration > context.output.metadata.generation)
isHealth: *_isHealth | bool
if context.output.metadata.annotations != _|_ {
    if context.output.metadata.annotations["app.oam.dev/disable-health-check"] != _|_ {
        isHealth: true
    }
}
```

### Custom Status

#### Composable Status Expressions

defkit provides a fully composable expression-based API for defining custom status messages. This enables dynamic status messages for **any** resource type, not just Kubernetes workloads.

**Core Primitives:**

All status expression methods are accessed via the `Status()` builder, providing a unified API:

| Primitive | Purpose | Example |
|-----------|---------|---------|
| `Status().Condition(type)` | Check `status.conditions[]` array | `Status().Condition("Ready").StatusValue()` |
| `Status().Field(path)` | Extract any status field value | `Status().Field("status.replicas")` |
| `Status().SpecField(path)` | Extract spec field value | `Status().SpecField("spec.replicas")` |
| `Status().Exists(path)` | Check field existence | `Status().Exists("status.endpoint")` |
| `Status().Format(template, ...)` | Build formatted message | `Status().Format("Ready: %v/%v", readyExpr, desiredExpr)` |
| `Status().Concat(...)` | Concatenate string parts | `Status().Concat(part1, " - ", part2)` |
| `Status().Switch(...)` | Conditional message selection | `Status().Switch(case1, case2, default)` |
| `Status().HealthAware(...)` | Message based on health status | `Status().HealthAware(healthyMsg, unhealthyMsg)` |
| `Status().WithDetails(...)` | Add structured details | `Status().WithDetails(detail1, detail2)` |

**Field Expressions:**

```go
s := defkit.Status()

// Extract status fields with defaults
readyReplicas := s.Field("status.readyReplicas").Default(0)
phase := s.Field("status.phase").Default("Unknown")
endpoint := s.Field("status.endpoint").Default("pending")

// Extract spec fields
desiredReplicas := s.SpecField("spec.replicas")

// Build formatted message
CustomStatusExpr(s.Format("Ready: %v/%v", readyReplicas, desiredReplicas))
```

**Generated CUE for field extraction:**
```cue
_readyReplicas: *0 | int
if context.output.status.readyReplicas != _|_ {
    _readyReplicas: context.output.status.readyReplicas
}
_desiredReplicas: context.output.spec.replicas
message: "Ready: \(_readyReplicas)/\(_desiredReplicas)"
```

**Condition Expressions:**

```go
s := defkit.Status()

// Extract condition message
CustomStatusExpr(s.Condition("Ready").Message())

// Extract condition reason
CustomStatusExpr(s.Condition("Ready").Reason())

// Extract condition status
CustomStatusExpr(s.Condition("Ready").StatusValue())

// Combine condition info
CustomStatusExpr(s.Format("%v: %v",
    s.Condition("Ready").StatusValue(),
    s.Condition("Ready").Message(),
))
```

**Generated CUE for condition extraction:**
```cue
_readyCond: [ for c in context.output.status.conditions if c.type == "Ready" { c } ]
_readyStatus: *"Unknown" | string
_readyMessage: *"" | string
if len(_readyCond) > 0 {
    _readyStatus: _readyCond[0].status
    _readyMessage: _readyCond[0].message
}
message: "\(_readyStatus): \(_readyMessage)"
```

**Conditional Status Messages:**

```go
s := defkit.Status()

// Switch based on field value
CustomStatusExpr(s.Switch(
    s.Case(s.Field("status.phase").Eq("Running"), "Service is running"),
    s.Case(s.Field("status.phase").Eq("Pending"), "Service is starting..."),
    s.Case(s.Field("status.phase").Eq("Failed"), s.Concat("Failed: ", s.Field("status.reason"))),
    s.Default("Unknown status"),
))

// Health-aware status (references context.status.healthy set by healthPolicy)
CustomStatusExpr(s.HealthAware(
    "All systems operational",  // when healthy
    s.Concat("Degraded: ", s.Condition("Ready").Message()),  // when unhealthy
))
```

**Generated CUE for Switch:**
```cue
message: *"Unknown status" | string
if context.output.status.phase == "Running" {
    message: "Service is running"
}
if context.output.status.phase == "Pending" {
    message: "Service is starting..."
}
if context.output.status.phase == "Failed" {
    _failReason: *"" | string
    if context.output.status.reason != _|_ {
        _failReason: context.output.status.reason
    }
    message: "Failed: \(_failReason)"
}
```

**Composite Status Messages:**

```go
s := defkit.Status()

// Build rich status message with multiple components
CustomStatusExpr(s.Concat(
    "Replicas: ", s.Field("status.readyReplicas").Default(0),
    "/", s.SpecField("spec.replicas"),
    " | Phase: ", s.Field("status.phase").Default("Unknown"),
    " | Generation: ", s.Field("status.observedGeneration").Default(0),
))

// Real-world: Custom CRD status
CustomStatusExpr(s.Concat(
    "State: ", s.Field("status.state").Default("unknown"),
    " | Endpoint: ", s.Field("status.endpoint").Default("pending"),
    " | Connections: ", s.Field("status.activeConnections").Default(0),
))
```

**Generated CUE for Concat:**
```cue
_readyReplicas: *0 | int
if context.output.status.readyReplicas != _|_ {
    _readyReplicas: context.output.status.readyReplicas
}
_phase: *"Unknown" | string
if context.output.status.phase != _|_ {
    _phase: context.output.status.phase
}
_observedGen: *0 | int
if context.output.status.observedGeneration != _|_ {
    _observedGen: context.output.status.observedGeneration
}
message: "Replicas: \(_readyReplicas)/\(context.output.spec.replicas) | Phase: \(_phase) | Generation: \(_observedGen)"
```

**Status Details (Structured Data):**

```go
s := defkit.Status()

// Add structured details alongside message
CustomStatusExpr(s.
    Message(s.Format("Ready: %v/%v",
        s.Field("status.readyReplicas").Default(0),
        s.SpecField("spec.replicas"),
    )).
    WithDetails(
        s.Detail("endpoint", s.Field("status.endpoint")),
        s.Detail("version", s.Field("status.version")),
        s.Detail("lastSync", s.Field("status.lastSyncTime")),
    ))
```

**Field Expression Methods:**

| Method | CUE Output | Description |
|--------|------------|-------------|
| `.Field(path)` | Extract from `context.output.path` | Status field value |
| `.SpecField(path)` | Extract from `context.output.path` | Spec field value |
| `.Default(value)` | `*value \| type` | Default if field missing |
| `.Exists(path)` | `path != _\|_` | Check field exists |
| `.Condition(type)` | Array filter | Access condition by type |
| `.StatusValue()` | Condition status | Get condition status |
| `.Is(value)` | `status == value` | Check condition status equals value |
| `.Message()` | Condition message | Get condition message |
| `.Reason()` | Condition reason | Get condition reason |

**Message Building Methods:**

| Method | Description | Example |
|--------|-------------|---------|
| `Format(template, args...)` | Printf-style formatting | `Format("Ready: %v/%v", ready, total)` |
| `Concat(parts...)` | String concatenation | `Concat("State: ", state, " - ", msg)` |
| `Switch(cases...)` | Conditional selection | `Switch(case1, case2, default)` |
| `Case(cond, msg)` | Switch case | `Case(phase.Eq("Running"), "OK")` |
| `Default(msg)` | Switch default | `Default("Unknown")` |
| `HealthAware(ok, fail)` | Health-based | `HealthAware("OK", errMsg)` |

#### Built-in Workload Helpers

For common Kubernetes workloads, defkit provides pre-configured helpers:

```go
// For Deployments - use the built-in helper
CustomStatus(defkit.DeploymentStatus().Build())
```

**Generated CUE:**
```cue
ready: {
    readyReplicas: *0 | int
} & {
    if context.output.status.readyReplicas != _|_ {
        readyReplicas: context.output.status.readyReplicas
    }
}
message: "Ready:\(ready.readyReplicas)/\(context.output.spec.replicas)"
```

#### Creating Custom Status Helpers

Users can create their own helpers by composing the primitives:

```go
// Example: Crossplane-style status
func CrossplaneStatus() defkit.StatusExpression {
    s := defkit.Status()
    return s.Switch(
        s.Case(s.Condition("Ready").Is("True"),
            s.Concat("Ready: ", s.Condition("Ready").Message())),
        s.Case(s.Condition("Synced").Is("False"),
            s.Concat("Syncing: ", s.Condition("Synced").Message())),
        s.Default(s.Concat(
            "Ready: ", s.Condition("Ready").StatusValue(),
            " | Synced: ", s.Condition("Synced").StatusValue(),
        )),
    )
}

// Example: Database CRD status
func DatabaseStatus() defkit.StatusExpression {
    s := defkit.Status()
    return s.Concat(
        "State: ", s.Field("status.state").Default("initializing"),
        " | Connections: ", s.Field("status.connections").Default(0),
        " | Endpoint: ", s.Field("status.endpoint").Default("pending"),
    )
}

// Example: Certificate status
func CertificateStatus() defkit.StatusExpression {
    s := defkit.Status()
    return s.HealthAware(
        s.Concat("Valid until ", s.Field("status.notAfter")),
        s.Concat("Not ready: ", s.Condition("Ready").Message()),
    )
}

// Usage - pick ONE status per component:
CustomStatusExpr(CrossplaneStatus())
CustomStatusExpr(DatabaseStatus())
```

### Workload Helper Reference

defkit provides pre-configured helpers for common Kubernetes workloads:

| Workload | Status Helper | Health Helper |
|----------|---------------|---------------|
| Deployment | `DeploymentStatus()` | `DeploymentHealth()` |
| DaemonSet | `DaemonSetStatus()` | `DaemonSetHealth()` |
| StatefulSet | `StatefulSetStatus()` | `StatefulSetHealth()` |
| Job | - | `JobHealth()` |
| CronJob | - | `CronJobHealth()` |

Example for DaemonSet:

```go
// DaemonSet health policy
HealthPolicy(defkit.DaemonSetHealth().Build())
```

**Generated CUE:**
```cue
ready: {
    replicas: *0 | int
} & {
    if context.output.status.numberReady != _|_ {
        replicas: context.output.status.numberReady
    }
}
desired: {
    replicas: *0 | int
} & {
    if context.output.status.desiredNumberScheduled != _|_ {
        replicas: context.output.status.desiredNumberScheduled
    }
}
// ... additional fields for current, updated, generation
isHealth: desired.replicas == ready.replicas &&
          desired.replicas == updated.replicas &&
          desired.replicas == current.replicas &&
          (generation.observed == generation.metadata || generation.observed > generation.metadata)
```

### Creating Custom Helpers

Users can create their own helpers by composing the primitives:

```go
// Example: Crossplane-style health check
func CrossplaneHealth() defkit.HealthExpression {
    h := defkit.Health()
    return h.AllTrue("Ready", "Synced")
}

// Example: Certificate health check
func CertificateHealth() defkit.HealthExpression {
    h := defkit.Health()
    return h.Condition("Ready").IsTrue()
}

// Example: Custom database CRD
func DatabaseHealth() defkit.HealthExpression {
    h := defkit.Health()
    return h.And(
        h.Condition("Ready").IsTrue(),
        h.Field("status.phase").Eq("Running"),
        h.Exists("status.connectionEndpoint"),
    )
}

// Usage - pick ONE health policy per component:
// For a Crossplane managed resource:
HealthPolicyExpr(CrossplaneHealth())

// For a database CRD:
HealthPolicyExpr(DatabaseHealth())
```

### Cluster Version Check

```go
// Go - inside template function using conditional apiVersion
Template(func(tpl *defkit.Template) {
    vela := defkit.VelaCtx()

    // Use VersionIf for cluster-version-aware resources
    cronjob := defkit.NewResourceWithConditionalVersion("batch/v1", "CronJob").
        VersionIf(vela.ClusterVersion().Minor().Lt(defkit.Lit(25)), "batch/v1beta1").
        Set("metadata.name", vela.Name()).
        Set("spec.schedule", schedule)

    tpl.Output(cronjob)
})

// Generated CUE
// if context.clusterVersion.minor < 25 {
//     apiVersion: "batch/v1beta1"
// }
// if context.clusterVersion.minor >= 25 {
//     apiVersion: "batch/v1"
// }
```

### Conditional Fields

```go
// Go - use SetIf for conditional field setting
Template(func(tpl *defkit.Template) {
    vela := defkit.VelaCtx()
    cpu := defkit.String("cpu")  // optional parameter

    deploy := defkit.NewResource("apps/v1", "Deployment").
        Set("metadata.name", vela.Name()).
        Set("spec.replicas", replicas).
        SetIf(cpu.IsSet(), "spec.template.spec.containers[0].resources.limits.cpu", cpu)

    tpl.Output(deploy)
})

// Generated CUE
// if parameter.cpu != _|_ {
//     spec: template: spec: containers: [{resources: limits: cpu: parameter.cpu}]
// }
```

### Conditional Patterns

#### Checking if an optional parameter is set

Use `param.IsSet()` to conditionally set fields when an optional parameter has a value:

```go
// Single condition - set field only if parameter was provided
deploy.SetIf(cpu.IsSet(), "spec.resources.limits.cpu", cpu)
deploy.SetIf(labels.IsSet(), "spec.template.metadata.labels", labels)
```

#### Checking if a boolean parameter's value is true

For boolean parameters, use `Eq(param, Lit(true))` to check if the value is `true`:

```go
// Check if boolean parameter is true
addRevisionLabel := defkit.Bool("addRevisionLabel").Default(false)

deploy.SetIf(
    defkit.Eq(addRevisionLabel, defkit.Lit(true)),
    "spec.template.metadata.labels[app.oam.dev/revision]",
    ctx.Revision(),
)
```

**Why this pattern?** Using `param.IsSet()` on a boolean only checks if it was provided, not its value. A boolean set to `false` would pass `IsSet()`. Use `Eq(param, Lit(true))` when you need to check the actual value.

### Compound Conditions

Use `defkit.And()` and `defkit.Or()` to compose conditions:

```go
// Single condition
deploy.SetIf(cpu.IsSet(), "spec.resources.limits.cpu", cpu)

// AND - both must be true (vela := defkit.VelaCtx())
deploy.SetIf(
    defkit.And(
        vela.ClusterVersion().Minor().Gte(25),
        enableNewAPI.IsSet(),
    ),
    "apiVersion", "batch/v1",
)

// OR - either condition
deploy.SetIf(
    defkit.Or(
        vela.ClusterVersion().Minor().Lt(21),
        legacyMode.IsSet(),
    ),
    "apiVersion", "batch/v1beta1",
)

// Complex: (A && B) || C
deploy.SetIf(
    defkit.Or(
        defkit.And(isProduction, highAvailability),
        forceHA.IsSet(),
    ),
    "spec.replicas", 3,
)
```

**Generated CUE:**
```cue
if parameter.cpu != _|_ {
    spec: resources: limits: cpu: parameter.cpu
}
if context.clusterVersion.minor >= 25 && parameter.enableNewAPI != _|_ {
    apiVersion: "batch/v1"
}
if context.clusterVersion.minor < 21 || parameter.legacyMode != _|_ {
    apiVersion: "batch/v1beta1"
}
if (parameter.isProduction && parameter.highAvailability) || parameter.forceHA != _|_ {
    spec: replicas: 3
}
```

**Multiple fields under the same condition:**

```go
// Use If().EndIf() block for multiple fields
deploy.
    If(defkit.And(persistence.IsSet(), persistence.Field("enabled"))).
        Set("spec.volumeClaimTemplates", []map[string]any{{...}}).
        Set("spec.template.spec.volumes", []map[string]any{{...}}).
    EndIf()
```

---

## RawCUE() Escape Hatch

For the ~5% of patterns that cannot be expressed in Go, use `RawCUE()`:

### True Unification Semantics

```go
// CUE's & operator has mathematical semantics Go cannot replicate
defkit.RawCUE(`
    baseConfig: {
        replicas: >=1
        image: string
    }
    prodConfig: baseConfig & {
        replicas: >=3 & <=10
    }
`)
```

### Complex Nested Comprehensions

```go
// Triple-nested comprehension with complex conditions
defkit.RawCUE(`
    result: [
        for ns in namespaces
        for svc in ns.services if svc.exposed
        for port in svc.ports if port.protocol == "TCP" {
            name: "\(ns.name)-\(svc.name)-\(port.port)"
        }
    ]
`)
```

---

## Parameter Types

| defkit Type | CUE Equivalent | Go Type |
|-------------|----------------|---------|
| `defkit.String("name")` | `name: string` | `string` |
| `defkit.Int("name")` | `name: int` | `int` |
| `defkit.Bool("name")` | `name: bool` | `bool` |
| `defkit.Float("name")` | `name: float` | `float64` |
| `defkit.List("name").WithFields(...)` | `name: [...{...}]` | `[]T` |
| `defkit.Map("name").Of(V)` | `name: {[string]: V}` | `map[string]V` |
| `defkit.Object("name").WithFields(...)` | `name: {field: type}` | struct |
| `defkit.Enum("name").Values("a", "b")` | `name: "a" \| "b"` | string with validation |
| `defkit.OneOf("name", ...)` | `name: close({...}) \| close({...})` | discriminated union |
| `defkit.Struct("name")` | `name: {...}` | struct with named fields |
| `defkit.StringList("name")` | `name: [...string]` | convenience alias |
| `defkit.IntList("name")` | `name: [...int]` | convenience alias |
| `defkit.StringKeyMap("name")` | `name: {[string]: string}` | string→string map |

### Parameter Modifiers

```go
defkit.String("image").
    Required().                    // Must be provided
    Default("nginx:latest").       // CUE: *"nginx:latest" | string
    Optional().                    // Can be omitted
    Description("Container image")  // Schema description

defkit.Int("replicas").
    Min(1).Max(100).              // Numeric constraints
    Default(3)

defkit.String("name").
    Pattern("^[a-z][a-z0-9-]*$")  // Regex validation
```

### Complex Parameter Types

**Object parameters** for nested configuration:

```go
persistence := defkit.Object("persistence").WithFields(
    defkit.Bool("enabled").Default(false),
    defkit.String("storageClass").Required(),
    defkit.String("size").Default("10Gi"),
).Optional()

// Usage in template - fluent conditional for optional object
deploy.
    If(defkit.And(persistence.IsSet(), persistence.Field("enabled"))).
    Set("spec.volumeClaimTemplates", []map[string]any{{
        "spec": map[string]any{
            "storageClassName": persistence.Field("storageClass"),
            "resources": map[string]any{
                "requests": map[string]any{
                    "storage": persistence.Field("size"),
                },
            },
        },
    }})
```

**Discriminated unions** for variant types:

```go
volume := defkit.OneOf("volume",
    defkit.Variant("emptyDir",
        defkit.String("medium").Default(""),
    ),
    defkit.Variant("pvc",
        defkit.String("claimName").Required(),
    ),
    defkit.Variant("configMap",
        defkit.String("name").Required(),
        defkit.Int("defaultMode").Default(420),
    ),
)

// Usage: volume.Discriminator() returns "emptyDir", "pvc", or "configMap"
```

---

## Collection Operations

defkit provides comprehensive collection operations for transforming arrays and lists. These are commonly used for patterns like ports, volumeMounts, and environment variables.

### Basic Collection Pipeline

```go
// Each() creates a collection pipeline from a list parameter
ports := defkit.Array("ports")

// Filter, transform, and map fields
exposedPorts := defkit.Each(ports).
    Filter(defkit.FieldEquals("expose", true)).
    Map(defkit.FieldMap{
        "containerPort": defkit.FieldRef("port"),
        "name":          defkit.FieldRef("name"),
        "protocol":      defkit.FieldRef("protocol"),
    }).
    DefaultField("name", defkit.Format("port-%v", defkit.FieldRef("port")))
```

### Collection Operations Reference

| Operation | Description | Example |
|-----------|-------------|---------|
| `Each(source)` | Start a collection pipeline | `defkit.Each(ports)` |
| `.Filter(pred)` | Keep items matching predicate | `.Filter(FieldEquals("expose", true))` |
| `.Map(fieldMap)` | Transform item fields | `.Map(FieldMap{"newKey": FieldRef("oldKey")})` |
| `.Pick(fields...)` | Select only specified fields | `.Pick("name", "mountPath")` |
| `.Rename(from, to)` | Rename a field | `.Rename("port", "containerPort")` |
| `.Wrap(key)` | Wrap item under a key | `.Wrap("name")` → `{name: value}` |
| `.DefaultField(f, v)` | Set default for missing field | `.DefaultField("name", Lit("default"))` |
| `.Flatten()` | Flatten nested arrays | `.Flatten()` |

### Multi-Source Collections

For patterns like volumeMounts where items come from multiple sub-fields:

```go
volumeMounts := defkit.Object("volumeMounts")

// Combine items from multiple sources (pvc, configMap, secret, etc.)
// Use tpl.Helper() inside the template function
mounts := tpl.Helper("mounts").
    FromFields(volumeMounts, "pvc", "configMap", "secret", "emptyDir", "hostPath").
    Pick("name", "mountPath").
    Dedupe("name").
    Build()
```

| Operation | Description | Example |
|-----------|-------------|---------|
| `FromFields(src, fields...)` | Combine items from multiple object fields | `FromFields(volumeMounts, "pvc", "configMap")` |
| `.MapBySource(map)` | Apply different mappings per source type | `.MapBySource(map[string]FieldMap{...})` |
| `.Dedupe(keyField)` | Remove duplicates by key | `.Dedupe("name")` |

### Field Value Helpers

| Helper | Description | Example |
|--------|-------------|---------|
| `FieldRef("field")` | Reference item field | `FieldRef("port")` |
| `FieldRef("f").Or(fallback)` | Field with fallback | `FieldRef("name").Or(LitField("default"))` |
| `LitField(value)` | Literal value | `LitField("TCP")` |
| `Format(fmt, args...)` | Formatted string | `Format("port-%v", FieldRef("port"))` |
| `Nested(fieldMap)` | Nested object | `Nested(FieldMap{"claimName": FieldRef("name")})` |
| `Optional("field")` | Include only if field exists | `Optional("subPath")` |
| `FieldEquals(f, v)` | Predicate: field equals value | `FieldEquals("expose", true)` |
| `FieldExists("field")` | Predicate: field is set | `FieldExists("mountPath")` |

---

## Helper Builder Pattern

For complex collection transformations (like volumeMounts and volumes), defkit provides a helper builder pattern that creates named template-level helpers.

### Basic Helper

```go
Template(func(tpl *defkit.Template) {
    ports := defkit.Array("ports")

    // Create a named helper for port mapping
    portsArray := tpl.Helper("portsArray").
        From(ports).
        Pick("port", "name", "protocol").
        Rename("port", "containerPort").
        DefaultField("name", defkit.Format("port-%v", defkit.FieldRef("port"))).
        Build()

    deploy := defkit.NewResource("apps/v1", "Deployment").
        Set("spec.template.spec.containers[0].ports", portsArray)

    tpl.Output(deploy)
})
```

### Helper Builder Methods

| Method | Description |
|--------|-------------|
| `tpl.Helper(name)` | Start building a named helper |
| `.From(source)` | Set single source |
| `.FromFields(src, fields...)` | Set multiple sources |
| `.FromHelper(helper)` | Reference another helper |
| `.Guard(cond)` | Outer condition for comprehension |
| `.Each(fn)` | Transform each item with function |
| `.Pick(fields...)` | Select fields |
| `.PickIf(cond, field)` | Conditionally include field |
| `.Map(fieldMap)` | Transform fields |
| `.MapBySource(map)` | Per-source transformations |
| `.Filter(cond)` | Filter by condition |
| `.FilterPred(pred)` | Filter by predicate |
| `.Wrap(key)` | Wrap items under key |
| `.Dedupe(keyField)` | Remove duplicates |
| `.DefaultField(f, v)` | Set default value |
| `.Rename(from, to)` | Rename field |
| `.AfterOutput()` | Place helper after output block |
| `.Build()` | Finalize and register helper |

### Advanced Helpers

For complex patterns like volumeMounts (struct-based arrays):

```go
// Struct array helper for mounting volumes
mountsArray := tpl.StructArrayHelper("mountsArray", volumeMounts).
    Field("pvc", defkit.FieldMap{"name": defkit.FieldRef("name"), "mountPath": defkit.FieldRef("mountPath")}).
    Field("configMap", defkit.FieldMap{"name": defkit.FieldRef("name"), "mountPath": defkit.FieldRef("mountPath")}).
    Field("secret", defkit.FieldMap{"name": defkit.FieldRef("name"), "mountPath": defkit.FieldRef("mountPath")}).
    Build()

// Volumes array with per-source mappings
volumesArray := tpl.StructArrayHelper("volumesArray", volumeMounts).
    Field("pvc", defkit.FieldMap{
        "name": defkit.FieldRef("name"),
        "persistentVolumeClaim": defkit.Nested(defkit.FieldMap{"claimName": defkit.FieldRef("claimName")}),
    }).
    Field("configMap", defkit.FieldMap{
        "name": defkit.FieldRef("name"),
        "configMap": defkit.Nested(defkit.FieldMap{"name": defkit.FieldRef("cmName")}),
    }).
    Build()

// Concat helper combines struct arrays
volumesList := tpl.ConcatHelper("volumesList", volumesArray).
    Fields("pvc", "configMap", "secret", "emptyDir", "hostPath").
    Build()

// Dedupe helper removes duplicates
deDupVolumes := tpl.DedupeHelper("deDupVolumesArray", volumesList).
    ByKey("name").
    Build()
```

### Helper Types

| Type | Purpose | Example |
|------|---------|---------|
| `*HelperVar` | Reference to basic helper | Returned by `Helper(...).Build()` |
| `*StructArrayHelper` | Struct with array fields | `tpl.StructArrayHelper(name, src)` |
| `*ConcatHelper` | Concatenate arrays | `tpl.ConcatHelper(name, src)` |
| `*DedupeHelper` | Deduplicate by key | `tpl.DedupeHelper(name, src)` |

---

## Definition Types

### ComponentDefinition

```go
func init() {
    defkit.Register(MyComponent())
}

func MyComponent() *defkit.ComponentDefinition {
    image := defkit.String("image").Required()

    return defkit.NewComponent("webservice").
        Description("...").
        Workload("apps/v1", "Deployment").
        Params(image).
        Template(func(tpl *defkit.Template) {
            vela := defkit.VelaCtx()
            deploy := defkit.NewResource("apps/v1", "Deployment").
                Set("metadata.name", vela.Name()).
                Set("spec.template.spec.containers[0].image", image)
            tpl.Output(deploy)
        }).
        HealthPolicy(defkit.DeploymentHealth().Build()).
        CustomStatus(defkit.DeploymentStatus().Build())
}
```

### TraitDefinition

```go
func init() {
    defkit.Register(RateLimitTrait())
}

func RateLimitTrait() *defkit.TraitDefinition {
    rps := defkit.Int("rps").Required()

    return defkit.NewTrait("rate-limit").
        Description("...").
        AppliesTo("webservice", "microservice").
        Params(rps).
        Template(func(tpl *defkit.Template) {
            // Trait templates patch the workload
            tpl.Patch().Set("metadata.annotations[ratelimit.example.com/rps]", rps)
        })
}
```

### PolicyDefinition

```go
// Example: Topology policy for multi-cluster deployment
func Topology() *defkit.PolicyDefinition {
    clusters := defkit.StringList("clusters").Description("Specify the names of the clusters to select.")
    clusterLabelSelector := defkit.StringKeyMap("clusterLabelSelector").Description("Specify the label selector for clusters")
    allowEmpty := defkit.Bool("allowEmpty").Description("Ignore empty cluster error")
    namespace := defkit.String("namespace").Description("Specify the target namespace to deploy in the selected clusters")

    return defkit.NewPolicy("topology").
        Description("Describe the destination where components should be deployed to.").
        Params(clusters, clusterLabelSelector, allowEmpty, namespace)
}
```

**Example policies**: topology, apply-once, garbage-collect, override, read-only, replication, resource-update, shared-resource, take-over

### WorkflowStepDefinition

```go
// Example: Deploy step with multi-cluster support
func Deploy() *defkit.WorkflowStepDefinition {
    return defkit.NewWorkflowStep("deploy").
        Description("A powerful and unified deploy step for components multi-cluster delivery with policies.").
        Category("Application Delivery").
        Scope("Application").
        WithImports("vela/multicluster", "vela/builtin").
        RawCUE(`...`) // Complex CUE patterns use RawCUE() escape hatch
}
```

**Example workflow steps**: deploy, suspend, apply-component, apply-deployment, apply-object, apply-terraform-config, apply-terraform-provider, build-push-image, check-metrics, clean-jobs, collect-service-endpoints, create-config, delete-config, depends-on-app, deploy-cloud-resource, export-data, export-service, export2config, export2secret, generate-jdbc-connection, list-config, notification, print-message-in-status, read-config, read-object, request, share-cloud-resource, step-group, webhook

---

## Schema-Agnostic Resource Construction

defkit does NOT ship typed Kubernetes helpers. Instead, it provides a universal builder that works with any resource:

```go
// Core Kubernetes
defkit.NewResource("apps/v1", "Deployment").
    SetName("my-app").
    Set("spec.replicas", 3)

// CRDs (Crossplane, KRO, etc.)
defkit.NewResource("database.aws.crossplane.io/v1beta1", "DBInstance").
    SetName("my-db").
    Set("spec.forProvider.engine", "postgres")
```

**Optional typed adapter** for users who want compile-time type safety:

```go
import appsv1 "k8s.io/api/apps/v1"

deployment := &appsv1.Deployment{...}
return defkit.FromTyped(deployment)
```

---

## CLI Commands

The following commands extend the existing `vela def` command group to support Go definitions:

```bash
# Apply Go definitions (extends existing `vela def apply`)
# CUE compilation is transparent - .go files are automatically detected
vela def apply ./definitions/webservice.go

# Apply all definitions in directory (supports mixed .cue and .go)
vela def apply ./definitions/

# Dry-run to see generated resources (existing flag)
vela def apply ./definitions/ --dry-run

# Validate Go definitions without applying (existing command, extended for .go)
vela def vet ./definitions/webservice.go

# Initialize a new Go definition module with scaffolding
vela def init-module ./my-definitions --name my-definitions

# Validate all definitions in a Go module without cluster connection
vela def validate-module ./my-definitions
vela def validate-module ./my-definitions --verbose
```

### New Subcommands

| Command | Description |
|---------|-------------|
| `vela def init-module` | Scaffold a complete Go definition module with example components |
| `vela def validate-module` | Validate all Go definitions in a module without requiring a cluster |
| `vela def gen-go` | Generate Go defkit code from existing CUE definitions (Phase 3 - migration) |

### Extended Existing Commands

| Command | Extension |
|---------|-----------|
| `vela def apply` | Accepts `.go` files, compiles to CUE transparently |
| `vela def vet` | Validates `.go` definitions including Go compilation |
| `vela def gen-api` | Already generates Go SDK from CUE (inverse direction) |

### Module Scaffolding

The `init-module` command creates a complete Go module structure:

```bash
$ vela def init-module ./my-platform --name my-platform

Created Go definition module at ./my-platform:
  go.mod                    - Go module definition
  components/
    webservice.go           - Example webservice component
    webservice_test.go      - Example tests
  main.go                   - Module entry point for validation
```

The scaffolded module can be validated immediately:

```bash
$ cd my-platform
$ vela def validate-module .
Validating Go definition module...
Found 1 definitions
✓ webservice (ComponentDefinition) - CUE validation passed
All definitions validated successfully
```

### Module Versioning

Definition modules use **git tags** for versioning rather than storing version in `module.yaml`. This follows Go module conventions and provides a single source of truth for version information.

**How Version is Derived:**

When a module is loaded (via `vela def apply-module`, `list-module`, `validate-module`, or `gen-module`), the version is automatically derived from git in the following order:

| Priority | Git Command | Example Output | Description |
|----------|-------------|----------------|-------------|
| 1 | `git describe --tags --exact-match HEAD` | `v1.0.0` | Exact tag on current commit |
| 2 | `git describe --tags --always` | `v1.0.0-5-gabcdef` | Tag with commit distance |
| 3 | `git rev-parse --short HEAD` | `v0.0.0-dev+abcdef` | Commit hash only |
| 4 | (fallback) | `v0.0.0-local` | Not in a git repository |

**Best Practices:**

```bash
# Tag releases with semantic versions
git tag v1.0.0
git push origin v1.0.0

# For pre-release versions
git tag v1.0.0-beta.1
git tag v1.0.0-rc.1

# View current derived version
vela def list-module .  # Shows version in module summary
```

**Why Git-Based Versioning?**

1. **Single source of truth**: Version is defined once in git, not duplicated in metadata files
2. **Go module alignment**: Follows the same versioning model as Go modules (`go get module@v1.0.0`)
3. **CI/CD friendly**: Version tags integrate naturally with release workflows
4. **Immutable releases**: Tagged commits provide reproducible builds

**Example module.yaml (no version field):**

```yaml
apiVersion: core.oam.dev/v1beta1
kind: DefinitionModule
metadata:
  name: my-platform
spec:
  description: Platform definitions for my organization
  maintainers:
    - name: Platform Team
      email: platform@example.com
  minVelaVersion: v1.9.0
  categories:
    - platform
    - production
```

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    Definition Authoring Pipeline                 │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐       │
│  │   defkit     │    │      IR      │    │   Compiler   │       │
│  │   Go API     │───▶│   (JSON)     │───▶│   Go → CUE   │──▶ CR │
│  │              │    │              │    │              │       │
│  │ tpl.Output() │    │ • Schema     │    │ • Validation │       │
│  │ param vars   │    │ • Template   │    │ • CUE Gen    │       │
│  └──────────────┘    └──────────────┘    └──────────────┘       │
│                                                                  │
│  (CUE compilation is transparent - developers never see it)      │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Compilation Approach

The SDK uses **declarative capture** rather than Go AST transformation:
- Template functions execute with a tracing context
- Each `Set()`, `If()`, `tpl.Output()` call is recorded
- Parameter variables track their usage and generate corresponding CUE paths
- Recorded operations form a declarative tree that maps directly to CUE

### How Parameter Variables Work

Parameter variables like `image := defkit.String("image").Required()` are **expression builders**, not value holders:

```go
// What you write:
image := defkit.String("image").Required()
deploy.Set("spec.image", image)

// What happens internally:
// 1. defkit.String("image") creates a Param{name: "image", type: "string"}
// 2. deploy.Set() receives this Param and records: SetOp{path: "spec.image", value: ParamRef("image")}
// 3. During compilation, ParamRef("image") becomes `parameter.image` in CUE
```

**Conditional handling** uses the fluent API rather than Go's `if`:

```go
// For optional parameters, use SetIf or the If() fluent method:
deploy.SetIf(cpu.IsSet(), "spec.resources.limits.cpu", cpu)

// Or with fluent chaining:
deploy.If(cpu.IsSet()).Set("spec.resources.limits.cpu", cpu)

// Both compile to CUE:
// if parameter.cpu != _|_ {
//     spec: resources: limits: cpu: parameter.cpu
// }
```

This approach is similar to how query builders (like GORM, Squirrel) work - the Go code describes operations declaratively without executing them at definition time.

---

## Testing

defkit provides testing support using Ginkgo and Gomega with custom matchers, enabling BDD-style test-driven development without requiring a Kubernetes cluster.

### Test Levels

| Test Level | Framework | Cluster Required | What It Tests |
|------------|-----------|------------------|---------------|
| Unit tests | Ginkgo/Gomega | No | Parameter validation, template output, conditional logic |
| CUE compilation | Ginkgo/Gomega | No | Generated CUE is syntactically valid |
| Integration | envtest | No | Controller reconciliation with fake cluster |
| E2E | Full cluster | Yes | Complete deployment lifecycle |

### Custom Matchers

defkit provides custom Gomega matchers for readable, expressive tests:

```go
// Resource type matchers
BeDeployment(), BeService(), BeIngress(), BeConfigMap(), BeSecret()
BeResourceOfKind(kind string)

// Metadata matchers
HaveAPIVersion(version string), HaveName(name string), HaveNamespace(ns string)
HaveLabel(key, value string), HaveAnnotation(key, value string)

// Spec matchers
HaveReplicas(count int), HaveImage(image string), HaveContainerNamed(name string)
HavePort(port int), HaveEnvVar(name, value string)
HaveResourceLimit(resource, value string), HaveResourceRequest(resource, value string)

// Path-based matcher for any field
HaveFieldPath(path string, value any)

// Validation and health matchers
FailValidationWith(substring string), PassValidation()
BeHealthy(), BeUnhealthy(), HaveHealthMessage(msg string)
```

### Example: Testing a ComponentDefinition

```go
package webservice_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/oam-dev/kubevela/pkg/definition/defkit"
    . "github.com/oam-dev/kubevela/pkg/definition/defkit/testing/matchers"
)

var _ = Describe("Webservice ComponentDefinition", func() {
    var def *defkit.ComponentDefinition

    BeforeEach(func() {
        def = webservice.Webservice()
    })

    Describe("Template Rendering", func() {
        It("should render a deployment with defaults", func() {
            ctx := defkit.TestContext().
                WithName("my-app").
                WithNamespace("production").
                WithParam("image", "nginx:1.21")

            output := def.Render(ctx)

            Expect(output).To(BeDeployment())
            Expect(output).To(HaveAPIVersion("apps/v1"))
            Expect(output).To(HaveName("my-app"))
            Expect(output).To(HaveReplicas(3)) // default value
            Expect(output).To(HaveImage("nginx:1.21"))
        })

        It("should set resource limits when cpu is provided", func() {
            ctx := defkit.TestContext().
                WithName("my-app").
                WithParam("image", "nginx:1.21").
                WithParam("cpu", "500m")

            Expect(def.Render(ctx)).To(HaveResourceLimit("cpu", "500m"))
        })
    })

    Describe("Parameter Validation", func() {
        It("should fail when required image is missing", func() {
            ctx := defkit.TestContext().WithName("my-app")
            Expect(def.Validate(ctx)).To(FailValidationWith("image is required"))
        })

        It("should fail when replicas exceeds maximum", func() {
            ctx := defkit.TestContext().
                WithName("my-app").
                WithParam("image", "nginx:1.21").
                WithParam("replicas", 200)

            Expect(def.Validate(ctx)).To(FailValidationWith("replicas must be <= 100"))
        })
    })

    Describe("Health Policy", func() {
        It("should report healthy when all replicas are ready", func() {
            ctx := defkit.TestContext().
                WithName("my-app").
                WithParam("image", "nginx:1.21").
                WithParam("replicas", 3).
                WithOutputStatus(map[string]any{"readyReplicas": 3})

            Expect(def.EvaluateHealth(ctx)).To(BeHealthy())
            Expect(def.EvaluateHealth(ctx)).To(HaveHealthMessage("Ready: 3/3"))
        })
    })
})
```

### Table-Driven Tests

```go
var def = webservice.Webservice() // definition under test

var _ = DescribeTable("parameter combinations",
    func(params map[string]any, expectedField string, expectedValue any, shouldFail bool) {
        ctx := defkit.TestContext().WithName("test")
        for k, v := range params {
            ctx = ctx.WithParam(k, v)
        }

        if shouldFail {
            Expect(def.Validate(ctx)).NotTo(PassValidation())
            return
        }
        Expect(def.Render(ctx)).To(HaveFieldPath(expectedField, expectedValue))
    },
    Entry("minimal config uses default replicas",
        map[string]any{"image": "nginx:1.21"}, "spec.replicas", 3, false),
    Entry("custom replicas are respected",
        map[string]any{"image": "nginx:1.21", "replicas": 5}, "spec.replicas", 5, false),
    Entry("missing image fails validation",
        map[string]any{"replicas": 3}, "", nil, true),
)
```

### Testing Traits

```go
var _ = Describe("RateLimit Trait", func() {
    It("should patch workload with annotation", func() {
        workload := defkit.NewResource("apps/v1", "Deployment").SetName("my-app")

        ctx := defkit.TestContext().
            WithWorkload(workload).
            WithParam("rps", 1000)

        patches := ratelimit.New().Patch(ctx)
        Expect(patches).To(HaveAnnotation("ratelimit.example.com/rps", "1000"))
    })
})
```

### Testing Cluster Version Conditionals

```go
var _ = Describe("CronJob", func() {
    It("should use v1beta1 on old clusters", func() {
        ctx := defkit.TestContext().
            WithName("my-job").
            WithParam("schedule", "0 * * * *").
            WithClusterVersion(1, 24)

        Expect(cronjob.New().Render(ctx)).To(HaveAPIVersion("batch/v1beta1"))
    })

    It("should use v1 on new clusters", func() {
        ctx := defkit.TestContext().
            WithName("my-job").
            WithParam("schedule", "0 * * * *").
            WithClusterVersion(1, 25)

        Expect(cronjob.New().Render(ctx)).To(HaveAPIVersion("batch/v1"))
    })
})
```

### What to Test vs What NOT to Test

#### DO Test

| What | Why | Example |
|------|-----|---------|
| Parameter validation | Catch invalid inputs early | Required fields, min/max constraints |
| Template output structure | Ensure correct K8s resources | Resource kind, API version, spec fields |
| Conditional logic | Verify branches work correctly | If cpu is set, limits are added |
| Default values | Ensure defaults are applied | replicas defaults to 3 |
| Health policy evaluation | Verify health checks work | Ready when replicas match |
| Auxiliary outputs | Verify all resources generated | Service, Ingress alongside Deployment |
| Edge cases | Handle unusual inputs | Empty arrays, nil values |

#### DO NOT Test

| What | Why |
|------|-----|
| CUE language behavior | CUE is well-tested; trust it |
| Controller reconciliation logic | Out of defkit's scope |
| Kubernetes API behavior | Not your code |
| Network operations | Use mocks/fakes for integration tests |

### Testing Anti-Patterns to Avoid

```go
// ❌ BAD: Testing CUE internals
func TestBad(t *testing.T) {
    cue, _ := def.ToCUE()
    assert.Contains(t, cue, "replicas: parameter.replicas") // Too brittle
}

// ✅ GOOD: Test the behavior, not the implementation
func TestGood(t *testing.T) {
    ctx := defkit.TestContext().WithParam("replicas", 5)
    output := def.Render(ctx)
    assert.Equal(t, 5, output.Get("spec.replicas"))
}

// ❌ BAD: Hardcoding exact output
func TestBad2(t *testing.T) {
    output := def.Render(ctx)
    expected := `{"apiVersion":"apps/v1",...}` // Brittle, hard to maintain
    assert.Equal(t, expected, output.ToJSON())
}

// ✅ GOOD: Assert on specific fields you care about
func TestGood2(t *testing.T) {
    output := def.Render(ctx)
    assert.Equal(t, "apps/v1", output.APIVersion())
    assert.Equal(t, 3, output.Get("spec.replicas"))
}
```

### Integration Testing with envtest

For testing controller integration without a full cluster:

```go
package integration_test

import (
    "context"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
    "sigs.k8s.io/controller-runtime/pkg/envtest"

    "myplatform/definitions/webservice"
)

func TestWebserviceIntegration(t *testing.T) {
    // Start envtest
    testEnv := &envtest.Environment{}
    cfg, err := testEnv.Start()
    require.NoError(t, err)
    defer testEnv.Stop()

    // Create client
    k8sClient, err := client.New(cfg, client.Options{})
    require.NoError(t, err)

    // Apply definition
    def := webservice.New()
    cr, err := def.BuildCR()
    require.NoError(t, err)

    err = k8sClient.Create(context.Background(), cr)
    require.NoError(t, err)

    // Verify definition was created
    var retrieved v1beta1.ComponentDefinition
    err = k8sClient.Get(context.Background(),
        types.NamespacedName{Name: "webservice", Namespace: "vela-system"},
        &retrieved)
    require.NoError(t, err)
    assert.Equal(t, "webservice", retrieved.Name)
}
```

---

## Definition Deployment Workflow

### Development Cycle

```
┌─────────────────────────────────────────────────────────────────┐
│                    Definition Development Cycle                   │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  1. WRITE          2. TEST           3. VALIDATE    4. APPLY    │
│  ┌──────────┐     ┌──────────┐      ┌──────────┐  ┌──────────┐  │
│  │  Go Code │────▶│go test   │─────▶│vela def  │─▶│vela def  │  │
│  │          │     │          │      │vet       │  │apply     │  │
│  └──────────┘     └──────────┘      └──────────┘  └──────────┘  │
│       │                │                  │             │        │
│       │                │                  │             │        │
│       └────────────────┴──────────────────┴─────────────┘        │
│                         ITERATE                                  │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### CI/CD Integration

```yaml
# .github/workflows/definitions.yaml
name: Definition CI

on:
  push:
    paths:
      - 'definitions/**'

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.23'

      - name: Run Unit Tests
        run: go test ./definitions/... -v

      - name: Validate Definitions
        run: vela def vet ./definitions/

      - name: Generate CUE (dry-run)
        run: vela def apply ./definitions/ --dry-run

  deploy:
    needs: test
    if: github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Apply Definitions
        run: vela def apply ./definitions/
        env:
          KUBECONFIG: ${{ secrets.KUBECONFIG }}
```

---

## Distribution

### Go Modules

Go definitions are distributed as standard Go packages:

```go
// go.mod for your definitions package
module github.com/myorg/platform-defs

require github.com/oam-dev/kubevela v1.10.0  // includes pkg/definition/defkit
```

```bash
# Consumers import your definitions
go get github.com/myorg/platform-defs@v1.0.0
```

```go
// Use in application code
import "github.com/myorg/platform-defs/components/webservice"

app := application.New("my-app").
    AddComponent(webservice.New("api").Image("myapp:v1"))
```

This leverages Go's existing ecosystem: semantic versioning, checksums, proxy caching, and private module support.

### Module Dependencies

Defkit modules could import definitions from other Go modules, enabling composition and reuse across teams and organizations:

```go
// go.mod
module github.com/myorg/platform-defs

require (
    github.com/oam-dev/kubevela v1.10.0          // defkit SDK
    github.com/myorg/base-components v1.2.0      // shared base definitions
    github.com/anotherorg/monitoring-defs v2.0.0 // third-party definitions
)
```

```go
// components/monitored_webservice.go
package components

import (
    "github.com/oam-dev/kubevela/pkg/definition/defkit"
    "github.com/myorg/base-components/components"       // import base webservice
    "github.com/anotherorg/monitoring-defs/traits"      // import monitoring traits
)

// MonitoredWebservice composes a webservice with monitoring capabilities
func MonitoredWebservice() *defkit.ComponentDefinition {
    // Extend or compose with imported definitions
    base := components.WebserviceParams()

    // Add monitoring-specific parameters
    metricsPort := defkit.Int("metricsPort").Default(9090)
    enableTracing := defkit.Bool("enableTracing").Default(true)

    return defkit.NewComponent("monitored-webservice").
        Description("Webservice with built-in monitoring").
        Workload("apps/v1", "Deployment").
        Params(append(base, metricsPort, enableTracing)...).
        Template(monitoredWebserviceTemplate)
}
```

This would enable:
- **Layered platforms**: Base definitions from central team, extended by product teams
- **Third-party ecosystems**: Community-contributed definition libraries
- **Version control**: Semantic versioning for definition dependencies via Go modules

### Addon Integration

KubeVela addons support Go-based definitions via the `godef/` folder:

```
my-addon/
├── metadata.yaml           # Addon metadata
├── definitions/            # Traditional CUE definitions (optional)
├── godef/                  # Go-based definitions
│   ├── module.yaml         # DefKit module configuration
│   ├── go.mod
│   ├── components/
│   │   └── webservice.go
│   └── traits/
│       └── scaler.go
└── README.md
```

**Initialize addon with Go definitions:**

```bash
# Basic scaffolding
vela addon init my-addon --godef

# With specific definitions
vela addon init my-addon --godef \
    --components webservice,worker \
    --traits scaler,ingress
```

**Enable addon:**

```bash
# Go definitions are automatically compiled to CUE
vela addon enable ./my-addon

# If both CUE and Go define the same name, use --override-definitions
vela addon enable ./my-addon --override-definitions
```

**Conflict detection**: If a definition name exists in both `definitions/` and `godef/`, addon enable fails with an error unless `--override-definitions` is specified (Go takes precedence).

**Development workflow**:

```bash
cd my-addon/godef
go mod tidy           # Resolve dependencies
go test ./...         # Test definitions locally
cd ..
vela addon enable .   # Deploy to cluster
```

### GitOps

```bash
# Render Go definitions to CUE/YAML for GitOps (extends `vela def render`)
vela def render ./definitions/ --output ./dist/ --format yaml

# Commit and sync with ArgoCD/Flux
git add ./dist/
git commit -m "Update definitions"
git push
```

---

## Module Hooks

Module hooks provide lifecycle management for definition modules, enabling actions before and after definitions are applied.

### Use Cases

- **CRD installation**: Install CRDs and wait for them to be established before applying definitions that depend on them
- **Setup scripts**: Create namespaces, install operators, or run migrations
- **Post-install samples**: Apply example applications after definitions are deployed

### Configuration

Hooks are declared in `module.yaml`:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: DefinitionModule
metadata:
  name: my-module
spec:
  hooks:
    pre-apply:
      - path: hooks/crds/
        wait: true
        waitFor: Established
        timeout: "2m"
      - script: hooks/setup.sh
        optional: true
    post-apply:
      - path: hooks/samples/
        optional: true
```

### Hook Types

| Type | Description |
|------|-------------|
| `path` | Apply YAML manifests from a directory (alphabetically ordered) |
| `script` | Execute a shell script with `MODULE_PATH` and `NAMESPACE` env vars |

### The `waitFor` Field

Different resources have different readiness semantics. The `waitFor` field supports:

**Simple condition name** (for standard Kubernetes conditions):
```yaml
waitFor: Established    # CRDs
waitFor: Ready          # Most resources
waitFor: Available      # Deployments
```

**CUE expression** (for complex readiness logic):
```yaml
waitFor: "status.replicas == status.readyReplicas"
waitFor: 'status.phase == "Running"'
waitFor: "status.succeeded >= 1"
```

### CLI Usage

```bash
vela def apply-module ./my-module               # Run with hooks
vela def apply-module ./my-module --skip-hooks  # Skip all hooks
vela def apply-module ./my-module --dry-run     # Preview without applying
vela def apply-module ./my-module --stats       # Show module statistics (definitions, hooks, placement)
```

---

## Definition Placement

### Motivation

In enterprise environments, organizations often manage multiple Kubernetes clusters with different characteristics:

- **Cloud provider clusters**: EKS (AWS), GKE (Google Cloud), AKS (Azure)
- **Virtual clusters**: vclusters running inside host clusters for dev/test isolation
- **On-premises clusters**: Self-managed Kubernetes in data centers
- **Environment tiers**: Production, staging, development clusters

Not all definitions are appropriate for all cluster types. For example:

| Definition | Should Run On | Should NOT Run On |
|------------|---------------|-------------------|
| `aws-load-balancer` | EKS clusters | GKE, AKS, vclusters |
| `gcp-cloud-sql` | GKE clusters | EKS, AKS, on-prem |
| `dev-namespace-provisioner` | vclusters, dev clusters | Production clusters |
| `production-pdb` | Production clusters | Dev/test clusters |
| `lightweight-ingress` | vclusters | Full clusters with real LBs |

Without placement constraints, platform engineers must manually track which definitions belong where, leading to:
- Accidental deployment of cloud-specific definitions to wrong providers
- Production-grade components wasting resources in dev environments
- Definitions failing at runtime because required infrastructure isn't available

### Solution: Definition Placement Constraints

Definition Placement allows authors to declare **where a definition can run** using cluster labels. This provides:

1. **Guardrails**: Prevent accidental misdeployment
2. **Self-documenting**: Definition declares its requirements
3. **Automation-friendly**: CI/CD can validate before deployment
4. **Multi-cloud support**: Same module can contain definitions for different providers

### Cluster Labels

Clusters are identified by labels stored in a ConfigMap:

```yaml
# vela-system/vela-cluster-identity ConfigMap
apiVersion: v1
kind: ConfigMap
metadata:
  name: vela-cluster-identity
  namespace: vela-system
data:
  provider: aws
  cluster-type: eks
  environment: production
  region: us-east-1
  team: platform
```

**Well-known labels**:

| Label | Values | Description |
|-------|--------|-------------|
| `provider` | `aws`, `gcp`, `azure`, `on-prem`, `local` | Cloud provider |
| `cluster-type` | `eks`, `gke`, `aks`, `vcluster`, `kind`, `k3s`, `openshift` | Cluster type |
| `environment` | `production`, `staging`, `dev`, `test` | Environment tier |
| `region` | `us-east-1`, `eu-west-1`, etc. | Geographic region |

**Custom labels** can be added for organization-specific needs (team, cost-center, tier, etc.).

### Fluent API for Placement

Placement constraints use a fluent API in the `placement` package:

```go
import (
    "github.com/oam-dev/kubevela/pkg/definition/defkit"
    "github.com/oam-dev/kubevela/pkg/definition/defkit/placement"
)
```

#### Label Condition Builder

```go
placement.Label("provider")         // Returns a label condition builder
    .Eq("aws")                      // Equals
    .Ne("azure")                    // Not equals
    .In("aws", "gcp", "azure")      // In list (OR)
    .NotIn("on-prem", "local")      // Not in list
    .Exists()                       // Label exists (any value)
    .NotExists()                    // Label doesn't exist
```

#### Logical Combinators

```go
placement.All(cond1, cond2, ...)   // AND - all conditions must match
placement.Any(cond1, cond2, ...)   // OR - at least one must match
placement.Not(cond)                // NOT - negates the condition
```

#### RunOn / NotRunOn Methods

```go
func AwsLoadBalancer() *defkit.ComponentDefinition {
    return defkit.NewComponent("aws-load-balancer").
        Description("AWS Application Load Balancer ingress controller").
        RunOn(
            placement.Label("provider").Eq("aws"),
            placement.Label("cluster-type").In("eks", "self-managed"),
        ).
        NotRunOn(
            placement.Label("cluster-type").Eq("vcluster"),
        ).
        Params(...).
        Template(...)
}
```

### Placement Examples

#### Simple: Single Provider

```go
// Only runs on AWS
func S3Bucket() *defkit.ComponentDefinition {
    return defkit.NewComponent("s3-bucket").
        RunOn(placement.Label("provider").Eq("aws"))
}
```

#### Multiple Conditions (Implicit AND)

```go
// AWS EKS in production only
func ProductionALB() *defkit.ComponentDefinition {
    return defkit.NewComponent("production-alb").
        RunOn(
            placement.Label("provider").Eq("aws"),
            placement.Label("cluster-type").Eq("eks"),
            placement.Label("environment").Eq("production"),
        )
}
```

#### OR Conditions

```go
// Runs on any major cloud provider
func MultiCloudLB() *defkit.ComponentDefinition {
    return defkit.NewComponent("multi-cloud-lb").
        RunOn(
            placement.Any(
                placement.Label("provider").Eq("aws"),
                placement.Label("provider").Eq("gcp"),
                placement.Label("provider").Eq("azure"),
            ),
        )
}

// Simpler with In()
func MultiCloudLBSimpler() *defkit.ComponentDefinition {
    return defkit.NewComponent("multi-cloud-lb").
        RunOn(placement.Label("provider").In("aws", "gcp", "azure"))
}
```

#### Complex: Nested Logic

```go
// (AWS EKS OR GCP GKE) AND production AND NOT vcluster
func EnterpriseIngress() *defkit.ComponentDefinition {
    return defkit.NewComponent("enterprise-ingress").
        RunOn(
            placement.All(
                placement.Any(
                    placement.All(
                        placement.Label("provider").Eq("aws"),
                        placement.Label("cluster-type").Eq("eks"),
                    ),
                    placement.All(
                        placement.Label("provider").Eq("gcp"),
                        placement.Label("cluster-type").Eq("gke"),
                    ),
                ),
                placement.Label("environment").Eq("production"),
            ),
        ).
        NotRunOn(
            placement.Label("cluster-type").Eq("vcluster"),
        )
}
```

#### Exclusion Only

```go
// Runs everywhere EXCEPT staging
func NotForStaging() *defkit.TraitDefinition {
    return defkit.NewTrait("production-pdb").
        NotRunOn(placement.Label("environment").Eq("staging"))
}
```

#### No Constraints (Universal)

```go
// Runs on all clusters (no placement constraints)
func UniversalConfigMap() *defkit.ComponentDefinition {
    return defkit.NewComponent("configmap-generator")
    // No RunOn/NotRunOn = applies everywhere
}
```

### Module-Level Placement Defaults

Modules can specify default placement for all definitions:

```yaml
# module.yaml
apiVersion: core.oam.dev/v1beta1
kind: DefinitionModule
metadata:
  name: aws-definitions
spec:
  description: AWS-specific KubeVela definitions

  # Default placement for all definitions in this module
  placement:
    runOn:
      - provider = aws
    notRunOn:
      - cluster-type = vcluster
```

**Inheritance behavior:**
- Definition without `RunOn`/`NotRunOn` → inherits module defaults
- Definition with `RunOn`/`NotRunOn` → uses its own constraints (overrides module)

### Placement Evaluation Logic

```
┌─────────────────────────────────────────────────────────────────┐
│                    Placement Evaluation                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  eligible = true                                                 │
│                                                                  │
│  if RunOn is specified:                                          │
│      eligible = cluster labels MATCH RunOn conditions            │
│                                                                  │
│  if NotRunOn is specified:                                       │
│      eligible = eligible AND NOT(cluster labels MATCH NotRunOn)  │
│                                                                  │
│  Final: Apply definition only if eligible = true                 │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

| RunOn | NotRunOn | Cluster Matches RunOn | Cluster Matches NotRunOn | Result |
|-------|----------|----------------------|-------------------------|--------|
| not set | not set | n/a | n/a | ✓ Apply |
| set | not set | yes | n/a | ✓ Apply |
| set | not set | no | n/a | ✗ Skip |
| not set | set | n/a | no | ✓ Apply |
| not set | set | n/a | yes | ✗ Skip |
| set | set | yes | no | ✓ Apply |
| set | set | yes | yes | ✗ Skip |
| set | set | no | no | ✗ Skip |
| set | set | no | yes | ✗ Skip |

### CLI Experience

#### Managing Cluster Labels

Cluster labels for placement decisions are stored in the `vela-cluster-identity` ConfigMap in the `vela-system` namespace (see [Cluster Labels](#cluster-labels) section above).

```bash
# View current cluster's labels (reads from ConfigMap)
$ kubectl get configmap vela-cluster-identity -n vela-system -o yaml
apiVersion: v1
kind: ConfigMap
data:
  provider: aws
  cluster-type: eks
  environment: production
  team: platform

# Set labels by editing the ConfigMap
$ kubectl edit configmap vela-cluster-identity -n vela-system
```

> **Note**: Integration with `vela cluster labels` command is planned for a future release. Currently, labels must be managed directly via the ConfigMap.

#### Applying Definitions - Success Case

```bash
$ vela def apply-module ./aws-definitions

Loading module: aws-definitions (v1.2.0)
Checking cluster placement...

Cluster: (current)
  provider: aws
  cluster-type: eks
  environment: production

Definitions to apply:
  ✓ aws-alb-controller      [component]  placement: OK
  ✓ aws-ebs-provisioner     [component]  placement: OK
  ✓ aws-cloudwatch-logs     [trait]      placement: OK

Applying 3 definitions to namespace vela-system...
  ✓ aws-alb-controller applied
  ✓ aws-ebs-provisioner applied
  ✓ aws-cloudwatch-logs applied

Successfully applied 3 definitions.
```

#### Applying Definitions - Partial Match

```bash
$ vela def apply-module ./multi-cloud-definitions

Loading module: multi-cloud-definitions (v1.0.0)
Checking cluster placement...

Cluster: (current)
  provider: aws
  cluster-type: eks

Definitions to apply:
  ✓ universal-scaler        [trait]      placement: OK (no constraints)
  ✓ aws-alb-controller      [component]  placement: OK
  ✗ gcp-load-balancer       [component]  placement: SKIP
    └─ requires: provider = gcp
  ✗ azure-disk              [component]  placement: SKIP
    └─ requires: provider = azure

Applying 2 definitions...
  ✓ universal-scaler applied
  ✓ aws-alb-controller applied

Skipped 2 definitions (placement constraints not met).
Successfully applied 2 definitions.
```

#### Applying Definitions - All Blocked

```bash
$ vela def apply-module ./aws-definitions

Loading module: aws-definitions (v1.2.0)
Checking cluster placement...

Cluster: (current)
  provider: gcp
  cluster-type: gke

Definitions to apply:
  ✗ aws-alb-controller      [component]  placement: SKIP
    └─ requires: provider = aws
  ✗ aws-ebs-provisioner     [component]  placement: SKIP
    └─ requires: provider = aws

No definitions match this cluster's placement constraints.

Hint: Use --ignore-placement to force apply (admin override).
```

#### NotRunOn Exclusion

```bash
$ vela def apply-module ./platform-definitions

Cluster: (current)
  provider: aws
  cluster-type: vcluster
  environment: dev

Definitions to apply:
  ✓ dev-namespace-provisioner  [component]  placement: OK
  ✗ production-lb              [component]  placement: SKIP
    └─ excluded by: cluster-type = vcluster (notRunOn)

Applying 1 definition...
```

#### Dry Run with Placement Details

```bash
$ vela def apply-module ./aws-definitions --dry-run

Loading module: aws-definitions (v1.2.0)

Cluster: provider=aws, cluster-type=eks, environment=production

─────────────────────────────────────────────────────────
Definition: aws-alb-controller (ComponentDefinition)
─────────────────────────────────────────────────────────
Placement:
  runOn:
    - provider = aws
    - cluster-type IN [eks, self-managed]
  notRunOn:
    - cluster-type = vcluster

Evaluation:
  ✓ provider = aws             → matches (cluster: aws)
  ✓ cluster-type IN [eks, ...] → matches (cluster: eks)
  ✓ NOT cluster-type = vcluster → passes (cluster: eks)

Status: WOULD APPLY
```

#### Force Apply (Admin Override)

```bash
$ vela def apply-module ./aws-definitions --ignore-placement

⚠️  WARNING: Ignoring placement constraints.
    Definitions may not work correctly on this cluster.

Proceed? [y/N]: y

Applying 3 definitions (placement ignored)...
  ✓ aws-alb-controller applied
  ✓ aws-ebs-provisioner applied
  ✓ aws-cloudwatch-logs applied
```

#### List with Placement Check

```bash
$ vela def list-module ./aws-definitions --check-placement

Module: aws-definitions (v1.2.0)
Current cluster: provider=gcp, cluster-type=gke

NAME                  TYPE        PLACEMENT STATUS
───────────────────────────────────────────────────
aws-alb-controller    component   ✗ requires provider=aws
aws-ebs-provisioner   component   ✗ requires provider=aws
universal-helper      trait       ✓ no constraints

Summary: 1 of 3 definitions can run on this cluster
```

### Storage in Definition CR

Placement constraints are stored in the definition CR for runtime reference:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: aws-load-balancer
  labels:
    definition.oam.dev/placement: "restricted"
  annotations:
    definition.oam.dev/placement-runon: "provider=aws,cluster-type in (eks,self-managed)"
    definition.oam.dev/placement-notrunon: "cluster-type=vcluster"
spec:
  # ... definition spec
```

### Future: Multi-Cluster Integration

This design is forward-compatible with KubeVela's multi-cluster features:

```bash
# Future: Apply to specific clusters by label selector
$ vela def apply-module ./aws-definitions --clusters "provider=aws"

# Future: Use existing cluster secret labels
$ vela cluster labels add prod-eks provider=aws cluster-type=eks
```

---

## Implementation Plan

> **Note**: This implementation plan represents the proposed design direction. API names, method signatures, and specific features may evolve during implementation as we discover edge cases, gather community feedback, or identify opportunities for improvement. The core goals and architecture will remain stable, but implementation details are subject to refinement.

### Phase 1: Core Framework
- Single `defkit` package with `NewComponent()`, `NewTrait()`, `NewPolicy()`, `NewWorkflowStep()` functions
- `VelaCtx()` API for runtime context (`Name()`, `Namespace()`, `AppName()`, `ClusterVersion()`, etc.)
- Parameter type system: String, Int, Bool, Float, Enum, List, Object, Map, Struct, OneOf, Variant with validation
- Convenience types: StringList, IntList, StringKeyMap
- Schema-agnostic resource builder with `Set()`, `SetIf()`, `SpreadIf()`, `If()/EndIf()`, `VersionIf()`
- Collection operations: `Each()`, `Filter()`, `Map()`, `Pick()`, `Rename()`, `Wrap()`, `DefaultField()`, `Flatten()`, `Dedupe()`
- Multi-source collections: `FromFields()`, `MapBySource()`, `Nested()`, `Optional()`
- Helper builder pattern: `tpl.Helper()`, `StructArrayHelper()`, `ConcatHelper()`, `DedupeHelper()`
- Transform utilities: `Transform()`, `HasExposedPorts()`, `Format()`
- Expression helpers: `Lit()`, `Eq()`, `Ne()`, `Lt()`, `Le()`, `Gt()`, `Ge()`, `And()`, `Or()`, `Not()`
- IR→CUE compiler via `cuegen.go`
- CLI integration: `vela def apply`, `vela def vet` for Go files
- CLI: `vela def init-module`, `vela def validate-module`
- Registry pattern with `defkit.Register()`, `All()`, `Components()`, `Clear()`, `Count()`, `ToJSON()`
- Status/Health helpers for all workload types:
  - Deployment: `DeploymentStatus()`, `DeploymentHealth()`
  - DaemonSet: `DaemonSetStatus()`, `DaemonSetHealth()`
  - StatefulSet: `StatefulSetStatus()`, `StatefulSetHealth()`
  - Job: `JobHealth()`
  - CronJob: `CronJobHealth()`
- Composable Health Expressions via unified `Health()` API (generic for any resource type):
  - Condition checks: `Health().Condition()`, `Health().AllTrue()`, `Health().AnyTrue()`
  - Phase checks: `Health().Phase()`, `Health().PhaseField()`
  - Field comparisons: `Health().Field()`, `Health().FieldRef()`, `Health().Exists()`, `Health().NotExists()`
  - Combinators: `Health().And()`, `Health().Or()`, `Health().Not()`
  - Always healthy: `Health().Always()`
  - Policy generation: `Health().Policy(expr)`, `HealthPolicyExpr(expr)`
- Composable Status Expressions via unified `Status()` API (generic for any resource type):
  - Field extraction: `Status().Field()`, `Status().SpecField()`, `Status().Exists()`
  - Condition access: `Status().Condition().StatusValue()`, `.Message()`, `.Reason()`
  - Message building: `Status().Format()`, `Status().Concat()`
  - Conditional messages: `Status().Switch()`, `Status().Case()`, `Status().Default()`
  - Health-aware: `Status().HealthAware(healthyMsg, unhealthyMsg)`
  - Structured details: `Status().WithDetails()`, `Status().Detail()`
  - Status generation: `Status().Build()`, `CustomStatusExpr(expr)`
- TestContext for unit testing: `TestContext()`, `WithName()`, `WithNamespace()`, `WithParam()`, `WithClusterVersion()`, etc.
- Custom matchers for Gomega-based testing
- Example components: webservice, worker, task, cron-task, daemon, statefulset, k8s-objects, ref-objects

### Phase 2: Complete Definition Support
- PolicyDefinition with `NewPolicy()`
  - Example policies: topology, apply-once, garbage-collect, override, read-only, replication, resource-update, shared-resource, take-over
- WorkflowStepDefinition with `NewWorkflowStep()`
  - Example workflow steps: deploy, suspend, apply-component, apply-deployment, apply-object, apply-terraform-config, apply-terraform-provider, build-push-image, check-metrics, clean-jobs, collect-service-endpoints, create-config, delete-config, depends-on-app, deploy-cloud-resource, export-data, export-service, export2config, export2secret, generate-jdbc-connection, list-config, notification, print-message-in-status, read-config, read-object, request, share-cloud-resource, step-group, webhook
- TraitDefinition with `NewTrait()`
  - Container patches: command, env, container-image, container-ports, init-container, startup-probe, resource, securitycontext
  - Pod-level: affinity, hostalias, lifecycle, podsecuritycontext, sidecar, topologyspreadconstraints
  - Scaling: scaler, cpuscaler, hpa
  - Networking: expose, gateway, pure-ingress, service-account, service-binding
  - Storage: storage
  - Metadata: labels, annotations
  - Advanced: json-merge-patch, json-patch, k8s-update-strategy, nocalhost
- `RawCUE()` escape hatch for complex CUE patterns
- Trait patterns: PatchContainer helper, SetRawPatchBlock, SetRawOutputsBlock

### Phase 3: Definition Placement
- **Cluster labels**: ConfigMap-based cluster label storage (`vela-system/vela-cluster-identity`)
- **CLI cluster labels** (future): Integration with `vela cluster labels` command
- **Placement API** (`placement` package): `Label()`, `Eq()`, `Ne()`, `In()`, `NotIn()`, `Exists()`, `NotExists()`, `All()`, `Any()`, `Not()`
- **Combinators**: `All()`, `Any()`, `Not()` for logical composition
- **Definition methods**: `RunOn()`, `NotRunOn()` on ComponentDefinition, TraitDefinition, etc.
- **Module placement**: Default placement in `module.yaml` with inheritance
- **CLI enforcement**: Placement checking in `apply-module`, `--ignore-placement` override
- **Placement storage**: Store constraints in definition CR annotations

### Phase 4: Distribution & Ecosystem
- **Addon integration**: Support `godef/` folder in addon structure for Go-based definitions
- **Module dependencies**: Enable defkit modules to import definitions from other Go modules
- **CLI addon commands**: `vela addon enable` detects and compiles Go definitions
- **Addon validation**: `vela addon validate` includes Go definition validation
- OCI registry support for distributing compiled definitions
- Migration tooling (`vela def gen-go` for CUE→Go)
- Enhanced documentation and tutorials

### Phase 5: Advanced Features
- Multi-cluster placement: Integration with KubeVela cluster management
- Other languages based on community demand
- IDE plugins
- Definition composition

---

## Compatibility

### Coexistence with CUE

- CUE definitions remain fully supported
- defkit is an alternative path, not a replacement
- Both produce valid X-Definition CRs

### Migration

```bash
# Optional: Import existing CUE to Go (inverse of gen-api, NEW subcommand)
vela def gen-go ./legacy-definitions/*.cue --output ./converted/
```

---

## Security Considerations

### Code Execution Model

defkit definitions involve executing Go code, which requires careful security consideration:

1. **Compile-Time Execution Only**: Go definition code runs during `vela addon enable` or `vela def apply` on the CLI, NOT at application deployment time. The output is static CUE that the controller interprets.

2. **No Runtime Code Execution**: Once compiled to CUE, definitions are evaluated by the CUE engine within the controller. User-provided Go code does not execute at runtime.

3. **Trust Model**: Definition authors are platform engineers with cluster-admin privileges, not end-users. This is the same trust model as CUE definitions today.

4. **Isolated Compilation**: The goloader executes `go build` and `go run` in temporary directories containing only the definition module. While Go code technically has the same filesystem permissions as the CLI user, the trust model (point 3) ensures only platform engineers with appropriate privileges author definitions.

5. **No Network Access During Compilation**: Definition compilation is a pure transformation from Go to CUE. Definitions should not make network calls during compilation.

### Security Benefits Over CUE

| Aspect | CUE Definitions | Go Definitions |
|--------|-----------------|----------------|
| Static Analysis | Limited tooling | Standard Go security tools (gosec, staticcheck) |
| Dependency Scanning | Manual review | Go modules enable vulnerability scanning |
| Type Safety | Runtime errors | Compile-time type checking prevents many error classes |
| Code Review | CUE-specific knowledge required | Standard Go code review practices apply |

### Threat Model

| Threat | Mitigation |
|--------|------------|
| Malicious addon code | Code runs in CLI context with user's permissions, not in controller. Addon installation requires explicit user action. |
| Dependency hijacking | Go module checksums (go.sum) verify dependency integrity. Use `go mod verify` in CI. |
| Code injection via parameters | Parameters are schema-validated. Go type system prevents injection into generated CUE. |
| Privilege escalation | Generated CUE runs with same privileges as any CUE definition. No additional capabilities granted. |

---

## FAQ

**Q: Can I still write CUE definitions?**
A: Yes. CUE definitions remain fully supported. defkit is an alternative.

**Q: How do I access runtime values like readyReplicas?**
A: Use `defkit.Status().Field("status.readyReplicas")` when defining custom status expressions via `CustomStatus()`. This generates CUE that accesses `context.output.status.readyReplicas` at runtime. For common workloads, use pre-built helpers like `defkit.DeploymentStatus().Build()`.

**Q: What about CUE unification (`&`)?**
A: Common patterns like defaults with runtime values are handled automatically. For complex unification, use `RawCUE()`.

**Q: Why Go first?**
A: Go is KubeVela's implementation language and widely used in the Kubernetes ecosystem. Other languages may follow based on community demand.

**Q: How do I test definitions without a cluster?**
A: Use `defkit.TestContext()` to create mock contexts with parameters, cluster version, and output status. All testing can be done with standard Go testing frameworks.

**Q: Can I see the generated CUE?**
A: Yes. Use `vela def render ./definition.go --output cue` or `def.ToCUE()` in tests.

**Q: How do I restrict a definition to specific cluster types?**
A: Use the `RunOn()` and `NotRunOn()` methods with `placement.Label()` conditions:
```go
defkit.NewComponent("aws-lb").
    RunOn(placement.Label("provider").Eq("aws")).
    NotRunOn(placement.Label("cluster-type").Eq("vcluster"))
```

**Q: What happens if I don't specify any placement constraints?**
A: The definition will be applied to all clusters. Placement constraints are opt-in.

**Q: How do I set cluster labels?**
A: Edit the `vela-cluster-identity` ConfigMap in the `vela-system` namespace directly using `kubectl edit configmap vela-cluster-identity -n vela-system`. Integration with the `vela cluster labels` command is planned for a future release.

**Q: Can I override placement constraints during apply?**
A: Yes, use `vela def apply-module ./module --ignore-placement` for admin override. A warning will be shown.
