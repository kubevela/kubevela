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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	apitypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/config"
	"github.com/oam-dev/kubevela/pkg/config/writer"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/oam"

	"github.com/oam-dev/kubevela/pkg/sources"
)

// fakeConfigFactory implements config.Factory. Only the three methods this store
// calls do anything; the rest exist to satisfy the interface.
type fakeConfigFactory struct {
	getConfig *config.Config
	getErr    error
	parsed    *config.Config
	parseErr  error
	written   *config.Config
	writeErr  error
	writeNS   string
}

func (f *fakeConfigFactory) GetConfig(_ context.Context, _, _ string, _ bool) (*config.Config, error) {
	return f.getConfig, f.getErr
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
func (f *fakeConfigFactory) DeleteTemplate(context.Context, string, string) error { return nil }
func (f *fakeConfigFactory) ListTemplates(context.Context, string, string) ([]*config.Template, error) {
	return nil, nil
}
func (f *fakeConfigFactory) ReadConfig(context.Context, string, string) (map[string]interface{}, error) {
	return nil, nil
}
func (f *fakeConfigFactory) ListConfigs(context.Context, string, string, string, bool) ([]*config.Config, error) {
	return nil, nil
}
func (f *fakeConfigFactory) DeleteConfig(context.Context, string, string) error { return nil }
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

// A missing entry is a cache miss, not a failure. Getting this wrong fails the
// source outright instead of re-fetching it.
func TestConfigStoreReadTreatsAMissingEntryAsAMiss(t *testing.T) {
	s := storeWith(&fakeConfigFactory{getErr: config.ErrConfigNotFound}, nil, nil)
	data, found, stale, expires, err := s.Read(context.Background(), "k", time.Minute)
	require.NoError(t, err)
	require.False(t, found)
	require.False(t, stale)
	require.Nil(t, data)
	require.True(t, expires.IsZero())
}

// The same, when the factory wraps the sentinel. An equality check passes the
// test above and fails this one.
func TestConfigStoreReadUnwrapsTheNotFoundSentinel(t *testing.T) {
	wrapped := fmt.Errorf("reading config %q: %w", "k", config.ErrConfigNotFound)
	s := storeWith(&fakeConfigFactory{getErr: wrapped}, nil, nil)
	_, found, _, _, err := s.Read(context.Background(), "k", time.Minute)
	require.NoError(t, err, "a wrapped not-found is still a miss")
	require.False(t, found)
}

func TestConfigStoreReadPropagatesOtherErrors(t *testing.T) {
	s := storeWith(&fakeConfigFactory{getErr: fmt.Errorf("apiserver is down")}, nil, nil)
	_, found, _, _, err := s.Read(context.Background(), "k", time.Minute)
	require.Error(t, err)
	require.False(t, found)
}

func TestConfigStoreReadReportsFreshnessFromTheSyncAnnotation(t *testing.T) {
	now := time.Now().UTC()
	cfgWith := func(created time.Time, syncAt string) *config.Config {
		c := &config.Config{
			CreateTime: created,
			Metadata:   config.Metadata{Properties: map[string]interface{}{"image": "nginx"}},
			Secret:     &corev1.Secret{},
		}
		if syncAt != "" {
			c.Secret.Annotations = map[string]string{sourceCacheSyncAtKey: syncAt}
		}
		return c
	}

	t.Run("fresh", func(t *testing.T) {
		s := storeWith(&fakeConfigFactory{getConfig: cfgWith(now.Add(-time.Minute), "")}, nil, nil)
		data, found, stale, expires, err := s.Read(context.Background(), "k", time.Hour)
		require.NoError(t, err)
		require.True(t, found)
		require.False(t, stale)
		require.Equal(t, map[string]interface{}{"image": "nginx"}, data)
		require.True(t, expires.After(now))
	})

	t.Run("expired", func(t *testing.T) {
		s := storeWith(&fakeConfigFactory{getConfig: cfgWith(now.Add(-2*time.Hour), "")}, nil, nil)
		_, found, stale, _, err := s.Read(context.Background(), "k", time.Hour)
		require.NoError(t, err)
		require.True(t, found, "an expired entry is still there to serve stale")
		require.True(t, stale)
	})

	t.Run("the annotation wins over the creation time", func(t *testing.T) {
		// Created long ago but refreshed a moment ago: freshness follows the
		// refresh, or every long-lived entry would read as permanently stale.
		s := storeWith(&fakeConfigFactory{getConfig: cfgWith(
			now.Add(-99*time.Hour), now.Add(-time.Minute).Format(time.RFC3339))}, nil, nil)
		_, _, stale, _, err := s.Read(context.Background(), "k", time.Hour)
		require.NoError(t, err)
		require.False(t, stale)
	})

	t.Run("an unparseable annotation falls back to the creation time", func(t *testing.T) {
		s := storeWith(&fakeConfigFactory{getConfig: cfgWith(now.Add(-time.Minute), "not-a-time")}, nil, nil)
		_, _, stale, _, err := s.Read(context.Background(), "k", time.Hour)
		require.NoError(t, err)
		require.False(t, stale, "a bad annotation must not make a fresh entry stale")
	})

	t.Run("a non-positive ttl takes the default", func(t *testing.T) {
		s := storeWith(&fakeConfigFactory{getConfig: cfgWith(now.Add(-time.Minute), "")}, nil, nil)
		_, _, stale, expires, err := s.Read(context.Background(), "k", 0)
		require.NoError(t, err)
		require.False(t, stale)
		require.True(t, expires.Before(now.Add(16*time.Minute)), "defaults to 15m, not forever")
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
