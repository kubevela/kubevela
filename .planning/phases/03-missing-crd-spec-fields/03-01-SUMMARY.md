---
phase: 03-missing-crd-spec-fields
plan: "01"
subsystem: defkit
tags: [go, defkit, crd, version, kubevela]

requires:
  - phase: 02-definition-level-additions
    provides: baseDefinition with annotations and statusDetails patterns

provides:
  - version field on baseDefinition with setVersion/GetVersion
  - Version(string) fluent setter on all 4 definition types
  - Conditional CUE render of version field (omit when empty)
  - spec.version in ToYAML for all 4 definition types

affects:
  - phase 03-02 (03-02 and 03-03 were already done before this plan ran)
  - any future plan that adds to baseDefinition

tech-stack:
  added: []
  patterns:
    - "version field on baseDefinition following setStatusDetails/GetStatusDetails pattern"
    - "CUE formatter (formatCUE) aligns fields — test assertions must use MatchRegexp for traits, ContainSubstring for others"
    - "Conditional emit: if GetVersion() != string{} then render; omit entirely otherwise"

key-files:
  created:
    - pkg/definition/defkit/version_test.go
  modified:
    - pkg/definition/defkit/base.go
    - pkg/definition/defkit/trait.go
    - pkg/definition/defkit/component.go
    - pkg/definition/defkit/policy.go
    - pkg/definition/defkit/workflow_step.go
    - pkg/definition/defkit/cuegen.go

key-decisions:
  - "CUE formatter (formatCUE) aligns field values in trait output; use MatchRegexp for version field assertions on TraitDefinition.ToCue()"
  - "version placed after description, before attributes in CUE header block — consistent across all 4 generators"
  - "version string field added to baseDefinition (not as separate per-type field) — follows established statusDetails pattern"

patterns-established:
  - "CUE test assertions: use ContainSubstring for unformatted generators; use MatchRegexp for trait (formatCUE aligns)"

requirements-completed: [C1]

duration: 15min
completed: 2026-03-06
---

# Phase 03 Plan 01: Version Field Summary

**Version(string) fluent setter + GetVersion() on all 4 definition types via baseDefinition, with conditional CUE render and spec.version in ToYAML**

## Performance

- **Duration:** ~15 min
- **Started:** 2026-03-06T15:45:00Z
- **Completed:** 2026-03-06T15:58:00Z
- **Tasks:** 2
- **Files modified:** 7

## Accomplishments
- `version string` field added to `baseDefinition` with `setVersion`/`GetVersion` following `setStatusDetails`/`GetStatusDetails` pattern
- `Version(string)` fluent setter on `TraitDefinition`, `ComponentDefinition`, `PolicyDefinition`, `WorkflowStepDefinition`
- Conditional CUE render: `version: "..."` emitted after `description:` in all 4 CUE generators; omitted entirely when empty
- `spec.version` added to all 4 `ToYAML()` outputs conditionally

## Task Commits

1. **Task 1: Add version field to baseDefinition and expose on all 4 types** - `c39e7d2e5` (feat)
2. **Task 2: Render version in CUE output and ToYAML spec** - `48a1d5cef` (feat)

## Files Created/Modified
- `pkg/definition/defkit/base.go` - version field + setVersion/GetVersion
- `pkg/definition/defkit/trait.go` - Version() fluent method + CUE render + ToYAML spec.version
- `pkg/definition/defkit/component.go` - Version() fluent method + ToYAML spec.version
- `pkg/definition/defkit/policy.go` - Version() fluent method + CUE render + ToYAML spec.version
- `pkg/definition/defkit/workflow_step.go` - Version() fluent method + CUE render + ToYAML spec.version
- `pkg/definition/defkit/cuegen.go` - version CUE render in ComponentDefinition GenerateFullDefinition
- `pkg/definition/defkit/version_test.go` - Ginkgo tests for all behaviors (20 tests)

## Decisions Made
- Trait `ToCue()` calls `formatCUE(Simplify())` which aligns field values; version test assertions for traits use `MatchRegexp` to handle whitespace alignment. All other generators (component, policy, workflowstep) output raw unformatted CUE so `ContainSubstring` works there.
- `version` placed after `description:` line and before `attributes:` block in CUE header — consistent across all generators.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
- CUE Simplify formatter in `formatCUE` aligns trait header fields, producing `version:     "1.0"` instead of `version: "1.0"`. Resolved by using `MatchRegexp` in the test for trait assertions. This is consistent with how the trait formatter works for all other fields.

## Next Phase Readiness
- C1 satisfied — `Version()` available on all 4 definition types
- 03-02 and 03-03 were already executed (ManageWorkload, ControlPlaneOnly, RevisionEnabled, ManageHealthCheck, ChildResourceKind, PodSpecPath)
- Phase 3 complete pending state update

---
*Phase: 03-missing-crd-spec-fields*
*Completed: 2026-03-06*
