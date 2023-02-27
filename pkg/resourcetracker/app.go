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
	"github.com/kubevela/pkg/controller/sharding"
	"github.com/kubevela/pkg/util/compression"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cache"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/monitor/metrics"
	"github.com/oam-dev/kubevela/pkg/oam"
	velaerrors "github.com/oam-dev/kubevela/pkg/utils/errors"
)

const (
	// Finalizer for resourcetracker to clean up recorded resources
	Finalizer = "resourcetracker.core.oam.dev/finalizer"
)

var (
	applicationResourceTrackerGroupVersionKind = schema.GroupVersionKind{
		Group:   "prism.oam.dev",
		Version: "v1alpha1",
		Kind:    "ApplicationResourceTracker",
	}
)

func getPublishVersion(obj client.Object) string {
	if obj.GetAnnotations() != nil {
		return obj.GetAnnotations()[oam.AnnotationPublishVersion]
	}
	return ""
}

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
		if publishVersion := getPublishVersion(app); publishVersion != "" {
			meta.AddAnnotations(rt, map[string]string{oam.AnnotationPublishVersion: publishVersion})
		}
	} else {
		rt.Spec.ApplicationGeneration = 0
	}
	if utilfeature.DefaultMutableFeatureGate.Enabled(features.GzipResourceTracker) {
		rt.Spec.Compression.Type = compression.Gzip
	}
	// zstd compressor will have higher priority when both gzip and zstd are enabled.
	if utilfeature.DefaultMutableFeatureGate.Enabled(features.ZstdResourceTracker) {
		rt.Spec.Compression.Type = compression.Zstd
	}
	sharding.PropagateScheduledShardIDLabel(app, rt)
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
	if publishVersion := getPublishVersion(app); publishVersion != "" {
		return createResourceTracker(ctx, cli, app, fmt.Sprintf("%s-%s-%s", app.Name, publishVersion, app.Namespace), v1beta1.ResourceTrackerTypeVersioned)
	}
	return createResourceTracker(ctx, cli, app, getCurrentResourceTrackerName(app), v1beta1.ResourceTrackerTypeVersioned)
}

// CreateComponentRevisionResourceTracker create resourcetracker to record all component revision for application
func CreateComponentRevisionResourceTracker(ctx context.Context, cli client.Client, app *v1beta1.Application) (*v1beta1.ResourceTracker, error) {
	return createResourceTracker(ctx, cli, app, getComponentRevisionResourceTrackerName(app), v1beta1.ResourceTrackerTypeComponentRevision)
}

