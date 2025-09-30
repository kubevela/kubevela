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

package addon

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	velatypes "github.com/oam-dev/kubevela/apis/types"
)

func TestAddonRegistry(t *testing.T) {
	ctx := context.Background()
	testRegistry := Registry{
		Name: "test-registry",
		Git: &GitAddonSource{
			URL:   "http://github.com/test/repo",
			Token: "test-token",
		},
	}
	scheme := runtime.NewScheme()
	assert.NoError(t, v1.AddToScheme(scheme))
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	ds := NewRegistryDataStore(client)

	t.Run("add registry", func(t *testing.T) {
		err := ds.AddRegistry(ctx, testRegistry)
		assert.NoError(t, err)

		var cm v1.ConfigMap
		err = client.Get(ctx, types.NamespacedName{Name: registryConfigMapName, Namespace: velatypes.DefaultKubeVelaNS}, &cm)
		assert.NoError(t, err)
		var registries map[string]Registry
		err = json.Unmarshal([]byte(cm.Data[registriesKey]), &registries)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(registries))
		gotRegistry := registries["test-registry"]
		assert.Equal(t, "", gotRegistry.Git.Token)
		assert.Equal(t, "addon-registry-test-registry", gotRegistry.Git.TokenSecretRef)

		var secret v1.Secret
		err = client.Get(ctx, types.NamespacedName{Name: "addon-registry-test-registry", Namespace: velatypes.DefaultKubeVelaNS}, &secret)
		assert.NoError(t, err)
		assert.Equal(t, "test-token", string(secret.Data["token"]))
	})

	t.Run("update registry", func(t *testing.T) {
		updatedRegistry := Registry{
			Name: "test-registry",
			Git: &GitAddonSource{
				URL:   "http://github.com/test/repo-updated",
				Token: "test-token-updated",
			},
		}
		err := ds.UpdateRegistry(ctx, updatedRegistry)
		assert.NoError(t, err)

		var secret v1.Secret
		err = client.Get(ctx, types.NamespacedName{Name: "addon-registry-test-registry", Namespace: velatypes.DefaultKubeVelaNS}, &secret)
		assert.NoError(t, err)
		assert.Equal(t, "test-token-updated", string(secret.Data["token"]))
	})

	t.Run("list and get registry", func(t *testing.T) {
		registries, err := ds.ListRegistries(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(registries))
		assert.Equal(t, "test-token-updated", registries[0].Git.Token)

		reg, err := ds.GetRegistry(ctx, "test-registry")
		assert.NoError(t, err)
		assert.Equal(t, "test-token-updated", reg.Git.Token)
	})

	t.Run("delete registry", func(t *testing.T) {
		err := ds.DeleteRegistry(ctx, "test-registry")
		assert.NoError(t, err)

		var cm v1.ConfigMap
		err = client.Get(ctx, types.NamespacedName{Name: registryConfigMapName, Namespace: velatypes.DefaultKubeVelaNS}, &cm)
		assert.NoError(t, err)
		var registries map[string]Registry
		err = json.Unmarshal([]byte(cm.Data[registriesKey]), &registries)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(registries))

		var secret v1.Secret
		err = client.Get(ctx, types.NamespacedName{Name: "addon-registry-test-registry", Namespace: velatypes.DefaultKubeVelaNS}, &secret)
		assert.Error(t, err)
		assert.True(t, apierrors.IsNotFound(err))
	})
}

