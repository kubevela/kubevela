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
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	apitypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/config"
	"github.com/oam-dev/kubevela/pkg/config/writer"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/oam"

	"k8s.io/apimachinery/pkg/types"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/sources"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// fakeConfigFactory implements config.Factory. Only the two methods this store
// calls do anything; the rest exist to satisfy the interface.
type fakeConfigFactory struct {
	parsed   *config.Config
	parseErr error
	written  *config.Config
	writeErr error
	writeNS  string
}

func (f *fakeConfigFactory) GetConfig(context.Context, string, string, bool) (*config.Config, error) {
	return nil, nil
}

func (f *fakeConfigFactory) ParseConfig(_ context.Context, tmpl config.NamespacedName, meta config.Metadata) (*config.Config, error) {
	if f.parseErr != nil {
		return nil, f.parseErr
	}
	f.parsed = &config.Config{
		Metadata: meta,
		Template: config.Template{NamespacedName: tmpl},
		Secret:   &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: meta.Name, Namespace: meta.Namespace}},
	}
	return f.parsed, nil
}

func (f *fakeConfigFactory) CreateOrUpdateConfig(_ context.Context, c *config.Config, ns string) error {
	// Mirror the real factory: caller labels and annotations are merged onto the
	// written Secret, and the ones already there win. Without this the fake would
	// silently drop everything the store stamps.
	if c != nil && c.Secret != nil {
		if c.Secret.Labels == nil {
			c.Secret.Labels = map[string]string{}
		}
		for k, v := range c.Labels {
			if _, reserved := c.Secret.Labels[k]; !reserved {
				c.Secret.Labels[k] = v
			}
		}
		if c.Secret.Annotations == nil {
			c.Secret.Annotations = map[string]string{}
		}
		for k, v := range c.Annotations {
			c.Secret.Annotations[k] = v
		}
	}
	f.written, f.writeNS = c, ns
	return f.writeErr
}

func (f *fakeConfigFactory) ParseTemplate(context.Context, string, []byte) (*config.Template, error) {
	return nil, nil
}
func (f *fakeConfigFactory) LoadTemplate(context.Context, string, string) (*config.Template, error) {
	return nil, nil
}
func (f *fakeConfigFactory) CreateOrUpdateConfigTemplate(context.Context, string, *config.Template) error {
	return nil
}
func (f *fakeConfigFactory) CreateOrUpdateConfigTemplateCR(context.Context, string, *config.Template) error {
	return nil
}
func (f *fakeConfigFactory) DeleteTemplate(context.Context, string, string) error { return nil }
func (f *fakeConfigFactory) ListTemplates(context.Context, string, string) ([]*config.Template, error) {
	return nil, nil
}
func (f *fakeConfigFactory) ListLegacyTemplates(context.Context, string, string) ([]*config.Template, error) {
	return nil, nil
}
func (f *fakeConfigFactory) ReadConfig(context.Context, string, string) (map[string]interface{}, error) {
	return nil, nil
}
func (f *fakeConfigFactory) ListConfigs(context.Context, string, string, string, bool) ([]*config.Config, error) {
	return nil, nil
}
func (f *fakeConfigFactory) DeleteConfig(context.Context, string, string) error { return nil }
func (f *fakeConfigFactory) ListAllConfigs(context.Context, string, string) ([]*config.Config, error) {
	return nil, nil
}
func (f *fakeConfigFactory) ApplyConfigCR(context.Context, *configv1alpha1.Config, *corev1.Secret) error {
	return nil
}
func (f *fakeConfigFactory) IsExist(context.Context, string, string) (bool, error) {
	return false, nil
}
func (f *fakeConfigFactory) CreateOrUpdateDistribution(context.Context, string, string, *config.CreateDistributionSpec) error {
	return nil
}
func (f *fakeConfigFactory) ListDistributions(context.Context, string) ([]*config.Distribution, error) {
	return nil, nil
}
func (f *fakeConfigFactory) DeleteDistribution(context.Context, string, string) error { return nil }
func (f *fakeConfigFactory) MergeDistributionStatus(context.Context, *config.Config, string) error {
	return nil
}

