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
	"fmt"
	"slices"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/kubevela/pkg/controller/sharding"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
)

// applicationTracked is an Application's resourcetracker.Tracked. Besides the owner.oam.dev/*
// labels every kind carries it has its own: app.oam.dev/* labels, which the renderer puts on
// resources, trackers named <app>[-v<gen>|-<publishVersion>|-comp-rev]-<namespace>, and the
// <namespace>/<name> key sharer annotations use.
//
// Migration: owner labels. Objects written before them carry app.oam.dev/* only, so trackers
// are found by those labels and ownership falls back to them.
type applicationTracked struct {
	*resourcetracker.Base
	app *v1beta1.Application
}

// NewAppResourceTracker returns the resourcetracker.Tracked for an Application.
func NewAppResourceTracker(app *v1beta1.Application) resourcetracker.Tracked {
	return &applicationTracked{
		Base: resourcetracker.NewBase(resourcetracker.Owner{Kind: v1beta1.ApplicationKind, Namespace: app.Namespace, Name: app.Name, UID: app.UID, Generation: app.Generation, Deleting: app.DeletionTimestamp != nil}),
		app:  app,
	}
}

// Key is <namespace>/<name>, the form sharer annotations have always used.
func (a *applicationTracked) Key() string {
	ns := a.app.Namespace
	if ns == "" {
		ns = metav1.NamespaceDefault
	}
	return fmt.Sprintf("%s/%s", ns, a.app.GetName())
}

// Deleting reads the Application, which can change while the keeper holds it.
func (a *applicationTracked) Deleting() bool {
	return a.app.GetDeletionTimestamp() != nil
}

// ControlledBy reads the owner.oam.dev/* labels: an Application as <namespace>/<name>, any
// other kind as <kind>/<namespace>/<name>, so a conflict names it. Migration: owner labels,
// a resource carrying app.oam.dev/* only falls back to those.
func (a *applicationTracked) ControlledBy(obj client.Object) string {
	labels := obj.GetLabels() // an unstructured object copies its labels on every read
	if by := resourcetracker.OwnerKeyFromLabels(labels); by != "" {
		if labels[oam.LabelOwnerKind] == v1beta1.ApplicationKind {
			return labels[oam.LabelOwnerNamespace] + "/" + labels[oam.LabelOwnerName]
		}
		return by
	}
	if resourcetracker.HasAnyOwnerLabel(labels) {
		// Some owner marked this and the set is incomplete. Reading the app labels here would
		// hand another kind's resource to an Application, so treat it as nobody's.
		return ""
	}
	name, ns := labels[oam.LabelAppName], labels[oam.LabelAppNamespace]
	if name == "" || ns == "" {
		return ""
	}
	return ns + "/" + name
}

// Release removes the owner.oam.dev/* labels, and the app.oam.dev/* ones a resource may
// still be carrying instead (Migration: owner labels).
func (a *applicationTracked) Release(obj *unstructured.Unstructured) {
	a.Base.Release(obj)
	if labels := obj.GetLabels(); labels != nil {
		delete(labels, oam.LabelAppName)
		delete(labels, oam.LabelAppNamespace)
		obj.SetLabels(labels)
	}
}

// Orphaning is true while the Application carries the orphan-resource finalizer.
func (a *applicationTracked) Orphaning() bool {
	return slices.Contains(a.app.GetFinalizers(), oam.FinalizerOrphanResource)
}

// TrackerLabels are the app.oam.dev/* labels trackers have always carried, plus the
// owner.oam.dev/* labels every kind's trackers carry.
func (a *applicationTracked) TrackerLabels() map[string]string {
	labels := a.Base.TrackerLabels()
	labels[oam.LabelAppName] = a.app.Name
	labels[oam.LabelAppNamespace] = a.app.Namespace
	labels[oam.LabelAppUID] = string(a.app.UID)
	return labels
}

// NewTracker names and labels an Application tracker.
func (a *applicationTracked) NewTracker(rtType v1beta1.ResourceTrackerType) (*v1beta1.ResourceTracker, error) {
	app := a.app
	publishVersion := getPublishVersion(app)
	rt := &v1beta1.ResourceTracker{}
	switch rtType {
	case v1beta1.ResourceTrackerTypeRoot:
		rt.SetName(getRootResourceTrackerName(app))
	case v1beta1.ResourceTrackerTypeVersioned:
		if publishVersion != "" {
			rt.SetName(fmt.Sprintf("%s-%s-%s", app.Name, publishVersion, app.Namespace))
		} else {
			rt.SetName(getCurrentResourceTrackerName(app))
		}
	case v1beta1.ResourceTrackerTypeComponentRevision:
		rt.SetName(getComponentRevisionResourceTrackerName(app))
	default:
		return nil, errors.Errorf("application %s has no %s resource tracker", a.Key(), rtType)
	}
	if app.Status.LatestRevision != nil { // CreateTracker adds TrackerLabels
		meta.AddLabels(rt, map[string]string{oam.LabelAppRevision: app.Status.LatestRevision.Name})
	}
	rt.Spec.Type = rtType
	if rtType == v1beta1.ResourceTrackerTypeVersioned {
		rt.Spec.ApplicationGeneration = app.GetGeneration()
		if publishVersion != "" {
			meta.AddAnnotations(rt, map[string]string{oam.AnnotationPublishVersion: publishVersion})
		}
	}
	sharding.PropagateScheduledShardIDLabel(app, rt)
	return rt, nil
}

// LoadTrackers lists the Application's trackers by its app.oam.dev/* labels.
func (a *applicationTracked) LoadTrackers(ctx context.Context, cli client.Client) (resourcetracker.Trackers, error) {
	root, current, history, cr, err := ListApplicationResourceTrackers(ctx, cli, a.app)
	return resourcetracker.Trackers{Root: root, Current: current, History: history, ComponentRevision: cr}, err
}
