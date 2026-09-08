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

package module

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	velatypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/registry/component"
)

// gitRegistry builds a git-sourced registry entry for the tests.
func gitRegistry(name string) component.Registry {
	return component.Registry{
		Name: name,
		Git: &component.GitAddonSource{
			URL:  "https://github.com/org/" + name,
			Path: DefaultGitPath,
		},
	}
}

// helmRegistry builds a helm-sourced registry entry: a valid addon registry
// entry, but not one modules can use. The module ConfigMap shares its format
// with the addon one, so this can show up hand-edited or written by
// `vela addon registry` if pointed at the module ConfigMap.
func helmRegistry(name string) component.Registry {
	return component.Registry{
		Name: name,
		Helm: &component.HelmSource{URL: "https://charts.example.com/" + name},
	}
}

// failingListStore is a RegistryDataStore stub whose ListRegistries always
// fails with a fixed error. Embedding a nil RegistryDataStore and overriding
// only ListRegistries keeps this to the one method the read-failure test
// needs, without fabricating a fake client that fails in some specific way.
type failingListStore struct {
	component.RegistryDataStore
	err error
}

func (f failingListStore) ListRegistries(context.Context) ([]component.Registry, error) {
	return nil, f.err
}

// moduleStoreWith returns a store backed by a fake client holding the module
// registry ConfigMap populated with the given entries. With no entries, the
// ConfigMap is absent entirely, which is what a cluster looks like before the
// chart is installed.
func moduleStoreWith(t *testing.T, registries ...component.Registry) component.RegistryDataStore {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	builder := fake.NewClientBuilder().WithScheme(scheme)
	if len(registries) > 0 {
		byName := map[string]component.Registry{}
		for _, reg := range registries {
			byName[reg.Name] = reg
		}
		body, err := json.Marshal(byName)
		require.NoError(t, err)
		builder = builder.WithObjects(&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ModuleRegistryConfigMap,
				Namespace: velatypes.DefaultKubeVelaNS,
			},
			Data: map[string]string{"registries": string(body)},
		})
	}
	var cli client.Client = builder.Build()
	return NewStore(cli)
}

func TestResolveRegistry(t *testing.T) {
	ctx := context.Background()

	t.Run("named registry resolves", func(t *testing.T) {
		got, err := ResolveRegistry(ctx, moduleStoreWith(t, gitRegistry("catalog"), gitRegistry("mine")), "mine")
		require.NoError(t, err)
		assert.Equal(t, "mine", got.Name)
		assert.Equal(t, "https://github.com/org/mine", got.Git.URL)
	})

	t.Run("unknown name lists what exists", func(t *testing.T) {
		_, err := ResolveRegistry(ctx, moduleStoreWith(t, gitRegistry("catalog")), "nope")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"nope"`)
		assert.Contains(t, err.Error(), "catalog")
	})

	t.Run("a single registry is the default", func(t *testing.T) {
		got, err := ResolveRegistry(ctx, moduleStoreWith(t, gitRegistry("mine")), "")
		require.NoError(t, err)
		assert.Equal(t, "mine", got.Name)
	})

	t.Run("catalog wins when several exist", func(t *testing.T) {
		store := moduleStoreWith(t, gitRegistry("mine"), gitRegistry("catalog"), gitRegistry("other"))
		got, err := ResolveRegistry(ctx, store, "")
		require.NoError(t, err)
		assert.Equal(t, DefaultRegistryName, got.Name)
	})

	t.Run("several registries and no catalog is ambiguous", func(t *testing.T) {
		_, err := ResolveRegistry(ctx, moduleStoreWith(t, gitRegistry("mine"), gitRegistry("other")), "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--registry")
		assert.Contains(t, err.Error(), "mine")
		assert.Contains(t, err.Error(), "other")
	})

	t.Run("empty store reports none configured", func(t *testing.T) {
		_, err := ResolveRegistry(ctx, moduleStoreWith(t), "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no module registry")
	})

	t.Run("a helm-sourced entry is rejected", func(t *testing.T) {
		_, err := ResolveRegistry(ctx, moduleStoreWith(t, helmRegistry("legacy")), "legacy")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "legacy")
		assert.Contains(t, err.Error(), "helm")
		assert.Contains(t, err.Error(), "git and OCI")
	})
}

func TestNotFoundError(t *testing.T) {
	ctx := context.Background()

	t.Run("a read failure is reported, not mistaken for an empty store", func(t *testing.T) {
		failing := failingListStore{err: errors.New("configmaps is forbidden: user cannot list resource")}
		err := notFoundError(ctx, failing, "nope")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nope")
		assert.Contains(t, err.Error(), "failed to list")
		assert.NotContains(t, err.Error(), "no module registry is configured")
	})

	t.Run("a genuinely empty store says none are configured", func(t *testing.T) {
		err := notFoundError(ctx, moduleStoreWith(t), "nope")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nope")
		assert.Contains(t, err.Error(), "no module registry is configured")
	})
}