var _ config.Factory = &fakeConfigFactory{}
var _ = writer.ExpandedWriterData{}

func storeWith(f *fakeConfigFactory, cli client.Client, templates map[string]string) *configAPISourceCacheStore {
	s := newConfigAPISourceCacheStore(cli, templates)
	s.factory = f
	return s
}

// cacheEntry is a cache entry's Secret as the store writes it: created at created,
// last synced at syncAt when set, marked sensitive as its Config makes it, with the
// source cache's label and controlled by the Config of its name.
func cacheEntry(created time.Time, syncAt string) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "k", Namespace: sources.CacheNamespace(),
			CreationTimestamp: metav1.NewTime(created),
			Labels:            map[string]string{apitypes.LabelSourceDefinitionName: "http-get"},
			Annotations:       map[string]string{apitypes.AnnotationConfigSensitive: "true"},
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(
				&configv1alpha1.Config{ObjectMeta: metav1.ObjectMeta{Name: "k", UID: "cfg-uid"}},
				configv1alpha1.SchemeGroupVersion.WithKind(configv1alpha1.ConfigKind))},
		},
		Data: map[string][]byte{config.SaveInputPropertiesKey: []byte(`{"image":"nginx"}`)},
	}
	if syncAt != "" {
		secret.Annotations[sourceCacheSyncAtKey] = syncAt
	}
	return secret
}

func storeReading(objs ...client.Object) *configAPISourceCacheStore {
	return storeWith(&fakeConfigFactory{}, fake.NewClientBuilder().WithObjects(objs...).Build(), nil)
}

// A missing entry is a cache miss, not a failure. Getting this wrong fails the
// source outright instead of re-fetching it.
func TestConfigStoreReadTreatsAMissingEntryAsAMiss(t *testing.T) {
	got, err := storeReading().Read(context.Background(), "k", time.Minute)
	require.NoError(t, err)
	require.Equal(t, velaprocess.SourceCacheRead{}, got)
}

func TestConfigStoreReadPropagatesOtherErrors(t *testing.T) {
	cli := fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
		Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
			return fmt.Errorf("apiserver is down")
		},
	}).Build()
	got, err := storeWith(&fakeConfigFactory{}, cli, nil).Read(context.Background(), "k", time.Minute)
	require.Error(t, err)
	require.False(t, got.Found)
}

// The entry's Config keeps the values in spec.propertiesFrom, so the Config
// controller marks its Secret sensitive. The factory's readers refuse that, and a
// refusal reads as a miss: the persistent cache would never hit, and an entry kept
// for onStaleFailure: useStale would never be served.
func TestConfigStoreReadsAnEntryMarkedSensitive(t *testing.T) {
	got, err := storeReading(cacheEntry(time.Now().UTC().Add(-time.Minute), "")).Read(context.Background(), "k", time.Hour)
	require.NoError(t, err)
	require.True(t, got.Found)
	require.Equal(t, map[string]interface{}{"image": "nginx"}, got.Data)
}

