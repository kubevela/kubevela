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
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	monitorContext "github.com/oam-dev/kubevela/pkg/monitor/context"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/policy/envbinding"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
	errors2 "github.com/oam-dev/kubevela/pkg/utils/errors"
)

func getClustersFromRootResourceTracker(ctx monitorContext.Context, c client.Client, app *v1beta1.Application) []string {
	rt, err := resourcetracker.GetApplicationRootResourceTracker(ctx, c, app)
	if err != nil {
		return nil
	}
	return rt.GetTrackedClusters()
}

func getAppliedClusters(ctx monitorContext.Context, c client.Client, app *v1beta1.Application) []string {
	appliedClusters := map[string]bool{}
	for _, v := range app.Status.AppliedResources {
		appliedClusters[v.Cluster] = true
	}
	status, err := envbinding.GetEnvBindingPolicyStatus(app, "")
	if err != nil {
		// fallback
		ctx.Info("failed to get envbinding policy status during gc", "err", err.Error())
	}
	if status != nil {
		for _, conn := range status.ClusterConnections {
			appliedClusters[conn.ClusterName] = true
		}
	}
	for _, cluster := range getClustersFromRootResourceTracker(ctx, c, app) {
		appliedClusters[cluster] = true
	}
	var clusters []string
	for cluster := range appliedClusters {
		clusters = append(clusters, cluster)
	}
	return clusters
}

// GarbageCollectionForOutdatedResourcesInSubClusters run garbage collection in sub clusters and remove outdated ResourceTrackers with their associated resources
func GarbageCollectionForOutdatedResourcesInSubClusters(ctx monitorContext.Context, c client.Client, app *v1beta1.Application, gcHandler func(monitorContext.Context) error) error {
	var errs errors2.ErrorList
	for _, clusterName := range getAppliedClusters(ctx, c, app) {
		if err := gcHandler(TracerWithClusterName(ctx, clusterName)); err != nil {
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

func garbageCollectResourceTrackers(ctx monitorContext.Context, c client.Client, app *v1beta1.Application, cluster string) error {
	if cluster != "" {
		ctx = TracerWithClusterName(ctx, cluster)
	}
	listOpts := []client.ListOption{
		client.MatchingLabels{
			oam.LabelAppName:      app.Name,
			oam.LabelAppNamespace: app.Namespace,
		}}
	rtList := &v1beta1.ResourceTrackerList{}
	if err := c.List(ctx, rtList, listOpts...); err != nil {
		ctx.Error(err, "failed to list resource tracker of app", "name", app.Name, "cluster", cluster)
		return errors.WithMessage(err, "cannot remove finalizer")
	}
	for _, rt := range rtList.Items {
		if err := c.Delete(ctx, rt.DeepCopy()); err != nil && !kerrors.IsNotFound(err) {
			ctx.Error(err, "failed to delete resource tracker", "name", rt.Name)
			return errors.WithMessage(err, "cannot remove finalizer")
		}
	}
	return nil
}

// GarbageCollectionForAllResourceTrackersInSubCluster run garbage collection in sub clusters and remove all ResourceTrackers
func GarbageCollectionForAllResourceTrackersInSubCluster(ctx monitorContext.Context, c client.Client, app *v1beta1.Application) error {
	// delete subCluster resourceTracker
	for _, cluster := range getAppliedClusters(ctx, c, app) {
		if err := garbageCollectResourceTrackers(ctx, c, app, cluster); err != nil {
			return err
		}
	}
	return nil
}

// GarbageCollectionForAllResourceTrackers run garbage collection in sub clusters and remove all ResourceTrackers, including managed cluster
func GarbageCollectionForAllResourceTrackers(ctx monitorContext.Context, c client.Client, app *v1beta1.Application) error {
	if err := GarbageCollectionForAllResourceTrackersInSubCluster(ctx, c, app); err != nil {
		return err
	}
	return garbageCollectResourceTrackers(ctx, c, app, "")
}
