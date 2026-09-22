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

package service

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart/loader"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/oam-dev/kubevela/pkg/module"
	"github.com/oam-dev/kubevela/pkg/registry/component"
)

// fakeStore is a component.RegistryDataStore over an in-memory slice. Unknown names
// return a k8s NotFound (as the real ConfigMap-backed store does), so
// module.ResolveRegistry takes its not-found path.
type fakeStore struct{ regs []component.Registry }

func (s fakeStore) GetRegistry(_ context.Context, name string) (component.Registry, error) {
	for i := range s.regs {
		if s.regs[i].Name == name {
			return s.regs[i], nil
		}
	}
	return component.Registry{}, apierrors.NewNotFound(schema.GroupResource{Resource: "Registry"}, name)
}

func (s fakeStore) ListRegistries(_ context.Context) ([]component.Registry, error) {
	return s.regs, nil
}

func (s fakeStore) AddRegistry(context.Context, component.Registry) error { return nil }

func (s fakeStore) UpdateRegistry(context.Context, component.Registry) error { return nil }

func (s fakeStore) DeleteRegistry(context.Context, string) error { return nil }

// unrevisioned stands in for a source that cannot name its revision, which
// turns the cache off and makes every fetch read -- the behaviour these cases
// were written against.
func unrevisioned(_ context.Context, _ *component.Registry, _, _, _ string) (string, error) {
	return "", component.ErrRevisionUnsupported
}

// newServiceWithFakes builds a Service whose OCI pull returns files instead of
// reaching a registry. files is keyed "<module>/<rel>", which is exactly the
// naming a Helm chart carries and MemoryReader expects, so the real
// pull -> MemoryReader -> readerFS path still runs underneath.
//
// A faked puller needs a faked revision too, or the probe reaches for the
// registry the fake stands in for.
func newServiceWithFakes(store component.RegistryDataStore, files map[string]string) *Service {
	bufs := make([]*loader.BufferedFile, 0, len(files))
	for name, data := range files {
		bufs = append(bufs, &loader.BufferedFile{Name: name, Data: []byte(data)})
	}
	s := NewService(store)
	s.revision = unrevisioned
	s.pullChart = func(_ context.Context, _ *component.Registry, _, _ string) ([]*loader.BufferedFile, error) {
		return bufs, nil
	}
	return s
}

func TestFetchModule(t *testing.T) {
	files := map[string]string{
		"s3/_module.cue":                "module: \"s3\"\nversion: \"1.0.0\"",
		"s3/v1/_version.cue":            "apiVersion: \"v1\"",
		"s3/v1/definitions/bucket.yaml": "apiVersion: core.oam.dev/v1beta1\nkind: ComponentDefinition\nmetadata:\n  name: atmos-s3-v1\n",
	}
	s := newServiceWithFakes(fakeStore{regs: []component.Registry{ociRegistry("catalog")}}, files)

	mod, err := s.FetchModule(context.Background(), "catalog", "s3", "")
	require.NoError(t, err)
	require.Equal(t, "s3", mod.Name)
	require.Equal(t, "1.0.0", mod.Version)
	require.Contains(t, mod.Lines, "v1")
}

// TestFetchModule_PullerDecidesVersion asserts FetchModule reports whatever the
// puller returned rather than the version that was asked for. The fake ignores
// the requested tag, so a mismatch here would mean FetchModule had taken the
// request as the answer instead of reading _module.cue.
func TestFetchModule_PullerDecidesVersion(t *testing.T) {
	files := map[string]string{
		"s3/_module.cue":                "module: \"s3\"\nversion: \"1.0.0\"",
		"s3/v1/_version.cue":            "apiVersion: \"v1\"",
		"s3/v1/definitions/bucket.yaml": "apiVersion: core.oam.dev/v1beta1\nkind: ComponentDefinition\nmetadata:\n  name: atmos-s3-v1\n",
	}
	s := newServiceWithFakes(fakeStore{regs: []component.Registry{ociRegistry("catalog")}}, files)

	mod, err := s.FetchModule(context.Background(), "catalog", "s3", "9.9.9-does-not-exist")
	require.NoError(t, err)
	require.Equal(t, "s3", mod.Name)
	require.Equal(t, "1.0.0", mod.Version)
}

