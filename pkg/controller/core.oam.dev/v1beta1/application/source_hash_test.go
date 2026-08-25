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
	"fmt"
	"testing"

	"github.com/oam-dev/kubevela/pkg/sources"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

var cmGVK = schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}

func compWithConsumed(consumed map[string]map[string]interface{}) *appfile.Component {
	comp := &appfile.Component{Name: "web"}
	comp.Ctx = appfile.NewBasicContext(velaprocess.ContextData{
		Namespace: "default",
		AppName:   "app",
		CompName:  "web",
	}, nil)
	if consumed != nil {
		statuses := map[string]sources.SourceResolutionStatus{}
		for name, fields := range consumed {
			statuses[name] = sources.SourceResolutionStatus{Name: name, ConsumedFields: fields}
		}
		comp.Ctx.PushData(sources.SourceResolutionStatusKey, statuses)
	}
	return comp
}

func TestResolvedSourceHashes(t *testing.T) {
	t.Run("no source context -> not consumed", func(t *testing.T) {
		_, ok := resolvedSourceHashes(&appfile.Component{Name: "web"})
		assert.False(t, ok)
	})

	t.Run("statuses present but nothing consumed -> not consumed", func(t *testing.T) {
		_, ok := resolvedSourceHashes(compWithConsumed(map[string]map[string]interface{}{"rng": {}}))
		assert.False(t, ok)
	})

	t.Run("per-source hashes are stable and value-sensitive", func(t *testing.T) {
		h1, ok := resolvedSourceHashes(compWithConsumed(map[string]map[string]interface{}{
			"rng": {"value": 3}, "tenant": {"name": "acme"},
		}))
		assert.True(t, ok)
		assert.Len(t, h1, 2)

		// Same inputs -> identical per-source hashes.
		h2, _ := resolvedSourceHashes(compWithConsumed(map[string]map[string]interface{}{
			"rng": {"value": 3}, "tenant": {"name": "acme"},
		}))
		assert.Equal(t, h1, h2)

		// Change only rng -> only rng's hash changes.
		h3, _ := resolvedSourceHashes(compWithConsumed(map[string]map[string]interface{}{
			"rng": {"value": 4}, "tenant": {"name": "acme"},
		}))
		assert.NotEqual(t, h1["rng"], h3["rng"])
		assert.Equal(t, h1["tenant"], h3["tenant"])
	})
}

func TestChangedSources(t *testing.T) {
	current := map[string]string{"rng": "a", "tenant": "b"}
	assert.ElementsMatch(t, []string{}, changedSources(current, map[string]string{"rng": "a", "tenant": "b"}))
	assert.ElementsMatch(t, []string{"rng"}, changedSources(current, map[string]string{"rng": "x", "tenant": "b"}))
	// Missing from live (first apply / new source) counts as changed.
	assert.ElementsMatch(t, []string{"rng", "tenant"}, changedSources(current, nil))
}

func TestStampAndLiveResolvedSourceHashes(t *testing.T) {
	wl := &unstructured.Unstructured{}
	wl.SetGroupVersionKind(cmGVK)
	wl.SetNamespace("default")
	wl.SetName("web")
	stampResolvedSourceHashes(wl, map[string]string{"rng": "aaa", "tenant": "bbb"})
	assert.NotEmpty(t, wl.GetAnnotations()[oam.AnnotationSourceResolvedHash])

	// no-op cases
	stampResolvedSourceHashes(nil, map[string]string{"x": "y"})
	wl2 := &unstructured.Unstructured{}
	stampResolvedSourceHashes(wl2, nil)
	assert.Empty(t, wl2.GetAnnotations()[oam.AnnotationSourceResolvedHash])

	t.Run("live read round-trips the stamped map", func(t *testing.T) {
		cli := fake.NewClientBuilder().WithObjects(wl.DeepCopy()).Build()
		desired := &unstructured.Unstructured{}
		desired.SetGroupVersionKind(cmGVK)
		desired.SetNamespace("default")
		desired.SetName("web")
		got := liveResolvedSourceHashes(context.Background(), cli, "", desired)
		assert.Equal(t, map[string]string{"rng": "aaa", "tenant": "bbb"}, got)
	})

	t.Run("absent workload -> nil (everything counts as changed)", func(t *testing.T) {
		cli := fake.NewClientBuilder().Build()
		desired := &unstructured.Unstructured{}
		desired.SetGroupVersionKind(cmGVK)
		desired.SetNamespace("default")
		desired.SetName("missing")
		assert.Nil(t, liveResolvedSourceHashes(context.Background(), cli, "", desired))
	})
}

