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
	"fmt"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsSkippableRegistryErrorClassification(t *testing.T) {
	cases := map[string]struct {
		err  error
		want bool
	}{
		"ErrNotExist":                 {err: ErrNotExist, want: true},
		"wrapped ErrNotExist":         {err: fmt.Errorf("wrap: %w", ErrNotExist), want: true},
		"ErrFetch":                    {err: ErrFetch, want: true},
		"wrapped ErrFetch":            {err: errors.Wrap(ErrFetch, "OCI registry ecr"), want: true},
		"ErrOCICatalogAbsent":         {err: ErrOCICatalogAbsent, want: true},
		"wrapped ErrOCICatalogAbsent": {err: errors.Wrap(ErrOCICatalogAbsent, "no tags"), want: true},
		"auth error is not skippable": {err: errors.New("401 unauthorized"), want: false},
		"nil is not skippable":        {err: nil, want: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := isSkippableRegistryError(tc.err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// stubInfoLister implements ItemInfoLister for tests.
type stubInfoLister struct {
	addons map[string]ItemInfo
	err    error
}

func (s *stubInfoLister) ListAddonInfo() (map[string]ItemInfo, error) {
	return s.addons, s.err
}

func TestListAvailableAddonsSkipsSkippableErrors(t *testing.T) {
	good := &stubInfoLister{
		addons: map[string]ItemInfo{
			"fluxcd": {Name: "fluxcd", AvailableVersions: []string{"1.0.0"}},
		},
	}

	t.Run("skippable error is skipped, good registry still counted", func(t *testing.T) {
		broken := &stubInfoLister{err: ErrNotExist}
		result, err := listAvailableAddons([]ItemInfoLister{broken, good})
		require.NoError(t, err)
		assert.Contains(t, result, "fluxcd")
	})

	t.Run("ErrFetch is skipped", func(t *testing.T) {
		broken := &stubInfoLister{err: errors.Wrap(ErrFetch, "OCI registry ecr")}
		result, err := listAvailableAddons([]ItemInfoLister{broken, good})
		require.NoError(t, err)
		assert.Contains(t, result, "fluxcd")
	})

	t.Run("non-skippable error stops the listing", func(t *testing.T) {
		broken := &stubInfoLister{err: errors.New("401 unauthorized")}
		_, err := listAvailableAddons([]ItemInfoLister{broken, good})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list addons")
	})

	t.Run("empty registries returns empty map", func(t *testing.T) {
		result, err := listAvailableAddons(nil)
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}

func TestVersionUnMatchError(t *testing.T) {
	t.Run("with available version", func(t *testing.T) {
		e := VersionUnMatchError{
			err:                      errors.New("vela too old"),
			addonName:                "fluxcd",
			userSelectedAddonVersion: "3.0.0",
			availableVersion:         "2.0.0",
		}
		assert.Contains(t, e.Error(), "fail to install 3.0.0 version of fluxcd")
		assert.Contains(t, e.Error(), "Install fluxcd(v2.0.0)")
		v, err := e.GetAvailableVersion()
		require.NoError(t, err)
		assert.Equal(t, "2.0.0", v)
	})

	t.Run("without available version", func(t *testing.T) {
		e := VersionUnMatchError{
			err:                      errors.New("vela too old"),
			addonName:                "fluxcd",
			userSelectedAddonVersion: "3.0.0",
		}
		assert.Contains(t, e.Error(), "fail to install 3.0.0 version of fluxcd")
		assert.NotContains(t, e.Error(), "Install fluxcd")
		_, err := e.GetAvailableVersion()
		require.Error(t, err)
	})
}

func TestToVersionedRegistryConversion(t *testing.T) {
	t.Run("OCI-only registry converts to OCI versioned registry", func(t *testing.T) {
		r := Registry{Name: "ecr", OCI: &OCIAddonSource{URL: "oci://reg.example.com/addon"}}
		vr, err := ToVersionedRegistry(r)
		require.NoError(t, err)
		assert.NotNil(t, vr)
	})

	t.Run("Helm registry converts to Helm versioned registry", func(t *testing.T) {
		r := Registry{Name: "helm", Helm: &HelmSource{URL: "https://charts.example.com"}}
		vr, err := ToVersionedRegistry(r)
		require.NoError(t, err)
		assert.NotNil(t, vr)
	})

	t.Run("Helm+OCI keeps Helm precedence", func(t *testing.T) {
		r := Registry{
			Name: "both",
			Helm: &HelmSource{URL: "https://charts.example.com"},
			OCI:  &OCIAddonSource{URL: "oci://reg.example.com/addon"},
		}
		vr, err := ToVersionedRegistry(r)
		require.NoError(t, err)
		assert.IsType(t, &versionedRegistry{}, vr, "Helm must take precedence over OCI")
	})

	t.Run("git-only registry is not versioned", func(t *testing.T) {
		r := Registry{Name: "git", Git: &GitAddonSource{URL: "https://github.com/x/y"}}
		_, err := ToVersionedRegistry(r)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a versioned registry")
	})
}

func TestSetFieldsFromEnv(t *testing.T) {
	envs := map[string]string{
		"HELM_REPO_USERNAME":     "envuser",
		"HELM_REPO_PASSWORD":     "envpass",
		"HELM_REPO_ACCESS_TOKEN": "envtoken",
		"HELM_REPO_AUTH_HEADER":  "Bearer xxx",
		"HELM_REPO_CONTEXT_PATH": "/ctx",
		"HELM_REPO_USE_HTTP":     "true",
		"HELM_REPO_CA_FILE":      "/ca.pem",
		"HELM_REPO_CERT_FILE":    "/cert.pem",
		"HELM_REPO_KEY_FILE":     "/key.pem",
		"HELM_REPO_INSECURE":     "true",
	}
	for k, v := range envs {
		t.Setenv(k, v)
	}

	p := &PushCmd{}
	p.SetFieldsFromEnv()
	assert.Equal(t, "envuser", p.Username)
	assert.Equal(t, "envpass", p.Password)
	assert.Equal(t, "envtoken", p.AccessToken)
	assert.Equal(t, "Bearer xxx", p.AuthHeader)
	assert.Equal(t, "/ctx", p.ContextPath)
	assert.True(t, p.UseHTTP)
	assert.Equal(t, "/ca.pem", p.CaFile)
	assert.Equal(t, "/cert.pem", p.CertFile)
	assert.Equal(t, "/key.pem", p.KeyFile)
	assert.True(t, p.InsecureSkipVerify)

	// When fields are pre-set, env vars do not override
	p2 := &PushCmd{Username: "explicit", Password: "explicit"}
	p2.SetFieldsFromEnv()
	assert.Equal(t, "explicit", p2.Username)
	assert.Equal(t, "explicit", p2.Password)
}
