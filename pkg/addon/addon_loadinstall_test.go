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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestLoadInstallPackage covers the branch loadInstallPackage now takes: OCI
// and Helm registries are resolved through ToVersionedRegistry, everything
// else keeps going through the registry-meta + reader path.
func TestLoadInstallPackage(t *testing.T) {
	t.Run("OCI registry takes the versioned path and fails fast against an unreachable host", func(t *testing.T) {
		h := &Installer{
			ctx: context.Background(),
			r:   &Registry{Name: "ecr", OCI: &OCIAddonSource{URL: "oci://127.0.0.1:1/addon"}},
		}
		_, err := h.loadInstallPackage("fluxcd", "1.0.0")
		require.Error(t, err)
	})

	t.Run("non-versioned registry with no source info fails getting addon meta", func(t *testing.T) {
		h := &Installer{
			ctx:   context.Background(),
			r:     &Registry{Name: "bare"},
			cache: NewCache(nil),
		}
		_, err := h.loadInstallPackage("fluxcd", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "fail to get addon meta")
	})
}

// TestInstallDependencyListInstalledAddonsError covers the first step past the
// no-dependencies early return: a listInstalledAddons failure for an addon
// that does declare a dependency must be propagated as-is.
func TestInstallDependencyListInstalledAddonsError(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	h := &Installer{ctx: context.Background(), cli: kubeClient}
	addonPkg := &InstallPackage{Meta: Meta{Name: "demo", Dependencies: []*Dependency{{Name: "dep-addon"}}}}

	err := h.installDependency(context.Background(), addonPkg)
	require.Error(t, err)
}
