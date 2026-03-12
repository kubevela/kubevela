# Fix Helm Revision Bumping and Release Cleanup

## Problem Summary

| # | Problem | Root Cause |
|---|---------|------------|
| 1 | Revision count increases on every reconcile even without changes | [installOrUpgradeChart()](file:///Users/co/gwre_kubevela/kubevela/pkg/cue/cuex/providers/helm/helm.go#629-727) runs `helm upgrade` unconditionally — no dedup check |
| 2 | Initial apply creates 14 revisions | Controller reconciles multiple times during startup; each call triggers a new upgrade |
| 3 | Deleting KubeVela Application doesn't delete the Helm release | No `helm uninstall` is called during application deletion |

---

## Proposed Changes

### Problem 1 & 2: Revision Bumping — Add Fingerprint-Based Dedup

#### [MODIFY] [helm.go](file:///Users/co/gwre_kubevela/kubevela/pkg/cue/cuex/providers/helm/helm.go)

**1. Restore in-memory release fingerprint tracking** to the [Provider](file:///Users/co/gwre_kubevela/kubevela/pkg/cue/cuex/providers/helm/helm.go#177-182) struct:
```go
type Provider struct {
    cache      *utils.MemoryCacheStore
    helmClient *cli.EnvSettings
    cacheTTL   *CacheTTLConfig
    releaseMu  sync.Mutex           // serialize install/upgrade calls
    releaseFingerprints map[string]string // releaseName → "chartVersion|valuesHash"
}
```

**2. Add `computeReleaseFingerprint()` helper** — builds a deterministic string from chart version + values hash:
```go
func computeReleaseFingerprint(ch *chart.Chart, values map[string]interface{}) string {
    version := ""
    if ch.Metadata != nil { version = ch.Metadata.Version }
    valuesJSON, _ := json.Marshal(values)
    h := sha256.Sum256(valuesJSON)
    return version + "|" + hex.EncodeToString(h[:])
}
```

**3. Modify [installOrUpgradeChart()](file:///Users/co/gwre_kubevela/kubevela/pkg/cue/cuex/providers/helm/helm.go#629-727)** — add early-return dedup:
- **Before** checking whether to install or upgrade, compute the fingerprint
- Compare with the in-memory cache **and** the existing release in the cluster:
  - If release exists AND the deployed chart version + values match → **skip**, return existing `rel.Manifest` directly
  - If no match → proceed with install/upgrade
- Store the fingerprint after successful install/upgrade
- This also needs to compare the actual release status (if it's `failed` or `pending-*`, we should re-deploy)

The flow becomes:
```
1. Compute fingerprint(chartVersion, valuesHash)
2. Lock releaseMu
3. Check in-memory cache → if match, return cached manifest
4. action.NewGet → get existing release
5. If release exists AND status==deployed AND manifest matches fingerprint → skip, cache, return
6. If no release → helm install
7. If release exists but fingerprint differs → helm upgrade
8. Cache fingerprint + manifest
9. Unlock + return
```

**4. Cache the last successful manifest** — add a `releaseManifests map[string]string` field so we can return the manifest without calling Helm again on no-op reconciles.

---

### Problem 3: Application Deletion — Add Helm Uninstall

This requires a new provider function that KubeVela can call during cleanup.

#### [MODIFY] [helm.go](file:///Users/co/gwre_kubevela/kubevela/pkg/cue/cuex/providers/helm/helm.go)

**1. New `UninstallParams` / `UninstallReturns` structs:**
```go
type UninstallParams struct {
    Release   ReleaseParams `json:"release"`
    KeepHistory bool        `json:"keepHistory,omitempty"`
}
type UninstallReturns struct {
    Success bool   `json:"success"`
    Message string `json:"message,omitempty"`
}
```

**2. New `Uninstall()` provider function:**
- Gets action config for the release namespace
- Runs `action.NewUninstall` to delete the Helm release
- Returns success/failure

**3. Register the `uninstall` provider function** alongside `render`.

#### [MODIFY] [helm.cue](file:///Users/co/gwre_kubevela/kubevela/pkg/cue/cuex/providers/helm/helm.cue)

Add CUE schema for `#Uninstall`:
```cue
#Uninstall: {
    #do:       "uninstall"
    #provider: "helm"
    $params: {
        release: { name: string, namespace: string }
        keepHistory?: bool
    }
    $returns?: {
        success: bool
        message?: string
    }
}
```

#### Application Component Definition — Cleanup Hook

> [!IMPORTANT]
> KubeVela components don't have a built-in "on-delete" lifecycle hook in the CUE template. The cleanup needs to be triggered from the **resource keeper's deletion path** or via a **workflow step** or **finalizer**. The cleanest approach is to add a Go-level check in the [Render()](file:///Users/co/gwre_kubevela/kubevela/pkg/cue/cuex/providers/helm/helm.go#838-923) function itself:
> - Accept a new boolean parameter `uninstall` in [RenderParams](file:///Users/co/gwre_kubevela/kubevela/pkg/cue/cuex/providers/helm/helm.go#145-153)
> - When `uninstall == true`, run `helm uninstall` instead of install/upgrade
> - The resource keeper or a finalizer workflow step can invoke this

**Alternative approach** (simpler): Add cleanup directly in [installOrUpgradeChart()](file:///Users/co/gwre_kubevela/kubevela/pkg/cue/cuex/providers/helm/helm.go#629-727) — before each install, clean in-memory state. And for application deletion, we create a **separate `Uninstall` provider** that can be called from a workflow or an operator-level hook.

---

## Verification Plan

### Automated Tests
```bash
go test ./pkg/cue/cuex/providers/helm/... -v
go build ./...
go vet ./pkg/cue/cuex/providers/helm/...
```

### Manual Verification
1. Apply a helmchart Application → verify only **1 revision** is created after reconciliation settles
2. Wait for multiple reconcile cycles → verify revision count stays at **1**
3. Update values in the Application → verify revision bumps to **2** (exactly once)
4. Delete a deployed resource → verify KubeVela reconciles it back **without** bumping the revision
5. `kubectl delete application <name>` → verify `helm list` shows the release is **gone**
