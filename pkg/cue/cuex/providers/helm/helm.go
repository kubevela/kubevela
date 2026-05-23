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

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/release"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/cue/cuex/providers"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/runtime"
	"github.com/kubevela/pkg/util/singleton"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)




// computeReleaseFingerprint builds a deterministic string from chart version and a
// SHA-256 hash of the values so repeated reconciles with no real changes can be
// detected cheaply without calling the Kubernetes API.
//
// Empty-values inputs are normalised to an empty map before hashing. Helm
// stores release.Config as nil when no values were supplied, but mergeValues
// returns map[string]interface{}{} for the same logical input — without this
// guard the two would hash to sha256("null") and sha256("{}") respectively,
// causing the dedup check below to mis-fire and trigger spurious helm upgrades
// on every reconcile for any release that was installed with empty/optional
// values.
func computeReleaseFingerprint(ch *chart.Chart, values map[string]interface{}) string {
	version := ""
	if ch != nil && ch.Metadata != nil {
		version = ch.Metadata.Version
	}
	if values == nil {
		values = map[string]interface{}{}
	}
	valuesJSON, _ := json.Marshal(values)
	h := sha256.Sum256(valuesJSON)
	return version + "|" + hex.EncodeToString(h[:])
}

// installOrUpgradeChart performs a real Helm install or upgrade against the cluster.
// It uses a post-renderer to inject KubeVela ownership labels so the deployed
// resources are immediately owned by the application.
//
// Dedup: a SHA-256 fingerprint of (chartVersion, values) is checked against an
// in-memory cache and the live release in the cluster. If the release is already
// deployed with an identical fingerprint the call is a no-op and the cached
// manifest is returned, preventing spurious revision bumps on every reconcile.
//
// KubeVela labels are also set on the Helm action (install.Labels / upgrade.Labels)
// so they are embedded in the Kubernetes release Secret by the Helm SDK. This allows
// KubeVela to track the release Secret in its ResourceTracker and delete it when the
// Application is deleted — which removes the release from `helm list`.
//
// Returns the release manifest string, notes, release version, and any error.
func (p *Provider) installOrUpgradeChart(ctx context.Context, ch *chart.Chart, releaseName, releaseNamespace string, values map[string]interface{}, options *RenderOptionsParams, velaCtx *ContextParams) (string, string, int, error) {
	fingerprint := computeReleaseFingerprint(ch, values)
	cacheKey := releaseCacheKey(releaseNamespace, releaseName)

	p.releaseMu.Lock()
	defer p.releaseMu.Unlock()

	actionConfig, err := p.getActionConfig(releaseNamespace)
	if err != nil {
		return "", "", 0, err
	}

	postRenderer := &velaLabelPostRenderer{
		context:          velaCtx,
		releaseName:      releaseName,
		releaseNamespace: releaseNamespace,
	}

	// Build labels to embed in the release Secret so KubeVela can track and
	// delete it via the ResourceTracker when the Application is deleted.
	releaseLabels := velaOwnerLabels(velaCtx)

	// Always check the live release in the cluster before using cached data.
	// This prevents stale cache entries from masking externally-deleted releases
	// (e.g., helm uninstall, deleted secrets, namespace deletion).
	getAction := action.NewGet(actionConfig)
	existingRelease, getErr := getAction.Run(releaseName)

	if getErr != nil {
		// Release not found in cluster — clear any stale cache entry so we
		// fall through to a fresh install below.
		if cached, ok := p.releaseFingerprints[cacheKey]; ok {
			klog.Infof("Helm provider [%s]: Release %s not found in cluster but cached (fingerprint=%s), clearing stale cache", velaContextStr(velaCtx), releaseName, cached[:16])
			delete(p.releaseFingerprints, cacheKey)
			delete(p.releaseManifests, cacheKey)
			delete(p.releaseVersions, cacheKey)
		}
	}

	if getErr == nil && existingRelease != nil {
		// Check if this release was installed by KubeVela (has our ownership labels
		// on the release Secret). If not, it's an external release that we need to
		// adopt by forcing an upgrade — even if the fingerprint matches — so the
		// post-renderer injects KubeVela ownership labels onto every resource.
		needsAdoption := velaCtx != nil && !isOwnedByVela(existingRelease, velaCtx)
		if needsAdoption {
			klog.Infof("Helm provider [%s]: Release %s exists but was not installed by KubeVela (missing ownership labels), forcing upgrade to adopt", velaContextStr(velaCtx), releaseName)
			// Label all existing release secrets with KubeVela ownership so they
			// can be tracked by the ResourceTracker and cleaned up on App deletion.
			p.labelReleaseSecrets(releaseNamespace, releaseName, velaCtx)
		}

		// publishVersion pin short-circuit: when the App is at a stable
		// publishVersion pin AND the deployed release was installed at the
		// same pin AND the chart version is unchanged, return the deployed
		// manifest unchanged regardless of any apparent values drift.
		//
		// Without this, a render path that bypasses the workflow gate
		// (state-keep / drift detection / post-dispatch traits / periodic
		// CUE evaluation) re-merges valuesFrom sources and the cluster-side
		// fingerprint compare below would mis-fire whenever a referenced
		// CM/Secret was edited. The user's explicit pin is the contract:
		// nothing changes until they bump the pin.
		//
		// Initial install has no existingRelease so this branch is skipped,
		// and the initial mergeValues runs normally — picking up the
		// referenced CM/Secret content and stamping it into the release.
		if !needsAdoption && velaCtx != nil && velaCtx.PublishVersion != "" &&
			existingRelease.Info != nil && existingRelease.Info.Status == release.StatusDeployed &&
			existingRelease.Chart != nil && existingRelease.Chart.Metadata != nil &&
			existingRelease.Chart.Metadata.Version == ch.Metadata.Version &&
			existingRelease.Labels["app.oam.dev/publishVersion"] == velaCtx.PublishVersion {
			klog.V(2).Infof("Helm provider [%s]: Release %s held by publishVersion pin %q, skipping upgrade",
				velaContextStr(velaCtx), releaseName, velaCtx.PublishVersion)
			p.releaseFingerprints[cacheKey] = fingerprint
			p.releaseManifests[cacheKey] = existingRelease.Manifest
			p.releaseVersions[cacheKey] = existingRelease.Version
			return existingRelease.Manifest, existingRelease.Info.Notes, existingRelease.Version, nil
		}

		// Release exists — check if it is already deployed with the same fingerprint
		if !needsAdoption && existingRelease.Info.Status == release.StatusDeployed {
			clusterFingerprint := computeReleaseFingerprint(existingRelease.Chart, existingRelease.Config)
			if clusterFingerprint == fingerprint {
				klog.V(3).Infof("Helm provider [%s]: Release %s already deployed and unchanged (cluster fingerprint match), skipping upgrade", velaContextStr(velaCtx), releaseName)
				p.releaseFingerprints[cacheKey] = fingerprint
				p.releaseManifests[cacheKey] = existingRelease.Manifest
				p.releaseVersions[cacheKey] = existingRelease.Version
				// Run health validation in the background to detect corrupted
				// or missing release secrets early, without blocking this call.
				go p.validateReleaseHealth(releaseName, releaseNamespace)
				return existingRelease.Manifest, existingRelease.Info.Notes, existingRelease.Version, nil
			}
		}

		// Fingerprint differs, needs adoption, or release is not in a clean deployed state — upgrade
		upgrade := action.NewUpgrade(actionConfig)
		upgrade.Namespace = releaseNamespace
		upgrade.PostRenderer = postRenderer
		upgrade.Labels = releaseLabels

		if options != nil {
			if options.Atomic {
				upgrade.Atomic = true
			}
			if options.Wait || options.Atomic {
				upgrade.Wait = true
			}
			if options.Timeout != "" {
				if d, err := time.ParseDuration(options.Timeout); err == nil {
					upgrade.Timeout = d
				}
			}
			if options.Force {
				upgrade.Force = true
			}
			if options.CleanupOnFail {
				upgrade.CleanupOnFail = true
			}
			if options.RecreatePods {
				upgrade.Recreate = true
			}
			if options.MaxHistory > 0 {
				upgrade.MaxHistory = options.MaxHistory
			}
			if options.SkipHooks != nil {
				upgrade.DisableHooks = *options.SkipHooks
			}
		}

		klog.Infof("Helm provider [%s]: Upgrading release %s in namespace %s", velaContextStr(velaCtx), releaseName, releaseNamespace)
		rel, err := upgrade.RunWithContext(ctx, releaseName, ch, values)
		if err != nil {
			return "", "", 0, errors.Wrapf(err, "failed to upgrade helm release %s", releaseName)
		}
		klog.Infof("Helm provider [%s]: Successfully upgraded release %s", velaContextStr(velaCtx), releaseName)
		p.releaseFingerprints[cacheKey] = fingerprint
		p.releaseManifests[cacheKey] = rel.Manifest
		p.releaseVersions[cacheKey] = rel.Version
		return rel.Manifest, rel.Info.Notes, rel.Version, nil
	}

	// No existing release — perform a fresh install
	rel, err := p.freshInstall(ctx, actionConfig, ch, releaseName, releaseNamespace, values, options, postRenderer, releaseLabels, velaCtx)
	if err != nil {
		return "", "", 0, err
	}
	klog.Infof("Helm provider [%s]: Successfully installed release %s", velaContextStr(velaCtx), releaseName)
	p.releaseFingerprints[cacheKey] = fingerprint
	p.releaseManifests[cacheKey] = rel.Manifest
	p.releaseVersions[cacheKey] = rel.Version
	return rel.Manifest, rel.Info.Notes, rel.Version, nil
}

