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

package resourcekeeper

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/cache"
	"github.com/oam-dev/kubevela/pkg/monitor/metrics"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils"
)

// GarbageCollectApplicationRevision execute garbage collection functions, including:
// - clean up app revisions
// - clean up legacy component revisions
func (h *gcHandler) GarbageCollectApplicationRevision(ctx context.Context) error {
	t := time.Now()
	defer func() {
		metrics.AppReconcileStageDurationHistogram.WithLabelValues("gc-app-rev").Observe(time.Since(t).Seconds())
	}()
	if err := cleanUpComponentRevision(ctx, h); err != nil {
		return err
	}
	return cleanUpApplicationRevision(ctx, h)
}

// cleanUpApplicationRevision check all appRevisions of the application, remove them if the number of them exceed the limit
func cleanUpApplicationRevision(ctx context.Context, h *gcHandler) error {
	if h.cfg.disableApplicationRevisionGC {
		return nil
	}
	t := time.Now()
	defer func() {
		metrics.AppReconcileStageDurationHistogram.WithLabelValues("gc-rev.apprev").Observe(time.Since(t).Seconds())
	}()
	sortedRevision, err := getSortedAppRevisions(ctx, h.Client, h.app.Name, h.app.Namespace)
	if err != nil {
		return err
	}
	appRevisionInUse := gatherUsingAppRevision(h.app)
	appRevisionLimit := getApplicationRevisionLimitForApp(h.app, h.cfg.appRevisionLimit)
	needKill := len(sortedRevision) - appRevisionLimit - len(appRevisionInUse)
	if h._rootRT == nil && h._currentRT == nil && len(h._historyRTs) == 0 && h._crRT == nil && h.app.DeletionTimestamp != nil {
		needKill = len(sortedRevision)
		appRevisionInUse = nil
	}
	if needKill <= 0 {
		return nil
	}
	klog.InfoS("Going to garbage collect app revisions", "limit", h.cfg.appRevisionLimit,
		"total", len(sortedRevision), "using", len(appRevisionInUse), "kill", needKill)

	for _, rev := range sortedRevision {
		if needKill <= 0 {
			break
		}
		// don't delete app revision in use
		if appRevisionInUse[rev.Name] {
			continue
		}
		if err := h.Client.Delete(ctx, rev.DeepCopy()); err != nil && !kerrors.IsNotFound(err) {
			return err
		}
		needKill--
	}
	return nil
}

func cleanUpComponentRevision(ctx context.Context, h *gcHandler) error {
	if h.cfg.disableComponentRevisionGC {
		return nil
	}
	t := time.Now()
	defer func() {
		metrics.AppReconcileStageDurationHistogram.WithLabelValues("gc-rev.comprev").Observe(time.Since(t).Seconds())
	}()
	// collect component revision in use
	compRevisionInUse := map[string]map[string]struct{}{}
	ctx = auth.ContextWithUserInfo(ctx, h.app)
	for i, resource := range h.app.Status.AppliedResources {
		compName := resource.Name
		ns := resource.Namespace
		r := &unstructured.Unstructured{}
		r.GetObjectKind().SetGroupVersionKind(resource.GroupVersionKind())
		_ctx := multicluster.ContextWithClusterName(ctx, resource.Cluster)
		err := h.Client.Get(_ctx, ktypes.NamespacedName{Name: compName, Namespace: ns}, r)
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

	for _, curComp := range h.app.Status.AppliedResources {
		crList := &appsv1.ControllerRevisionList{}
		listOpts := []client.ListOption{client.MatchingLabels{
			oam.LabelControllerRevisionComponent: utils.EscapeResourceNameToLabelValue(curComp.Name),
		}, client.InNamespace(h.getComponentRevisionNamespace(ctx))}
		_ctx := multicluster.ContextWithClusterName(ctx, curComp.Cluster)
		if err := h.Client.List(_ctx, crList, listOpts...); err != nil {
			return err
		}
		needKill := len(crList.Items) - h.cfg.appRevisionLimit - len(compRevisionInUse[curComp.Name])
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
			if err := h.resourceKeeper.DeleteComponentRevision(_ctx, _rev); err != nil {
				return err
			}
			needKill--
		}
	}
	return nil
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
		ir, _ := util.ExtractRevisionNum(revs[i].Name, "-")
		ij, _ := util.ExtractRevisionNum(revs[j].Name, "-")
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

func (h *gcHandler) getComponentRevisionNamespace(ctx context.Context) string {
	if ns, ok := ctx.Value(0).(string); ok && ns != "" {
		return ns
	}
	return h.app.Namespace
}

type historiesByComponentRevision []appsv1.ControllerRevision

func (h historiesByComponentRevision) Len() int      { return len(h) }
func (h historiesByComponentRevision) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h historiesByComponentRevision) Less(i, j int) bool {
	ir, _ := util.ExtractRevisionNum(h[i].Name, "-")
	ij, _ := util.ExtractRevisionNum(h[j].Name, "-")
	return ir < ij
}
