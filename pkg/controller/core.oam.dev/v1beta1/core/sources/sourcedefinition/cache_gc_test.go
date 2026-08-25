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

package sourcedefinition

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	apitypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/config"
)

func rfc3339(t time.Time) string { return t.Format(time.RFC3339) }

func TestShouldCollectCacheSecret(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name        string
		annotations map[string]string
		want        bool
	}{
		{
			name:        "no annotations",
			annotations: nil,
			want:        false,
		},
		{
			name:        "no last-sync",
			annotations: map[string]string{apitypes.AnnotationConfigTTL: "10m"},
			want:        false,
		},
		{
			name: "fresh: within TTL",
			annotations: map[string]string{
				apitypes.AnnotationConfigLastSyncAt: rfc3339(now.Add(-5 * time.Minute)),
				apitypes.AnnotationConfigTTL:        "10m",
			},
			want: false,
		},
		{
			name: "stale but recently accessed",
			annotations: map[string]string{
				apitypes.AnnotationConfigLastSyncAt:   rfc3339(now.Add(-60 * time.Minute)),
				apitypes.AnnotationConfigLastAccessed: rfc3339(now.Add(-1 * time.Minute)),
				apitypes.AnnotationConfigTTL:          "10m",
			},
			want: false,
		},
		{
			name: "stale and unaccessed past 3xTTL (last-accessed defaults to last-sync)",
			annotations: map[string]string{
				apitypes.AnnotationConfigLastSyncAt: rfc3339(now.Add(-31 * time.Minute)),
				apitypes.AnnotationConfigTTL:        "10m",
			},
			want: true,
		},
		{
			name: "stale, last-accessed just past 3xTTL",
			annotations: map[string]string{
				apitypes.AnnotationConfigLastSyncAt:   rfc3339(now.Add(-90 * time.Minute)),
				apitypes.AnnotationConfigLastAccessed: rfc3339(now.Add(-31 * time.Minute)),
				apitypes.AnnotationConfigTTL:          "10m",
			},
			want: true,
		},
		{
			name: "missing TTL uses default 15m: stale past 45m",
			annotations: map[string]string{
				apitypes.AnnotationConfigLastSyncAt: rfc3339(now.Add(-46 * time.Minute)),
			},
			want: true,
		},
		{
			name: "missing TTL uses default 15m: not yet collectible at 40m",
			annotations: map[string]string{
				apitypes.AnnotationConfigLastSyncAt: rfc3339(now.Add(-40 * time.Minute)),
			},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, shouldCollectCacheSecret(tc.annotations, now))
		})
	}
}

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	assert.NoError(t, v1beta1.AddToScheme(scheme))
	assert.NoError(t, corev1.AddToScheme(scheme))
	return scheme
}

func cacheSecret(name, templateName string, ann map[string]string) *corev1.Secret {
	a := map[string]string{}
	for k, v := range ann {
		a[k] = v
	}
	if templateName != "" {
		a[apitypes.AnnotationConfigTemplate] = templateName
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   sourceTemplateNamespace,
			Labels:      map[string]string{apitypes.LabelConfigCatalog: apitypes.VelaCoreConfig},
			Annotations: a,
		},
	}
}

func templateCM(name string, created time.Time) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:              config.TemplateConfigMapNamePrefix + name,
			Namespace:         sourceTemplateNamespace,
			CreationTimestamp: metav1.NewTime(created),
			Labels: map[string]string{
				apitypes.LabelConfigCatalog:        apitypes.VelaCoreConfig,
				apitypes.LabelSourceDefinitionName: "owner",
			},
		},
	}
}

// A ConfigTemplate somebody else created that happens to start with "source-".
// The controller stamps its own with an owning-SourceDefinition label; this has
// none.
func foreignTemplateCM(name string, created time.Time) *corev1.ConfigMap {
	cm := templateCM(name, created)
	delete(cm.Labels, apitypes.LabelSourceDefinitionName)
	return cm
}