// Only an entry the store wrote is read: anything else of the name, a Secret planted in
// the cache namespace among them, is an error, which the resolver logs and treats as a
// miss, so the source is fetched and no stale value is served from it.
func TestConfigStoreReadsOnlyItsOwnEntries(t *testing.T) {
	ns := sources.CacheNamespace()
	cases := map[string]struct {
		mutate func(*corev1.Secret)
		want   string
	}{
		"no source cache label": {
			mutate: func(s *corev1.Secret) { s.Labels = nil },
			want:   "secret " + ns + "/k is not a source cache entry: no sourcedefinition.oam.dev/name label",
		},
		"no controller": {
			mutate: func(s *corev1.Secret) { s.OwnerReferences = nil },
			want:   "secret " + ns + "/k is not a source cache entry: the Config k does not control it",
		},
		"controlled by a Config of another name": {
			mutate: func(s *corev1.Secret) { s.OwnerReferences[0].Name = "other" },
			want:   "secret " + ns + "/k is not a source cache entry: the Config k does not control it",
		},
		"controlled by something that is not a Config": {
			mutate: func(s *corev1.Secret) { s.OwnerReferences[0].Kind = "Application" },
			want:   "secret " + ns + "/k is not a source cache entry: the Config k does not control it",
		},
		"controlled by a Config of another API group": {
			mutate: func(s *corev1.Secret) { s.OwnerReferences[0].APIVersion = "example.com/v1" },
			want:   "secret " + ns + "/k is not a source cache entry: the Config k does not control it",
		},
		"owned by the Config but not as controller": {
			mutate: func(s *corev1.Secret) { s.OwnerReferences[0].Controller = nil },
			want:   "secret " + ns + "/k is not a source cache entry: the Config k does not control it",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			secret := cacheEntry(time.Now().UTC().Add(-time.Minute), "")
			tc.mutate(secret)
			got, err := storeReading(secret).Read(context.Background(), "k", time.Hour)
			require.EqualError(t, err, tc.want)
			require.Equal(t, velaprocess.SourceCacheRead{}, got, "nothing of the Secret is served")
		})
	}

	t.Run("an entry with no input-properties is an error, not a found-but-empty entry", func(t *testing.T) {
		secret := cacheEntry(time.Now().UTC().Add(-time.Minute), "")
		secret.Data = nil
		got, err := storeReading(secret).Read(context.Background(), "k", time.Hour)
		require.Error(t, err)
		require.False(t, got.Found)
	})
}

func TestConfigStoreReadReportsFreshnessFromTheSyncAnnotation(t *testing.T) {
	now := time.Now().UTC()

	t.Run("fresh", func(t *testing.T) {
		got, err := storeReading(cacheEntry(now.Add(-time.Minute), "")).Read(context.Background(), "k", time.Hour)
		require.NoError(t, err)
		require.True(t, got.Found)
		require.False(t, got.Stale)
		require.Equal(t, map[string]interface{}{"image": "nginx"}, got.Data)
		require.True(t, got.ExpiresAt.After(now))
	})

	t.Run("expired", func(t *testing.T) {
		got, err := storeReading(cacheEntry(now.Add(-2*time.Hour), "")).Read(context.Background(), "k", time.Hour)
		require.NoError(t, err)
		require.True(t, got.Found, "an expired entry is still there to serve stale")
		require.True(t, got.Stale)
	})

	t.Run("the annotation wins over the creation time", func(t *testing.T) {
		// Created long ago but refreshed a moment ago: freshness follows the
		// refresh, or every long-lived entry would read as permanently stale.
		got, err := storeReading(cacheEntry(now.Add(-99*time.Hour), now.Add(-time.Minute).Format(time.RFC3339))).
			Read(context.Background(), "k", time.Hour)
		require.NoError(t, err)
		require.False(t, got.Stale)
	})

	t.Run("an unparseable annotation falls back to the creation time", func(t *testing.T) {
		got, err := storeReading(cacheEntry(now.Add(-time.Minute), "not-a-time")).Read(context.Background(), "k", time.Hour)
		require.NoError(t, err)
		require.False(t, got.Stale, "a bad annotation must not make a fresh entry stale")
	})

	t.Run("a non-positive ttl takes the default", func(t *testing.T) {
		got, err := storeReading(cacheEntry(now.Add(-time.Minute), "")).Read(context.Background(), "k", 0)
		require.NoError(t, err)
		require.False(t, got.Stale)
		require.True(t, got.ExpiresAt.Before(now.Add(16*time.Minute)), "defaults to 15m, not forever")
	})
}

