/*
 Copyright 2021. The KubeVela Authors.

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

package envbinding

import (
	"context"
	"fmt"
	"reflect"

	"k8s.io/klog/v2"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ocmclusterv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	ocmworkv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// ClusterManagerEngine defines Cluster Manage interface
type ClusterManagerEngine interface {
	prepare(ctx context.Context, configs []v1alpha1.EnvConfig) error
	initEnvBindApps(ctx context.Context, envBinding *v1alpha1.EnvBinding, baseApp *v1beta1.Application, appParser *appfile.Parser) ([]*EnvBindApp, error)
	schedule(ctx context.Context, apps []*EnvBindApp) ([]v1alpha1.ClusterDecision, error)
}

// OCMEngine represents Open-Cluster-Management multi-cluster management solution
type OCMEngine struct {
	cli              client.Client
	clusterDecisions map[string]string
	appNs            string
	envBindingName   string
	appName          string
}

// NewOCMEngine create Open-Cluster-Management ClusterManagerEngine
func NewOCMEngine(cli client.Client, appName, appNs, envBindingName string) ClusterManagerEngine {
	return &OCMEngine{
		cli:            cli,
		appNs:          appNs,
		appName:        appName,
		envBindingName: envBindingName,
	}
}

// prepare complete the pre-work of cluster scheduling and select the target cluster
// 1) if user directly specify the cluster name, Prepare will do nothing
// 2) if user use Labels to select the target cluster, Prepare will create the Placement to select cluster
func (o *OCMEngine) prepare(ctx context.Context, configs []v1alpha1.EnvConfig) error {
	var err error
	for _, config := range configs {
		if len(config.Placement.ClusterSelector.Name) != 0 {
			continue
		}
		err = o.dispatchPlacement(ctx, config)
		if err != nil {
			return err
		}
	}

	clusterDecisions := make(map[string]string)
	for _, config := range configs {
		if len(config.Placement.ClusterSelector.Name) != 0 {
			clusterDecisions[config.Name] = config.Placement.ClusterSelector.Name
			continue
		}
		placementName := generatePlacementName(o.appName, config.Name)
		clusterDecisions[config.Name], err = o.getSelectedCluster(ctx, placementName, o.appNs)
		if err != nil {
			return err
		}
	}
	o.clusterDecisions = clusterDecisions
	return nil
}

func (o *OCMEngine) initEnvBindApps(ctx context.Context, envBinding *v1alpha1.EnvBinding, baseApp *v1beta1.Application, appParser *appfile.Parser) ([]*EnvBindApp, error) {
	envBindApps, err := CreateEnvBindApps(envBinding, baseApp)
	if err != nil {
		return nil, err
	}
	if err = RenderEnvBindApps(ctx, envBindApps, appParser); err != nil {
		return nil, err
	}
	if err = AssembleEnvBindApps(envBindApps); err != nil {
		return nil, err
	}
	return envBindApps, nil
}

// Schedule decides which cluster the apps is scheduled to
func (o *OCMEngine) schedule(ctx context.Context, apps []*EnvBindApp) ([]v1alpha1.ClusterDecision, error) {
	var clusterDecisions []v1alpha1.ClusterDecision

	for i := range apps {
		app := apps[i]
		app.ScheduledManifests = make(map[string]*unstructured.Unstructured, 1)
		clusterName := o.clusterDecisions[app.envConfig.Name]
		manifestWork := new(ocmworkv1.ManifestWork)
		workloads := make([]ocmworkv1.Manifest, 0, len(app.assembledManifests))
		for _, component := range app.patchedApp.Spec.Components {
			manifest := app.assembledManifests[component.Name]
			for j := range manifest {
				workloads = append(workloads, ocmworkv1.Manifest{
					RawExtension: util.Object2RawExtension(manifest[j]),
				})
			}
		}
		manifestWork.Spec.Workload.Manifests = workloads
		obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(manifestWork)
		if err != nil {
			return nil, err
		}
		unstructuredManifestWork := &unstructured.Unstructured{
			Object: obj,
		}
		unstructuredManifestWork.SetGroupVersionKind(ocmworkv1.GroupVersion.WithKind(reflect.TypeOf(ocmworkv1.ManifestWork{}).Name()))
		envBindAppName := constructEnvBindAppName(o.envBindingName, app.envConfig.Name, o.appName)
		unstructuredManifestWork.SetName(envBindAppName)
		unstructuredManifestWork.SetNamespace(clusterName)
		app.ScheduledManifests[envBindAppName] = unstructuredManifestWork
	}

	for env, cluster := range o.clusterDecisions {
		clusterDecisions = append(clusterDecisions, v1alpha1.ClusterDecision{
			Env:     env,
			Cluster: cluster,
		})
	}
	return clusterDecisions, nil
}

// dispatchPlacement dispatch Placement Object of OCM for cluster selected
func (o *OCMEngine) dispatchPlacement(ctx context.Context, config v1alpha1.EnvConfig) error {
	placement := new(ocmclusterv1alpha1.Placement)
	placementName := generatePlacementName(o.appName, config.Name)
	placement.SetName(placementName)
	placement.SetNamespace(o.appNs)

	clusterNum := int32(1)
	placement.Spec.NumberOfClusters = &clusterNum
	placement.Spec.Predicates = []ocmclusterv1alpha1.ClusterPredicate{{
		RequiredClusterSelector: ocmclusterv1alpha1.ClusterSelector{
			LabelSelector: metav1.LabelSelector{
				MatchLabels: config.Placement.ClusterSelector.Labels,
			},
		},
	}}

	oldPd := new(ocmclusterv1alpha1.Placement)
	if err := o.cli.Get(ctx, client.ObjectKey{Namespace: placement.Namespace, Name: placement.Name}, oldPd); err != nil {
		if kerrors.IsNotFound(err) {
			return o.cli.Create(ctx, placement)
		}
		return err
	}
	return o.cli.Patch(ctx, placement, client.Merge)
}

// getSelectedCluster get selected cluster from PlacementDecision
func (o *OCMEngine) getSelectedCluster(ctx context.Context, name, namespace string) (string, error) {
	var clusterName string
	listOpts := []client.ListOption{
		client.MatchingLabels{
			"cluster.open-cluster-management.io/placement": name,
		},
		client.InNamespace(namespace),
	}

	pdList := new(ocmclusterv1alpha1.PlacementDecisionList)
	err := o.cli.List(ctx, pdList, listOpts...)
	if err != nil {
		return "", err
	}
	if len(pdList.Items) < 1 {
		return "", errors.New("fail to get PlacementDecision")
	}

	if len(pdList.Items[0].Status.Decisions) < 1 {
		return "", errors.New("no matched cluster")
	}
	clusterName = pdList.Items[0].Status.Decisions[0].ClusterName
	return clusterName, nil
}

// generatePlacementName generate placementName from app Name and env Name
func generatePlacementName(appName, envName string) string {
	return fmt.Sprintf("%s-%s", appName, envName)
}

// SingleClusterEngine represents deploy resources to the local cluster
type SingleClusterEngine struct {
	cli                client.Client
	appNs              string
	appName            string
	envBindingName     string
	clusterDecisions   map[string]string
	namespaceDecisions map[string]string
}

// NewSingleClusterEngine create a single cluster ClusterManagerEngine
func NewSingleClusterEngine(cli client.Client, appName, appNs, envBindingName string) ClusterManagerEngine {
	return &SingleClusterEngine{
		cli:            cli,
		appNs:          appNs,
		appName:        appName,
		envBindingName: envBindingName,
	}
}

func (s *SingleClusterEngine) prepare(ctx context.Context, configs []v1alpha1.EnvConfig) error {
	clusterDecisions := make(map[string]string)
	for _, config := range configs {
		clusterDecisions[config.Name] = string(v1alpha1.SingleClusterEngine)
	}
	s.clusterDecisions = clusterDecisions
	return nil
}

func (s *SingleClusterEngine) initEnvBindApps(ctx context.Context, envBinding *v1alpha1.EnvBinding, baseApp *v1beta1.Application, appParser *appfile.Parser) ([]*EnvBindApp, error) {
	return CreateEnvBindApps(envBinding, baseApp)
}

func (s *SingleClusterEngine) schedule(ctx context.Context, apps []*EnvBindApp) ([]v1alpha1.ClusterDecision, error) {
	var clusterDecisions []v1alpha1.ClusterDecision
	namespaceDecisions := make(map[string]string)
	for i := range apps {
		app := apps[i]

		selectedNamespace, err := s.getSelectedNamespace(ctx, app)
		namespaceDecisions[app.envConfig.Name] = selectedNamespace
		if err != nil {
			return nil, err
		}

		app.ScheduledManifests = make(map[string]*unstructured.Unstructured, 1)
		unstructuredApp, err := util.Object2Unstructured(app.patchedApp)
		if err != nil {
			return nil, err
		}
		envBindAppName := constructEnvBindAppName(s.envBindingName, app.envConfig.Name, s.appName)
		unstructuredApp.SetName(envBindAppName)
		unstructuredApp.SetNamespace(selectedNamespace)
		app.ScheduledManifests[envBindAppName] = unstructuredApp
	}

	s.namespaceDecisions = namespaceDecisions
	for env, cluster := range s.clusterDecisions {
		clusterDecisions = append(clusterDecisions, v1alpha1.ClusterDecision{
			Env:       env,
			Cluster:   cluster,
			Namespace: s.namespaceDecisions[env],
		})
	}
	return clusterDecisions, nil
}

func (s *SingleClusterEngine) getSelectedNamespace(ctx context.Context, envBindApp *EnvBindApp) (string, error) {
	if envBindApp.envConfig.Placement.NamespaceSelector != nil {
		selector := envBindApp.envConfig.Placement.NamespaceSelector
		if len(selector.Name) != 0 {
			return selector.Name, nil
		}
		if len(selector.Labels) != 0 {
			namespaceList := new(corev1.NamespaceList)
			listOpts := []client.ListOption{
				client.MatchingLabels(selector.Labels),
			}
			err := s.cli.List(ctx, namespaceList, listOpts...)
			if err != nil || len(namespaceList.Items) == 0 {
				return "", errors.Wrapf(err, "fail to list selected namespace for env %s", envBindApp.envConfig.Name)
			}
			return namespaceList.Items[0].Name, nil
		}
	}
	return envBindApp.patchedApp.Namespace, nil
}

func validatePlacement(envBinding *v1alpha1.EnvBinding) error {
	if envBinding.Spec.Engine == v1alpha1.OCMEngine || len(envBinding.Spec.Engine) == 0 {
		for _, config := range envBinding.Spec.Envs {
			if config.Placement.ClusterSelector == nil {
				return errors.New("the cluster selector of placement shouldn't be empty")
			}
		}
	}
	return nil
}

func constructEnvBindAppName(envBindingName, envName, appName string) string {
	return fmt.Sprintf("%s-%s-%s", envBindingName, envName, appName)
}

func constructResourceTrackerName(envBindingName, namespace string) string {
	return fmt.Sprintf("%s-%s-%s", "envbinding", envBindingName, namespace)
}

func garbageCollect(ctx context.Context, k8sClient client.Client, envBinding *v1alpha1.EnvBinding, apps []*EnvBindApp) error {
	rtRef := envBinding.Status.ResourceTracker
	if rtRef == nil {
		return nil
	}

	rt := new(v1beta1.ResourceTracker)
	if envBinding.Spec.OutputResourcesTo != nil && len(envBinding.Spec.OutputResourcesTo.Name) != 0 {
		rt.SetName(rtRef.Name)
		err := k8sClient.Delete(ctx, rt)
		return client.IgnoreNotFound(err)
	}

	rtKey := client.ObjectKey{Namespace: rtRef.Namespace, Name: rtRef.Name}
	if err := k8sClient.Get(ctx, rtKey, rt); err != nil {
		return err
	}
	var manifests []*unstructured.Unstructured
	for _, app := range apps {
		for _, obj := range app.ScheduledManifests {
			manifests = append(manifests, obj)
		}
	}
	for _, oldRsc := range rt.Status.TrackedResources {
		isRemoved := true
		for _, newRsc := range manifests {
			if equalMateData(oldRsc, newRsc) {
				isRemoved = false
				break
			}
		}
		if isRemoved {
			if err := deleteOldResource(ctx, k8sClient, oldRsc); err != nil {
				return err
			}
			klog.InfoS("Successfully GC a resource", "name", oldRsc.Name, "apiVersion", oldRsc.APIVersion, "kind", oldRsc.Kind)
		}
	}
	return nil
}

func equalMateData(rscRef corev1.ObjectReference, newRsc *unstructured.Unstructured) bool {
	if rscRef.APIVersion == newRsc.GetAPIVersion() && rscRef.Kind == newRsc.GetKind() &&
		rscRef.Namespace == newRsc.GetNamespace() && rscRef.Name == newRsc.GetName() {
		return true
	}
	return false
}

func deleteOldResource(ctx context.Context, k8sClient client.Client, ref corev1.ObjectReference) error {
	obj := new(unstructured.Unstructured)
	obj.SetAPIVersion(ref.APIVersion)
	obj.SetKind(ref.Kind)
	obj.SetNamespace(ref.Namespace)
	obj.SetName(ref.Name)
	if err := k8sClient.Delete(ctx, obj); err != nil && !kerrors.IsNotFound(err) {
		return errors.Wrapf(err, "cannot delete resource %v", ref)
	}
	return nil
}
