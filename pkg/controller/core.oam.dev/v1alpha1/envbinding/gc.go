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

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	errors2 "github.com/oam-dev/kubevela/pkg/utils/errors"
)

func isEnvBindingPolicy(policy *unstructured.Unstructured) bool {
	policyKindAPIVersion := policy.GetKind() + "." + policy.GetAPIVersion()
	return policyKindAPIVersion == v1alpha1.EnvBindingKindAPIVersion
}

// GarbageCollectionForOutdatedResourceInSubClusters run garbage collection in sub clusters and remove outdated ResourceTrackers with their associated resources
func GarbageCollectionForOutdatedResourceInSubClusters(ctx context.Context, c client.Client, policies []*unstructured.Unstructured, gcHandler func(context.Context) error) error {
	subClusters := make(map[string]bool)
	for _, raw := range policies {
		if !isEnvBindingPolicy(raw) {
			continue
		}
		policy := &v1alpha1.EnvBinding{}
		if err := c.Get(ctx, types.NamespacedName{Namespace: raw.GetNamespace(), Name: raw.GetName()}, policy); err != nil {
			klog.Infof("failed to run gc for envBinding subClusters: %v", err)
		}
		if policy.Status.ClusterDecisions == nil {
			continue
		}
		for _, decision := range policy.Status.ClusterDecisions {
			subClusters[decision.Cluster] = true
		}
	}
	var errs errors2.ErrorList
	for clusterName := range subClusters {
		if err := gcHandler(multicluster.ContextWithClusterName(ctx, clusterName)); err != nil {
			errs.Append(errors.Wrapf(err, "failed to run gc in subCluster %s", clusterName))
		}
	}
	if errs.HasError() {
		return errs
	}
	return nil
}

// GarbageCollectionForAllResourceTrackersInSubCluster run garbage collection in sub clusters and remove all ResourceTrackers for the EnvBinding
func GarbageCollectionForAllResourceTrackersInSubCluster(ctx context.Context, c client.Client, envBinding *v1alpha1.EnvBinding) error {
	baseApp, err := util.RawExtension2Application(envBinding.Spec.AppTemplate.RawExtension)
	if err != nil {
		klog.ErrorS(err, "Failed to parse AppTemplate of EnvBinding")
		return errors.WithMessage(err, "cannot remove finalizer")
	}
	// delete subCluster resourceTracker
	for _, decision := range envBinding.Status.ClusterDecisions {
		subCtx := multicluster.ContextWithClusterName(ctx, decision.Cluster)
		listOpts := []client.ListOption{
			client.MatchingLabels{
				oam.LabelAppName:      baseApp.Name,
				oam.LabelAppNamespace: baseApp.Namespace,
			}}
		rtList := &v1beta1.ResourceTrackerList{}
		if err := c.List(subCtx, rtList, listOpts...); err != nil {
			klog.ErrorS(err, "Failed to list resource tracker of app", "name", baseApp.Name, "env", decision.Env)
			return errors.WithMessage(err, "cannot remove finalizer")
		}
		for _, rt := range rtList.Items {
			if err := c.Delete(subCtx, rt.DeepCopy()); err != nil && !kerrors.IsNotFound(err) {
				klog.ErrorS(err, "Failed to delete resource tracker", "name", rt.Name)
				return errors.WithMessage(err, "cannot remove finalizer")
			}
		}
	}
	return nil
}