// freshInstall performs a helm install with retry logic for orphaned/corrupted state.
func (p *Provider) freshInstall(ctx context.Context, actionConfig *action.Configuration, ch *chart.Chart, releaseName, releaseNamespace string, values map[string]interface{}, options *RenderOptionsParams, postRenderer *velaLabelPostRenderer, releaseLabels map[string]string, velaCtx *ContextParams) (*release.Release, error) {
	install := p.newInstallAction(actionConfig, releaseName, releaseNamespace, options, postRenderer, releaseLabels)

	klog.Infof("Helm provider [%s]: Installing release %s in namespace %s", velaContextStr(velaCtx), releaseName, releaseNamespace)
	rel, err := install.RunWithContext(ctx, ch, values)
	if err == nil {
		return rel, nil
	}

	// If install fails due to corrupted/orphaned release secrets or ownership
	// conflicts, clean up the broken state and retry once.
	if !isRetryableInstallError(err) {
		return nil, errors.Wrapf(err, "failed to install helm release %s", releaseName)
	}

	klog.Warningf("Helm provider [%s]: Install failed for %s due to orphaned state (%v), cleaning up and retrying", velaContextStr(velaCtx), releaseName, err)
	if cleanErr := p.cleanOrphanedReleaseSecrets(actionConfig, releaseName, releaseNamespace, velaCtx); cleanErr != nil {
		klog.Warningf("Helm provider [%s]: Failed to clean orphaned secrets for %s: %v", velaContextStr(velaCtx), releaseName, cleanErr)
		return nil, errors.Wrapf(err, "failed to install helm release %s (cleanup also failed: %v)", releaseName, cleanErr)
	}

	retry := p.newInstallAction(actionConfig, releaseName, releaseNamespace, options, postRenderer, releaseLabels)
	klog.Infof("Helm provider [%s]: Retrying install for release %s after cleanup", velaContextStr(velaCtx), releaseName)
	rel, err = retry.RunWithContext(ctx, ch, values)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to install helm release %s after cleanup retry", releaseName)
	}
	return rel, nil
}

