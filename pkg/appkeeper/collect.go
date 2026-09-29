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
	"encoding/json"
	"slices"
	"sort"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cache"
	"github.com/oam-dev/kubevela/pkg/kubeutil"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcekeeper"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
)

// appCollector is the Application's garbage collection beyond the keeper's own: the
// component-revision tracker and ControllerRevisions (both legacy: nothing has written them
// since the rollout API was removed), and ApplicationRevisions. It runs as the keeper's
// Options.Collect.
type appCollector struct {
	cli       client.Client
	app       *v1beta1.Application
	tracked   resourcetracker.Tracked
	requester func(context.Context) context.Context
	observe   func(stage string, took time.Duration)
	// crRT is the component-revision tracker, created on demand when a revision is deleted
	crRT *v1beta1.ResourceTracker
}

func (c *appCollector) asRequester(ctx context.Context) context.Context {
	if c.requester == nil {
		return ctx
	}
	return c.requester(ctx)
}

func (c *appCollector) observeStage(stage string, since time.Time) {
	if c.observe != nil {
		c.observe(stage, time.Since(since))
	}
}

func (c *appCollector) collect(ctx context.Context, s resourcekeeper.CollectState) error {
	c.crRT = s.Trackers.ComponentRevision
	if !s.DisableComponentRevisionGC {
		if err := c.gcComponentRevisionTracker(ctx, s); err != nil {
			return errors.Wrapf(err, "failed to garbage collect component revisions in unused components")
		}
	}
	if !s.DisableRevisionGC {
		if err := c.gcRevisions(ctx, s); err != nil {
			return errors.Wrapf(err, "failed to garbage collect application revision")
		}
	}
	return nil
}

// gcComponentRevisionTracker deletes ControllerRevisions recorded in the component-revision
// tracker whose components are no longer in use.
func (c *appCollector) gcComponentRevisionTracker(ctx context.Context, s resourcekeeper.CollectState) error {
	defer c.observeStage("gc-rt.comp-rev", time.Now())
	if c.crRT == nil {
		return nil
	}
	inUse := s.InUseComponents() // walks the resource cache, so only once this tracker exists
	var managedResources []v1beta1.ManagedResource
	for _, cr := range c.crRT.Spec.ManagedResources { // legacy code for rollout-plan
		_ctx := pkgmulticluster.WithCluster(ctx, cr.Cluster)
		_ctx = c.asRequester(_ctx)
		if !inUse[cr.ComponentKey()] {
			_cr := &appsv1.ControllerRevision{}
			err := c.cli.Get(_ctx, cr.NamespacedName(), _cr)
			if err != nil && !kubeutil.IsNotFoundOrClusterNotExists(err) {
				return errors.Wrapf(err, "failed to get component revision %s", cr.ResourceKey())
			}
			if err == nil {
				if err = c.cli.Delete(_ctx, _cr); err != nil && !kerrors.IsNotFound(err) {
					return errors.Wrapf(err, "failed to delete component revision %s", cr.ResourceKey())
				}
			}
		} else {
			managedResources = append(managedResources, cr)
		}
	}
	c.crRT.Spec.ManagedResources = managedResources
	if len(managedResources) == 0 && c.crRT.GetDeletionTimestamp() != nil {
		meta.RemoveFinalizer(c.crRT, resourcetracker.Finalizer)
	}
	if err := c.cli.Update(ctx, c.crRT); err != nil {
		return errors.Wrapf(err, "failed to update controllerrevision RT %s", c.crRT.Name)
	}
	return nil
}

// gcRevisions cleans up legacy component revisions, then ApplicationRevisions.
func (c *appCollector) gcRevisions(ctx context.Context, s resourcekeeper.CollectState) error {
	defer c.observeStage("gc-app-rev", time.Now())
	if err := c.cleanUpComponentRevision(ctx, s); err != nil {
		return err
	}
	return c.cleanUpApplicationRevision(ctx, s)
}

