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

// TestListAvailableAddonsWithNilRegistries pins the nil-slice edge case; the
// skippable-error and fatal-error branches are covered by
// TestListAvailableAddonsSkipsFailingRegistry, TestListAvailableAddonsPropagatesFatalError
// and TestListAvailableAddonsSkipsEveryUnreachableRegistry in addon_test.go.
func TestListAvailableAddonsWithNilRegistries(t *testing.T) {
	result, err := listAvailableAddons(nil)
	require.NoError(t, err)
	assert.Empty(t, result)
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