// newInstallAction creates a configured helm install action.
func (p *Provider) newInstallAction(actionConfig *action.Configuration, releaseName, releaseNamespace string, options *RenderOptionsParams, postRenderer *velaLabelPostRenderer, releaseLabels map[string]string) *action.Install {
	install := action.NewInstall(actionConfig)
	install.ReleaseName = releaseName
	install.Namespace = releaseNamespace
	install.DryRun = false
	install.ClientOnly = false
	install.PostRenderer = postRenderer
	install.CreateNamespace = true
	install.Labels = releaseLabels
	if options != nil {
		if options.Atomic {
			install.Atomic = true
		}
		if options.Wait || options.Atomic {
			install.Wait = true
		}
		if options.Timeout != "" {
			if d, err := time.ParseDuration(options.Timeout); err == nil {
				install.Timeout = d
			}
		}
		if options.CreateNamespace != nil {
			install.CreateNamespace = *options.CreateNamespace
		}
		if options.SkipHooks != nil {
			install.DisableHooks = *options.SkipHooks
		}
	}
	return install
}

// isRetryableInstallError returns true if the error indicates orphaned state
// that can be fixed by cleaning up and retrying.
func isRetryableInstallError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "cannot be imported") ||
		strings.Contains(msg, "invalid ownership metadata") ||
		strings.Contains(msg, "no revision for release") ||
		strings.Contains(msg, "release: already exists")
}

