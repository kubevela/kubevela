---
phase: 02-definition-level-additions-with-cue-changes
plan: "01"
subsystem: defkit
tags: [go, kubevela, defkit, policy, cue, labels]

requires:
  - phase: 01-param-method-completeness
    provides: baseDefinition with statusDetails, FloatParam/IntParam/EnumParam completeness

provides:
  - Labels()/GetLabels() on PolicyDefinition
  - Nil-conditional labels CUE rendering with sorted keys in PolicyCUEGenerator
  - Ginkgo tests covering all four labels scenarios

affects:
  - 02-02 (annotations feature — same pattern, adjacent lines in policy.go)

tech-stack:
  added: ["sort" stdlib in policy.go]
  patterns:
    - "Nil-pointer distinguishes unset (omit block) from empty map (emit labels: {}) from populated map (emit sorted entries)"

key-files:
  created: []
  modified:
    - pkg/definition/defkit/policy.go
    - pkg/definition/defkit/policy_ginkgo_test.go

key-decisions:
  - "Used sorted keys for labels CUE output (deterministic output, unlike trait.go which uses range)"
  - "labels field lives directly on PolicyDefinition (not baseDefinition) — consistent with TraitDefinition, ComponentDefinition, WorkflowStepDefinition patterns"

patterns-established:
  - "Nil-sentinel pattern: nil = never called (omit), empty map = explicit empty (labels: {}), non-empty = sorted key-value pairs"

requirements-completed: [B1]

duration: 8min
completed: 2026-03-06
---

# Phase 2 Plan 01: Labels on PolicyDefinition + Fix Hardcoded CUE Summary

**Labels()/GetLabels() added to PolicyDefinition with nil-conditional sorted CUE rendering replacing hardcoded `labels: {}`**

## Performance

- **Duration:** 8 min
- **Started:** 2026-03-06T09:10:00Z
- **Completed:** 2026-03-06T09:18:00Z
- **Tasks:** 3
- **Files modified:** 2

## Accomplishments
- Added `labels map[string]string` field, `Labels()` fluent setter, and `GetLabels()` getter to `PolicyDefinition`
- Replaced unconditional `labels: {}` in `PolicyCUEGenerator.GenerateFullDefinition` with nil-conditional block that omits the block when never called, emits empty braces for empty map, and emits sorted key-value pairs for non-empty map
- Added 4 Ginkgo tests covering: store/return, sorted output, omit when unset, empty block

## Task Commits

1. **Tasks 1+2: Add Labels to PolicyDefinition and fix CUE generator** - `6564225` (feat)
2. **Task 3: Ginkgo tests for PolicyDefinition.Labels** - `c379a39` (test)

## Files Created/Modified
- `pkg/definition/defkit/policy.go` - Added `labels` field, `Labels()`/`GetLabels()` methods, nil-conditional CUE rendering, `sort` import
- `pkg/definition/defkit/policy_ginkgo_test.go` - Added `Context("Labels", ...)` with 4 specs; added `strings` import

## Decisions Made
- Sorted keys in CUE output for determinism — trait.go uses `range` (non-deterministic) but policy.go uses `sort.Strings` for consistent output
- Labels field kept on `PolicyDefinition` directly (not promoted to `baseDefinition`) — matches existing pattern in other definition types

## Deviations from Plan
None - plan executed exactly as written.

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Plan 02-02 (annotations feature) can proceed immediately — same nil-conditional pattern, adjacent code in `policy.go`
- B1 requirement satisfied

---
*Phase: 02-definition-level-additions-with-cue-changes*
*Completed: 2026-03-06*