func TestAddRegistry(t *testing.T) {
	t.Run("Test adding a registry", func(t *testing.T) {
		ctx := context.Background()
		testRegistry := Registry{
			Name: "test-registry",
			Git: &GitAddonSource{
				URL:   "http://github.com/test/repo",
				Token: "test-token",
			},
		}
		scheme := runtime.NewScheme()
		assert.NoError(t, v1.AddToScheme(scheme))
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		ds := NewRegistryDataStore(client)

		err := ds.AddRegistry(ctx, testRegistry)
		assert.NoError(t, err)

		var cm v1.ConfigMap
		err = client.Get(ctx, types.NamespacedName{Name: registryConfigMapName, Namespace: velatypes.DefaultKubeVelaNS}, &cm)
		assert.NoError(t, err)
		var registries map[string]Registry
		err = json.Unmarshal([]byte(cm.Data[registriesKey]), &registries)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(registries))
		gotRegistry := registries["test-registry"]
		assert.Equal(t, "", gotRegistry.Git.Token)
		assert.Equal(t, "addon-registry-test-registry", gotRegistry.Git.TokenSecretRef)

		var secret v1.Secret
		err = client.Get(ctx, types.NamespacedName{Name: "addon-registry-test-registry", Namespace: velatypes.DefaultKubeVelaNS}, &secret)
		assert.NoError(t, err)
		assert.Equal(t, "test-token", string(secret.Data["token"]))
	})
}

func TestUpdateRegistry(t *testing.T) {
	t.Run("Test updating a registry", func(t *testing.T) {
		ctx := context.Background()
		updatedRegistry := Registry{
			Name: "test-registry",
			Git: &GitAddonSource{
				URL:   "http://github.com/test/repo-updated",
				Token: "test-token-updated",
			},
		}
		// Pre-existing state
		existingRegistry := Registry{
			Name: "test-registry",
			Git: &GitAddonSource{
				URL:            "http://github.com/test/repo",
				TokenSecretRef: "addon-registry-test-registry",
			},
		}
		registries := map[string]Registry{"test-registry": existingRegistry}
		registriesBytes, err := json.Marshal(registries)
		assert.NoError(t, err)
		cm := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      registryConfigMapName,
				Namespace: velatypes.DefaultKubeVelaNS,
			},
			Data: map[string]string{
				registriesKey: string(registriesBytes),
			},
		}
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "addon-registry-test-registry",
				Namespace: velatypes.DefaultKubeVelaNS,
			},
			Data: map[string][]byte{
				"token": []byte("test-token"),
			},
		}

		scheme := runtime.NewScheme()
		assert.NoError(t, v1.AddToScheme(scheme))
		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm, secret).Build()
		ds := NewRegistryDataStore(client)

		err = ds.UpdateRegistry(ctx, updatedRegistry)
		assert.NoError(t, err)

		var updatedSecret v1.Secret
		err = client.Get(ctx, types.NamespacedName{Name: "addon-registry-test-registry", Namespace: velatypes.DefaultKubeVelaNS}, &updatedSecret)
		assert.NoError(t, err)
		assert.Equal(t, "test-token-updated", string(updatedSecret.Data["token"]))

		var updatedCm v1.ConfigMap
		err = client.Get(ctx, types.NamespacedName{Name: registryConfigMapName, Namespace: velatypes.DefaultKubeVelaNS}, &updatedCm)
		assert.NoError(t, err)
		var updatedRegistries map[string]Registry
		err = json.Unmarshal([]byte(updatedCm.Data[registriesKey]), &updatedRegistries)
		assert.NoError(t, err)
		assert.Equal(t, "http://github.com/test/repo-updated", updatedRegistries["test-registry"].Git.URL)
	})
}

func TestListRegistry(t *testing.T) {
	t.Run("Test listing registries", func(t *testing.T) {
		ctx := context.Background()
		// Pre-existing state
		existingRegistry := Registry{
			Name: "test-registry",
			Git: &GitAddonSource{
				URL:            "http://github.com/test/repo",
				TokenSecretRef: "addon-registry-test-registry",
			},
		}
		registries := map[string]Registry{"test-registry": existingRegistry}
		registriesBytes, err := json.Marshal(registries)
		assert.NoError(t, err)
		cm := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      registryConfigMapName,
				Namespace: velatypes.DefaultKubeVelaNS,
			},
			Data: map[string]string{
				registriesKey: string(registriesBytes),
			},
		}
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "addon-registry-test-registry",
				Namespace: velatypes.DefaultKubeVelaNS,
			},
			Data: map[string][]byte{
				"token": []byte("test-token"),
			},
		}

		scheme := runtime.NewScheme()
		assert.NoError(t, v1.AddToScheme(scheme))
		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm, secret).Build()
		ds := NewRegistryDataStore(client)

		// Test List
		listedRegistries, err := ds.ListRegistries(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(listedRegistries))
		assert.Equal(t, "test-token", listedRegistries[0].Git.Token)
		assert.Equal(t, "http://github.com/test/repo", listedRegistries[0].Git.URL)
	})
}