func TestConfigStoreWriteStampsIdentityAndSyncTime(t *testing.T) {
	f := &fakeConfigFactory{}
	s := storeWith(f, nil, map[string]string{"http-get": "http-get-a1b2"})

	err := s.Write(context.Background(), "http-get-abc", "http-get",
		map[string]interface{}{"body": "hi"}, velaprocess.SourceCacheWriteMeta{})
	require.NoError(t, err)
	require.NotNil(t, f.written)
	require.Equal(t, sources.CacheNamespace(), f.writeNS)
	require.Equal(t, "http-get-a1b2", f.parsed.Template.Name,
		"the template registered for this source type is the one used")
	require.NotEmpty(t, f.written.Secret.Annotations[sourceCacheSyncAtKey],
		"without a sync time every entry reads as stale from its creation")
	require.Equal(t, "http-get", f.written.Secret.Labels[apitypes.LabelConfigType],
		"the source type is the config type, which is what the GC sweep selects on")
	require.Equal(t, apitypes.VelaCoreConfig, f.written.Secret.Labels[apitypes.LabelConfigCatalog],
		"unlabelled, the entry is invisible to vela config and to the sweep")
	require.Equal(t, "true", f.written.Secret.Annotations[apitypes.AnnotationConfigSensitive],
		"sensitive from the first write, as the Config controller marks it")
}

// A source type with no registered template falls back to the one the write
// carries, rather than writing an entry no template can validate.
func TestConfigStoreWriteFallsBackToTheMetaTemplate(t *testing.T) {
	f := &fakeConfigFactory{}
	s := storeWith(f, nil, nil)
	require.NoError(t, s.Write(context.Background(), "k", "http-get",
		map[string]interface{}{}, velaprocess.SourceCacheWriteMeta{TemplateName: "from-meta"}))
	require.Equal(t, "from-meta", f.parsed.Template.Name)
}

func TestConfigStoreWritePropagatesParseFailure(t *testing.T) {
	s := storeWith(&fakeConfigFactory{parseErr: fmt.Errorf("schema mismatch")}, nil, nil)
	err := s.Write(context.Background(), "k", "http-get", nil, velaprocess.SourceCacheWriteMeta{})
	require.ErrorContains(t, err, "schema mismatch")
}

func TestConfigStoreTouch(t *testing.T) {
	entry := func(accessed string) *corev1.Secret {
		s := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Name: "k", Namespace: sources.CacheNamespace(), Annotations: map[string]string{}}}
		if accessed != "" {
			s.Annotations[sourceCacheAccessedKey] = accessed
		}
		return s
	}
	read := func(t *testing.T, cli client.Client) *corev1.Secret {
		t.Helper()
		got := &corev1.Secret{}
		require.NoError(t, cli.Get(context.Background(),
			client.ObjectKey{Namespace: sources.CacheNamespace(), Name: "k"}, got))
		return got
	}

	t.Run("no client is a no-op", func(t *testing.T) {
		require.NoError(t, storeWith(&fakeConfigFactory{}, nil, nil).Touch(context.Background(), "k"))
	})

	t.Run("a missing entry is not an error", func(t *testing.T) {
		cli := fake.NewClientBuilder().Build()
		require.NoError(t, storeWith(&fakeConfigFactory{}, cli, nil).Touch(context.Background(), "gone"))
	})

	t.Run("an unmarked entry gets marked", func(t *testing.T) {
		cli := fake.NewClientBuilder().WithObjects(entry("")).Build()
		require.NoError(t, storeWith(&fakeConfigFactory{}, cli, nil).Touch(context.Background(), "k"))
		require.NotEmpty(t, read(t, cli).Annotations[sourceCacheAccessedKey])
	})

	t.Run("a recently marked entry is left alone", func(t *testing.T) {
		recent := time.Now().UTC().Format(time.RFC3339)
		cli := fake.NewClientBuilder().WithObjects(entry(recent)).Build()
		require.NoError(t, storeWith(&fakeConfigFactory{}, cli, nil).Touch(context.Background(), "k"))
		require.Equal(t, recent, read(t, cli).Annotations[sourceCacheAccessedKey],
			"throttled, or a hot stale entry is rewritten on every reconcile")
	})
}