func newResourceTrackerFromApplicationResourceTracker(appRt *unstructured.Unstructured) (*v1beta1.ResourceTracker, error) {
	rt := &v1beta1.ResourceTracker{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(appRt.Object, rt); err != nil {
		return nil, err
	}
	namespace := metav1.NamespaceDefault
	if ns := appRt.GetNamespace(); ns != "" {
		namespace = ns
	}
	rt.SetName(appRt.GetName() + "-" + namespace)
	rt.SetNamespace("")
	rt.SetGroupVersionKind(v1beta1.ResourceTrackerKindVersionKind)
	return rt, nil
}

func listApplicationResourceTrackers(ctx context.Context, cli client.Client, app *v1beta1.Application) ([]v1beta1.ResourceTracker, error) {
	rts := v1beta1.ResourceTrackerList{}
	var err error
	if cache.OptimizeListOp {
		err = cli.List(ctx, &rts, client.MatchingFields{cache.AppIndex: app.Namespace + "/" + app.Name})
	} else {
		err = cli.List(ctx, &rts, client.MatchingLabels{
			oam.LabelAppName:      app.Name,
			oam.LabelAppNamespace: app.Namespace,
		})
	}
	if err == nil {
		return rts.Items, nil
	}
	rtError := err
	if !kerrors.IsForbidden(err) && !kerrors.IsUnauthorized(err) {
		return nil, err
	}
	appRts := &unstructured.UnstructuredList{}
	appRts.SetGroupVersionKind(applicationResourceTrackerGroupVersionKind)
	if err = cli.List(ctx, appRts, client.MatchingLabels{
		oam.LabelAppName: app.Name,
	}, client.InNamespace(app.Namespace)); err != nil {
		if velaerrors.IsCRDNotExists(err) {
			return nil, errors.Wrapf(rtError, "no permission for ResourceTracker and vela-prism is not serving ApplicationResourceTracker")
		}
		return nil, err
	}
	var rtArr []v1beta1.ResourceTracker
	for _, appRt := range appRts.Items {
		rt, err := newResourceTrackerFromApplicationResourceTracker(appRt.DeepCopy())
		if err != nil {
			return nil, err
		}
		rtArr = append(rtArr, *rt)
	}
	return rtArr, nil
}

// ListApplicationResourceTrackers list resource trackers for application with all historyRTs sorted by version number
// rootRT -> The ResourceTracker that records life-long resources. These resources will only be recycled when application is removed.
// currentRT -> The ResourceTracker that tracks the resources used by the latest version of application.
// historyRTs -> The ResourceTrackers that tracks the resources in outdated versions.
// crRT -> The ResourceTracker that tracks the component revisions created by the application.
func ListApplicationResourceTrackers(ctx context.Context, cli client.Client, app *v1beta1.Application) (rootRT *v1beta1.ResourceTracker, currentRT *v1beta1.ResourceTracker, historyRTs []*v1beta1.ResourceTracker, crRT *v1beta1.ResourceTracker, err error) {
	metrics.ListResourceTrackerCounter.WithLabelValues("application").Inc()
	rts, err := listApplicationResourceTrackers(ctx, cli, app)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	for _, _rt := range rts {
		rt := _rt.DeepCopy()
		if rt.GetLabels() != nil && rt.GetLabels()[oam.LabelAppUID] != "" && rt.GetLabels()[oam.LabelAppUID] != string(app.UID) {
			return nil, nil, nil, nil, fmt.Errorf("resourcetracker %s exists but controlled by another application (uid: %s), this could probably be cased by some mistakes while garbage collecting outdated resource. Please check this resourcetrakcer and delete it manually", rt.Name, rt.GetLabels()[oam.LabelAppUID])
		}
		switch rt.Spec.Type {
		case v1beta1.ResourceTrackerTypeRoot:
			rootRT = rt
		case v1beta1.ResourceTrackerTypeVersioned:
			if publishVersion := getPublishVersion(app); publishVersion != "" {
				if getPublishVersion(rt) == publishVersion {
					currentRT = rt
				} else {
					historyRTs = append(historyRTs, rt)
				}
			} else {
				if rt.Spec.ApplicationGeneration == app.GetGeneration() {
					currentRT = rt
				} else {
					historyRTs = append(historyRTs, rt)
				}
			}
		case v1beta1.ResourceTrackerTypeComponentRevision:
			crRT = rt
		}
	}
	historyRTs = SortResourceTrackersByVersion(historyRTs, false)
	if currentRT != nil && len(historyRTs) > 0 && currentRT.Spec.ApplicationGeneration < historyRTs[len(historyRTs)-1].Spec.ApplicationGeneration {
		return nil, nil, nil, nil, fmt.Errorf("current publish version %s(gen-%d) is in-use and outdated, found newer gen-%d", getPublishVersion(app), currentRT.Spec.ApplicationGeneration, historyRTs[len(historyRTs)-1].Spec.ApplicationGeneration)
	}
	return rootRT, currentRT, historyRTs, crRT, nil
}

// RecordManifestsInResourceTracker records resources in ResourceTracker
func RecordManifestsInResourceTracker(
	ctx context.Context,
	cli client.Client,
	rt *v1beta1.ResourceTracker,
	manifests []*unstructured.Unstructured,
	metaOnly bool,
	skipGC bool,
	creator string) error {
	if len(manifests) != 0 {
		updated := false
		for _, manifest := range manifests {
			updated = rt.AddManagedResource(manifest, metaOnly, skipGC, creator) || updated
		}
		if updated {
			return cli.Update(ctx, rt)
		}
	}
	return nil
}

// DeletedManifestInResourceTracker marks resources as deleted in resourcetracker, if remove is true, resources will be removed from resourcetracker
func DeletedManifestInResourceTracker(ctx context.Context, cli client.Client, rt *v1beta1.ResourceTracker, manifest *unstructured.Unstructured, remove bool) error {
	if updated := rt.DeleteManagedResource(manifest, remove); !updated {
		return nil
	}
	return cli.Update(ctx, rt)
}