// cleanUpApplicationRevision check all appRevisions of the application, remove them if the number of them exceed the limit
func (c *appCollector) cleanUpApplicationRevision(ctx context.Context, s resourcekeeper.CollectState) error {
	if s.DisableRevisionGC {
		return nil
	}
	defer c.observeStage("gc-rev.apprev", time.Now())
	sortedRevision, err := getSortedAppRevisions(ctx, c.cli, c.app.Name, c.app.Namespace)
	if err != nil {
		return err
	}
	appRevisionInUse := gatherUsingAppRevision(c.app)
	appRevisionLimit := getApplicationRevisionLimitForApp(c.app, s.RevisionLimit)
	needKill := len(sortedRevision) - appRevisionLimit - len(appRevisionInUse)
	t := s.Trackers
	if t.Root == nil && t.Current == nil && len(t.History) == 0 && t.ComponentRevision == nil && c.app.DeletionTimestamp != nil {
		needKill = len(sortedRevision)
		appRevisionInUse = nil
	}
	if needKill <= 0 {
		return nil
	}
	klog.InfoS("Going to garbage collect app revisions", "limit", s.RevisionLimit,
		"total", len(sortedRevision), "using", len(appRevisionInUse), "kill", needKill)

	for _, rev := range sortedRevision {
		if needKill <= 0 {
			break
		}
		// don't delete app revision in use
		if appRevisionInUse[rev.Name] {
			continue
		}
		if err := c.cli.Delete(ctx, rev.DeepCopy()); err != nil && !kerrors.IsNotFound(err) {
			return err
		}
		needKill--
	}
	return nil
}

func (c *appCollector) cleanUpComponentRevision(ctx context.Context, s resourcekeeper.CollectState) error {
	if s.DisableComponentRevisionGC {
		return nil
	}
	defer c.observeStage("gc-rev.comprev", time.Now())
	// collect component revision in use
	compRevisionInUse := map[string]map[string]struct{}{}
	ctx = c.asRequester(ctx)
	for i, resource := range c.app.Status.AppliedResources {
		compName := resource.Name
		ns := resource.Namespace
		r := &unstructured.Unstructured{}
		r.GetObjectKind().SetGroupVersionKind(resource.GroupVersionKind())
		_ctx := pkgmulticluster.WithCluster(ctx, resource.Cluster)
		err := c.cli.Get(_ctx, ktypes.NamespacedName{Name: compName, Namespace: ns}, r)
		notFound := kerrors.IsNotFound(err)
		if err != nil && !notFound {
			return errors.WithMessagef(err, "get applied resource index=%d", i)
		}
		if compRevisionInUse[compName] == nil {
			compRevisionInUse[compName] = map[string]struct{}{}
		}
		if notFound {
			continue
		}
		compRevision, ok := r.GetLabels()[oam.LabelAppComponentRevision]
		if ok {
			compRevisionInUse[compName][compRevision] = struct{}{}
		}
	}

	for _, curComp := range c.app.Status.AppliedResources {
		crList := &appsv1.ControllerRevisionList{}
		listOpts := []client.ListOption{client.MatchingLabels{
			oam.LabelControllerRevisionComponent: kubeutil.EscapeResourceNameToLabelValue(curComp.Name),
		}, client.InNamespace(c.getComponentRevisionNamespace(ctx))}
		_ctx := pkgmulticluster.WithCluster(ctx, curComp.Cluster)
		if err := c.cli.List(_ctx, crList, listOpts...); err != nil {
			return err
		}
		needKill := len(crList.Items) - s.RevisionLimit - len(compRevisionInUse[curComp.Name])
		if needKill < 1 {
			continue
		}
		sortedRevision := crList.Items
		sort.Sort(historiesByComponentRevision(sortedRevision))
		for _, rev := range sortedRevision {
			if needKill <= 0 {
				break
			}
			if _, inUse := compRevisionInUse[curComp.Name][rev.Name]; inUse {
				continue
			}
			_rev := rev.DeepCopy()
			oam.SetCluster(_rev, curComp.Cluster)
			if err := c.deleteComponentRevision(_ctx, _rev); err != nil {
				return err
			}
			needKill--
		}
	}
	return nil
}

// deleteComponentRevision deletes a component revision and removes its record from the
// component-revision tracker.
func (c *appCollector) deleteComponentRevision(ctx context.Context, cr *appsv1.ControllerRevision) error {
	if c.crRT == nil {
		rt, err := resourcetracker.CreateTracker(pkgmulticluster.WithCluster(ctx, pkgmulticluster.Local), c.cli, c.tracked, v1beta1.ResourceTrackerTypeComponentRevision)
		if err != nil {
			return errors.Wrapf(err, "failed to get resourcetracker")
		}
		c.crRT = rt
	}
	obj := &unstructured.Unstructured{}
	obj.SetName(cr.Name)
	obj.SetNamespace(cr.Namespace)
	obj.SetLabels(cr.Labels)
	if err := c.cli.Delete(c.asRequester(pkgmulticluster.WithCluster(ctx, oam.GetCluster(cr))), cr); err != nil && !kerrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to delete componentrevision %s/%s/%s", oam.GetCluster(cr), cr.Namespace, cr.Name)
	}
	if err := resourcetracker.DeletedManifestInResourceTracker(pkgmulticluster.WithCluster(ctx, pkgmulticluster.Local), c.cli, c.crRT, obj, true); err != nil {
		return errors.Wrapf(err, "failed to componentrevision resourcetracker record %s/%s/%s", oam.GetCluster(cr), cr.Namespace, cr.Name)
	}
	return nil
}

