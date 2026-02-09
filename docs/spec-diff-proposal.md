# Proposal: Store Spec Diffs for Policy Transforms

## Problem

Currently when a policy modifies the Application spec, we only set `specModified: true`. This isn't helpful for debugging:

```json
{
  "name": "inject-sidecar",
  "specModified": true  // ❌ What did it change?
}
```

Users can't tell:
- What exactly was added/removed/modified
- How multiple policies interact
- If a policy is working correctly

## Proposed Solution

Store a structured diff when `specModified=true`:

```json
{
  "name": "inject-sidecar",
  "specModified": true,
  "specDiff": "eyJhZGRlZCI6e...}",  // Base64 JSON patch
  "specDiffSummary": "Added 1 component, modified 2 properties"
}
```

## Implementation Options

### Option 1: JSON Patch (RFC 6902)

**Format**: Industry standard, compact
```json
[
  {"op": "add", "path": "/components/1", "value": {...}},
  {"op": "replace", "path": "/components/0/properties/replicas", "value": 3}
]
```

**Pros**:
- Standard format (kubectl diff uses this)
- Reversible (can undo changes)
- Compact representation
- Libraries available

**Cons**:
- Harder to read for humans
- Requires JSON marshal/unmarshal

**Cost**: ~10-15ms overhead

### Option 2: Structured Summary

**Format**: Human-readable summary
```json
{
  "componentsAdded": 1,
  "componentsModified": 0,
  "propertiesChanged": ["components[0].properties.replicas"],
  "beforeHash": "abc123",
  "afterHash": "def456"
}
```

**Pros**:
- Very readable
- Lightweight (no full diff)
- Fast to compute (~2-3ms)

**Cons**:
- Can't see actual values
- Not reversible
- Less useful for complex changes

**Cost**: ~2-5ms overhead

### Option 3: Hybrid Approach (RECOMMENDED)

**Store both summary + diff (only if diff is small)**

```go
const MaxSpecDiffSize = 10 * 1024 // 10KB

type SpecChange struct {
    Summary      SpecChangeSummary  `json:"summary"`
    FullDiff     string            `json:"fullDiff,omitempty"`  // Base64 JSON patch (if <10KB)
    DiffTooLarge bool              `json:"diffTooLarge,omitempty"`
}

type SpecChangeSummary struct {
    ComponentsAdded    int      `json:"componentsAdded,omitempty"`
    ComponentsModified int      `json:"componentsModified,omitempty"`
    ComponentsRemoved  int      `json:"componentsRemoved,omitempty"`
    FieldsChanged      []string `json:"fieldsChanged,omitempty"`
    BeforeHash         string   `json:"beforeHash"`
    AfterHash          string   `json:"afterHash"`
}
```

**Example**:
```json
{
  "name": "inject-sidecar",
  "specModified": true,
  "specChange": {
    "summary": {
      "componentsAdded": 0,
      "componentsModified": 2,
      "fieldsChanged": [
        "components[0].properties.env[0]",
        "components[1].properties.env[0]"
      ],
      "beforeHash": "abc123",
      "afterHash": "def456"
    },
    "fullDiff": "W3sib3AiOiJhZGQiLCJ...",  // Only if <10KB
    "diffTooLarge": false
  }
}
```

**Pros**:
- Human-readable summary for quick diagnosis
- Full diff available for detailed debugging (when needed)
- Avoids etcd bloat for large changes
- Fast path (summary only) is cheap (~5ms)

**Cons**:
- More complex implementation
- Two code paths to maintain

**Cost**: ~5-15ms depending on size

## Scope: Only Diff Spec Changes

**Do NOT diff labels/annotations** - we already track these explicitly:
```json
"addedLabels": {"team": "platform"},        // ✅ Already clear
"addedAnnotations": {"version": "v1.0"}     // ✅ Already clear
```

**Only diff spec transforms** - this is where we need help:
```json
"specModified": true,   // ❌ Not helpful
"specChange": {...}     // ✅ Shows what changed
```

## Computational Impact

### Per-Application Overhead:
- **Option 1 (JSON Patch)**: ~10-15ms
- **Option 2 (Summary Only)**: ~2-5ms
- **Option 3 (Hybrid)**: ~5-15ms (average ~8ms)

### Context:
- Typical reconciliation: 100-500ms
- Policy rendering (uncached): 30-100ms
- **8ms overhead = ~2-5% of total time** ✅ Acceptable