// The template a source's cache entries are validated against lives on the
// SourceDefinition's status, and definitions resolve app-namespace first then
// vela-system. A type that resolves to no template is left out rather than
// mapped to an empty name, which Write would treat as "no template at all".
func TestSourceTemplateRefsByType(t *testing.T) {
	def := func(ns, name, tmpl string) *v1beta1.SourceDefinition {
		d := &v1beta1.SourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		}
		if tmpl != "" {
			d.Status.ConfigTemplateRef = &v1beta1.SourceDefinitionConfigTemplateRef{Name: tmpl}
		}
		return d
	}
	af := func(types ...string) *appfile.Appfile {
		a := &appfile.Appfile{
			Namespace:                "app-ns",
			RelatedSourceDefinitions: map[string]*v1beta1.SourceDefinition{},
		}
		for _, ty := range types {
			a.RelatedSourceDefinitions[ty] = nil
		}
		return a
	}
	ctx := context.Background()

	t.Run("no appfile or no client", func(t *testing.T) {
		require.Empty(t, sourceTemplateRefsByType(ctx, nil, af("http-get")))
		require.Empty(t, sourceTemplateRefsByType(ctx, fake.NewClientBuilder().Build(), nil))
	})

	t.Run("found in the application namespace", func(t *testing.T) {
		cli := fake.NewClientBuilder().WithScheme(sourceTestScheme(t)).
			WithObjects(def("app-ns", "http-get", "tmpl-local")).Build()
		require.Equal(t, map[string]string{"http-get": "tmpl-local"},
			sourceTemplateRefsByType(ctx, cli, af("http-get")))
	})

	t.Run("falls back to vela-system", func(t *testing.T) {
		cli := fake.NewClientBuilder().WithScheme(sourceTestScheme(t)).
			WithObjects(def(oam.SystemDefinitionNamespace, "http-get", "tmpl-system")).Build()
		require.Equal(t, map[string]string{"http-get": "tmpl-system"},
			sourceTemplateRefsByType(ctx, cli, af("http-get")))
	})

	t.Run("a definition with no template ref is skipped", func(t *testing.T) {
		cli := fake.NewClientBuilder().WithScheme(sourceTestScheme(t)).
			WithObjects(def("app-ns", "http-get", "")).Build()
		require.Empty(t, sourceTemplateRefsByType(ctx, cli, af("http-get")),
			"an empty name would look to Write like no template at all")
	})

	t.Run("a type with no definition anywhere is skipped", func(t *testing.T) {
		cli := fake.NewClientBuilder().WithScheme(sourceTestScheme(t)).Build()
		require.Empty(t, sourceTemplateRefsByType(ctx, cli, af("http-get")))
	})

	t.Run("one missing type does not lose the others", func(t *testing.T) {
		cli := fake.NewClientBuilder().WithScheme(sourceTestScheme(t)).
			WithObjects(def("app-ns", "git-file", "tmpl-git")).Build()
		require.Equal(t, map[string]string{"git-file": "tmpl-git"},
			sourceTemplateRefsByType(ctx, cli, af("git-file", "absent")))
	})
}

// A scheme of its own: testScheme is populated in the Ginkgo BeforeSuite, which
// does not run for a plain Go test.
func sourceTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	sc := runtime.NewScheme()
	require.NoError(t, v1beta1.SchemeBuilder.AddToScheme(sc))
	require.NoError(t, corev1.AddToScheme(sc))
	return sc
}

