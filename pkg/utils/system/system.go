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
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/oam-dev/kubevela/apis/types"
)

const defaultVelaHome = ".vela"

const (
	// VelaHomeEnv defines vela home system env
	VelaHomeEnv = "VELA_HOME"
	// StorageDriverEnv defines vela storage driver env
	StorageDriverEnv = "STORAGE_DRIVER"
)

// GetVelaHomeDir return vela home dir
func GetVelaHomeDir() (string, error) {
	if custom := os.Getenv(VelaHomeEnv); custom != "" {
		return custom, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, defaultVelaHome), nil
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

// GetEnvDir return KubeVela environments dir
func GetEnvDir() (string, error) {
	homedir, err := GetVelaHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homedir, "envs"), nil
}

// GetCurrentEnvPath return current env config
func GetCurrentEnvPath() (string, error) {
	homedir, err := GetVelaHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homedir, "curenv"), nil
}

// InitDirs create dir if not exits
func InitDirs() error {
	if err := InitCapabilityDir(); err != nil {
		return err
	}
	if err := InitCapCenterDir(); err != nil {
		return err
	}
	if err := InitDefaultEnv(); err != nil {
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

// EnvConfigName defines config
const EnvConfigName = "config.json"

// InitDefaultEnv create dir if not exits
func InitDefaultEnv() error {
	envDir, err := GetEnvDir()
	if err != nil {
		return err
	}
	defaultEnvDir := filepath.Join(envDir, types.DefaultEnvName)
	exist, err := CreateIfNotExist(defaultEnvDir)
	if err != nil {
		return err
	}
	if exist {
		return nil
	}
	data, _ := json.Marshal(&types.EnvMeta{Namespace: types.DefaultAppNamespace, Name: types.DefaultEnvName})
	//nolint:gosec
	if err = ioutil.WriteFile(filepath.Join(defaultEnvDir, EnvConfigName), data, 0644); err != nil {
		return err
	}
	curEnvPath, err := GetCurrentEnvPath()
	if err != nil {
		return err
	}
	//nolint:gosec
	if err = ioutil.WriteFile(curEnvPath, []byte(types.DefaultEnvName), 0644); err != nil {
		return err
	}
	return nil
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
