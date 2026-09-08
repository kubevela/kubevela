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

package controllers_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	pkgmodule "github.com/oam-dev/kubevela/pkg/module"
	moduleservice "github.com/oam-dev/kubevela/pkg/module/service"
	"github.com/oam-dev/kubevela/pkg/registry/component"
)

// TestFetchModule_RoundTrip publishes the s3 testdata module to a real
// OCI registry, then fetches it back through moduleservice.Service.FetchModule
// -- the same production seam "vela module deploy" and the controller's
// render service call -- and asserts the parsed Module matches the source. It
// skips cleanly when MODULE_OCI_REGISTRY is unset, so a plain e2e run never
// touches a real registry.
//
// Only the OCI leg lives here: publishing to a git catalog registry is out of
// scope for GWCP-106685 (see pkg/module/publish.go), so there is nothing real
// to round-trip through a git registry yet.
func TestFetchModule_RoundTrip(t *testing.T) {
	ociURL := os.Getenv("MODULE_OCI_REGISTRY")
	if ociURL == "" {
		t.Skip("set MODULE_OCI_REGISTRY to run the round-trip")
	}

	fixtureDir := filepath.Join(modulePublishRepoRoot(), moduleRegistryFixtureRelPath)
	source, err := pkgmodule.ParseModuleDir(fixtureDir)
	require.NoError(t, err)

	reg := component.Registry{Name: "oci-live", Helm: &component.HelmSource{URL: ociURL}}

	ctx := context.Background()
	art, err := pkgmodule.PackageModule(fixtureDir, "")
	require.NoError(t, err)
	require.NoError(t, component.PushOCIChart(ctx, reg, art.Module.Name, art.Tag, art.Archive))

	mod, err := moduleservice.NewService(fetchIntegrationFakeStore{regs: []component.Registry{reg}}).
		FetchModule(ctx, reg.Name, art.Module.Name, "")
	require.NoError(t, err)

	require.Equal(t, source.Name, mod.Name)
	require.Equal(t, source.Version, mod.Version)
	require.Contains(t, mod.Lines, "v1")
}

// fetchIntegrationFakeStore is an component.RegistryDataStore over an in-memory
// slice, standing in for the ConfigMap-backed store so this test only reaches
// a real system at the registry, not at the Kubernetes API. Unknown names
// return a k8s NotFound, as the real store does.
type fetchIntegrationFakeStore struct{ regs []component.Registry }

func (s fetchIntegrationFakeStore) GetRegistry(_ context.Context, name string) (component.Registry, error) {
	for i := range s.regs {
		if s.regs[i].Name == name {
			return s.regs[i], nil
		}
	}
	return component.Registry{}, apierrors.NewNotFound(schema.GroupResource{Resource: "Registry"}, name)
}

func (s fetchIntegrationFakeStore) ListRegistries(_ context.Context) ([]component.Registry, error) {
	return s.regs, nil
}

func (s fetchIntegrationFakeStore) AddRegistry(context.Context, component.Registry) error { return nil }

func (s fetchIntegrationFakeStore) UpdateRegistry(context.Context, component.Registry) error {
	return nil
}

func (s fetchIntegrationFakeStore) DeleteRegistry(context.Context, string) error { return nil }
