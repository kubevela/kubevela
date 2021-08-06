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

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ocmclusterv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	ocmworkv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// ClusterManagerEngine defines Cluster Manage interface
type ClusterManagerEngine interface {
	Prepare(ctx context.Context, configs []v1alpha1.EnvConfig) error
	Schedule(ctx context.Context, apps []*EnvBindApp) error
	GetClusterDecisions() []v1alpha1.ClusterDecision
}

// OCMEngine represents Open-Cluster-Management multi-cluster management solution
type OCMEngine struct {
	cli              client.Client
	clusterDecisions map[string]string
	appNs            string
	appName          string
}

// NewOCMEngine create Open-Cluster-Management ClusterManagerEngine
func NewOCMEngine(cli client.Client, appName, appNs string) ClusterManagerEngine {
	return &OCMEngine{
		cli:     cli,
		appNs:   appNs,
		appName: appName,
	}
}

// Prepare complete the pre-work of cluster scheduling and select the target cluster
// 1) if user directly specify the cluster name, Prepare will do nothing
// 2) if user use Labels to select the target cluster, Prepare will create the Placement to select cluster
func (o *OCMEngine) Prepare(ctx context.Context, configs []v1alpha1.EnvConfig) error {
	var err error
	for _, config := range configs {
		if len(config.Placement.ClusterSelector.Name) != 0 {
			continue
		}
		err = o.DispatchPlacement(ctx, config)
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
		clusterDecisions[config.Name], err = o.GetSelectedCluster(ctx, placementName, o.appNs)
		if err != nil {
			return err
		}
	}
	o.clusterDecisions = clusterDecisions
	return nil
}

// Schedule decides which cluster the apps is scheduled to
func (o *OCMEngine) Schedule(ctx context.Context, apps []*EnvBindApp) error {
	for i := range apps {
		app := apps[i]
		app.ManifestWork = make(map[string]*unstructured.Unstructured, len(app.assembledManifests))
		clusterName := o.clusterDecisions[app.envConfig.Name]
		for componentName, manifest := range app.assembledManifests {
			manifestWork := new(ocmworkv1.ManifestWork)
			manifestWorkName := fmt.Sprintf("%s-%s-%s", app.envConfig.Name, o.appName, componentName)
			manifestWork.SetName(manifestWorkName)
			manifestWork.SetNamespace(clusterName)

			workloads := make([]ocmworkv1.Manifest, len(manifest))
			for j, workload := range manifest {
				workloads[j] = ocmworkv1.Manifest{
					RawExtension: util.Object2RawExtension(workload),
				}
			}

			manifestWork.Spec.Workload.Manifests = workloads
			obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(manifestWork)
			if err != nil {
				return err
			}
			unstructuredManifestWork := &unstructured.Unstructured{
				Object: obj,
			}
			unstructuredManifestWork.SetGroupVersionKind(ocmworkv1.GroupVersion.WithKind(reflect.TypeOf(ocmworkv1.ManifestWork{}).Name()))
			app.ManifestWork[componentName] = unstructuredManifestWork
		}
	}
	return nil
}

// GetClusterDecisions return ClusterDecisions
func (o *OCMEngine) GetClusterDecisions() []v1alpha1.ClusterDecision {
	var clusterDecisions []v1alpha1.ClusterDecision
	for env, cluster := range o.clusterDecisions {
		clusterDecisions = append(clusterDecisions, v1alpha1.ClusterDecision{
			EnvName:     env,
			ClusterName: cluster,
		})
	}
	return clusterDecisions
}

// DispatchPlacement dispatch Placement Object of OCM for cluster selected
func (o *OCMEngine) DispatchPlacement(ctx context.Context, config v1alpha1.EnvConfig) error {
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

// GetSelectedCluster get selected cluster from PlacementDecision
func (o *OCMEngine) GetSelectedCluster(ctx context.Context, name, namespace string) (string, error) {
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
		return clusterName, err
	}
	if len(pdList.Items) < 1 {
		return clusterName, errors.New("fail to get PlacementDecision")
	}

	if len(pdList.Items[0].Status.Decisions) < 1 {
		return clusterName, errors.New("no matched cluster")
	}
	clusterName = pdList.Items[0].Status.Decisions[0].ClusterName
	return clusterName, nil
}

// generatePlacementName generate placementName from app Name and env Name
func generatePlacementName(appName, envName string) string {
	return fmt.Sprintf("%s-%s", appName, envName)
}