func TestGetRegistry(t *testing.T) {
	t.Run("Test getting a single registry", func(t *testing.T) {
		ctx := context.Background()
		// Pre-existing state
		existingRegistry := Registry{
			Name: "test-registry",
			Git: &GitAddonSource{
				URL:            "http://github.com/test/repo",
				TokenSecretRef: "addon-registry-test-registry",
			},
		}
		registries := map[string]Registry{"test-registry": existingRegistry}
		registriesBytes, err := json.Marshal(registries)
		assert.NoError(t, err)
		cm := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      registryConfigMapName,
				Namespace: velatypes.DefaultKubeVelaNS,
			},
			Data: map[string]string{
				registriesKey: string(registriesBytes),
			},
		}
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "addon-registry-test-registry",
				Namespace: velatypes.DefaultKubeVelaNS,
			},
			Data: map[string][]byte{
				"token": []byte("test-token"),
			},
		}

		scheme := runtime.NewScheme()
		assert.NoError(t, v1.AddToScheme(scheme))
		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm, secret).Build()
		ds := NewRegistryDataStore(client)

		// Test Get
		reg, err := ds.GetRegistry(ctx, "test-registry")
		assert.NoError(t, err)
		assert.Equal(t, "test-token", reg.Git.Token)
		assert.Equal(t, "http://github.com/test/repo", reg.Git.URL)
	})
}

func TestDeleteRegistry(t *testing.T) {
	t.Run("Test deleting a registry", func(t *testing.T) {
		ctx := context.Background()
		// Pre-existing state
		existingRegistry := Registry{
			Name: "test-registry",
			Git: &GitAddonSource{
				URL:            "http://github.com/test/repo",
				TokenSecretRef: "addon-registry-test-registry",
			},
		}
		registries := map[string]Registry{"test-registry": existingRegistry}
		registriesBytes, err := json.Marshal(registries)
		assert.NoError(t, err)
		cm := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      registryConfigMapName,
				Namespace: velatypes.DefaultKubeVelaNS,
			},
			Data: map[string]string{
				registriesKey: string(registriesBytes),
			},
		}
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "addon-registry-test-registry",
				Namespace: velatypes.DefaultKubeVelaNS,
			},
			Data: map[string][]byte{
				"token": []byte("test-token"),
			},
		}

		scheme := runtime.NewScheme()
		assert.NoError(t, v1.AddToScheme(scheme))
		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm, secret).Build()
		ds := NewRegistryDataStore(client)

		err = ds.DeleteRegistry(ctx, "test-registry")
		assert.NoError(t, err)

		var updatedCm v1.ConfigMap
		err = client.Get(ctx, types.NamespacedName{Name: registryConfigMapName, Namespace: velatypes.DefaultKubeVelaNS}, &updatedCm)
		assert.NoError(t, err)
		var updatedRegistries map[string]Registry
		err = json.Unmarshal([]byte(updatedCm.Data[registriesKey]), &updatedRegistries)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(updatedRegistries))

		var deletedSecret v1.Secret
		err = client.Get(ctx, types.NamespacedName{Name: "addon-registry-test-registry", Namespace: velatypes.DefaultKubeVelaNS}, &deletedSecret)
		assert.Error(t, err)
		assert.True(t, apierrors.IsNotFound(err))
	})
}