// The live SourceDefinition's ConfigTemplate took precedence over the one the
// render actually used. A published ApplicationRevision resolves against the
// definition it snapshotted, so once the live definition's schema changed the
// entry was parsed against the wrong template and ParseConfig rejected it.
//
// meta.TemplateName is derived from the schema the resolver had in hand, so it
// is the version-accurate answer; the live lookup remains the fallback for a
// resolver that reported none.
func TestConfigStoreWritePrefersTheTemplateTheRenderUsed(t *testing.T) {
	t.Run("the render's template wins", func(t *testing.T) {
		f := &fakeConfigFactory{}
		s := storeWith(f, nil, map[string]string{"http-get": "http-get-live"})

		require.NoError(t, s.Write(context.Background(), "http-get-abc", "http-get",
			map[string]interface{}{"body": "hi"},
			velaprocess.SourceCacheWriteMeta{TemplateName: "http-get-pinned"}))
		require.Equal(t, "http-get-pinned", f.parsed.Template.Name)
		require.Equal(t, "http-get-pinned",
			f.written.Secret.Annotations[apitypes.AnnotationConfigTemplate])
	})

	t.Run("the live lookup is the fallback", func(t *testing.T) {
		f := &fakeConfigFactory{}
		s := storeWith(f, nil, map[string]string{"http-get": "http-get-live"})

		require.NoError(t, s.Write(context.Background(), "http-get-abc", "http-get",
			map[string]interface{}{"body": "hi"}, velaprocess.SourceCacheWriteMeta{}))
		require.Equal(t, "http-get-live", f.parsed.Template.Name)
	})
}

// A cache entry is two objects with distinct jobs: a Secret holding the resolved
// values, and a Config CR declaring it through the API.
//
// The values stay in the Secret because they must be assumed sensitive - a
// source schema can mark fields +sensitive and the resolver redacts them out of
// status. spec.properties on a CR is plainly readable, so the entry points at
// the Secret with propertiesFrom instead, which is what that field is for. The
// controller only ever reads a propertiesFrom Secret, so nothing overwrites the
// cached values.
// Write leaves a Secret of the key's name it did not write alone. The factory's template
// guard does not hold for a source with no schema (no template named), so without this a
// foreign Secret would be merged into and adopted by the entry's Config.
func TestConfigStoreWriteRefusesASecretItDidNotWrite(t *testing.T) {
	ns := sources.CacheNamespace()
	meta := velaprocess.SourceCacheWriteMeta{TTL: time.Minute, SourceDefName: "atlas", SourceDefNamespace: "vela-system"}
	data := map[string]interface{}{"host": "example.com"}
	planted := func(mutate func(*corev1.Secret)) *corev1.Secret {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "atlas-abc123", Namespace: ns},
			Data:       map[string][]byte{config.SaveInputPropertiesKey: []byte(`{"host":"planted"}`)},
		}
		mutate(secret)
		return secret
	}

	t.Run("no source cache label", func(t *testing.T) {
		r := require.New(t)
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(planted(func(*corev1.Secret) {})).Build()
		err := newConfigAPISourceCacheStore(cli, nil).Write(context.Background(), "atlas-abc123", "atlas", data, meta)
		r.EqualError(err, "secret "+ns+"/atlas-abc123 is not a source cache entry: no sourcedefinition.oam.dev/name label")
		var got corev1.Secret
		r.NoError(cli.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "atlas-abc123"}, &got))
		r.Equal(`{"host":"planted"}`, string(got.Data[config.SaveInputPropertiesKey]), "left as it was")
		r.Empty(got.OwnerReferences, "not adopted")
		r.True(apierrors.IsNotFound(cli.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "atlas-abc123"}, &configv1alpha1.Config{})), "no Config declared for it")
	})

	t.Run("marked but controlled by something else", func(t *testing.T) {
		r := require.New(t)
		secret := planted(func(s *corev1.Secret) {
			s.Labels = map[string]string{apitypes.LabelSourceDefinitionName: "atlas"}
			s.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(
				&v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "someone", UID: "app-uid"}},
				v1beta1.SchemeGroupVersion.WithKind(v1beta1.ApplicationKind))}
		})
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(secret).Build()
		err := newConfigAPISourceCacheStore(cli, nil).Write(context.Background(), "atlas-abc123", "atlas", data, meta)
		r.EqualError(err, "secret "+ns+"/atlas-abc123 is not a source cache entry: Application someone controls it")
	})

	t.Run("marked and controlled by nothing yet: an entry whose adoption the write completes", func(t *testing.T) {
		r := require.New(t)
		secret := planted(func(s *corev1.Secret) {
			s.Labels = map[string]string{apitypes.LabelSourceDefinitionName: "atlas", apitypes.LabelConfigCatalog: apitypes.VelaCoreConfig}
		})
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(secret).Build()
		s := newConfigAPISourceCacheStore(cli, nil)
		r.NoError(s.Write(context.Background(), "atlas-abc123", "atlas", data, meta))
		got, err := s.Read(context.Background(), "atlas-abc123", time.Hour)
		r.NoError(err)
		r.Equal("example.com", got.Data["host"])
	})
}