// dryRunRender performs a client-only Helm template render without touching the
// cluster. Used during webhook validation to verify the chart can be fetched,
// values are valid, and templates render without errors — without blocking on
// real resource creation, hooks, or waiting.
func (p *Provider) dryRunRender(ch *chart.Chart, releaseName, releaseNamespace string, values map[string]interface{}, options *RenderOptionsParams, velaCtx *ContextParams) (string, string, error) {
	install := action.NewInstall(&action.Configuration{})
	install.ReleaseName = releaseName
	install.Namespace = releaseNamespace
	install.DryRun = true
	install.ClientOnly = true

	// Set Kubernetes version capabilities so charts with kubeVersion constraints
	// don't fail against Helm's default v1.20.0. We query the real cluster version
	// via the REST config. If unreachable, the kubeVersion check is skipped —
	// the real install during reconciliation will validate it.
	if kv := p.getKubeVersion(); kv != nil {
		install.KubeVersion = kv
	}

	install.PostRenderer = &velaLabelPostRenderer{
		context:          velaCtx,
		releaseName:      releaseName,
		releaseNamespace: releaseNamespace,
	}

	if options != nil {
		if options.SkipHooks != nil {
			install.DisableHooks = *options.SkipHooks
		}
	}

	rel, err := install.Run(ch, values)
	if err != nil {
		return "", "", errors.Wrapf(err, "dry-run render failed for chart %s", ch.Name())
	}

	return rel.Manifest, rel.Info.Notes, nil
}

// getKubeVersion queries the cluster's Kubernetes version for use in dry-run
// rendering. Returns nil if the cluster is unreachable — Helm will then skip
// the kubeVersion constraint check, deferring it to the real install.
func (p *Provider) getKubeVersion() *chartutil.KubeVersion {
	cfg, err := p.helmClient.RESTClientGetter().ToRESTConfig()
	if err != nil {
		return nil
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil
	}
	info, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return nil
	}
	return &chartutil.KubeVersion{
		Version: fmt.Sprintf("v%s.%s", info.Major, info.Minor),
		Major:   info.Major,
		Minor:   info.Minor,
	}
}

