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

package appkeeper

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/hashicorp/go-version"
	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
	version2 "github.com/oam-dev/kubevela/version"
)

const velaVersionNumberToUpgradeResourceTracker = "v1.2.0"

// garbageCollectLegacyResourceTrackers removes ResourceTrackers written before v1.2 (those
// without spec.type) for app in every cluster it touched, then records the upgrade on app.
// The keeper calls it once app has a current tracker, or while app is being deleted.
func garbageCollectLegacyResourceTrackers(ctx context.Context, cli client.Client, app *v1beta1.Application) error {
	// skip legacy gc if controller not enable this feature
	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.LegacyResourceTrackerGC) {
		return nil
	}
	// check app version
	velaVersionToUpgradeResourceTracker, _ := version.NewVersion(velaVersionNumberToUpgradeResourceTracker)
	var currentVersionNumber string
	if annotations := app.GetAnnotations(); annotations != nil && annotations[oam.AnnotationKubeVelaVersion] != "" {
		currentVersionNumber = annotations[oam.AnnotationKubeVelaVersion]
	}
	currentVersion, err := version.NewVersion(currentVersionNumber)
	if err == nil && velaVersionToUpgradeResourceTracker.LessThanOrEqual(currentVersion) {
		return nil
	}
	// remove legacy ResourceTrackers
	clusters := map[string]bool{pkgmulticluster.Local: true}
	for _, rsc := range app.Status.AppliedResources {
		if rsc.Cluster != "" {
			clusters[rsc.Cluster] = true
		}
	}
	for _, policy := range app.Spec.Policies {
		if policy.Type == v1alpha1.EnvBindingPolicyType && policy.Properties != nil {
			spec := &v1alpha1.EnvBindingSpec{}
			if err = json.Unmarshal(policy.Properties.Raw, &spec); err == nil {
				for _, env := range spec.Envs {
					if env.Placement.ClusterSelector != nil && env.Placement.ClusterSelector.Name != "" {
						clusters[env.Placement.ClusterSelector.Name] = true
					}
				}
			}
		}
	}
	for cluster := range clusters {
		_ctx := pkgmulticluster.WithCluster(ctx, cluster)
		rts := &unstructured.UnstructuredList{}
		rts.SetGroupVersionKind(v1beta1.SchemeGroupVersion.WithKind("ResourceTrackerList"))
		if err = cli.List(_ctx, rts, client.MatchingLabels(map[string]string{
			oam.LabelAppName:      app.Name,
			oam.LabelAppNamespace: app.Namespace,
		})); err != nil {
			if strings.Contains(err.Error(), "could not find the requested resource") {
				continue
			}
			return errors.Wrapf(err, "failed to list resource trackers for app %s/%s in cluster %s", app.Namespace, app.Name, cluster)
		}
		for _, rt := range rts.Items {
			if s, exists, _ := unstructured.NestedString(rt.Object, "spec", "type"); !exists || s == "" {
				if err = cli.Delete(_ctx, rt.DeepCopy()); err != nil {
					return errors.Wrapf(err, "failed to delete legacy resource tracker %s for app %s/%s in cluster %s", rt.GetName(), app.Namespace, app.Name, cluster)
				}
			}
		}
	}
	// upgrade app version
	latest := &v1beta1.Application{}
	if err = cli.Get(ctx, client.ObjectKeyFromObject(app), latest); err != nil {
		return errors.Wrapf(err, "failed to get app %s/%s for upgrade version", app.Namespace, app.Name)
	}
	if _, err = version.NewVersion(version2.VelaVersion); err != nil {
		metav1.SetMetaDataAnnotation(&latest.ObjectMeta, oam.AnnotationKubeVelaVersion, velaVersionNumberToUpgradeResourceTracker)
	} else {
		metav1.SetMetaDataAnnotation(&latest.ObjectMeta, oam.AnnotationKubeVelaVersion, version2.VelaVersion)
	}
	if err = cli.Update(ctx, latest); err != nil {
		return errors.Wrapf(err, "failed to upgrade app %s/%s", app.Namespace, app.Name)
	}
	app.ObjectMeta = latest.ObjectMeta
	return nil
}