### When to Skip:
- If no spec transform: 0ms overhead
- If diff >10KB: compute summary only (~2ms)
- Labels/annotations only: 0ms overhead

## Storage Impact

### etcd Size:
- JSON Patch for typical sidecar injection: ~2-5KB
- Base64 encoding: +33% → ~3-7KB
- 5 policies with spec changes: ~15-35KB
- **Total Application size increase: <5%** ✅ Acceptable

### etcd Limits:
- Max object size: 1.5MB
- Typical Application: 20-100KB
- With diffs: 25-135KB
- **Still well under limit** ✅

## Implementation Plan

### Phase 1: Summary Only (Quick Win)
```go
type PolicyChanges struct {
    AddedLabels       map[string]string
    AddedAnnotations  map[string]string
    AdditionalContext map[string]interface{}
    SpecModified      bool
    SpecChangeSummary *SpecChangeSummary  // NEW
}
```

**Benefits**:
- Low overhead (~2ms)
- Helps with debugging
- No storage concerns

### Phase 2: Add Full Diff (If Needed)
```go
type PolicyChanges struct {
    // ... existing fields
    SpecChange *SpecChange  // Replaces SpecModified + Summary
}
```

**Benefits**:
- Complete visibility
- Can show diffs in UI
- Enables "undo" functionality

## Alternative: External Diff Storage

If storage is a concern, store diffs externally:

```go
type AppliedGlobalPolicy struct {
    // ... existing fields
    SpecDiffRef string  // "configmap/my-app-policy-diffs/inject-sidecar"
}
```

Create a ConfigMap per Application with all policy diffs:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-app-policy-diffs
data:
  inject-sidecar: |
    [{"op": "add", ...}]
  resource-limits: |
    [{"op": "replace", ...}]
```

**Pros**:
- Doesn't bloat Application status
- Can be cleaned up separately
- No etcd concerns

**Cons**:
- Extra API call to view diffs
- More objects to manage
- Lifecycle management complexity

## Decision Criteria

### When to Implement Full Diffs:

✅ **YES** if:
- Users frequently ask "what did this policy change?"
- Debugging complex spec transforms is common
- UI/CLI tools will display diffs
- "Undo policy effects" is a requirement

❌ **NO** if:
- Current tracking (labels/annotations/specModified) is sufficient
- Performance is critical (every ms counts)
- Storage is limited

### Recommendation:

**Start with Phase 1 (Summary Only)**:
- Low cost (~2ms, ~1KB)
- Immediate value for debugging
- Easy to implement
- Can upgrade to full diffs later if needed

**Add Phase 2 (Full Diffs) if**:
- Users request it after using summaries
- UI/CLI tools are built to display diffs
- "Policy dry-run" feature is added

## Open Questions

1. **Should diffs be human-readable or machine-parseable?**
   - JSON Patch (machine) vs. kubectl-style diff (human)

2. **Should we store diffs for all policies or just spec changes?**
   - Current proposal: Only spec changes

3. **Should diffs be compressed?**
   - Could use gzip before base64 (saves ~60% space)

4. **Retention policy?**
   - Clear diffs on successful reconciliation?
   - Keep last N diffs?

5. **Should we support "reverting" policy changes?**
   - Would require storing inverse patches

## Example Usage

### CLI Tool
```bash
# Show what a policy changed
kubectl vela policy diff my-app inject-sidecar

# Output:
Spec changes by policy 'inject-sidecar':
  + Added component 'monitoring-sidecar'
  ~ Modified components[0].properties.env
    + Added env var: SIDECAR_ENABLED=true

# Show full JSON patch
kubectl vela policy diff my-app inject-sidecar --format=json-patch
```

### UI Dashboard
```
Application: my-app
Applied Policies:
  ✅ inject-sidecar (vela-system)
     Spec Changes:
       ├─ Added 1 component
       ├─ Modified 2 properties
       └─ [View Full Diff]
```

## Conclusion

**Summary diffs (~2ms, ~1KB) provide 80% of the value with 20% of the cost.**

Recommend:
1. ✅ Implement Phase 1 (Summary) now
2. 🤔 Evaluate Phase 2 (Full Diff) based on usage
3. 📊 Add metrics to track diff size distribution
4. 🔍 Monitor performance impact in production