func (c *appCollector) getComponentRevisionNamespace(ctx context.Context) string {
	if ns, ok := ctx.Value(0).(string); ok && ns != "" {
		return ns
	}
	return c.app.Namespace
}

// gatherUsingAppRevision get all using appRevisions include app's status pointing to
func gatherUsingAppRevision(app *v1beta1.Application) map[string]bool {
	usingRevision := map[string]bool{}
	if app.Status.LatestRevision != nil && len(app.Status.LatestRevision.Name) != 0 {
		usingRevision[app.Status.LatestRevision.Name] = true
	}
	return usingRevision
}

func getApplicationRevisionLimitForApp(app *v1beta1.Application, fallback int) int {
	for _, p := range app.Spec.Policies {
		if p.Type == v1alpha1.GarbageCollectPolicyType && p.Properties != nil && p.Properties.Raw != nil {
			prop := &v1alpha1.GarbageCollectPolicySpec{}
			if err := json.Unmarshal(p.Properties.Raw, prop); err == nil && prop.ApplicationRevisionLimit != nil && *prop.ApplicationRevisionLimit >= 0 {
				return *prop.ApplicationRevisionLimit
			}
		}
	}
	return fallback
}

// getSortedAppRevisions get application revisions by revision number
func getSortedAppRevisions(ctx context.Context, cli client.Client, appName string, appNs string) ([]v1beta1.ApplicationRevision, error) {
	revs, err := ListApplicationRevisions(ctx, cli, appName, appNs)
	if err != nil {
		return nil, err
	}
	sort.Slice(revs, func(i, j int) bool {
		ir, _ := kubeutil.ExtractRevisionNum(revs[i].Name, "-")
		ij, _ := kubeutil.ExtractRevisionNum(revs[j].Name, "-")
		return ir < ij
	})
	return revs, nil
}

// ListApplicationRevisions get application revisions by label
func ListApplicationRevisions(ctx context.Context, cli client.Client, appName string, appNs string) ([]v1beta1.ApplicationRevision, error) {
	appRevisionList := new(v1beta1.ApplicationRevisionList)
	var err error
	if cache.OptimizeListOp {
		err = cli.List(ctx, appRevisionList, client.MatchingFields{cache.AppIndex: appNs + "/" + appName})
	} else {
		err = cli.List(ctx, appRevisionList, client.InNamespace(appNs), client.MatchingLabels{oam.LabelAppName: appName})
	}
	if err != nil {
		return nil, err
	}
	return appRevisionList.Items, nil
}

type historiesByComponentRevision []appsv1.ControllerRevision

func (h historiesByComponentRevision) Len() int      { return len(h) }
func (h historiesByComponentRevision) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h historiesByComponentRevision) Less(i, j int) bool {
	ir, _ := kubeutil.ExtractRevisionNum(h[i].Name, "-")
	ij, _ := kubeutil.ExtractRevisionNum(h[j].Name, "-")
	return ir < ij
}

// applicationDependents returns the components that depend on a component of app: those
// that declare dependsOn it, and those whose inputs come from its outputs.
func applicationDependents(app *v1beta1.Application) func(component string) []string {
	return func(component string) []string {
		dependent := make([]string, 0)
		outputs := make([]string, 0)
		for _, comp := range app.Spec.Components {
			if comp.Name == component {
				for _, output := range comp.Outputs {
					outputs = append(outputs, output.Name)
				}
			} else {
				for _, dependsOn := range comp.DependsOn {
					if dependsOn == component {
						dependent = append(dependent, comp.Name)
						break
					}
				}
			}
		}
		for _, comp := range app.Spec.Components {
			for _, input := range comp.Inputs {
				if slices.Contains(outputs, input.From) {
					dependent = append(dependent, comp.Name)
				}
			}
		}
		return dependent
	}
}