func TestSweepSourceCache(t *testing.T) {
	now := time.Now()
	scheme := newTestScheme(t)

	old := func(d time.Duration) string { return rfc3339(now.Add(-d)) }

	// A collectible stale secret referencing an orphan template.
	staleSecret := cacheSecret("source-cache-stale", "source-orphan-aaaa1111", map[string]string{
		apitypes.AnnotationConfigLastSyncAt: old(60 * time.Minute),
		apitypes.AnnotationConfigTTL:        "10m",
	})
	// A fresh secret keeping its template alive.
	freshSecret := cacheSecret("source-cache-fresh", "source-live-bbbb2222", map[string]string{
		apitypes.AnnotationConfigLastSyncAt: old(1 * time.Minute),
		apitypes.AnnotationConfigTTL:        "10m",
	})
	// A non-source velacore-config secret (no last-sync-at) must be ignored.
	otherSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "some-distributed-config",
			Namespace: sourceTemplateNamespace,
			Labels:    map[string]string{apitypes.LabelConfigCatalog: apitypes.VelaCoreConfig},
		},
	}

	// Templates: orphan (unreferenced, old) -> delete; live (referenced by fresh secret) -> keep;
	// livesd (referenced by a live SourceDefinition status) -> keep; young orphan -> keep (grace).
	orphanTmpl := templateCM("source-orphan-aaaa1111", now.Add(-2*time.Hour))
	liveTmpl := templateCM("source-live-bbbb2222", now.Add(-2*time.Hour))
	sdRefTmpl := templateCM("source-livesd-cccc3333", now.Add(-2*time.Hour))
	youngOrphanTmpl := templateCM("source-young-dddd4444", now.Add(-1*time.Minute))
	// Old and unreferenced, so name-prefix matching alone would collect it. It
	// belongs to whoever made it, and the sweep does not own it.
	foreignTmpl := foreignTemplateCM("source-of-truth", now.Add(-2*time.Hour))

	liveSD := &v1beta1.SourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "livesd", Namespace: "vela-system"},
		Status: v1beta1.SourceDefinitionStatus{
			ConfigTemplateRef: &v1beta1.SourceDefinitionConfigTemplateRef{Name: "source-livesd-cccc3333"},
		},
	}

	cli := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(
		staleSecret, freshSecret, otherSecret,
		orphanTmpl, liveTmpl, sdRefTmpl, youngOrphanTmpl, foreignTmpl, liveSD,
	).Build()

	r := &Reconciler{Client: cli}
	res, err := r.sweepSourceCache(context.Background())
	assert.NoError(t, err)

	// Config pass: two source cache secrets scanned (otherSecret ignored), one deleted.
	assert.Equal(t, 2, res.ConfigsScanned)
	assert.Equal(t, 1, res.ConfigsDeleted)

	// Template pass: only the old, unreferenced orphan is deleted.
	assert.Equal(t, 1, res.TemplatesDeleted)

	assertGone := func(obj client.Object, key types.NamespacedName) {
		err := cli.Get(context.Background(), key, obj)
		assert.True(t, apierrors.IsNotFound(err), "expected %s to be deleted", key.Name)
	}
	assertExists := func(obj client.Object, key types.NamespacedName) {
		assert.NoError(t, cli.Get(context.Background(), key, obj), "expected %s to exist", key.Name)
	}

	ns := sourceTemplateNamespace
	assertGone(&corev1.Secret{}, types.NamespacedName{Namespace: ns, Name: "source-cache-stale"})
	assertExists(&corev1.Secret{}, types.NamespacedName{Namespace: ns, Name: "source-cache-fresh"})
	assertExists(&corev1.Secret{}, types.NamespacedName{Namespace: ns, Name: "some-distributed-config"})

	assertGone(&corev1.ConfigMap{}, types.NamespacedName{Namespace: ns, Name: config.TemplateConfigMapNamePrefix + "source-orphan-aaaa1111"})
	assertExists(&corev1.ConfigMap{}, types.NamespacedName{Namespace: ns, Name: config.TemplateConfigMapNamePrefix + "source-live-bbbb2222"})
	assertExists(&corev1.ConfigMap{}, types.NamespacedName{Namespace: ns, Name: config.TemplateConfigMapNamePrefix + "source-livesd-cccc3333"})
	assertExists(&corev1.ConfigMap{}, types.NamespacedName{Namespace: ns, Name: config.TemplateConfigMapNamePrefix + "source-young-dddd4444"})
	assertExists(&corev1.ConfigMap{}, types.NamespacedName{Namespace: ns, Name: config.TemplateConfigMapNamePrefix + "source-of-truth"})
}

// The sweep is a manager Runnable so it runs on its own timer rather than off
// SourceDefinition events. Two things have to hold: it stops when the manager
// does, and a failing sweep does not take the manager down with it.
func TestCacheGCRunnableStopsWithItsContext(t *testing.T) {
	g := &cacheGCRunnable{
		reconciler: &Reconciler{Client: fake.NewClientBuilder().Build()},
		interval:   time.Hour, // never fires; this is about the exit
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- g.Start(ctx) }()

	cancel()
	select {
	case err := <-done:
		assert.NoError(t, err, "a cancelled sweep is a clean stop, not a manager failure")
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return when its context was cancelled; the manager would hang on shutdown")
	}
}

func TestCacheGCRunnableSweepsOnEachTick(t *testing.T) {
	scheme := runtime.NewScheme()
	assert.NoError(t, corev1.AddToScheme(scheme))
	assert.NoError(t, v1beta1.AddToScheme(scheme))

	// A cache entry old enough to collect, so a sweep that ran leaves a mark.
	old := time.Now().Add(-72 * time.Hour)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "expired-entry",
			Namespace: "vela-system",
			Labels: map[string]string{
				apitypes.LabelConfigCatalog: apitypes.VelaCoreConfig,
				apitypes.LabelConfigType:    "http-get",
			},
			Annotations: map[string]string{
				apitypes.AnnotationConfigLastSyncAt:   rfc3339(old),
				apitypes.AnnotationConfigLastAccessed: rfc3339(old),
				apitypes.AnnotationConfigTTL:          "1m",
			},
		},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	g := &cacheGCRunnable{reconciler: &Reconciler{Client: cli}, interval: 10 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = g.Start(ctx) }()

	assert.Eventually(t, func() bool {
		err := cli.Get(context.Background(),
			types.NamespacedName{Namespace: "vela-system", Name: "expired-entry"}, &corev1.Secret{})
		return apierrors.IsNotFound(err)
	}, 5*time.Second, 20*time.Millisecond,
		"the ticker never reached the sweep, so entries would accumulate forever")
}

// A sweep that errors must be logged and the ticker must keep going: the
// alternative is one bad reconcile silently ending cache collection for the
// lifetime of the process.
func TestCacheGCRunnableSurvivesAFailingSweep(t *testing.T) {
	// No scheme registered, so every List inside the sweep fails.
	g := &cacheGCRunnable{
		reconciler: &Reconciler{Client: fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()},
		interval:   10 * time.Millisecond,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- g.Start(ctx) }()

	select {
	case err := <-done:
		assert.NoError(t, err, "a failing sweep is logged, not returned")
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return")
	}
}
