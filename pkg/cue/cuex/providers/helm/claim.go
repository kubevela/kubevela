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

// Marks objects a component already owns with Helm's ownership metadata, so an install can adopt them.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/klog/v2"
)

const (
	helmManagedByLabel             = "app.kubernetes.io/managed-by"
	helmReleaseNameAnnotation      = "meta.helm.sh/release-name"
	helmReleaseNamespaceAnnotation = "meta.helm.sh/release-namespace"
)

// claimPatch is the merge patch that marks an object for a release, when the object carries
// this component's KubeVela labels and is not already marked for that or any other release.
// KubeVela dispatches a chart's rendered objects itself, so they can exist without the
// metadata Helm requires before it adopts an object.
func claimPatch(labels, annotations map[string]string, velaCtx *ContextParams, releaseName, releaseNamespace string) ([]byte, bool) {
	// Without all three, an unlabelled object would match the missing ones.
	if velaCtx == nil || velaCtx.AppName == "" || velaCtx.AppNamespace == "" || velaCtx.Name == "" ||
		labels["app.oam.dev/name"] != velaCtx.AppName ||
		labels["app.oam.dev/namespace"] != velaCtx.AppNamespace ||
		labels["app.oam.dev/component"] != velaCtx.Name {
		return nil, false
	}
	// An empty annotation is as good as none.
	name, namespace := annotations[helmReleaseNameAnnotation], annotations[helmReleaseNamespaceAnnotation]
	hasName, hasNamespace := name != "", namespace != ""
	if (hasName && name != releaseName) || (hasNamespace && namespace != releaseNamespace) {
		return nil, false
	}
	if labels[helmManagedByLabel] == "Helm" && hasName && hasNamespace {
		return nil, false
	}
	patch, _ := json.Marshal(map[string]any{"metadata": map[string]any{
		"labels": map[string]string{helmManagedByLabel: "Helm"},
		"annotations": map[string]string{
			helmReleaseNameAnnotation:      releaseName,
			helmReleaseNamespaceAnnotation: releaseNamespace,
		},
	}})
	return patch, true
}

// claimOwnedResources marks the live objects a release would create that this component
// already owns, so a fresh install adopts them instead of failing on ownership metadata.
func (p *Provider) claimOwnedResources(ctx context.Context, actionConfig *action.Configuration, ch *chart.Chart, releaseName, releaseNamespace string, values map[string]interface{}, options *RenderOptionsParams, velaCtx *ContextParams) error {
	if velaCtx == nil {
		return nil
	}
	manifest, _, err := p.dryRunRender(ctx, ch, releaseName, releaseNamespace, values, options, velaCtx)
	if err != nil {
		return err
	}
	resources, err := actionConfig.KubeClient.Build(bytes.NewBufferString(manifest), false)
	if err != nil {
		return err
	}
	// One object that cannot be read or marked does not stop the others being marked.
	var errs []error
	visitErr := resources.Visit(func(info *resource.Info, err error) error {
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		if err := claimObject(info, velaCtx, releaseName, releaseNamespace); err != nil {
			errs = append(errs, fmt.Errorf("%s %s/%s: %w", info.Mapping.GroupVersionKind.Kind, info.Namespace, info.Name, err))
		}
		return nil
	})
	return utilerrors.NewAggregate(append(errs, visitErr))
}

// claimObject marks one live object for adoption by the release, if this component owns it.
func claimObject(info *resource.Info, velaCtx *ContextParams, releaseName, releaseNamespace string) error {
	live, err := resource.NewHelper(info.Client, info.Mapping).Get(info.Namespace, info.Name)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	accessor, err := meta.Accessor(live)
	if err != nil {
		return err
	}
	patch, ok := claimPatch(accessor.GetLabels(), accessor.GetAnnotations(), velaCtx, releaseName, releaseNamespace)
	if !ok {
		return nil
	}
	if _, err := resource.NewHelper(info.Client, info.Mapping).Patch(info.Namespace, info.Name, types.MergePatchType, patch, nil); err != nil {
		return err
	}
	klog.Infof("Helm provider [%s]: Marked %s %s/%s for adoption by release %s",
		velaContextStr(velaCtx), info.Mapping.GroupVersionKind.Kind, info.Namespace, info.Name, releaseName)
	return nil
}
