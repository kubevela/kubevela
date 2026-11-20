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
	"strings"

	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/release"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// validateReleaseHealth checks that the Helm release secret exists and is
// readable in the cluster. If the release is missing or corrupted, the
// in-memory cache is invalidated so the next reconciliation performs a fresh
// install/upgrade instead of returning stale cached data.
//
// This method is designed to be called asynchronously (in a goroutine) after
// a successful cache-hit reconciliation, so it does not block the main render
// path. It acquires the release mutex only when it needs to clear the cache.
func (p *Provider) validateReleaseHealth(ctx context.Context, releaseName, releaseNamespace string) {
	cacheKey := releaseCacheKey(clusterFrom(ctx), releaseNamespace, releaseName)
	actionConfig, err := p.actionConfigFactory(ctx, releaseNamespace)
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
func (p *Provider) cleanOrphanedReleaseSecrets(ctx context.Context, releaseName, releaseNamespace string, velaCtx *ContextParams) error {
	// Primary approach: delete secrets directly via Kubernetes API.
	// This is the most reliable method for corrupted secrets.
	klog.Infof("Helm provider [%s]: Cleaning up release secrets for %s in namespace %s via direct deletion", velaContextStr(velaCtx), releaseName, releaseNamespace)
	if err := p.deleteReleaseSecretsDirect(ctx, releaseNamespace, releaseName, velaCtx); err != nil {
		return fmt.Errorf("failed to clean release secrets for %s: %w", releaseName, err)
	}
	return nil
}

// listReleaseSecretNames returns the names of all Helm release secrets for the
// given release. Used to track all revision secrets in the ResourceTracker so
// GC cleans them all up on Application deletion.
func (p *Provider) listReleaseSecretNames(ctx context.Context, namespace, releaseName string) []string {
	clientset, err := p.kubeClientFactory(ctx)
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
		// Only secrets this Application owns, by its app.oam.dev/* labels. One from a vanilla
		// helm install has none, and dispatching it would fail the keeper's ownership check.
		if s.Labels["app.oam.dev/name"] != "" {
			names = append(names, s.Name)
		}
	}
	return names
}

// labelReleaseSecrets labels every Secret of a release as owned by this component, replacing
// a departed owner's labels. Called during adoption so that listReleaseSecretNames picks them
// up for GC tracking and the resource keeper accepts them.
func (p *Provider) labelReleaseSecrets(ctx context.Context, namespace, releaseName string, velaCtx *ContextParams) {
	if velaCtx == nil {
		return
	}
	clientset, err := p.kubeClientFactory(ctx)
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
		if s.Labels["app.oam.dev/name"] == velaCtx.AppName &&
			s.Labels["app.oam.dev/namespace"] == velaCtx.AppNamespace &&
			s.Labels["app.oam.dev/component"] == velaCtx.Name {
			continue
		}
		// Only a release with no owner, or one whose Application is gone, is taken over.
		if err := p.checkOwnerLabels(ctx, s.Labels, namespace, releaseName, velaCtx); err != nil {
			klog.Warningf("Helm provider [%s]: Not labelling release secret %s/%s: %v", velaContextStr(velaCtx), namespace, s.Name, err)
			continue
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
func (p *Provider) deleteReleaseSecretsDirect(ctx context.Context, namespace, releaseName string, velaCtx *ContextParams) error {
	clientset, err := p.kubeClientFactory(ctx)
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

	// A release Helm cannot read still names its owner on its Secrets, and one another Application
	// owns is not this component's to clear.
	for _, secret := range secretList.Items {
		if err := p.checkOwnerLabels(ctx, secret.Labels, namespace, releaseName, velaCtx); err != nil {
			return err
		}
	}

	// Helm's own Secrets are what block the retry, so a failed placeholder list does not stop them.
	placeholders, err := p.listReleasePlaceholders(ctx, clientset, namespace, releaseName, velaCtx)
	if err != nil {
		klog.Warningf("Helm provider [%s]: Failed to list placeholder secrets for %s, deleting helm-owned secrets only: %v", velaContextStr(velaCtx), releaseName, err)
	}
	secretList.Items = append(secretList.Items, placeholders...)

	// Any Secret left behind blocks the retry, so a failed delete fails the cleanup.
	var errs []error
	for _, secret := range secretList.Items {
		klog.Infof("Helm provider [%s]: Directly deleting corrupted release secret %s/%s", velaContextStr(velaCtx), namespace, secret.Name)
		if err := clientset.CoreV1().Secrets(namespace).Delete(context.Background(), secret.Name, metav1.DeleteOptions{}); client.IgnoreNotFound(err) != nil {
			errs = append(errs, errors.Wrapf(err, "failed to delete release secret %s/%s", namespace, secret.Name))
		}
	}
	if len(errs) > 0 {
		return utilerrors.NewAggregate(errs)
	}

	klog.Infof("Helm provider [%s]: Deleted %d orphaned release secrets for %s in namespace %s", velaContextStr(velaCtx), len(secretList.Items), releaseName, namespace)
	return nil
}

// InvalidateRelease clears the in-memory cache for a specific release. This
// can be called by external components (e.g., ResourceTracker GC) when they
// detect that a Helm release secret has been deleted or is missing.
func (p *Provider) InvalidateRelease(cluster, releaseName, releaseNamespace string) {
	cacheKey := releaseCacheKey(cluster, releaseNamespace, releaseName)
	p.releaseMu.Lock()
	defer p.releaseMu.Unlock()
	delete(p.releaseFingerprints, cacheKey)
	delete(p.releaseManifests, cacheKey)
	delete(p.releaseVersions, cacheKey)
	klog.Infof("Helm provider: Invalidated cache for release %s/%s", releaseNamespace, releaseName)
}

// listReleasePlaceholders returns copies of a release's Secrets that KubeVela dispatched as
// ordinary objects for this component. Helm does not see them (they lack its owner label) but
// they hold the release's Secret names, so an install there fails until they are removed.
func (p *Provider) listReleasePlaceholders(ctx context.Context, clientset kubernetes.Interface, namespace, releaseName string, velaCtx *ContextParams) ([]corev1.Secret, error) {
	if velaCtx == nil {
		return nil, nil
	}
	list, err := clientset.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.oam.dev/name=%s,app.oam.dev/namespace=%s,app.oam.dev/component=%s,!owner",
			velaCtx.AppName, velaCtx.AppNamespace, velaCtx.Name),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list placeholder secrets for release %s", releaseName)
	}
	prefix := "sh.helm.release.v1." + releaseName + ".v"
	var found []corev1.Secret
	for _, s := range list.Items {
		if s.Type == "helm.sh/release.v1" && strings.HasPrefix(s.Name, prefix) && s.Labels["owner"] == "" {
			found = append(found, s)
		}
	}
	return found, nil
}