// validateReleaseHealth checks that the Helm release secret exists and is
// readable in the cluster. If the release is missing or corrupted, the
// in-memory cache is invalidated so the next reconciliation performs a fresh
// install/upgrade instead of returning stale cached data.
//
// This method is designed to be called asynchronously (in a goroutine) after
// a successful cache-hit reconciliation, so it does not block the main render
// path. It acquires the release mutex only when it needs to clear the cache.
func (p *Provider) validateReleaseHealth(releaseName, releaseNamespace string) {
	cacheKey := releaseCacheKey(releaseNamespace, releaseName)
	actionConfig, err := p.getActionConfig(releaseNamespace)
	if err != nil {
		klog.Warningf("Helm provider health check: failed to get action config for release %s: %v", releaseName, err)
		return
	}

	getAction := action.NewGet(actionConfig)
	rel, err := getAction.Run(releaseName)
	if err != nil {
		// Release not found or unreadable (corrupted secret) — invalidate cache
		klog.Warningf("Helm provider health check: release %s not found or unreadable in cluster, invalidating cache: %v", releaseName, err)
		p.releaseMu.Lock()
		delete(p.releaseFingerprints, cacheKey)
		delete(p.releaseManifests, cacheKey)
		delete(p.releaseVersions, cacheKey)
		p.releaseMu.Unlock()
		return
	}

	// Release exists — verify it's in a healthy deployed state
	if rel.Info == nil || rel.Info.Status != release.StatusDeployed {
		status := "unknown"
		if rel.Info != nil {
			status = string(rel.Info.Status)
		}
		klog.Warningf("Helm provider health check: release %s is in state %q (expected deployed), invalidating cache", releaseName, status)
		p.releaseMu.Lock()
		delete(p.releaseFingerprints, cacheKey)
		delete(p.releaseManifests, cacheKey)
		delete(p.releaseVersions, cacheKey)
		p.releaseMu.Unlock()
		return
	}

	klog.V(4).Infof("Helm provider health check: release %s is healthy (deployed, revision %d)", releaseName, rel.Version)
}

// cleanOrphanedReleaseSecrets removes corrupted or orphaned Helm release
// secrets for a release. This is called when helm install fails because it
// finds existing secrets it cannot parse or adopt.
//
// Strategy: always delete the secrets directly via the Kubernetes API first,
// since corrupted secrets cannot be reliably handled by Helm's own storage
// driver or uninstall action.
func (p *Provider) cleanOrphanedReleaseSecrets(_ *action.Configuration, releaseName, releaseNamespace string, velaCtx *ContextParams) error {
	// Primary approach: delete secrets directly via Kubernetes API.
	// This is the most reliable method for corrupted secrets.
	klog.Infof("Helm provider [%s]: Cleaning up release secrets for %s in namespace %s via direct deletion", velaContextStr(velaCtx), releaseName, releaseNamespace)
	if err := p.deleteReleaseSecretsDirect(releaseNamespace, releaseName, velaCtx); err != nil {
		return fmt.Errorf("failed to clean release secrets for %s: %w", releaseName, err)
	}
	return nil
}

// listReleaseSecretNames returns the names of all Helm release secrets for the
// given release. Used to track all revision secrets in the ResourceTracker so
// GC cleans them all up on Application deletion.
func (p *Provider) listReleaseSecretNames(namespace, releaseName string) []string {
	cfg, err := p.helmClient.RESTClientGetter().ToRESTConfig()
	if err != nil {
		klog.V(4).Infof("Helm provider: Failed to get REST config for listing secrets: %v", err)
		return nil
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.V(4).Infof("Helm provider: Failed to create clientset for listing secrets: %v", err)
		return nil
	}

	secretList, err := clientset.CoreV1().Secrets(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("owner=helm,name=%s", releaseName),
	})
	if err != nil {
		klog.V(4).Infof("Helm provider: Failed to list release secrets for %s: %v", releaseName, err)
		return nil
	}

	names := make([]string, 0, len(secretList.Items))
	for _, s := range secretList.Items {
		// Only include secrets that have KubeVela ownership labels.
		// Secrets from vanilla helm installs (before KubeVela adoption) won't
		// have these labels, and including them would fail the MustBeControlledByApp
		// check during pre-dispatch dryrun.
		if s.Labels["app.oam.dev/name"] != "" {
			names = append(names, s.Name)
		}
	}
	return names
}

