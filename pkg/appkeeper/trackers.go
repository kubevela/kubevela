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

package appkeeper

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cache"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
	velaerrors "github.com/oam-dev/kubevela/pkg/utils/errors"
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
	// This is a projection, not the tracker: its name is rebuilt above, and the caller reads
	// these because it may not read trackers at all. Clearing the resource version marks it as
	// not read from the API server, which is what the keeper checks before writing a tracker.
	rt.SetResourceVersion("")
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
		return nil, errors.WithMessage(err, "failed to list ResourceTrackers")
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
	resourcetracker.OnList(v1beta1.ApplicationKind)
	rts, err := listApplicationResourceTrackers(ctx, cli, app)
	if err != nil {
		return nil, nil, nil, nil, errors.WithMessage(err, "failed to list ResourceTrackers")
	}
	for _, _rt := range rts {
		rt := _rt.DeepCopy()
		if uid := rt.GetLabels()[oam.LabelAppUID]; uid != "" && uid != string(app.UID) {
			return nil, nil, nil, nil, foreignTrackerError(rt.Name, uid, app)
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
	historyRTs = resourcetracker.SortResourceTrackersByVersion(historyRTs, false)
	if currentRT != nil && len(historyRTs) > 0 && currentRT.Spec.ApplicationGeneration < historyRTs[len(historyRTs)-1].Spec.ApplicationGeneration {
		return nil, nil, nil, nil, fmt.Errorf("current publish version %s(gen-%d) is in-use and outdated, found newer gen-%d", getPublishVersion(app), currentRT.Spec.ApplicationGeneration, historyRTs[len(historyRTs)-1].Spec.ApplicationGeneration)
	}
	return rootRT, currentRT, historyRTs, crRT, nil
}

// foreignTrackerError is the refusal of a ResourceTracker recorded for another Application of
// the same name. It is not adopted on its own: a tracker left by a deleted Application looks the
// same as one restored for this one, and adopting it would garbage-collect resources this one does
// not render. One without the uid label is adopted, so removing the label is the remedy for a
// restore. The finalizer is only removed by its own Application, so a plain delete would hang.
func foreignTrackerError(rtName, uid string, app *v1beta1.Application) error {
	return fmt.Errorf("ResourceTracker %s belongs to an earlier Application %s/%s (uid %s), not this one (uid %s). "+
		"This follows a restore that included ResourceTrackers, or an Application deleted and created again before its "+
		"resources were cleaned up. %s",
		rtName, app.Namespace, app.Name, uid, app.UID, ForeignTrackerRemedy(rtName))
}

// ForeignTrackerRemedy is what to do with an earlier Application's ResourceTracker rtName: the
// controller and the Application webhook both name it, in these words.
func ForeignTrackerRemedy(rtName string) string {
	return fmt.Sprintf("For this Application to take it over with its resources (a restore): "+
		"kubectl label resourcetracker %s app.oam.dev/uid-. Otherwise remove it, leaving its resources in place: "+
		`kubectl patch resourcetracker %s --type merge -p '{"metadata":{"finalizers":null}}' && kubectl delete resourcetracker %s`,
		rtName, rtName, rtName)
}
