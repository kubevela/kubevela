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
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetHelmRepo(t *testing.T) {
	t.Run("direct URL creates a temp repo", func(t *testing.T) {
		repo, err := GetHelmRepo(context.Background(), nil, "https://charts.example.com")
		require.NoError(t, err)
		require.NotNil(t, repo)
		assert.Equal(t, "https://charts.example.com", repo.Config.URL)
	})

	t.Run("lookup by configured addon registry name", func(t *testing.T) {
		scheme := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(scheme))
		kubeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		store := NewRegistryDataStore(kubeClient)
		require.NoError(t, store.AddRegistry(context.Background(), Registry{
			Name: "helm-private",
			Helm: &HelmSource{URL: "https://charts.internal.example", Username: "u", Password: "p"},
		}))

		repo, err := GetHelmRepo(context.Background(), kubeClient, "helm-private")
		require.NoError(t, err)
		require.NotNil(t, repo)
		assert.Equal(t, "helm-private", repo.Config.Name)
		assert.Equal(t, "https://charts.internal.example", repo.Config.URL)
		assert.Equal(t, "u", repo.Config.Username)
		assert.Equal(t, "p", repo.Config.Password)
	})

	t.Run("name not found returns a clear error", func(t *testing.T) {
		scheme := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(scheme))
		kubeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		_, err := GetHelmRepo(context.Background(), kubeClient, "missing")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot find Helm repository")
	})
}

func TestChartMuseumResponseHelpers(t *testing.T) {
	t.Run("success status codes are accepted", func(t *testing.T) {
		for _, code := range []int{http.StatusCreated, http.StatusAccepted} {
			resp := &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader("{}"))}
			assert.NoError(t, handlePushResponse(resp))
		}
	})

	t.Run("failure response returns parsed chartmuseum error", func(t *testing.T) {
		resp := &http.Response{StatusCode: http.StatusConflict, Body: io.NopCloser(strings.NewReader(`{"error":"already exists"}`))}
		err := handlePushResponse(resp)
		require.Error(t, err)
		assert.Equal(t, "409: already exists", err.Error())
	})

	t.Run("invalid JSON body returns parse error", func(t *testing.T) {
		err := getChartMuseumError([]byte("not-json"), http.StatusInternalServerError)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "could not properly parse response JSON")
	})
}

func TestFormatRepoNameAndURL(t *testing.T) {
	formatted := formatRepoNameAndURL("helm-private", "https://charts.internal.example")
	assert.Contains(t, formatted, "helm-private")
	assert.Contains(t, formatted, "https://charts.internal.example")

	formatted = formatRepoNameAndURL("https://charts.internal.example", "https://charts.internal.example")
	assert.Contains(t, formatted, "https://charts.internal.example")
}
