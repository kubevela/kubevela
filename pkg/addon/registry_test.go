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
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func TestGetTokenSource(t *testing.T) {
	gitSource := &GitAddonSource{URL: "https://github.com/kubevela/catalog.git"}
	giteeSource := &GiteeAddonSource{URL: "https://gitee.com/kubevela/catalog.git"}
	gitlabSource := &GitlabAddonSource{URL: "https://gitlab.com/kubevela/catalog.git"}

	testCases := []struct {
		name           string
		registry       *Registry
		expectedSource TokenSource
	}{
		{
			name: "git source",
			registry: &Registry{
				Git: gitSource,
			},
			expectedSource: gitSource,
		},
		{
			name: "gitee source",
			registry: &Registry{
				Gitee: giteeSource,
			},
			expectedSource: giteeSource,
		},
		{
			name: "gitlab source",
			registry: &Registry{
				Gitlab: gitlabSource,
			},
			expectedSource: gitlabSource,
		},
		{
			name: "git and gitee source",
			registry: &Registry{
				Git:   gitSource,
				Gitee: giteeSource,
			},
			expectedSource: gitSource,
		},
		{
			name: "gitee and gitlab source",
			registry: &Registry{
				Gitee:  giteeSource,
				Gitlab: gitlabSource,
			},
			expectedSource: giteeSource,
		},
		{
			name: "all token sources",
			registry: &Registry{
				Git:    gitSource,
				Gitee:  giteeSource,
				Gitlab: gitlabSource,
			},
			expectedSource: gitSource,
		},
		{
			name: "no token source",
			registry: &Registry{
				Helm: &HelmSource{},
			},
			expectedSource: nil,
		},
		{
			name:           "empty registry",
			registry:       &Registry{},
			expectedSource: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			source := tc.registry.GetTokenSource()
			assert.Equal(t, tc.expectedSource, source)
		})
	}
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

func TestGetRegistries(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	assert.NoError(t, v1.AddToScheme(scheme))

	// valid configmap with one registry
	validRegistries := map[string]Registry{"test-registry": {Name: "test-registry"}}
	validRegistriesBytes, err := json.Marshal(validRegistries)
	assert.NoError(t, err)
	validCm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: registryConfigMapName, Namespace: velatypes.DefaultKubeVelaNS},
		Data:       map[string]string{registriesKey: string(validRegistriesBytes)},
	}

	// configmap with invalid json
	invalidJSONCm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: registryConfigMapName, Namespace: velatypes.DefaultKubeVelaNS},
		Data:       map[string]string{registriesKey: "invalid-json"},
	}

	// configmap with missing key
	missingKeyCm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: registryConfigMapName, Namespace: velatypes.DefaultKubeVelaNS},
		Data:       map[string]string{"another-key": "some-data"},
	}

	testCases := map[string]struct {
		client       client.Client
		expectErr    bool
		expectRegNum int
	}{
		"success": {
			client:       fake.NewClientBuilder().WithScheme(scheme).WithObjects(validCm).Build(),
			expectErr:    false,
			expectRegNum: 1,
		},
		"configmap not found": {
			client:    fake.NewClientBuilder().WithScheme(scheme).Build(),
			expectErr: true,
		},
		"invalid json": {
			client:    fake.NewClientBuilder().WithScheme(scheme).WithObjects(invalidJSONCm).Build(),
			expectErr: true,
		},
		"registries key missing": {
			client:    fake.NewClientBuilder().WithScheme(scheme).WithObjects(missingKeyCm).Build(),
			expectErr: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ds := registryImpl{client: tc.client}
			registries, _, err := ds.getRegistries(ctx)
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectRegNum, len(registries))
			}
		})
	}
}

func TestLoadTokenFromSecret(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	assert.NoError(t, v1.AddToScheme(scheme))

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "addon-registry-test", Namespace: velatypes.DefaultKubeVelaNS},
		Data:       map[string][]byte{"token": []byte("test-token")},
	}

	testCases := map[string]struct {
		client      client.Client
		registry    *Registry
		expectErr   bool
		expectToken string
	}{
		"success": {
			client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build(),
			registry: &Registry{
				Name: "test",
				Git:  &GitAddonSource{URL: "http://github.com/test/repo", TokenSecretRef: "addon-registry-test"},
			},
			expectErr:   false,
			expectToken: "test-token",
		},
		"secret not found": {
			client: fake.NewClientBuilder().WithScheme(scheme).Build(),
			registry: &Registry{
				Name: "test",
				Git:  &GitAddonSource{URL: "http://github.com/test/repo", TokenSecretRef: "addon-registry-test"},
			},
			expectErr:   false,
			expectToken: "",
		},
		"no token source": {
			client:      fake.NewClientBuilder().WithScheme(scheme).Build(),
			registry:    &Registry{Name: "test"},
			expectErr:   false,
			expectToken: "",
		},
		"no secret ref": {
			client:      fake.NewClientBuilder().WithScheme(scheme).Build(),
			registry:    &Registry{Name: "test", Git: &GitAddonSource{URL: "http://github.com/test/repo"}},
			expectErr:   false,
			expectToken: "",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			err := loadTokenFromSecret(ctx, tc.client, tc.registry)
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tc.registry.Git != nil {
					assert.Equal(t, tc.expectToken, tc.registry.Git.Token)
				}
			}
		})
	}
}

func TestCreateOrUpdateTokenSecret(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	assert.NoError(t, v1.AddToScheme(scheme))

	existingSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "addon-registry-test", Namespace: velatypes.DefaultKubeVelaNS},
		Data:       map[string][]byte{"token": []byte("old-token")},
	}

	testCases := map[string]struct {
		client       client.Client
		registry     *Registry
		expectErr    bool
		expectToken  string
		expectSecret bool
	}{
		"create new secret": {
			client: fake.NewClientBuilder().WithScheme(scheme).Build(),
			registry: &Registry{
				Name: "test",
				Git:  &GitAddonSource{Token: "new-token"},
			},
			expectErr:    false,
			expectToken:  "new-token",
			expectSecret: true,
		},
		"update existing secret": {
			client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingSecret).Build(),
			registry: &Registry{
				Name: "test",
				Git:  &GitAddonSource{Token: "updated-token"},
			},
			expectErr:    false,
			expectToken:  "updated-token",
			expectSecret: true,
		},
		"no token source": {
			client:       fake.NewClientBuilder().WithScheme(scheme).Build(),
			registry:     &Registry{Name: "test"},
			expectErr:    false,
			expectSecret: false,
		},
		"empty token": {
			client:       fake.NewClientBuilder().WithScheme(scheme).Build(),
			registry:     &Registry{Name: "test", Git: &GitAddonSource{Token: ""}},
			expectErr:    false,
			expectSecret: false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			err := createOrUpdateTokenSecret(ctx, tc.client, tc.registry)
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tc.expectSecret {
					var secret v1.Secret
					err := tc.client.Get(ctx, types.NamespacedName{Name: "addon-registry-test", Namespace: velatypes.DefaultKubeVelaNS}, &secret)
					assert.NoError(t, err)
					assert.Equal(t, tc.expectToken, string(secret.Data["token"]))
					assert.Equal(t, "addon-registry-test", tc.registry.GetTokenSource().GetTokenSecretRef())
				} else {
					var secret v1.Secret
					err := tc.client.Get(ctx, types.NamespacedName{Name: "addon-registry-test", Namespace: velatypes.DefaultKubeVelaNS}, &secret)
					assert.True(t, apierrors.IsNotFound(err))
				}
			}
		})
	}
}
