/*
Copyright 2021 The KubeVela Authors.

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

package resourcetracker

import (
	"context"
	"fmt"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

const (
	// Finalizer for resourcetracker to clean up recorded resources
	Finalizer = "resourcetracker.core.oam.dev/finalizer"
)

func getRootResourceTrackerName(app *v1beta1.Application) string {
	return fmt.Sprintf("%s-%s", app.Name, app.Namespace)
}

func getCurrentResourceTrackerName(app *v1beta1.Application) string {
	return fmt.Sprintf("%s-v%d-%s", app.Name, app.GetGeneration(), app.Namespace)
}

func getComponentRevisionResourceTrackerName(app *v1beta1.Application) string {
	return fmt.Sprintf("%s-comp-rev-%s", app.Name, app.Namespace)
}

func createResourceTracker(ctx context.Context, cli client.Client, app *v1beta1.Application, rtName string, rtType v1beta1.ResourceTrackerType) (*v1beta1.ResourceTracker, error) {
	rt := &v1beta1.ResourceTracker{}
	rt.SetName(rtName)
	meta.AddFinalizer(rt, Finalizer)
	meta.AddLabels(rt, map[string]string{
		oam.LabelAppName:      app.Name,
		oam.LabelAppNamespace: app.Namespace,
		oam.LabelAppUID:       string(app.UID),
	})
	if app.Status.LatestRevision != nil {
		meta.AddLabels(rt, map[string]string{oam.LabelAppRevision: app.Status.LatestRevision.Name})
	}
	rt.Spec.Type = rtType
	if rtType == v1beta1.ResourceTrackerTypeVersioned {
		rt.Spec.ApplicationGeneration = app.GetGeneration()
	} else {
		rt.Spec.ApplicationGeneration = 0
	}
	if err := cli.Create(ctx, rt); err != nil {
		return nil, err
	}
	return rt, nil
}

// CreateRootResourceTracker create root resourcetracker for application
func CreateRootResourceTracker(ctx context.Context, cli client.Client, app *v1beta1.Application) (*v1beta1.ResourceTracker, error) {
	return createResourceTracker(ctx, cli, app, getRootResourceTrackerName(app), v1beta1.ResourceTrackerTypeRoot)
}

// CreateCurrentResourceTracker create versioned resourcetracker for the latest generation of application
func CreateCurrentResourceTracker(ctx context.Context, cli client.Client, app *v1beta1.Application) (*v1beta1.ResourceTracker, error) {
	return createResourceTracker(ctx, cli, app, getCurrentResourceTrackerName(app), v1beta1.ResourceTrackerTypeVersioned)
}

// CreateComponentRevisionResourceTracker create resourcetracker to record all component revision for application
func CreateComponentRevisionResourceTracker(ctx context.Context, cli client.Client, app *v1beta1.Application) (*v1beta1.ResourceTracker, error) {
	return createResourceTracker(ctx, cli, app, getComponentRevisionResourceTrackerName(app), v1beta1.ResourceTrackerTypeComponentRevision)
}

// ListApplicationResourceTrackers list resource trackers for application with all historyRTs sorted by version number
func ListApplicationResourceTrackers(ctx context.Context, cli client.Client, app *v1beta1.Application) (rootRT *v1beta1.ResourceTracker, currentRT *v1beta1.ResourceTracker, historyRTs []*v1beta1.ResourceTracker, crRT *v1beta1.ResourceTracker, err error) {
	rts := v1beta1.ResourceTrackerList{}
	if err = cli.List(ctx, &rts, client.MatchingLabels{
		oam.LabelAppName:      app.Name,
		oam.LabelAppNamespace: app.Namespace,
	}); err != nil {
		return nil, nil, nil, nil, err
	}
	for _, rt := range rts.Items {
		if rt.GetLabels() != nil && rt.GetLabels()[oam.LabelAppUID] != "" && rt.GetLabels()[oam.LabelAppUID] != string(app.UID) {
			return nil, nil, nil, nil, fmt.Errorf("resourcetracker %s exists but controlled by another application (uid: %s), this could probably be cased by some mistakes while garbage collecting outdated resource. Please check this resourcetrakcer and delete it manually", rt.Name, rt.GetLabels()[oam.LabelAppUID])
		}
		switch rt.Spec.Type {
		case v1beta1.ResourceTrackerTypeRoot:
			rootRT = rt.DeepCopy()
		case v1beta1.ResourceTrackerTypeVersioned:
			if rt.Spec.ApplicationGeneration == app.GetGeneration() {
				currentRT = rt.DeepCopy()
			} else {
				historyRTs = append(historyRTs, rt.DeepCopy())
			}
		case v1beta1.ResourceTrackerTypeComponentRevision:
			crRT = rt.DeepCopy()
		}
	}
	historyRTs = SortResourceTrackersByVersion(historyRTs, false)
	return rootRT, currentRT, historyRTs, crRT, nil
}

// RecordManifestInResourceTracker records resources in ResourceTracker
func RecordManifestInResourceTracker(ctx context.Context, cli client.Client, rt *v1beta1.ResourceTracker, manifest *unstructured.Unstructured, metaOnly bool) error {
	if err := rt.AddManagedResource(manifest, metaOnly); err != nil {
		return err
	}
	return cli.Update(ctx, rt)
}

// DeletedManifestInResourceTracker marks resources as deleted in resourcetracker, if remove is true, resources will be removed from resourcetracker
func DeletedManifestInResourceTracker(ctx context.Context, cli client.Client, rt *v1beta1.ResourceTracker, manifest *unstructured.Unstructured, remove bool) error {
	rt.DeleteManagedResource(manifest, remove)
	return cli.Update(ctx, rt)
}
