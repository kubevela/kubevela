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

package multicluster

import (
	"context"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	errors2 "github.com/oam-dev/kubevela/pkg/utils/errors"
)

func getAppliedClusters(app *v1beta1.Application) []string {
	appliedClusters := map[string]bool{}
	for _, v := range app.Status.AppliedResources {
		appliedClusters[v.Cluster] = true
	}
	var clusters []string
	for cluster := range appliedClusters {
		clusters = append(clusters, cluster)
	}
	return clusters
}

// GarbageCollectionForOutdatedResourcesInSubClusters run garbage collection in sub clusters and remove outdated ResourceTrackers with their associated resources
func GarbageCollectionForOutdatedResourcesInSubClusters(ctx context.Context, app *v1beta1.Application, gcHandler func(context.Context) error) error {
	var errs errors2.ErrorList
	for _, clusterName := range getAppliedClusters(app) {
		if err := gcHandler(ContextWithClusterName(ctx, clusterName)); err != nil {
			if !errors.As(err, &errors2.ResourceTrackerNotExistError{}) {
				errs.Append(errors.Wrapf(err, "failed to run gc in subCluster %s", clusterName))
			}
		}
	}
	if errs.HasError() {
		return errs
	}
	return nil
}

// GarbageCollectionForAllResourceTrackersInSubCluster run garbage collection in sub clusters and remove all ResourceTrackers for the EnvBinding
func GarbageCollectionForAllResourceTrackersInSubCluster(ctx context.Context, c client.Client, app *v1beta1.Application) error {
	// delete subCluster resourceTracker
	for _, cluster := range getAppliedClusters(app) {
		subCtx := ContextWithClusterName(ctx, cluster)
		listOpts := []client.ListOption{
			client.MatchingLabels{
				oam.LabelAppName:      app.Name,
				oam.LabelAppNamespace: app.Namespace,
			}}
		rtList := &v1beta1.ResourceTrackerList{}
		if err := c.List(subCtx, rtList, listOpts...); err != nil {
			klog.ErrorS(err, "failed to list resource tracker of app", "name", app.Name, "cluster", cluster)
			return errors.WithMessage(err, "cannot remove finalizer")
		}
		for _, rt := range rtList.Items {
			if err := c.Delete(subCtx, rt.DeepCopy()); err != nil && !kerrors.IsNotFound(err) {
				klog.ErrorS(err, "failed to delete resource tracker", "name", rt.Name)
				return errors.WithMessage(err, "cannot remove finalizer")
			}
		}
	}
	return nil
}
