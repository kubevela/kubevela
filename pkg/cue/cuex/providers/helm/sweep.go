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

// Removes hub releases an Application's components have left behind on the hub.

import (
	"context"
	"fmt"
	"strings"

	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/storage/driver"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/auth"
)

// releaseSecretPrefix is the name prefix of every Helm release Secret.
const releaseSecretPrefix = "sh.helm.release.v1."

// ComponentResourceLister is the part of a resource keeper the sweep reads: what the current
// ResourceTracker holds for a component.
type ComponentResourceLister interface {
	ComponentResources(component string) []v1beta1.ManagedResource
	GetAppliedResources() []common.ClusterObjectReference
}

// SweepHubReleases uninstalls the hub releases of an Application's components that its
// ResourceTracker places only on managed clusters. 1.11 installed a helmchart component on the
// hub whatever its placement; that copy is left running once the release is installed where
// the component is placed. Call it once a workflow run has succeeded, when every placement has
// been dispatched and recorded.
func SweepHubReleases(ctx context.Context, app *v1beta1.Application, keeper ComponentResourceLister) error {
	if !tracksManagedClusterRelease(keeper.GetAppliedResources()) {
		return nil
	}
	return NewProvider().sweepHubReleases(ctx, app, func(component string) (bool, bool) {
		var onHub, elsewhere bool
		for _, mr := range keeper.ComponentResources(component) {
			if pkgmulticluster.IsLocal(mr.Cluster) {
				onHub = true
			} else {
				elsewhere = true
			}
		}
		return onHub, elsewhere
	})
}

// tracksManagedClusterRelease reports whether any Helm release Secret is tracked on a managed
// cluster, the only case with a hub copy to sweep.
func tracksManagedClusterRelease(refs []common.ClusterObjectReference) bool {
	for _, ref := range refs {
		if ref.Kind == "Secret" && strings.HasPrefix(ref.Name, releaseSecretPrefix) && !pkgmulticluster.IsLocal(ref.Cluster) {
			return true
		}
	}
	return false
}

// sweepHubReleases uninstalls each hub release labelled for the Application whose component
// placement reports as rendered elsewhere and not on the hub. A component with no tracked
// resources is left alone: there is nothing to judge its placement by.
// Releases are found as the controller and uninstalled as the Application's user, as they were
// installed.
func (p *Provider) sweepHubReleases(ctx context.Context, app *v1beta1.Application, placement func(component string) (onHub, elsewhere bool)) error {
	appName, appNamespace := app.Name, app.Namespace
	ctx = pkgmulticluster.WithCluster(ctx, pkgmulticluster.Local)
	userCtx := auth.ContextWithUserInfo(ctx, app)
	ctx = auth.ContextClearUserInfo(ctx)
	clientset, err := p.kubeClientFactory(ctx)
	if err != nil {
		return err
	}
	secrets, err := clientset.CoreV1().Secrets(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("owner=helm,app.oam.dev/name=%s,app.oam.dev/namespace=%s", appName, appNamespace),
	})
	if err != nil {
		return errors.Wrap(err, "failed to list hub release secrets")
	}
	type releaseKey struct{ namespace, name, component string }
	releases := map[releaseKey]bool{}
	for _, s := range secrets.Items {
		releases[releaseKey{s.Namespace, s.Labels["name"], s.Labels["app.oam.dev/component"]}] = true
	}
	var errs []error
	for r := range releases {
		if onHub, elsewhere := placement(r.component); onHub || !elsewhere {
			continue
		}
		cfg, err := p.actionConfigFactory(userCtx, r.namespace)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		p.releaseMu.Lock()
		_, err = action.NewUninstall(cfg).Run(r.name)
		p.releaseMu.Unlock()
		if err != nil && !errors.Is(err, driver.ErrReleaseNotFound) {
			errs = append(errs, errors.Wrapf(err, "failed to uninstall hub release %s/%s", r.namespace, r.name))
			continue
		}
		p.InvalidateRelease(pkgmulticluster.Local, r.name, r.namespace)
		klog.Infof("Helm provider [app=%s/%s component=%s]: Uninstalled hub release %s/%s, which the component no longer places on the hub",
			appNamespace, appName, r.component, r.namespace, r.name)
	}
	return kerrors.NewAggregate(errs)
}
