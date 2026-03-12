# Helm SDK Integration — Use Real `helm install/upgrade` with KubeVela Resource Takeover

## Background

Currently, the `helmchart` component renders Helm charts **client-side only** (dry-run with `ClientOnly=true`), extracts the templated resources, and returns them for KubeVela's dispatch pipeline. The user wants real `helm install`/`upgrade` via the Helm SDK, with KubeVela adopting the deployed resources.

---

## Key Problem: Resource Ownership Conflict

When Helm deploys resources, they **won't have KubeVela's ownership labels** (`app.oam.dev/name`, `app.oam.dev/namespace`). When KubeVela's dispatch then tries to apply the same resources, [MustBeControlledByApp](file:///Users/co/gwre_kubevela/kubevela/pkg/utils/apply/apply.go#519-538) in [apply.go](file:///Users/co/gwre_kubevela/kubevela/pkg/utils/apply/apply.go#L520-L537) will fail with:

```
"<kind> <ns>/<name> exists but not managed by any application now"
```

### Solution: Helm Post-Renderer Injects KubeVela Labels

We'll use Helm's **post-renderer** mechanism (`action.Install.PostRenderer` / `action.Upgrade.PostRenderer`) to inject KubeVela ownership labels and annotations into every resource **before Helm actually deploys them**. This way:

1. Helm deploys resources **with** KubeVela's labels already set
2. KubeVela's dispatch sees them as already owned → patches succeed via three-way merge
3. No `take-over` policy required from the user

The post-renderer will add these labels/annotations to every manifest:
```yaml
metadata:
  labels:
    app.oam.dev/name: <appName>
    app.oam.dev/namespace: <appNamespace>
    app.oam.dev/component: <componentName>
  annotations:
    app.oam.dev/owner: helm-provider
```

The `appName`, `appNamespace`, and `componentName` will be passed through from the CUE context.

---

## Proposed Changes

### Helm Provider

#### [MODIFY] [helm.go](file:///Users/co/gwre_kubevela/kubevela/pkg/cue/cuex/providers/helm/helm.go)

**1. New struct fields in [RenderParams](file:///Users/co/gwre_kubevela/kubevela/pkg/cue/cuex/providers/helm/helm.go#143-150)** — add context fields for KubeVela ownership:
```go
type RenderParams struct {
    // ... existing fields ...
    Context  *ContextParams `json:"context,omitempty"` // NEW: KubeVela context for labeling
}
type ContextParams struct {
    AppName      string `json:"appName"`
    AppNamespace string `json:"appNamespace"`
    Name         string `json:"name"` // component name
    Namespace    string `json:"namespace"`
}
```

**2. New `getActionConfig()` helper** — properly initializes `action.Configuration` with:
- Kubernetes REST client getter (not in-memory)
- Secrets-based storage driver (so releases persist in cluster)

**3. New `installOrUpgradeChart()` method** — real Helm deployment:
- Uses `action.NewGet` to check if release exists
- **No release** → `action.NewInstall` with `DryRun=false`, `ClientOnly=false`
- **Release exists** → `action.NewUpgrade`
- Configures a post-renderer that injects KubeVela labels
- Passes through all options (Atomic, Wait, Force, CreateNamespace, etc.)
- On **failure**: returns error → [Render()](file:///Users/co/gwre_kubevela/kubevela/pkg/cue/cuex/providers/helm/helm.go#912-1011) surfaces it → application status becomes failed

**4. New `velaLabelPostRenderer` struct** — implements `postrender.PostRenderer`:
- Parses each YAML manifest from the rendered output
- Injects KubeVela ownership labels (`app.oam.dev/name`, `app.oam.dev/namespace`, `app.oam.dev/component`)
- Returns modified manifests

**5. Modify [Render()](file:///Users/co/gwre_kubevela/kubevela/pkg/cue/cuex/providers/helm/helm.go#912-1011) function** — new flow:
```
Before: fetch → render(dry-run) → createFakeRelease → return resources
After:  fetch → mergeValues → installOrUpgradeChart(real) → parse release manifest → return resources
```
If `installOrUpgradeChart` fails, the error propagates directly — no fallback, application goes to failed state.

**6. Remove [createChartRelease()](file:///Users/co/gwre_kubevela/kubevela/pkg/cue/cuex/providers/helm/helm.go#812-911)** — no longer needed (real Helm install creates the release).

**7. Remove [releaseTrackingKey()](file:///Users/co/gwre_kubevela/kubevela/pkg/cue/cuex/providers/helm/helm.go#802-811)** — no longer needed.

**8. Simplify [renderChart()](file:///Users/co/gwre_kubevela/kubevela/pkg/cue/cuex/providers/helm/helm.go#552-712)** → rename to `parseManifestResources()` — only responsible for parsing the release manifest string into `[]map[string]interface{}`, not for dry-run rendering.

---

### CUE Template

#### [MODIFY] [helm.cue](file:///Users/co/gwre_kubevela/kubevela/pkg/cue/cuex/providers/helm/helm.cue)

Add `context` field to `$params` to pass KubeVela ownership info:
```cue
$params: {
    // ... existing fields ...
    context?: {
        appName:      string
        appNamespace: string
        name:         string
        namespace:    string
    }
}
```

#### [MODIFY] [helmchart.cue](file:///Users/co/gwre_kubevela/kubevela/vela-templates/definitions/internal/component/helmchart.cue)

Pass KubeVela context into the render call:
```cue
_rendered: helm.#Render & {
    $params: {
        // ... existing ...
        context: {
            appName:      context.appName
            appNamespace: context.namespace
            name:         context.name
            namespace:    context.namespace
        }
    }
}
```

---

### Tests

#### [MODIFY] [helm_test.go](file:///Users/co/gwre_kubevela/kubevela/pkg/cue/cuex/providers/helm/helm_test.go)

- Keep existing unit tests for [detectChartSourceType](file:///Users/co/gwre_kubevela/kubevela/pkg/cue/cuex/providers/helm/helm.go#214-234), [orderResources](file:///Users/co/gwre_kubevela/kubevela/pkg/cue/cuex/providers/helm/helm.go#760-784), [isTestResource](file:///Users/co/gwre_kubevela/kubevela/pkg/cue/cuex/providers/helm/helm.go#713-723), [mergeValues](file:///Users/co/gwre_kubevela/kubevela/pkg/cue/cuex/providers/helm/helm.go#506-534)
- Add `TestVelaLabelPostRenderer` — verifies labels are injected correctly
- Add `TestParseManifestResources` — tests manifest parsing
- Add `TestGetActionConfig` — verifies proper initialization

---

## Verification Plan

### Automated Tests
```bash
go test ./pkg/cue/cuex/providers/helm/... -v
go build ./...
go vet ./pkg/cue/cuex/providers/helm/...
```

### Manual Verification
1. Deploy a `helmchart` Application → verify `helm list` shows the release
2. Delete a resource → verify KubeVela reconciles it back
3. `helm uninstall <release>` → verify KubeVela re-installs on next reconcile
4. Verify Helm hooks execute during deployment
5. Verify resources have `app.oam.dev/*` labels from the start