// labelReleaseSecrets adds KubeVela ownership labels to all existing Helm
// release secrets that don't already have them. Called during adoption of
// external releases so that listReleaseSecretNames picks them up for GC tracking.
func (p *Provider) labelReleaseSecrets(namespace, releaseName string, velaCtx *ContextParams) {
	if velaCtx == nil {
		return
	}
	cfg, err := p.helmClient.RESTClientGetter().ToRESTConfig()
	if err != nil {
		return
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return
	}

	secretList, err := clientset.CoreV1().Secrets(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("owner=helm,name=%s", releaseName),
	})
	if err != nil {
		return
	}

	for _, s := range secretList.Items {
		if s.Labels["app.oam.dev/name"] != "" {
			continue // already labeled
		}
		patch := fmt.Sprintf(`{"metadata":{"labels":{"app.oam.dev/name":%q,"app.oam.dev/namespace":%q,"app.oam.dev/component":%q}}}`,
			velaCtx.AppName, velaCtx.AppNamespace, velaCtx.Name)
		_, patchErr := clientset.CoreV1().Secrets(namespace).Patch(
			context.Background(), s.Name, "application/strategic-merge-patch+json",
			[]byte(patch), metav1.PatchOptions{},
		)
		if patchErr != nil {
			klog.Warningf("Helm provider [%s]: Failed to label release secret %s/%s: %v", velaContextStr(velaCtx), namespace, s.Name, patchErr)
		} else {
			klog.Infof("Helm provider [%s]: Labeled release secret %s/%s for adoption", velaContextStr(velaCtx), namespace, s.Name)
		}
	}
}

// deleteReleaseSecretsDirect uses the Kubernetes API directly to delete Helm
// release secrets. This is the last-resort cleanup for secrets that are too
// corrupted for Helm's own storage driver or uninstall action to handle.
func (p *Provider) deleteReleaseSecretsDirect(namespace, releaseName string, velaCtx *ContextParams) error {
	cfg, err := p.helmClient.RESTClientGetter().ToRESTConfig()
	if err != nil {
		return errors.Wrap(err, "failed to get REST config")
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return errors.Wrap(err, "failed to create kubernetes client")
	}

	// List secrets with Helm's labels for this release
	secretList, err := clientset.CoreV1().Secrets(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("owner=helm,name=%s", releaseName),
	})
	if err != nil {
		return errors.Wrapf(err, "failed to list helm secrets for release %s", releaseName)
	}

	for _, secret := range secretList.Items {
		klog.Infof("Helm provider [%s]: Directly deleting corrupted release secret %s/%s", velaContextStr(velaCtx), namespace, secret.Name)
		if err := clientset.CoreV1().Secrets(namespace).Delete(context.Background(), secret.Name, metav1.DeleteOptions{}); err != nil {
			klog.Warningf("Helm provider: Failed to delete secret %s/%s: %v", namespace, secret.Name, err)
		}
	}

	klog.Infof("Helm provider [%s]: Deleted %d orphaned release secrets for %s in namespace %s", velaContextStr(velaCtx), len(secretList.Items), releaseName, namespace)
	return nil
}

// InvalidateRelease clears the in-memory cache for a specific release. This
// can be called by external components (e.g., ResourceTracker GC) when they
// detect that a Helm release secret has been deleted or is missing.
func (p *Provider) InvalidateRelease(releaseName, releaseNamespace string) {
	cacheKey := releaseCacheKey(releaseNamespace, releaseName)
	p.releaseMu.Lock()
	defer p.releaseMu.Unlock()
	delete(p.releaseFingerprints, cacheKey)
	delete(p.releaseManifests, cacheKey)
	delete(p.releaseVersions, cacheKey)
	klog.Infof("Helm provider: Invalidated cache for release %s/%s", releaseNamespace, releaseName)
}

