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

package addon

import (
	"context"
	"errors"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestValidateSystemRequirements(t *testing.T) {
	t.Run("nil requirement always passes without touching the cluster", func(t *testing.T) {
		assert.NoError(t, ValidateSystemRequirements(context.Background(), nil, nil, nil))
	})

	t.Run("non-nil requirement delegates to checkAddonVersionMeetRequired", func(t *testing.T) {
		listErr := errors.New("boom")
		k8sClient := &test.MockClient{MockList: test.NewMockListFn(listErr)}

		err := ValidateSystemRequirements(context.Background(), &SystemRequirements{VelaVersion: ">=1.0.0"}, k8sClient, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, listErr)
	})
}

func TestGetAddonInstallPackageFromRegistry(t *testing.T) {
	newFakeClient := func() *fake.ClientBuilder {
		scheme := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(scheme))
		return fake.NewClientBuilder().WithScheme(scheme)
	}

	t.Run("registry not found is wrapped with its name", func(t *testing.T) {
		kubeClient := newFakeClient().Build()
		_, err := GetAddonInstallPackageFromRegistry(context.Background(), kubeClient, "missing-registry", "fluxcd", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `get registry "missing-registry"`)
	})

	t.Run("OCI registry resolves through the versioned OCI path and fails fast against an unreachable host", func(t *testing.T) {
		kubeClient := newFakeClient().Build()
		store := NewRegistryDataStore(kubeClient)
		require.NoError(t, store.AddRegistry(context.Background(), Registry{
			Name: "ecr",
			OCI:  &OCIAddonSource{URL: "oci://127.0.0.1:1/addon"},
		}))

		_, err := GetAddonInstallPackageFromRegistry(context.Background(), kubeClient, "ecr", "fluxcd", "1.0.0")
		require.Error(t, err)
	})

	t.Run("Helm registry resolves through the versioned Helm path and fails fast against an unreachable host", func(t *testing.T) {
		kubeClient := newFakeClient().Build()
		store := NewRegistryDataStore(kubeClient)
		require.NoError(t, store.AddRegistry(context.Background(), Registry{
			Name: "helm-reg",
			Helm: &HelmSource{URL: "http://127.0.0.1:1"},
		}))

		_, err := GetAddonInstallPackageFromRegistry(context.Background(), kubeClient, "helm-reg", "fluxcd", "1.0.0")
		require.Error(t, err)
	})

	t.Run("registry with no source info fails building a reader for the fallback path", func(t *testing.T) {
		kubeClient := newFakeClient().Build()
		store := NewRegistryDataStore(kubeClient)
		require.NoError(t, store.AddRegistry(context.Background(), Registry{Name: "bare"}))

		_, err := GetAddonInstallPackageFromRegistry(context.Background(), kubeClient, "bare", "fluxcd", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "enough info")
	})
}