func boolPtr(b bool) *bool { return &b }

// appWithSources builds an Application whose bindings carry the given autoUpdate
// settings. A nil entry means the field is unset, so the default applies.
func appWithSources(annotations map[string]string, autoUpdate ...*bool) *v1beta1.Application {
	sources := make([]v1beta1.ApplicationSource, 0, len(autoUpdate))
	for i, au := range autoUpdate {
		sources = append(sources, v1beta1.ApplicationSource{
			Name: fmt.Sprintf("src%d", i), Type: "configmap", AutoUpdate: au,
		})
	}
	return &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default", Annotations: annotations},
		Spec:       v1beta1.ApplicationSpec{Sources: sources},
	}
}

func TestSourceAutoUpdateEnabled(t *testing.T) {
	set := v1beta1.ApplicationSource{Name: "a", AutoUpdate: boolPtr(true)}
	unset := v1beta1.ApplicationSource{Name: "b"}
	off := v1beta1.ApplicationSource{Name: "c", AutoUpdate: boolPtr(false)}

	// An explicit value wins over the controller default in both directions,
	// so a fleet-wide setting is never one-way.
	assert.True(t, sourceAutoUpdateEnabled(set, false))
	assert.False(t, sourceAutoUpdateEnabled(off, true))
	// Unset defers.
	assert.False(t, sourceAutoUpdateEnabled(unset, false))
	assert.True(t, sourceAutoUpdateEnabled(unset, true))
}

func TestAutoUpdatingSources(t *testing.T) {
	// A registry worth picking up immediately beside a flag that must wait,
	// which is the case the per-source field exists for.
	sources := []v1beta1.ApplicationSource{
		{Name: "registry", AutoUpdate: boolPtr(true)},
		{Name: "flags", AutoUpdate: boolPtr(false)},
		{Name: "cluster"},
	}
	on := autoUpdatingSources(sources, false)
	assert.Contains(t, on, "registry")
	assert.NotContains(t, on, "flags")
	assert.NotContains(t, on, "cluster", "unset follows a default of off")

	on = autoUpdatingSources(sources, true)
	assert.Contains(t, on, "registry")
	assert.NotContains(t, on, "flags", "an explicit no survives a default of on")
	assert.Contains(t, on, "cluster")

	assert.Empty(t, autoUpdatingSources(nil, true))
}

func TestSourceRefreshEnabled(t *testing.T) {
	ok, _ := sourceRefreshEnabled(appWithSources(nil, boolPtr(true)), false)
	assert.True(t, ok)

	ok, reason := sourceRefreshEnabled(appWithSources(nil, boolPtr(false)), true)
	assert.False(t, ok, "every binding opted out, so there is nothing to refresh")
	assert.Contains(t, reason, "autoUpdate")

	ok, reason = sourceRefreshEnabled(appWithSources(nil), false)
	assert.False(t, ok)
	assert.Contains(t, reason, "no sources declared")

	// A pin freezes the Application regardless of what its bindings ask for.
	pinned := appWithSources(map[string]string{oam.AnnotationPublishVersion: "v1"}, boolPtr(true))
	ok, reason = sourceRefreshEnabled(pinned, true)
	assert.False(t, ok)
	assert.Contains(t, reason, "publishVersion")
}

func TestSourceAutoUpdateDefaultFollowsFeatureGate(t *testing.T) {
	orig := utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableSourceAutoUpdate)
	defer func() {
		_ = utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("EnableSourceAutoUpdate=%v", orig))
	}()

	unset := appWithSources(nil, nil)

	assert.NoError(t, utilfeature.DefaultMutableFeatureGate.Set("EnableSourceAutoUpdate=false"))
	assert.False(t, sourceAutoUpdateDefault())
	ok, _ := sourceRefreshEnabled(unset, sourceAutoUpdateDefault())
	assert.False(t, ok, "gate off, binding unset -> off")

	assert.NoError(t, utilfeature.DefaultMutableFeatureGate.Set("EnableSourceAutoUpdate=true"))
	assert.True(t, sourceAutoUpdateDefault())
	ok, _ = sourceRefreshEnabled(unset, sourceAutoUpdateDefault())
	assert.True(t, ok, "gate on, binding unset -> on")

	ok, _ = sourceRefreshEnabled(appWithSources(nil, boolPtr(false)), sourceAutoUpdateDefault())
	assert.False(t, ok, "an explicit no beats a gate that is on")
}