// parseManifestResources parses a Helm release manifest string into a slice of
// resource maps, skipping test hooks when requested and ordering CRDs first.
// Resources whose `metadata.namespace` is empty get defaulted to
// releaseNamespace unless their kind is cluster-scoped. Upstream Helm charts
// commonly omit metadata.namespace and rely on the helm install --namespace
// flag for placement; KubeVela's resource tracker re-applies these outputs
// independently and would otherwise default them to vela-system, creating
// shadow copies and tripping helm's ownership annotation guard on the next
// release. Defaulting at parse time keeps every output keyed to the correct
// namespace from the start.
func (p *Provider) parseManifestResources(manifestStr string, options *RenderOptionsParams, releaseNamespace string) ([]map[string]interface{}, error) {
	skipTests := true
	if options != nil && options.SkipTests != nil {
		skipTests = *options.SkipTests
	}

	resources := []map[string]interface{}{}
	decoder := kyaml.NewYAMLOrJSONDecoder(strings.NewReader(manifestStr), 4096)

	for {
		resource := &unstructured.Unstructured{}
		if err := decoder.Decode(&resource); err != nil {
			if stderrors.Is(err, io.EOF) {
				break
			}
			return nil, errors.Wrap(err, "failed to decode manifest")
		}

		// Skip empty resources
		if resource == nil || len(resource.Object) == 0 {
			continue
		}

		// Skip test resources if requested
		if skipTests && isTestResource(resource) {
			continue
		}

		// Default the namespace for namespaced resources whose template
		// omitted metadata.namespace. Cluster-scoped kinds (CRDs,
		// ClusterRoles, Namespaces, ...) are left as-is so the API server
		// does not reject them.
		if releaseNamespace != "" && resource.GetNamespace() == "" && !isClusterScopedGVK(resource.GroupVersionKind()) {
			resource.SetNamespace(releaseNamespace)
		}

		cleanedResource := cleanResource(resource.Object)
		resources = append(resources, cleanedResource)
	}

	// Order resources: CRDs first, then namespaces, then other resources
	return orderResources(resources), nil
}

// isClusterScopedGVK reports whether the given GroupVersionKind denotes a
// Kubernetes resource that lives at the cluster scope (no namespace).
//
// Resolution order:
//
//  1. Ask the cluster's RESTMapper. This sees built-in kinds AND third-party
//     CRDs (cert-manager's ClusterIssuer, Knative's ClusterIngress, etc.),
//     so the namespace-default logic doesn't mis-namespace custom
//     cluster-scoped resources.
//  2. If the RESTMapper is unavailable, or doesn't know the GVK (e.g.,
//     because the chart manifest itself defines a CRD whose kind hasn't
//     been registered with the API server yet), fall back to a static
//     allowlist of well-known cluster-scoped kinds.
//
// The fallback is intentionally conservative: an unrecognized kind is
// treated as namespaced, so a new namespaced custom resource gets the
// safe default (release namespace) rather than landing in vela-system.
func isClusterScopedGVK(gvk schema.GroupVersionKind) bool {
	if mapper := singleton.RESTMapper.Get(); mapper != nil {
		if mapping, mErr := mapper.RESTMapping(gvk.GroupKind(), gvk.Version); mErr == nil && mapping != nil {
			return mapping.Scope.Name() == meta.RESTScopeNameRoot
		}
	}
	return isClusterScopedKindStaticFallback(gvk.Kind)
}

// isClusterScopedKindStaticFallback returns true for the well-known set of
// built-in cluster-scoped kinds. Used only when the RESTMapper cannot answer
// authoritatively. New entries should be limited to stable upstream APIs;
// for third-party CRDs the RESTMapper path is the source of truth.
func isClusterScopedKindStaticFallback(kind string) bool {
	switch kind {
	case "CustomResourceDefinition",
		"Namespace",
		"ClusterRole",
		"ClusterRoleBinding",
		"PersistentVolume",
		"StorageClass",
		"VolumeAttachment",
		"CSIDriver",
		"CSINode",
		"PriorityClass",
		"RuntimeClass",
		"IngressClass",
		"MutatingWebhookConfiguration",
		"ValidatingWebhookConfiguration",
		"APIService",
		"FlowSchema",
		"PriorityLevelConfiguration",
		"Node",
		"ComponentStatus":
		return true
	}
	return false
}

// isTestResource checks if a resource is a test resource
func isTestResource(resource *unstructured.Unstructured) bool {
	annotations := resource.GetAnnotations()
	if annotations != nil {
		if hookType, exists := annotations["helm.sh/hook"]; exists {
			return strings.Contains(hookType, "test")
		}
	}
	return false
}

