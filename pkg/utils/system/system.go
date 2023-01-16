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

	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
)

const defaultVelaHome = ".vela"

const (
	// VelaHomeEnv defines vela home system env
	VelaHomeEnv = "VELA_HOME"
)

// GetVelaHomeDir return vela home dir
func GetVelaHomeDir() (string, error) {
	var velaHome string
	if custom := os.Getenv(VelaHomeEnv); custom != "" {
		velaHome = custom
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		velaHome = filepath.Join(home, defaultVelaHome)
	}
	if _, err := os.Stat(velaHome); err != nil && os.IsNotExist(err) {
		err := os.MkdirAll(velaHome, 0750)
		if err != nil {
			return "", errors.Wrap(err, "error when create vela home directory")
		}
	}
	return velaHome, nil
}

// GetDefaultFrontendDir return default vela frontend dir
func GetDefaultFrontendDir() (string, error) {
	home, err := GetVelaHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "frontend"), nil
}

// GetCapCenterDir return cap center dir
func GetCapCenterDir() (string, error) {
	home, err := GetVelaHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "centers"), nil
}

// GetRepoConfig return repo config
func GetRepoConfig() (string, error) {
	home, err := GetCapCenterDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "config.yaml"), nil
}

// GetCapabilityDir return capability dirs including workloads and traits
func GetCapabilityDir() (string, error) {
	home, err := GetVelaHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "capabilities"), nil
}

// GetCurrentEnvPath return current env config
func GetCurrentEnvPath() (string, error) {
	homedir, err := GetVelaHomeDir()
	if err != nil {
		return "", err
	}
	envPath := filepath.Join(homedir, "curenv")
	return envPath, nil
}

// InitDirs create dir if not exits
func InitDirs() error {
	if err := InitCapabilityDir(); err != nil {
		return err
	}
	if err := InitCapCenterDir(); err != nil {
		return err
	}
	return nil
}

// InitCapCenterDir create dir if not exits
func InitCapCenterDir() error {
	home, err := GetCapCenterDir()
	if err != nil {
		return err
	}
	_, err = CreateIfNotExist(filepath.Join(home, ".tmp"))
	return err
}

// InitCapabilityDir create dir if not exits
func InitCapabilityDir() error {
	dir, err := GetCapabilityDir()
	if err != nil {
		return err
	}
	_, err = CreateIfNotExist(dir)
	return err
}

// CreateIfNotExist create dir if not exist
func CreateIfNotExist(dir string) (bool, error) {
	_, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			// nolint:gosec
			return false, os.MkdirAll(dir, 0755)
		}
		return false, err
	}
	return true, nil
}

const (
	// LegacyKubeVelaSystemNamespaceEnv the legacy environment variable for kubevela system namespace
	LegacyKubeVelaSystemNamespaceEnv = "DEFAULT_VELA_NS"
	// KubeVelaSystemNamespaceEnv the environment variable for kubevela system namespace
	KubeVelaSystemNamespaceEnv = "KUBEVELA_SYSTEM_NAMESPACE"
	// KubeVelaDefinitionNamespaceEnv the environment variable for kubevela definition namespace
	KubeVelaDefinitionNamespaceEnv = "KUBEVELA_DEFINITION_NAMESPACE"
)

func bindEnv(variable *string, keys ...string) {
	for _, key := range keys {
		if val := os.Getenv(key); val != "" {
			*variable = val
			return
		}
	}
}

// BindEnvironmentVariables bind
func BindEnvironmentVariables() {
	bindEnv(&types.DefaultKubeVelaNS, KubeVelaSystemNamespaceEnv, LegacyKubeVelaSystemNamespaceEnv)
	bindEnv(&oam.SystemDefinitionNamespace, KubeVelaDefinitionNamespaceEnv, KubeVelaSystemNamespaceEnv)
}
