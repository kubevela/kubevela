# Roadmap: defkit Fluent API Improvements

**Milestone:** v1 — Complete API coverage and naming consistency
**Requirements:** 17 items across 5 phases
**Implementation principle:** Pure additions first (zero downstream risk), renames last (require call-site updates)

---

## Phase 1: Param Method Completeness

**Goal:** Add missing fluent methods to param types where `baseParam` already stores the data — zero CUE render changes required.

**Requirements:** B3, B2, B5

**Files to modify:**
- `pkg/definition/defkit/param.go` — `Short()`, `Ignore()` on FloatParam; `ForceOptional()` on IntParam, FloatParam, EnumParam
- `pkg/definition/defkit/base.go` — `StatusDetails()` on `baseDefinition` (and expose on all 4 types)

**Success criteria:**
1. `FloatParam` compiles with `Short(string)` and `Ignore()` methods returning `*FloatParam`
2. `IntParam`, `FloatParam`, and `EnumParam` each compile with `ForceOptional()` returning the receiver type
3. All 4 definition types expose `StatusDetails(string)` and it renders inside the `status:` CUE block alongside `customStatus` and `healthPolicy`
4. `go test ./pkg/definition/defkit/...` passes with new unit tests covering each added method

---

## Phase 2: Definition-Level Additions with CUE Changes

**Goal:** Add `Labels` to PolicyDefinition (fixing hardcoded CUE) and add `Annotations` support to all 4 definition types with correct `ToYAML()` merge behaviour.

**Requirements:** B1, B4

**Files to modify:**
- `pkg/definition/defkit/policy.go` — `Labels()` + `GetLabels()` on PolicyDefinition; replace hardcoded `labels: {}` in CUE render
- `pkg/definition/defkit/base.go` — `Annotations()` + `GetAnnotations()` on `baseDefinition`
- `pkg/definition/defkit/component.go`, `trait.go`, `workflowstep.go`, `policy.go` — propagate `GetAnnotations()` into `ToYAML()` with merge (never override `definition.oam.dev/description`)
- CUE render functions in each definition file — emit sorted annotation keys when annotations are set

**Success criteria:**
1. `PolicyDefinition.Labels(map[string]string)` renders label keys in CUE output; hardcoded `labels: {}` is gone
2. `Annotations()` on any definition type produces sorted annotation keys in CUE output
3. `ToYAML()` merges user annotations into `metadata.annotations` and preserves the `definition.oam.dev/description` key
4. Webhook validation passes: existing CUE render tests in `pkg/webhook/core.oam.dev/v1beta1/` do not regress

---

## Phase 3: Missing CRD Spec Fields

**Goal:** Add fluent setters for all 7 CRD spec fields that have no current Go API, mapping directly to verified CRD YAML fields.

**Requirements:** C1, C2, C3, C4, C5, C6, C7

**Files to modify:**
- `pkg/definition/defkit/base.go` — `Version()` + `GetVersion()` on `baseDefinition`
- `pkg/definition/defkit/trait.go` — `ManageWorkload()`, `ControlPlaneOnly()`, `RevisionEnabled()` + boolean getters
- `pkg/definition/defkit/component.go` — `ChildResourceKind()` + `GetChildResourceKinds()`, `PodSpecPath()` + `GetPodSpecPath()`
- `pkg/definition/defkit/policy.go` — `ManageHealthCheck()` + `IsManageHealthCheck()`

**Success criteria:**
1. `Version()` on all 4 definition types renders `version: "..."` in CUE output and omits the field when empty
2. `TraitDefinition` fluently accepts `ManageWorkload()`, `ControlPlaneOnly()`, `RevisionEnabled()` and each maps to the correct `spec.*` boolean in `ToYAML()` output
3. `ComponentDefinition.ChildResourceKind(apiVersion, kind, selector)` accumulates multiple entries and each serialises correctly via `ToYAML()`
4. `go test ./pkg/definition/defkit/...` passes with round-trip tests for every new CRD spec field

---

## Phase 4: Low-Risk Renames (Internal + Minimal Downstream)

**Goal:** Rename methods whose callers are entirely within `defkit` itself or a small number of known test files — safe to rename without touching `vela-go-definitions`.

**Requirements:** A1, A3, A5

**Files to modify:**
- `pkg/definition/defkit/policy_template.go` — `SetField()` → `Set()` (A1)
- `pkg/definition/defkit/param.go` — `StructField.ArrayOf()` → `Of()` (A3)
- `pkg/definition/defkit/helper.go` — `FilterPred()` → `Filter()`; `Filter()` → `FilterCond()` at all definition sites including `helper.go:535` (A5)
- `pkg/definition/defkit/collections_test.go` — update 3 test callers for A5

**Success criteria:**
1. `go build ./pkg/definition/defkit/...` succeeds with no reference to `SetField`, `ArrayOf`, `FilterPred`, or the old `Filter(Condition)` signature
2. `go test ./pkg/definition/defkit/...` passes — no test references the old names
3. `grep -r "SetField\|ArrayOf\|FilterPred" pkg/definition/defkit/` returns zero matches
4. No changes required in `vela-go-definitions` for this phase

---

## Phase 5: High-Impact Renames Requiring Downstream Updates

**Goal:** Rename the two methods with the highest call-site density in `vela-go-definitions`, then verify the downstream repo builds cleanly.

**Requirements:** A4, A2

**Files to modify (defkit):**
- `pkg/definition/defkit/param.go` — `EnumParam.Values()` → `Enum()` (A4)
- `pkg/definition/defkit/param.go` — `StructParam.Fields()` → `WithFields()`; `OneOfVariant.Fields()` → `WithFields()` (A2)

**Files to modify (downstream):**
- `/Users/viskumar/Open_Source/vela-go-definitions` — all call sites of `Values()`, `StructParam.Fields()`, and `OneOfVariant.Fields()` updated to new names

**Success criteria:**
1. `go build ./pkg/definition/defkit/...` succeeds with no `Values` or `Fields` method on the renamed types
2. `grep -r "\.Values()\|\.Fields()" pkg/definition/defkit/` returns zero matches on the renamed types
3. `go build ./...` in `vela-go-definitions` passes with zero errors after call-site updates
4. `go test ./pkg/definition/defkit/...` passes — all param builder tests use the new names

---

## Summary

| Phase | Requirements | Risk | Key constraint |
|-------|-------------|------|----------------|
| 1 | B3, B2, B5 | Complete    | 2026-03-06 |
| 2 | 1/3 | In Progress|  |
| 3 | C1–C7 | Low | CRD spec correctness |
| 4 | A1, A3, A5 | Medium | Internal callers only |
| 5 | A4, A2 | Highest | vela-go-definitions must build |