func TestConfigStoreWriteDeclaresTheEntryThroughTheAPI(t *testing.T) {
	r := require.New(t)
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	s := newConfigAPISourceCacheStore(cli, nil)

	data := map[string]interface{}{"host": "example.com"}
	r.NoError(s.Write(context.Background(), "atlas-abc123", "atlas", data,
		velaprocess.SourceCacheWriteMeta{
			TTL:                5 * time.Minute,
			SourceDefName:      "atlas",
			SourceDefNamespace: "vela-system",
		}))

	ns := sources.CacheNamespace()

	// The values live in the Secret.
	var secret corev1.Secret
	r.NoError(cli.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: "atlas-abc123"}, &secret))
	r.Contains(string(secret.Data[config.SaveInputPropertiesKey]), "example.com")
	r.Equal("true", secret.Annotations[apitypes.AnnotationConfigSensitive])

	// and reads back as a hit, marked sensitive as it is
	got, err := s.Read(context.Background(), "atlas-abc123", time.Hour)
	r.NoError(err)
	r.True(got.Found)
	r.Equal("example.com", got.Data["host"])

	// The Config CR declares it, pointing at that Secret rather than inlining.
	var cfg configv1alpha1.Config
	r.NoError(cli.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: "atlas-abc123"}, &cfg))
	r.Nil(cfg.Spec.Properties, "sensitive values must not be inlined in the CR spec")
	r.NotNil(cfg.Spec.PropertiesFrom)
	r.Equal("atlas-abc123", cfg.Spec.PropertiesFrom.SecretRef.Name)

	// Identity is queryable on the CR, so a sweep can find it.
	r.Equal("atlas", cfg.Labels[apitypes.LabelSourceDefinitionName])
	r.Equal("vela-system", cfg.Labels[apitypes.LabelSourceDefinitionNamespace])

	// Deleting the CR takes the values with it.
	r.Len(secret.OwnerReferences, 1, "the Secret is owned by its Config, so a delete cascades")
	r.Equal(configv1alpha1.ConfigKind, secret.OwnerReferences[0].Kind)
	r.Equal(cfg.UID, secret.OwnerReferences[0].UID)
}

// The resolver and the LRU in front of the store read an entry through the
// SourceCacheStore interface. A fresh entry read that way must be a hit, or every
// reconcile fetches the source again whatever its storageTTL.
func TestConfigStoreFreshEntryIsAHitThroughTheInterface(t *testing.T) {
	var store velaprocess.SourceCacheStore = storeReading(cacheEntry(time.Now().UTC().Add(-time.Minute), ""))

	got, err := store.Read(context.Background(), "k", time.Hour)
	require.NoError(t, err)
	require.True(t, got.Found, "a fresh entry is a hit")
	require.False(t, got.Stale, "a fresh entry is not stale")
	require.Equal(t, "nginx", got.Data["image"])
}
