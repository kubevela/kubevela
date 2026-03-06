# Requirements: defkit Fluent API Improvements

**Defined:** 2026-03-06
**Core Value:** A consistent, complete fluent API where every CRD spec field has a fluent setter and naming follows a single convention

## v1 Requirements

### Part A — Naming Inconsistencies

- [ ] **A1**: `PolicyTemplate.SetField()` renamed to `Set()` — align with Resource, PatchResource, WorkflowStepTemplate, ItemBuilder, ArrayElement
- [ ] **A2**: `StructParam.Fields()` and `OneOfVariant.Fields()` renamed to `WithFields()` — align with ArrayParam and MapParam
- [ ] **A3**: `StructField.ArrayOf()` renamed to `Of()` — align with ArrayParam.Of(); `Array` prefix is redundant
- [ ] **A4**: `EnumParam.Values()` renamed to `Enum()` — align with StringParam.Enum() and StructField.Enum()
- [ ] **A5**: `HelperBuilder.FilterPred(Predicate)` → `Filter(Predicate)`; `HelperBuilder.Filter(Condition)` → `FilterCond(Condition)` — align with CollectionOp.Filter(); update internal caller at `helper.go:535` and 3 test callers in `collections_test.go`

### Part B — Missing Methods

- [x] **B1**: `Labels(map[string]string)` + `GetLabels()` added to PolicyDefinition struct; CUE render updated to replace hardcoded `labels: {}` with conditional label output
- [x] **B2**: `ForceOptional()` added to IntParam, FloatParam, EnumParam (baseParam already has field; only fluent method missing)
- [x] **B3**: `Short(string)` + `Ignore()` added to FloatParam (baseParam already has fields; only fluent methods missing)
- [x] **B4**: `Annotations(map[string]string)` + `GetAnnotations()` added to all 4 definition types; CUE render outputs sorted annotation keys; `ToYAML()` merges user annotations into `metadata.annotations` without overriding `definition.oam.dev/description`
- [x] **B5**: `StatusDetails(string)` added to all 4 definition types via `baseDefinition`; renders alongside `customStatus` and `healthPolicy` in the `status:` CUE block

### Part C — Missing CRD Spec Fields

- [ ] **C1**: `Version(string)` + `GetVersion()` added to all 4 definition types; renders as `version: "..."` in CUE output (omit if empty); maps to `spec.version`
- [ ] **C2**: `ManageWorkload()` + `IsManageWorkload()` added to TraitDefinition; maps to `spec.manageWorkload`
- [ ] **C3**: `ControlPlaneOnly()` + `IsControlPlaneOnly()` added to TraitDefinition; maps to `spec.controlPlaneOnly`
- [ ] **C4**: `RevisionEnabled()` + `IsRevisionEnabled()` added to TraitDefinition; maps to `spec.revisionEnabled`
- [ ] **C5**: `ChildResourceKind(apiVersion, kind string, selector map[string]string)` + `GetChildResourceKinds()` added to ComponentDefinition; reuses `common.ChildResourceKind` if available; maps to `spec.childResourceKinds`
- [ ] **C6**: `PodSpecPath(string)` + `GetPodSpecPath()` added to ComponentDefinition; maps to `spec.podSpecPath`
- [ ] **C7**: `ManageHealthCheck()` + `IsManageHealthCheck()` added to PolicyDefinition; maps to `spec.manageHealthCheck`

## v2 Requirements

### Deferred CRD Fields

- **EXT-01**: `Extension(runtime.RawExtension)` — raw bytes, no type safety, niche advanced use; defer
- **COMP-REV**: `RevisionLabel` on ComponentDefinition — rarely set manually; defer

## Out of Scope

| Feature | Reason |
|---------|--------|
| `definitionRef` setter | Auto-generated from definition name by defkit; no user API needed |
| Deprecated aliases for renamed methods | Pre-release library; clean break preferred |
| New parameter types beyond existing set | Not part of identified gaps |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| B3 | Phase 1 — Param Method Completeness | Complete |
| B2 | Phase 1 — Param Method Completeness | Complete |
| B5 | Phase 1 — Param Method Completeness | Complete |
| B1 | Phase 2 — Definition-Level Additions with CUE Changes | Complete |
| B4 | Phase 2 — Definition-Level Additions with CUE Changes | Complete |
| C1 | Phase 3 — Missing CRD Spec Fields | Pending |
| C2 | Phase 3 — Missing CRD Spec Fields | Pending |
| C3 | Phase 3 — Missing CRD Spec Fields | Pending |
| C4 | Phase 3 — Missing CRD Spec Fields | Pending |
| C5 | Phase 3 — Missing CRD Spec Fields | Pending |
| C6 | Phase 3 — Missing CRD Spec Fields | Pending |
| C7 | Phase 3 — Missing CRD Spec Fields | Pending |
| A1 | Phase 4 — Low-Risk Renames | Pending |
| A3 | Phase 4 — Low-Risk Renames | Pending |
| A5 | Phase 4 — Low-Risk Renames | Pending |
| A4 | Phase 5 — High-Impact Renames + Downstream | Pending |
| A2 | Phase 5 — High-Impact Renames + Downstream | Pending |

**Coverage:**
- v1 requirements: 17 total
- Mapped to phases: 17
- Unmapped: 0 ✓

---
*Requirements defined: 2026-03-06*
*Last updated: 2026-03-06 after initial definition*
