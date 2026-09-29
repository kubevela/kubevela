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

package resourcekeeper

import (
	"context"
	"fmt"
	"slices"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
)

// appOwner is a test Tracked in the Application's style: key <namespace>/<name>,
// app.oam.dev/* labels, trackers named <name>[-v<gen>]-<namespace>. The library's own tests
// use it to exercise a kind that overrides Base. The real Application kind is pkg/appkeeper's.
type appOwner struct {
	*resourcetracker.Base
	app *v1beta1.Application
}

func newAppOwner(app *v1beta1.Application) resourcetracker.Tracked {
	return &appOwner{Base: resourcetracker.NewBase(resourcetracker.Owner{Kind: "Application", Namespace: app.Namespace, Name: app.Name, UID: app.UID, Generation: app.Generation, Deleting: false}), app: app}
}

func (a *appOwner) Key() string {
	ns := a.app.Namespace
	if ns == "" {
		ns = metav1.NamespaceDefault
	}
	return fmt.Sprintf("%s/%s", ns, a.app.Name)
}

func (a *appOwner) Deleting() bool { return a.app.GetDeletionTimestamp() != nil }

func (a *appOwner) Orphaning() bool {
	return slices.Contains(a.app.GetFinalizers(), oam.FinalizerOrphanResource)
}

func (a *appOwner) ControlledBy(obj client.Object) string {
	name, ns := obj.GetLabels()[oam.LabelAppName], obj.GetLabels()[oam.LabelAppNamespace]
	if name == "" || ns == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s", ns, name)
}

func (a *appOwner) Stamp(*unstructured.Unstructured) {}

func (a *appOwner) Release(obj *unstructured.Unstructured) {
	if labels := obj.GetLabels(); labels != nil {
		delete(labels, oam.LabelAppName)
		delete(labels, oam.LabelAppNamespace)
		obj.SetLabels(labels)
	}
}

func (a *appOwner) labels() map[string]string {
	return map[string]string{oam.LabelAppName: a.app.Name, oam.LabelAppNamespace: a.app.Namespace, oam.LabelAppUID: string(a.app.UID)}
}

func (a *appOwner) NewTracker(rtType v1beta1.ResourceTrackerType) (*v1beta1.ResourceTracker, error) {
	rt := &v1beta1.ResourceTracker{}
	switch rtType {
	case v1beta1.ResourceTrackerTypeRoot:
		rt.SetName(fmt.Sprintf("%s-%s", a.app.Name, a.app.Namespace))
	case v1beta1.ResourceTrackerTypeVersioned:
		rt.SetName(fmt.Sprintf("%s-v%d-%s", a.app.Name, a.app.Generation, a.app.Namespace))
		rt.Spec.ApplicationGeneration = a.app.Generation
	case v1beta1.ResourceTrackerTypeComponentRevision:
		rt.SetName(fmt.Sprintf("%s-comp-rev-%s", a.app.Name, a.app.Namespace))
	default:
		return nil, fmt.Errorf("no %s tracker", rtType)
	}
	meta.AddLabels(rt, a.labels())
	rt.Spec.Type = rtType
	return rt, nil
}

func (a *appOwner) LoadTrackers(ctx context.Context, cli client.Client) (resourcetracker.Trackers, error) {
	rts := v1beta1.ResourceTrackerList{}
	if err := cli.List(ctx, &rts, client.MatchingLabels{oam.LabelAppName: a.app.Name, oam.LabelAppNamespace: a.app.Namespace}); err != nil {
		return resourcetracker.Trackers{}, err
	}
	var t resourcetracker.Trackers
	for _, _rt := range rts.Items {
		rt := _rt.DeepCopy()
		if uid := rt.GetLabels()[oam.LabelAppUID]; uid != "" && uid != string(a.app.UID) {
			return resourcetracker.Trackers{}, fmt.Errorf("resourcetracker %s exists but controlled by another application (uid: %s)", rt.Name, uid)
		}
		switch rt.Spec.Type {
		case v1beta1.ResourceTrackerTypeRoot:
			t.Root = rt
		case v1beta1.ResourceTrackerTypeVersioned:
			if rt.Spec.ApplicationGeneration == a.app.Generation {
				t.Current = rt
			} else {
				t.History = append(t.History, rt)
			}
		case v1beta1.ResourceTrackerTypeComponentRevision:
			t.ComponentRevision = rt
		}
	}
	t.History = resourcetracker.SortResourceTrackersByVersion(t.History, false)
	return t, nil
}

// newAppKeeper is a keeper owned by appOwner(app).
func newAppKeeper(ctx context.Context, cli client.Client, app *v1beta1.Application, policies Policies, opts ...Options) (ResourceKeeper, error) {
	var options Options
	if len(opts) > 0 {
		options = opts[0]
	}
	return New(ctx, cli, newAppOwner(app), policies, options)
}
