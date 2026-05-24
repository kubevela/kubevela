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

// Lifecycle of Helm release Secrets: health validation, label adoption, listing, orphan cleanup, and cache invalidation.

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/release"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

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
	actionConfig, err := p.actionConfigFactory(releaseNamespace)
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
	clientset, err := p.kubeClientFactory()
	if err != nil {
		klog.V(4).Infof("Helm provider: Failed to build kubernetes client for listing secrets: %v", err)
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
	clientset, err := p.kubeClientFactory()
	if err != nil {
		klog.V(4).Infof("Helm provider [%s]: Failed to build kubernetes client for labeling release secrets: %v", velaContextStr(velaCtx), err)
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
	clientset, err := p.kubeClientFactory()
	if err != nil {
		return err
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
