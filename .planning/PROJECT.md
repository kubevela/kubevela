# defkit Fluent API Improvements

## What This Is

`pkg/definition/defkit` is a Go fluent builder API inside KubeVela that lets developers create KubeVela definition CRDs (ComponentDefinition, TraitDefinition, WorkflowStepDefinition, PolicyDefinition) programmatically — without writing raw CUE or YAML. It generates both CUE template output and Kubernetes YAML. This project addresses naming inconsistencies, missing fluent methods, and missing CRD spec field coverage identified through HTML analysis and deep code inspection.

## Core Value

A consistent, complete fluent API where every CRD spec field has a fluent setter and naming follows a single convention — so definition authors can build KubeVela definitions in pure Go without consulting CUE docs or raw YAML specs.

## Requirements

### Validated

- ✓ `ComponentDefinition`, `TraitDefinition`, `WorkflowStepDefinition`, `PolicyDefinition` types with CUE + YAML output — existing
- ✓ `StringParam`, `IntParam`, `FloatParam`, `BoolParam`, `ArrayParam`, `MapParam`, `EnumParam`, `StructParam`, `OneOfParam` parameter builders — existing
- ✓ `baseParam` with `Short()`, `Ignore()`, `ForceOptional()`, `Description()` on most param types — existing (partially)
- ✓ `Labels()` + `GetLabels()` on Component, Trait, WorkflowStep definitions — existing
- ✓ `CustomStatus()`, `HealthPolicy()` on all definition types — existing
- ✓ `HelperBuilder`, `CollectionOp`, `PolicyTemplate` — existing

### Active

#### Part A — Naming Inconsistencies (5 renames)
- [ ] **A1**: `PolicyTemplate.SetField()` → `Set()` — align with all other builder types
- [ ] **A2**: `StructParam.Fields()` + `OneOfVariant.Fields()` → `WithFields()` — align with ArrayParam/MapParam
- [ ] **A3**: `StructField.ArrayOf()` → `Of()` — align with ArrayParam.Of()
- [ ] **A4**: `EnumParam.Values()` → `Enum()` — align with StringParam.Enum() and StructField.Enum()
- [ ] **A5**: `HelperBuilder.FilterPred(Predicate)` → `Filter(Predicate)`; `HelperBuilder.Filter(Condition)` → `FilterCond(Condition)` — align with CollectionOp.Filter()

#### Part B — Missing Methods (5 additions)
- [ ] **B1**: `Labels()` + `GetLabels()` on PolicyDefinition + fix hardcoded `labels: {}` in CUE
- [ ] **B2**: `ForceOptional()` on IntParam, FloatParam, EnumParam
- [ ] **B3**: `Short()` + `Ignore()` on FloatParam
- [ ] **B4**: `Annotations()` + `GetAnnotations()` on all 4 definition types — CUE render + `ToYAML()` merge
- [ ] **B5**: `StatusDetails()` on all 4 definition types via `baseDefinition`

#### Part C — Missing CRD Spec Fields (7 additions)
- [ ] **C1**: `Version()` on all 4 definition types (maps to `spec.version`)
- [ ] **C2**: `ManageWorkload()` on TraitDefinition (maps to `spec.manageWorkload`)
- [ ] **C3**: `ControlPlaneOnly()` on TraitDefinition (maps to `spec.controlPlaneOnly`)
- [ ] **C4**: `RevisionEnabled()` on TraitDefinition (maps to `spec.revisionEnabled`)
- [ ] **C5**: `ChildResourceKind()` on ComponentDefinition (maps to `spec.childResourceKinds`)
- [ ] **C6**: `PodSpecPath()` on ComponentDefinition (maps to `spec.podSpecPath`)
- [ ] **C7**: `ManageHealthCheck()` on PolicyDefinition (maps to `spec.manageHealthCheck`)

### Out of Scope

- `Extension` (`runtime.RawExtension`) — raw bytes, no type safety, niche advanced use
- `RevisionLabel` (ComponentDefinition) — rarely set manually; auto-generated
- `definitionRef` — auto-generated from definition name; no user-facing API needed
- Deprecated aliases for renamed methods — library is pre-release, hard renames are safe

## Context

- **defkit** lives at `pkg/definition/defkit/` in the KubeVela monorepo
- Primary consumer: `github.com/oam-dev/vela-go-definitions` (`/Users/viskumar/Open_Source/vela-go-definitions`) — high frequency usage of renamed methods requires call site updates
- Secondary consumers: future external open-source definition authors (pre-release, no public API stability contract)
- CUE templates are validated by webhook: `pkg/webhook/core.oam.dev/v1beta1/*` — CUE render changes must not break webhook validation
- All 4 CRD specs confirmed in `charts/vela-core/crds/` — Part C fields are verified present in actual CRD YAMLs

## Constraints

- **Language**: Go 1.23 — idiomatic Go, no generics beyond what's already used
- **Backward compat**: Pre-release, hard renames allowed — no deprecated aliases
- **CUE correctness**: CUE output changes must parse correctly via `cuelang.org/go v0.14.1`
- **Downstream**: After all renames, `vela-go-definitions` must build with zero errors

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Hard rename, no deprecated aliases | Library is pre-release; clean API wins over migration burden | — Pending |
| `EnumParam.Enum()` (vs `Values()`) | Consistency with `StringParam.Enum()` outweighs slight redundancy of `defkit.Enum("x").Enum(...)` | — Pending |
| `FilterCond` + `Filter` (vs keeping old names) | Disambiguates condition-based vs predicate-based filter; matches CollectionOp convention | — Pending |
| `StatusDetails()` via `baseDefinition` | All 4 types share the field; if status fields already in base, add there; avoids 4-file duplication | — Pending |
| `Annotations.ToYAML()` merge strategy | User annotations are merged *into* `metadata.annotations`, never overriding `definition.oam.dev/description` | — Pending |
| Implementation order: pure additions first, renames last | Minimizes risk — additions can't break builds, renames can | — Pending |

---
*Last updated: 2026-03-06 after initialization*