// cleanResource removes any nil values from a resource map
func cleanResource(resource map[string]interface{}) map[string]interface{} {
	cleaned := make(map[string]interface{})
	for k, v := range resource {
		if v != nil {
			switch val := v.(type) {
			case map[string]interface{}:
				// Recursively clean nested maps
				cleanedMap := cleanResource(val)
				if len(cleanedMap) > 0 {
					cleaned[k] = cleanedMap
				}
			case []interface{}:
				// Clean arrays
				cleanedArray := make([]interface{}, 0)
				for _, item := range val {
					if item != nil {
						if m, ok := item.(map[string]interface{}); ok {
							cleanedArray = append(cleanedArray, cleanResource(m))
						} else {
							cleanedArray = append(cleanedArray, item)
						}
					}
				}
				if len(cleanedArray) > 0 {
					cleaned[k] = cleanedArray
				}
			default:
				// Keep non-nil values
				cleaned[k] = v
			}
		}
	}
	return cleaned
}

// orderResources orders resources with CRDs first, then namespaces, then others
func orderResources(resources []map[string]interface{}) []map[string]interface{} {
	var crds, namespaces, others []map[string]interface{}

	for _, r := range resources {
		kind, _, _ := unstructured.NestedString(r, "kind")
		switch kind {
		case "CustomResourceDefinition":
			crds = append(crds, r)
		case "Namespace":
			namespaces = append(namespaces, r)
		default:
			others = append(others, r)
		}
	}

	// Combine in order
	result := make([]map[string]interface{}, 0, len(resources))
	result = append(result, crds...)
	result = append(result, namespaces...)
	result = append(result, others...)

	return result
}

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

// Uninstall runs `helm uninstall` for the named release and clears the provider's
// in-memory fingerprint cache so a subsequent Render triggers a fresh install.
func Uninstall(ctx context.Context, params *providers.Params[UninstallParams]) (*providers.Returns[UninstallReturns], error) {
	p := NewProvider()
	up := params.Params

	releaseName := up.Release.Name
	releaseNamespace := up.Release.Namespace

	klog.Infof("Helm provider: Uninstalling release %s in namespace %s", releaseName, releaseNamespace)

	actionConfig, err := p.getActionConfig(releaseNamespace)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize helm action config for uninstall")
	}

	uninstallAction := action.NewUninstall(actionConfig)
	uninstallAction.KeepHistory = up.KeepHistory

	_, err = uninstallAction.Run(releaseName)
	if err != nil {
		// Treat "not found" as a success — the release is already gone
		if strings.Contains(err.Error(), "not found") {
			klog.Infof("Helm provider: Release %s not found, treating as already uninstalled", releaseName)
		} else {
			return &providers.Returns[UninstallReturns]{
				Returns: UninstallReturns{Success: false, Message: err.Error()},
			}, err
		}
	} else {
		klog.Infof("Helm provider: Successfully uninstalled release %s", releaseName)
	}

	// Clear in-memory state so the next Render performs a fresh install
	cacheKey := releaseCacheKey(releaseNamespace, releaseName)
	p.releaseMu.Lock()
	delete(p.releaseFingerprints, cacheKey)
	delete(p.releaseManifests, cacheKey)
	delete(p.releaseVersions, cacheKey)
	p.releaseMu.Unlock()

	return &providers.Returns[UninstallReturns]{
		Returns: UninstallReturns{Success: true},
	}, nil
}

// ProviderName is the name of this provider
const ProviderName = "helm"

//go:embed helm.cue
var template string

// Template exports the CUE template for use by workflow providers
var Template = template

// Package exports the provider package for registration
var Package = runtime.Must(cuexruntime.NewInternalPackage(ProviderName, template, map[string]cuexruntime.ProviderFn{
	"render":    cuexruntime.GenericProviderFn[providers.Params[RenderParams], providers.Returns[RenderReturns]](Render),
	"uninstall": cuexruntime.GenericProviderFn[providers.Params[UninstallParams], providers.Returns[UninstallReturns]](Uninstall),
}))
