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

package system

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func TestCreateIfNotExist(t *testing.T) {
	testCases := []struct {
		name        string
		setup       func(t *testing.T) string
		wantExisted bool
		wantErr     bool
	}{
		{
			name: "directory does not exist",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "new-dir")
			},
			wantExisted: false,
			wantErr:     false,
		},
		{
			name: "directory already exists",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			wantExisted: true,
			wantErr:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path := tc.setup(t)
			existed, err := CreateIfNotExist(path)

			assert.Equal(t, tc.wantExisted, existed)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				_, statErr := os.Stat(path)
				assert.NoError(t, statErr)
			}
		})
	}
}

func TestGetVelaHomeDir(t *testing.T) {
	testCases := []struct {
		name    string
		setup   func(t *testing.T) (expectedPath string, cleanup func())
		wantErr bool
	}{
		{
			name: "from VELA_HOME env var",
			setup: func(t *testing.T) (string, func()) {
				tmpDir := t.TempDir()
				t.Setenv(VelaHomeEnv, tmpDir)
				return tmpDir, func() {}
			},
			wantErr: false,
		},
		{
			name: "from default user home",
			setup: func(t *testing.T) (string, func()) {
				tmpHome := t.TempDir()
				t.Setenv("HOME", tmpHome)
				t.Setenv(VelaHomeEnv, "") // Ensure VELA_HOME is not set, so it falls back to HOME
				expectedPath := filepath.Join(tmpHome, defaultVelaHome)
				return expectedPath, func() {} // t.TempDir() handles cleanup
			},
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			expectedPath, cleanup := tc.setup(t)
			defer cleanup()

			got, err := GetVelaHomeDir()

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, expectedPath, got)
				_, statErr := os.Stat(got)
				assert.NoError(t, statErr)
			}
		})
	}
}

func TestGetDirectoryFunctions(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(VelaHomeEnv, tmpDir)

	testCases := []struct {
		name         string
		getFunc      func() (string, error)
		expectedPath string
	}{
		{"GetCapCenterDir", GetCapCenterDir, filepath.Join(tmpDir, "centers")},
		{"GetRepoConfig", GetRepoConfig, filepath.Join(tmpDir, "centers", "config.yaml")},
		{"GetCapabilityDir", GetCapabilityDir, filepath.Join(tmpDir, "capabilities")},
		{"GetCurrentEnvPath", GetCurrentEnvPath, filepath.Join(tmpDir, "curenv")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.getFunc()
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedPath, got)
		})
	}
}

func TestInitFunctions(t *testing.T) {
	testCases := []struct {
		name       string
		initFunc   func() error
		verifyFunc func(t *testing.T, velaHome string)
	}{
		{
			name:     "InitCapabilityDir",
			initFunc: InitCapabilityDir,
			verifyFunc: func(t *testing.T, velaHome string) {
				dir := filepath.Join(velaHome, "capabilities")
				_, err := os.Stat(dir)
				assert.NoError(t, err)
			},
		},
		{
			name:     "InitCapCenterDir",
			initFunc: InitCapCenterDir,
			verifyFunc: func(t *testing.T, velaHome string) {
				dir := filepath.Join(velaHome, "centers", ".tmp")
				_, err := os.Stat(dir)
				assert.NoError(t, err)
			},
		},
		{
			name:     "InitDirs",
			initFunc: InitDirs,
			verifyFunc: func(t *testing.T, velaHome string) {
				capDir := filepath.Join(velaHome, "capabilities")
				centerDir := filepath.Join(velaHome, "centers", ".tmp")
				_, err := os.Stat(capDir)
				assert.NoError(t, err)
				_, err = os.Stat(centerDir)
				assert.NoError(t, err)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			velaHome := t.TempDir()
			t.Setenv(VelaHomeEnv, velaHome)

			err := tc.initFunc()
			assert.NoError(t, err)
			tc.verifyFunc(t, velaHome)
		})
	}
}

func TestBindEnvironmentVariables(t *testing.T) {
	originalVelaNS := types.DefaultKubeVelaNS
	originalDefNS := oam.SystemDefinitionNamespace
	defer func() {
		types.DefaultKubeVelaNS = originalVelaNS
		oam.SystemDefinitionNamespace = originalDefNS
	}()

	testCases := []struct {
		name       string
		setup      func(t *testing.T)
		wantVelaNS string
		wantDefNS  string
	}{
		{
			name: "primary env vars",
			setup: func(t *testing.T) {
				t.Setenv(KubeVelaSystemNamespaceEnv, "vela-system-primary")
				t.Setenv(KubeVelaDefinitionNamespaceEnv, "vela-def-primary")
				t.Setenv(LegacyKubeVelaSystemNamespaceEnv, "vela-system-legacy")
			},
			wantVelaNS: "vela-system-primary",
			wantDefNS:  "vela-def-primary",
		},
		{
			name: "legacy fallback for vela ns",
			setup: func(t *testing.T) {
				t.Setenv(KubeVelaSystemNamespaceEnv, "")
				t.Setenv(LegacyKubeVelaSystemNamespaceEnv, "vela-system-legacy")
				t.Setenv(KubeVelaDefinitionNamespaceEnv, "vela-def-primary")
			},
			wantVelaNS: "vela-system-legacy",
			wantDefNS:  "vela-def-primary",
		},
		{
			name: "system ns fallback for definition ns",
			setup: func(t *testing.T) {
				t.Setenv(KubeVelaSystemNamespaceEnv, "vela-system-fallback")
				t.Setenv(KubeVelaDefinitionNamespaceEnv, "")
			},
			wantVelaNS: "vela-system-fallback",
			wantDefNS:  "vela-system-fallback",
		},
		{
			name: "no env vars set",
			setup: func(t *testing.T) {
				t.Setenv(KubeVelaSystemNamespaceEnv, "")
				t.Setenv(LegacyKubeVelaSystemNamespaceEnv, "")
				t.Setenv(KubeVelaDefinitionNamespaceEnv, "")
			},
			wantVelaNS: "default-vela",
			wantDefNS:  "default-def",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset to known state before each test
			if tc.name == "no env vars set" {
				types.DefaultKubeVelaNS = "default-vela"
				oam.SystemDefinitionNamespace = "default-def"
			} else {
				types.DefaultKubeVelaNS = originalVelaNS
				oam.SystemDefinitionNamespace = originalDefNS
			}

			tc.setup(t)
			BindEnvironmentVariables()

			assert.Equal(t, tc.wantVelaNS, types.DefaultKubeVelaNS)
			assert.Equal(t, tc.wantDefNS, oam.SystemDefinitionNamespace)
		})
	}
}
