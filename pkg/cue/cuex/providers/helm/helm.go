/*
Copyright 2026 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package helm

// Public Render entry point and package registration for the helm cuex provider.

import (
	"context"
	_ "embed"
	"encoding/json"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/cue/cuex/providers"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/runtime"
	"github.com/kubevela/pkg/util/singleton"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// Render is the main provider function: it performs a real Helm install/upgrade
// against the cluster and returns the deployed resources for KubeVela to adopt.
func Render(ctx context.Context, params *providers.Params[RenderParams]) (*providers.Returns[RenderReturns], error) {
	p := NewProvider()

	renderParams := params.Params

	klog.V(2).Infof("Helm provider [%s]: Starting render for chart %s from %s", velaContextStr(renderParams.Context), renderParams.Chart.Source, renderParams.Chart.RepoURL)

	// Application namespace is the tenant boundary. When the Application has no
	// explicit context, fall back to the release namespace below so the same
	// Application can be rendered outside a ComponentDefinition path.
	appNamespace := ""
	if renderParams.Context != nil {
		appNamespace = renderParams.Context.AppNamespace
	}

	releaseName := "release"
	releaseNamespace := appNamespace
	if renderParams.Release != nil {
		if renderParams.Release.Name != "" {
			releaseName = renderParams.Release.Name
		}
		if renderParams.Release.Namespace != "" {
			releaseNamespace = renderParams.Release.Namespace
		}
	}
	// Guarantee a non-empty release namespace. Under the normal KubeVela
	// code path the controller always sets Context.AppNamespace before
	// calling Render, but callers that invoke the provider directly (tests,
	// CLI tooling) may leave both context and Release.Namespace empty.
	// Falling back to "default" preserves the pre-refactor behavior and
	// keeps Helm's namespace resolution from depending on the caller's
	// kubeconfig default.
	if releaseNamespace == "" {
		releaseNamespace = "default"
	}
	if appNamespace == "" {
		appNamespace = releaseNamespace
	}

	// Resolve the App's publishVersion annotation, if any. We pass it through
	// ContextParams.PublishVersion so installOrUpgradeChart can short-circuit
	// when the deployed release is already at the current pin and so
	// velaOwnerLabels can stamp the pin onto the release at install time.
	// Skipped in dry-run: admission validation must not depend on cluster
	// state, and the user-visible behaviour (CUE shape OK / not OK) is
	// independent of the pin.
	//
	// IsNotFound is treated as "App is being deleted" and falls through with
	// an empty pin — the subsequent uninstall path handles cleanup. Any other
	// error (RBAC change, transient API failure, network blip) is surfaced
	// rather than silently swallowed: a swallowed error would leave the pin
	// empty for this reconcile and bypass the pin short-circuit downstream,
	// allowing an unintended helm upgrade to fire even though the user's
	// publishVersion annotation is still in place.
	if !isDryRun(ctx) && renderParams.Context != nil && renderParams.Context.AppName != "" && appNamespace != "" {
		var app v1beta1.Application
		switch getErr := singleton.KubeClient.Get().Get(ctx, client.ObjectKey{Name: renderParams.Context.AppName, Namespace: appNamespace}, &app); {
		case getErr == nil:
			if pin := app.GetAnnotations()[oam.AnnotationPublishVersion]; pin != "" {
				renderParams.Context.PublishVersion = pin
			}
		case apierrors.IsNotFound(getErr):
			// App is gone (deletion in flight). Proceed without a pin.
		default:
			return nil, errors.Wrapf(getErr,
				"failed to read Application %s/%s for publishVersion lookup; refusing to proceed without pin context",
				appNamespace, renderParams.Context.AppName)
		}
	}

	klog.V(3).Infof("Helm provider: Release name=%s, namespace=%s", releaseName, releaseNamespace)

	// Fetch the chart
	ch, err := p.fetchChart(ctx, &renderParams.Chart, renderParams.Options, appNamespace, releaseNamespace)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch chart")
	}
	klog.V(2).Infof("Helm provider: Successfully fetched chart %s", ch.Name())

	// Skip valuesFrom resolution in dry-run (webhook admission): the webhook
	// validates CUE shape and renders the chart, not the final merged values,
	// and running loadValuesFromSource during admission adds N cluster reads
	// per Application create/update plus ordering hazards when the referenced
	// CM/Secret is applied in the same kubectl batch.
	var values map[string]interface{}
	if isDryRun(ctx) {
		if inline, ok := renderParams.Values.(map[string]interface{}); ok {
			values = inline
		} else {
			values = map[string]interface{}{}
		}
	} else {
		values, err = p.mergeValues(ctx, renderParams.Values, renderParams.ValuesFrom, appNamespace, releaseNamespace)
		if err != nil {
			return nil, errors.Wrapf(err, "%s: failed to merge values", velaContextStr(renderParams.Context))
		}
	}

	// In dry-run mode (webhook validation), render client-side only — no cluster
	// interaction, no real install, no hooks. This prevents the webhook from
	// blocking for 30-60s on large charts.
	var manifest string
	var notes string
	if isDryRun(ctx) {
		klog.V(2).Infof("Helm provider: Dry-run mode — rendering chart %s client-side only", ch.Name())
		manifest, notes, err = p.dryRunRender(ch, releaseName, releaseNamespace, values, renderParams.Options, renderParams.Context)
		if err != nil {
			return nil, errors.Wrap(err, "failed to dry-run render chart")
		}
	} else {
		// Install or upgrade the chart via the Helm SDK
		manifest, notes, _, err = p.installOrUpgradeChart(ctx, ch, releaseName, releaseNamespace, values, renderParams.Options, renderParams.Context)
		if err != nil {
			return nil, errors.Wrap(err, "failed to install/upgrade chart")
		}
	}

	// Parse the release manifest into KubeVela resource maps
	resources, err := p.parseManifestResources(manifest, renderParams.Options, releaseNamespace)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse release manifest")
	}

	// Include ALL Helm release Secrets as tracked resources so KubeVela's
	// ResourceTracker records them and GC deletes them when the Application
	// is deleted. We query the cluster for every secret belonging to this
	// release (v1, v2, v3, …) so that:
	// - On Application deletion: all secrets are cleaned up, no orphans
	// - During upgrades: all existing secrets remain tracked, preventing
	//   accidental GC. Helm's own maxHistory handles old revision pruning.
	// Include ALL Helm release Secrets as skeleton resources so KubeVela's
	// ResourceTracker records them and GC deletes them on Application deletion.
	// The skeleton intentionally omits the data field — KubeVela's merge-patch
	// strategy preserves unspecified fields, so Helm's data.release blob is
	// untouched. No special dispatcher changes needed.
	if renderParams.Context != nil {
		releaseSecretNames := p.listReleaseSecretNames(releaseNamespace, releaseName)
		for _, secName := range releaseSecretNames {
			secretMeta := map[string]interface{}{
				"name":      secName,
				"namespace": releaseNamespace,
			}
			// Add KubeVela ownership labels so MustBeControlledByApp passes
			// during pre-dispatch dryrun (especially for adoption of vanilla releases)
			if renderParams.Context != nil {
				secretMeta["labels"] = map[string]interface{}{
					"app.oam.dev/name":      renderParams.Context.AppName,
					"app.oam.dev/namespace": renderParams.Context.AppNamespace,
					"app.oam.dev/component": renderParams.Context.Name,
				}
			}
			releaseSecret := map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata":   secretMeta,
				"type":       "helm.sh/release.v1",
			}
			resources = append(resources, releaseSecret)
		}
		if len(releaseSecretNames) > 0 {
			klog.V(3).Infof("Helm provider: Tracking %d release secrets for %s", len(releaseSecretNames), releaseName)
		}
	}

	klog.Infof("Helm provider [%s]: Deployed %d resources for chart %s", velaContextStr(renderParams.Context), len(resources), renderParams.Chart.Source)

	// Log resource summary for debugging
	if len(resources) > 0 {
		if kind, found, _ := unstructured.NestedString(resources[0], "kind"); found {
			if name, found, _ := unstructured.NestedString(resources[0], "metadata", "name"); found {
				klog.Infof("Helm provider [%s]: First resource is %s/%s", velaContextStr(renderParams.Context), kind, name)
			}
		}

		if jsonBytes, err := json.MarshalIndent(resources[0], "", "  "); err == nil {
			klog.V(4).Infof("Helm provider: First resource JSON:\n%s", string(jsonBytes))
		}

		klog.V(3).Infof("Helm provider: All resources summary:")
		for i, res := range resources {
			if kind, found, _ := unstructured.NestedString(res, "kind"); found {
				if name, found, _ := unstructured.NestedString(res, "metadata", "name"); found {
					klog.V(3).Infof("  [%d] %s/%s", i, kind, name)
				}
			}
		}
	}

	result := &providers.Returns[RenderReturns]{
		Returns: RenderReturns{
			Resources: resources,
			Notes:     notes,
		},
	}

	klog.V(3).Infof("Helm provider: Returning result with %d resources", len(result.Returns.Resources))

	return result, nil
}

// ProviderName is the name of this provider
const ProviderName = "helm"

//go:embed helm.cue
var template string

// Template exports the CUE template for use by workflow providers
var Template = template

// Package exports the provider package for registration
var Package = runtime.Must(cuexruntime.NewInternalPackage(ProviderName, template, map[string]cuexruntime.ProviderFn{
	"render": cuexruntime.GenericProviderFn[providers.Params[RenderParams], providers.Returns[RenderReturns]](Render),
}))
