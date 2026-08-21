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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIsVersionCapableRegistry pins the classification the cache uses to choose
// between a VersionedRegistry and an AsyncReader. BuildReader has no OCI branch,
// so an OCI registry taking the non-versioned path fails outright.
func TestIsVersionCapableRegistry(t *testing.T) {
	cases := map[string]struct {
		registry Registry
		want     bool
	}{
		"helm registry is version capable": {
			registry: Registry{Name: "helm", Helm: &HelmSource{URL: "https://example.com"}},
			want:     true,
		},
		"oci registry is version capable": {
			registry: Registry{Name: "oci", OCI: &OCIAddonSource{URL: "oci://example.com/addon"}},
			want:     true,
		},
		"git registry is not": {
			registry: Registry{Name: "git", Git: &GitAddonSource{URL: "https://github.com/x/y"}},
			want:     false,
		},
		"oss registry is not": {
			registry: Registry{Name: "oss", OSS: &OSSAddonSource{Endpoint: "https://oss.example.com"}},
			want:     false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, isVersionCapableRegistry(tc.registry))
		})
	}
}

// TestOCIRegistryReachesTheVersionedCachePath is the regression guard for the
// actual defect: an OCI registry used to fall through to BuildReader, which has
// no OCI branch and fails with "registry don't have enough info to build a
// reader". It must now resolve to a VersionedRegistry instead.
func TestOCIRegistryReachesTheVersionedCachePath(t *testing.T) {
	oci := Registry{Name: "ecr", OCI: &OCIAddonSource{URL: "oci://registry.example.com/addon"}}

	_, err := oci.BuildReader()
	require.Error(t, err, "BuildReader still has no OCI branch; the cache must not send OCI there")
	assert.Contains(t, err.Error(), "don't have enough info")

	vr, err := ToVersionedRegistry(oci)
	require.NoError(t, err, "OCI must resolve to a versioned registry")
	assert.NotNil(t, vr)

	assert.True(t, isVersionCapableRegistry(oci),
		"the cache branches must classify OCI as version capable so it never reaches BuildReader")
}