func TestFetchModule_EmptyNameResolvesSole(t *testing.T) {
	files := map[string]string{
		"s3/_module.cue":                "module: \"s3\"\nversion: \"1.0.0\"",
		"s3/v1/_version.cue":            "apiVersion: \"v1\"",
		"s3/v1/definitions/bucket.yaml": "apiVersion: core.oam.dev/v1beta1\nkind: ComponentDefinition\nmetadata:\n  name: atmos-s3-v1\n",
	}
	s := newServiceWithFakes(fakeStore{regs: []component.Registry{ociRegistry("only")}}, files)

	mod, err := s.FetchModule(context.Background(), "", "s3", "")
	require.NoError(t, err)
	require.Equal(t, "s3", mod.Name)
}

func TestFetchModule_EmptyNameAmbiguous(t *testing.T) {
	s := newServiceWithFakes(fakeStore{regs: []component.Registry{ociRegistry("a"), ociRegistry("b")}}, nil)

	_, err := s.FetchModule(context.Background(), "", "s3", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "a")
	require.Contains(t, err.Error(), "b")
}

func TestFetchModule_UnknownRegistry(t *testing.T) {
	s := newServiceWithFakes(fakeStore{regs: []component.Registry{ociRegistry("catalog")}}, nil)

	_, err := s.FetchModule(context.Background(), "missing", "s3", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing")
	require.Contains(t, err.Error(), "catalog") // lists configured registries (reused module.NotFoundError)
	require.ErrorIs(t, err, module.ErrRegistryNotFound)
}

func TestFetchModule_RejectsUnsupportedSource(t *testing.T) {
	// A helm entry can live in the shared ConfigMap; module.ResolveRegistry must
	// reject it before any fetch. Verifies fetch honors the OCI-only scope.
	helmReg := component.Registry{Name: "legacy", Helm: &component.HelmSource{URL: "https://charts.example.com"}}
	s := newServiceWithFakes(fakeStore{regs: []component.Registry{helmReg}}, nil)

	_, err := s.FetchModule(context.Background(), "legacy", "s3", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "only OCI registries")
}

func TestFetchModule_EmptyNameDefaultsToCatalog(t *testing.T) {
	// With several registries and no name, the one named "catalog" wins (the
	// reused module.ResolveRegistry default policy).
	files := map[string]string{
		"s3/_module.cue":                "module: \"s3\"\nversion: \"1.0.0\"",
		"s3/v1/_version.cue":            "apiVersion: \"v1\"",
		"s3/v1/definitions/bucket.yaml": "apiVersion: core.oam.dev/v1beta1\nkind: ComponentDefinition\nmetadata:\n  name: atmos-s3-v1\n",
	}
	s := newServiceWithFakes(fakeStore{regs: []component.Registry{ociRegistry("other"), ociRegistry("catalog")}}, files)

	mod, err := s.FetchModule(context.Background(), "", "s3", "")
	require.NoError(t, err)
	require.Equal(t, "s3", mod.Name)
}

// TestFetchModule_ModuleNotFound covers a module the registry does not carry.
// An OCI registry answers that at the pull, with a manifest-unknown error, so
// there is no listing to be absent from and the failure names the module and
// the registry rather than wrapping module.ErrModuleNotFound.
func TestFetchModule_ModuleNotFound(t *testing.T) {
	s := NewService(fakeStore{regs: []component.Registry{ociRegistry("catalog")}})
	s.revision = unrevisioned
	s.pullChart = func(_ context.Context, _ *component.Registry, name, _ string) ([]*loader.BufferedFile, error) {
		return nil, errors.Errorf("failed to pull addon chart %s: manifest unknown", name)
	}

	_, err := s.FetchModule(context.Background(), "catalog", "s3", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "s3")
	require.Contains(t, err.Error(), "catalog")
}

func ociRegistry(name string) component.Registry {
	return component.Registry{Name: name, Helm: &component.HelmSource{URL: "oci://registry.example.com/modules"}}
}

// TestFetchModule_OCI_ParsesChartLayout drives the OCI branch
// (pull -> MemoryReader -> readerFS) through an injected puller, including the
// Chart.yaml wrapper a real chart carries. The real pull is exercised live in
// the round-trip test.
func TestFetchModule_OCI_ParsesChartLayout(t *testing.T) {
	// A Helm chart carries files prefixed by the chart (module) name — exactly
	// what pullModuleChart returns and what MemoryReader expects.
	bufs := []*loader.BufferedFile{
		{Name: "s3/_module.cue", Data: []byte("module: \"s3\"\nversion: \"1.0.0\"")},
		{Name: "s3/v1/_version.cue", Data: []byte("apiVersion: \"v1\"")},
		{Name: "s3/v1/definitions/bucket.yaml", Data: []byte("apiVersion: core.oam.dev/v1beta1\nkind: ComponentDefinition\nmetadata:\n  name: atmos-s3-v1\n")},
		{Name: "s3/Chart.yaml", Data: []byte("name: s3\nversion: 1.0.0\n")}, // chart wrapper; parser ignores it
	}

	s := NewService(fakeStore{regs: []component.Registry{ociRegistry("oci")}})
	// A faked puller needs a faked revision too, or the probe reaches for the
	// registry the fake stands in for.
	s.revision = unrevisioned
	s.pullChart = func(_ context.Context, _ *component.Registry, _, _ string) ([]*loader.BufferedFile, error) {
		return bufs, nil
	}

	mod, err := s.FetchModule(context.Background(), "oci", "s3", "")
	require.NoError(t, err)
	require.Equal(t, "s3", mod.Name)
	require.Equal(t, "1.0.0", mod.Version)
	require.Contains(t, mod.Lines, "v1")
	require.Len(t, mod.Lines["v1"].Definitions, 1)
}

// TestFetchModule_OCI_PassesRequestedVersion asserts a non-empty version
// requested by the caller reaches the OCI puller unchanged, so an exact tag
// can be pinned end to end.
func TestFetchModule_OCI_PassesRequestedVersion(t *testing.T) {
	bufs := []*loader.BufferedFile{
		{Name: "s3/_module.cue", Data: []byte("module: \"s3\"\nversion: \"1.2.0\"")},
		{Name: "s3/v1/_version.cue", Data: []byte("apiVersion: \"v1\"")},
		{Name: "s3/v1/definitions/bucket.yaml", Data: []byte("apiVersion: core.oam.dev/v1beta1\nkind: ComponentDefinition\nmetadata:\n  name: atmos-s3-v1\n")},
	}

	var gotVersion string
	s := NewService(fakeStore{regs: []component.Registry{ociRegistry("oci")}})
	// A faked puller needs a faked revision too, or the probe reaches for the
	// registry the fake stands in for.
	s.revision = unrevisioned
	s.pullChart = func(_ context.Context, _ *component.Registry, _, version string) ([]*loader.BufferedFile, error) {
		gotVersion = version
		return bufs, nil
	}

	mod, err := s.FetchModule(context.Background(), "oci", "s3", "1.2.0")
	require.NoError(t, err)
	require.Equal(t, "1.2.0", gotVersion)
	require.Equal(t, "1.2.0", mod.Version)
}

// TestFetchModule_OCI_UnknownVersionFails asserts an OCI puller error (what the
// real registry client returns for a tag that does not exist) surfaces as a
// FetchModule error naming the module, before any parse is attempted.
func TestFetchModule_OCI_UnknownVersionFails(t *testing.T) {
	s := NewService(fakeStore{regs: []component.Registry{ociRegistry("oci")}})
	// A faked puller needs a faked revision too, or the probe reaches for the
	// registry the fake stands in for.
	s.revision = unrevisioned
	s.pullChart = func(_ context.Context, _ *component.Registry, _, version string) ([]*loader.BufferedFile, error) {
		return nil, errors.Errorf("failed to pull addon chart s3:%s: manifest unknown", version)
	}

	mod, err := s.FetchModule(context.Background(), "oci", "s3", "9.9.9-does-not-exist")
	require.Error(t, err)
	require.Nil(t, mod)
	require.Contains(t, err.Error(), "s3")
	require.Contains(t, err.Error(), "9.9.9-does-not-exist")
}
