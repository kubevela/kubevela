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

package application

import (
	"context"
	"errors"
	"testing"

	monitorContext "github.com/kubevela/pkg/monitor/context"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"cuelang.org/go/cue"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func deployObj(ns, name string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetAPIVersion("apps/v1")
	u.SetKind("Deployment")
	u.SetNamespace(ns)
	u.SetName(name)
	return u
}

func traitObj(ns, name string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetAPIVersion("v1")
	u.SetKind("Service")
	u.SetNamespace(ns)
	u.SetName(name)
	return u
}

// appWithService is an Application with one placed, source-reading component.
func appWithService(comp, ns string) (*v1beta1.Application, map[string]common.ApplicationComponent) {
	app := &v1beta1.Application{}
	app.Name = "demo"
	app.Namespace = ns
	app.Status.Services = []common.ApplicationComponentStatus{{Name: comp, Namespace: ns}}
	return app, map[string]common.ApplicationComponent{comp: {Name: comp, Type: "webservice"}}
}

// A component whose re-apply returns nothing at all has told us nothing about
// what it renders, and must not be pruned against.
//
// applyComponentFunc returns (nil, nil, false, nil) when the dispatcher finds
// the component unhealthy, and a workload created moments earlier is always
// unhealthy - zero replicas are ready yet. Treating that as "renders nothing"
// pruned the workload the same reconcile that created it, which made the
// component unhealthy for good, which made the next reconcile do it again. The
// Deployment disappeared and never came back.
func TestRenderedForPruneKeepsAComponentThatReportedNothing(t *testing.T) {
	app, compByName := appWithService("web2", "demo-ns")
	logCtx := monitorContext.NewTraceContext(context.Background(), "test")

	apply := func(_ context.Context, _ common.ApplicationComponent, _ *cue.Value, _ string, _ string) (*unstructured.Unstructured, []*unstructured.Unstructured, bool, error) {
		// Exactly what the unhealthy-dispatcher path returns: no error.
		return nil, nil, false, nil
	}

	rendered, incomplete := renderedForPrune(logCtx, app, compByName, apply,
		func(string, string, error) { t.Fatal("no error was reported, so onErr must not fire") })

	require.Contains(t, incomplete, "web2",
		"a component that reported nothing must be marked incomplete")
	require.NotContains(t, rendered, "web2",
		"and must not appear in the rendered set, or the prune runs against an empty keep list")
}

// The existing guard: a failed apply is incomplete too.
func TestRenderedForPruneKeepsAComponentThatFailed(t *testing.T) {
	app, compByName := appWithService("web2", "demo-ns")
	logCtx := monitorContext.NewTraceContext(context.Background(), "test")

	reported := 0
	apply := func(_ context.Context, _ common.ApplicationComponent, _ *cue.Value, _ string, _ string) (*unstructured.Unstructured, []*unstructured.Unstructured, bool, error) {
		return nil, nil, false, errors.New("render blew up")
	}

	rendered, incomplete := renderedForPrune(logCtx, app, compByName, apply,
		func(string, string, error) { reported++ })

	require.Contains(t, incomplete, "web2")
	require.NotContains(t, rendered, "web2")
	require.Equal(t, 1, reported, "a failure has to be surfaced, not swallowed")
}

// A healthy apply reports its workload and traits, and they are what the prune
// keeps. This is the case that must keep working: the guard must not make every
// component un-prunable.
func TestRenderedForPruneCollectsWhatAComponentRenders(t *testing.T) {
	app, compByName := appWithService("web2", "demo-ns")
	logCtx := monitorContext.NewTraceContext(context.Background(), "test")

	apply := func(_ context.Context, _ common.ApplicationComponent, _ *cue.Value, _ string, _ string) (*unstructured.Unstructured, []*unstructured.Unstructured, bool, error) {
		return deployObj("demo-ns", "web2"), []*unstructured.Unstructured{traitObj("demo-ns", "web2")}, true, nil
	}

	rendered, incomplete := renderedForPrune(logCtx, app, compByName, apply,
		func(string, string, error) { t.Fatal("unexpected error report") })

	require.Empty(t, incomplete)
	require.Len(t, rendered["web2"], 2, "the workload and its trait are both kept")
}

// A component that skips its workload still reports its traits, and those are a
// complete answer - it is prunable on them.
func TestRenderedForPruneAcceptsATraitOnlyComponent(t *testing.T) {
	app, compByName := appWithService("web2", "demo-ns")
	logCtx := monitorContext.NewTraceContext(context.Background(), "test")

	apply := func(_ context.Context, _ common.ApplicationComponent, _ *cue.Value, _ string, _ string) (*unstructured.Unstructured, []*unstructured.Unstructured, bool, error) {
		return nil, []*unstructured.Unstructured{traitObj("demo-ns", "web2")}, true, nil
	}

	rendered, incomplete := renderedForPrune(logCtx, app, compByName, apply,
		func(string, string, error) { t.Fatal("unexpected error report") })

	require.Empty(t, incomplete, "skipping the workload is not an incomplete report")
	require.Len(t, rendered["web2"], 1)
}

// A component placed in two clusters is judged on the union, so one placement
// reporting nothing cannot strand the other's resources.
func TestRenderedForPruneJudgesEveryPlacement(t *testing.T) {
	app, compByName := appWithService("web2", "demo-ns")
	app.Status.Services = append(app.Status.Services,
		common.ApplicationComponentStatus{Name: "web2", Namespace: "demo-ns", Cluster: "remote"})
	logCtx := monitorContext.NewTraceContext(context.Background(), "test")

	apply := func(_ context.Context, _ common.ApplicationComponent, _ *cue.Value, cluster string, _ string) (*unstructured.Unstructured, []*unstructured.Unstructured, bool, error) {
		if cluster == "remote" {
			return nil, nil, false, nil
		}
		return deployObj("demo-ns", "web2"), nil, true, nil
	}

	_, incomplete := renderedForPrune(logCtx, app, compByName, apply,
		func(string, string, error) { t.Fatal("unexpected error report") })

	require.Contains(t, incomplete, "web2",
		"one placement reporting nothing makes the whole component un-prunable")
}

// Declining to prune a component that reported nothing does not strand its
// resources, because nothing else relies on this prune to shed them.
//
// A component reaches "renders nothing" only by skipping its workload, which
// only a manageWorkload trait does, which is a change to the Application spec -
// and a spec change mints a revision, so revision garbage collection retires the
// tracker. This prune exists for the case that mints no revision: a rendered set
// that shrank because a source value changed, most visibly when that value feeds
// a resource name. That case always reports a workload, so the guard never sees
// it.
func TestRenderedForPruneStillReapsASourceDrivenRename(t *testing.T) {
	app, compByName := appWithService("web2", "demo-ns")
	logCtx := monitorContext.NewTraceContext(context.Background(), "test")

	// The source value now names "cfg-silver"; "cfg-gold" is what it used to
	// render and is what the prune has to reap.
	apply := func(_ context.Context, _ common.ApplicationComponent, _ *cue.Value, _ string, _ string) (*unstructured.Unstructured, []*unstructured.Unstructured, bool, error) {
		return deployObj("demo-ns", "web2"),
			[]*unstructured.Unstructured{traitObj("demo-ns", "cfg-silver")}, true, nil
	}

	rendered, incomplete := renderedForPrune(logCtx, app, compByName, apply,
		func(string, string, error) { t.Fatal("unexpected error report") })

	require.Empty(t, incomplete, "a component reporting a workload is prunable")
	require.Len(t, rendered["web2"], 2)

	var names []string
	for _, u := range rendered["web2"] {
		names = append(names, u.GetName())
	}
	require.Contains(t, names, "cfg-silver")
	require.NotContains(t, names, "cfg-gold",
		"the old name is absent from the keep set, so the prune reaps it")
}
