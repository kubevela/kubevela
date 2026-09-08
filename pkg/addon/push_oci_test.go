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
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestIsDirectAddonPushTarget(t *testing.T) {
	assert.True(t, IsDirectAddonPushTarget("oci://registry.example.com/addons"))
	assert.True(t, IsDirectAddonPushTarget("https://charts.example.com"))
	assert.True(t, IsDirectAddonPushTarget("http://localhost:8080"))
	assert.False(t, IsDirectAddonPushTarget("configured-registry"))
}

func TestPushCmdRoutesDirectOCI(t *testing.T) {
	var captured *HelmSource
	var capturedPlainHTTP bool
	p := &PushCmd{
		RepoName:  "oci://registry.example.com/addons",
		Username:  "robot",
		Password:  "secret",
		ChartName: "unused-by-test-seam",
		UseHTTP:   true,
		ociPushFn: func(_ context.Context, source *HelmSource, plainHTTP bool) error {
			captured = source
			capturedPlainHTTP = plainHTTP
			return nil
		},
	}

	require.NoError(t, p.Push(context.Background()))
	require.NotNil(t, captured)
	assert.Equal(t, "oci://registry.example.com/addons", captured.URL)
	assert.Equal(t, "robot", captured.Username)
	assert.Equal(t, "secret", captured.Token)
	assert.True(t, capturedPlainHTTP)
}

func TestPushCmdRoutesConfiguredOCI(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	store := NewRegistryDataStore(kubeClient)
	require.NoError(t, store.AddRegistry(context.Background(), Registry{
		Name: "private",
		Helm: &HelmSource{
			URL:      "oci://registry.example.com/addons",
			Username: "stored-user",
			Token:    "stored-password",
		},
	}))

	t.Run("stored credentials", func(t *testing.T) {
		var captured *HelmSource
		p := &PushCmd{
			RepoName:  "private",
			Client:    kubeClient,
			ChartName: "unused-by-test-seam",
			ociPushFn: func(_ context.Context, source *HelmSource, _ bool) error {
				captured = source
				return nil
			},
		}

		require.NoError(t, p.Push(context.Background()))
		require.NotNil(t, captured, "a Helm block with an oci:// URL must take the OCI route")
		assert.Equal(t, "oci://registry.example.com/addons", captured.URL)
		assert.Equal(t, "stored-user", captured.Username)
		assert.Equal(t, "stored-password", captured.Token)
	})

	t.Run("command line credentials override stored credentials", func(t *testing.T) {
		var captured *HelmSource
		p := &PushCmd{
			RepoName:  "private",
			Client:    kubeClient,
			ChartName: "unused-by-test-seam",
			Username:  "override-user",
			Password:  "override-password",
			ociPushFn: func(_ context.Context, source *HelmSource, _ bool) error {
				captured = source
				return nil
			},
		}

		require.NoError(t, p.Push(context.Background()))
		require.NotNil(t, captured)
		assert.Equal(t, "override-user", captured.Username)
		assert.Equal(t, "override-password", captured.Token)
	})

	// Rotating a registry's password (e.g. a freshly minted ECR token piped via
	// --password-stdin) should not require re-supplying the stored username: the
	// merged credential pair is still complete, so this must not be rejected as
	// ambiguous partial authentication.
	t.Run("password-only override rotates stored credential without a username flag", func(t *testing.T) {
		var captured *HelmSource
		p := &PushCmd{
			RepoName:  "private",
			Client:    kubeClient,
			ChartName: "unused-by-test-seam",
			Password:  "rotated-password",
			ociPushFn: func(_ context.Context, source *HelmSource, _ bool) error {
				captured = source
				return nil
			},
		}

		require.NoError(t, p.Push(context.Background()))
		require.NotNil(t, captured)
		assert.Equal(t, "stored-user", captured.Username)
		assert.Equal(t, "rotated-password", captured.Token)
	})
}

func TestPushCmdRejectsOCIAuthAmbiguity(t *testing.T) {
	tests := map[string]PushCmd{
		"ChartMuseum access token": {
			RepoName:    "oci://registry.example.com/addons",
			AccessToken: "bearer-token",
		},
		"username without password": {
			RepoName: "oci://registry.example.com/addons",
			Username: "user",
		},
		"password without username": {
			RepoName: "oci://registry.example.com/addons",
			Password: "password",
		},
		"ChartMuseum auth header": {
			RepoName:   "oci://registry.example.com/addons",
			AuthHeader: "Bearer xxx",
		},
		"ChartMuseum CA file": {
			RepoName: "oci://registry.example.com/addons",
			CaFile:   "/ca.pem",
		},
		"ChartMuseum client cert": {
			RepoName: "oci://registry.example.com/addons",
			CertFile: "/cert.pem",
			KeyFile:  "/key.pem",
		},
		"ChartMuseum insecure flag": {
			RepoName:           "oci://registry.example.com/addons",
			InsecureSkipVerify: true,
		},
	}

	for name, pushCmd := range tests {
		t.Run(name, func(t *testing.T) {
			pushCmd.ChartName = "unused-by-test-seam"
			pushCmd.ociPushFn = func(_ context.Context, _ *HelmSource, _ bool) error {
				t.Fatal("OCI push must not run when authentication is invalid")
				return nil
			}

			err := pushCmd.Push(context.Background())
			require.Error(t, err)
		})
	}
}

func TestGetHelmRepoSkipsOCIRegistries(t *testing.T) {
	// ChartMuseum cannot speak to an OCI registry. Handing it one would fail deep
	// inside the upload with a confusing error instead of here.
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	store := NewRegistryDataStore(kubeClient)
	require.NoError(t, store.AddRegistry(context.Background(), Registry{
		Name: "helm-oci",
		Helm: &HelmSource{URL: "oci://registry.example.com/addons"},
	}))

	_, err := GetHelmRepo(context.Background(), kubeClient, "helm-oci")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "oci://", "the OCI URL must not reach the ChartMuseum client")
}

// TestPushOCI exercises the real pushOCI implementation (the existing
// push_oci_test.go tests only cover routing via the ociPushFn test seam, never
// pushOCI itself). A closed loopback port makes the final network push fail
// fast and deterministically, so the test still walks the whole function --
// chart loading, packaging, and building the OCI ref -- without depending on
// a live registry.
func TestPushOCI(t *testing.T) {
	t.Run("bad chart path fails before contacting the registry", func(t *testing.T) {
		p := &PushCmd{ChartName: "/this/this/not/a/chart"}
		err := p.pushOCI(&HelmSource{URL: "oci://127.0.0.1:1/addon"})
		require.Error(t, err)
	})

	t.Run("valid chart pushes to an unreachable registry and returns a wrapped error", func(t *testing.T) {
		var out bytes.Buffer
		p := &PushCmd{
			ChartName:    "testdata/charts/sample-1.0.1.tgz",
			ChartVersion: "9.9.9",
			AppVersion:   "9.9.9",
			Out:          &out,
		}

		err := p.pushOCI(&HelmSource{URL: "oci://127.0.0.1:1/addon"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to push OCI addon")
		assert.Contains(t, out.String(), "Pushing")
	})
}
