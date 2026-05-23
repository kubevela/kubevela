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

// Dispatches between install and upgrade based on existing release state, fingerprint match, and publishVersion pin.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	"k8s.io/klog/v2"
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
		rel, err := p.performUpgrade(ctx, actionConfig, ch, releaseName, releaseNamespace, values, options, postRenderer, releaseLabels, velaCtx)
		if err != nil {
			return "", "", 0, err
		}
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
