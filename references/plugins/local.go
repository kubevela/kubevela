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

package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/system"
)

// LoadCapabilityByName will load capability from local by name
func LoadCapabilityByName(name string, userNamespace string, c common.Args) (types.Capability, error) {
	caps, err := LoadAllInstalledCapability(userNamespace, c)
	if err != nil {
		return types.Capability{}, err
	}
	for _, c := range caps {
		if c.Name == name {
			return c, nil
		}
	}
	return types.Capability{}, fmt.Errorf("%s not found", name)
}

// LoadAllInstalledCapability will list all capability
func LoadAllInstalledCapability(userNamespace string, c common.Args) ([]types.Capability, error) {
	caps, err := GetCapabilitiesFromCluster(context.TODO(), userNamespace, c, nil)
	if err != nil {
		return nil, err
	}
	systemCaps, err := GetCapabilitiesFromCluster(context.TODO(), types.DefaultKubeVelaNS, c, nil)
	if err != nil {
		return nil, err
	}
	caps = append(caps, systemCaps...)
	return caps, nil
}

// LoadInstalledCapabilityWithType will load cap list by type
func LoadInstalledCapabilityWithType(userNamespace string, c common.Args, capT types.CapType) ([]types.Capability, error) {
	switch capT {
	case types.TypeComponentDefinition:
		caps, _, err := GetComponentsFromCluster(context.TODO(), userNamespace, c, nil)
		if err != nil {
			return nil, err
		}
		systemCaps, _, err := GetComponentsFromCluster(context.TODO(), types.DefaultKubeVelaNS, c, nil)
		if err != nil {
			return nil, err
		}
		caps = append(caps, systemCaps...)
		return caps, nil
	case types.TypeTrait:
		caps, _, err := GetTraitsFromCluster(context.TODO(), userNamespace, c, nil)
		if err != nil {
			return nil, err
		}
		systemCaps, _, err := GetTraitsFromCluster(context.TODO(), types.DefaultKubeVelaNS, c, nil)
		if err != nil {
			return nil, err
		}
		caps = append(caps, systemCaps...)
		return caps, nil
	case types.TypeScope:
	case types.TypeWorkload:
	}
	return nil, nil
}

// GetInstalledCapabilityWithCapName will get cap by alias
func GetInstalledCapabilityWithCapName(capT types.CapType, capName string) (types.Capability, error) {
	dir, err := system.GetCapabilityDir()
	if err != nil {
		return types.Capability{}, err
	}
	return loadInstalledCapabilityWithCapName(dir, capT, capName)
}

// leave dir as argument for test convenience
func loadInstalledCapabilityWithType(dir string, capT types.CapType) ([]types.Capability, error) {
	dir = GetSubDir(dir, capT)
	return loadInstalledCapability(dir, "")
}

func loadInstalledCapabilityWithCapName(dir string, capT types.CapType, capName string) (types.Capability, error) {
	var cap types.Capability
	dir = GetSubDir(dir, capT)
	capList, err := loadInstalledCapability(dir, capName)
	if err != nil {
		return cap, err
	} else if len(capList) != 1 {
		return cap, fmt.Errorf("could not get installed capability by %s", capName)
	}
	return capList[0], nil
}

func loadInstalledCapability(dir string, name string) ([]types.Capability, error) {
	var tmps []types.Capability
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if strings.HasSuffix(f.Name(), ".cue") {
			continue
		}
		data, err := ioutil.ReadFile(filepath.Clean(filepath.Join(dir, f.Name())))
		if err != nil {
			fmt.Printf("read file %s err %v\n", f.Name(), err)
			continue
		}
		var tmp types.Capability
		decoder := json.NewDecoder(bytes.NewBuffer(data))
		decoder.UseNumber()
		if err = decoder.Decode(&tmp); err != nil {
			fmt.Printf("ignore invalid format file: %s\n", f.Name())
			continue
		}
		// Get the specified installed capability: workload or trait
		if name != "" {
			if name == f.Name() {
				tmps = append(tmps, tmp)
				break
			}
			continue
		}
		tmps = append(tmps, tmp)
	}
	return tmps, nil
}

// GetSubDir will get dir for capability
func GetSubDir(dir string, capT types.CapType) string {
	switch capT {
	case types.TypeWorkload:
		return filepath.Join(dir, "workloads")
	case types.TypeTrait:
		return filepath.Join(dir, "traits")
	case types.TypeScope:
		return filepath.Join(dir, "scopes")
	case types.TypeComponentDefinition:
		return filepath.Join(dir, "components")
	}
	return dir
}

// SinkTemp2Local will sink template to local file
func SinkTemp2Local(templates []types.Capability, dir string) int {
	success := 0
	for _, tmp := range templates {
		subDir := GetSubDir(dir, tmp.Type)
		_, _ = system.CreateIfNotExist(subDir)
		data, err := json.Marshal(tmp)
		if err != nil {
			fmt.Printf("sync %s err: %v\n", tmp.Name, err)
			continue
		}
		//nolint:gosec
		err = ioutil.WriteFile(filepath.Join(subDir, tmp.Name), data, 0644)
		if err != nil {
			fmt.Printf("sync %s err: %v\n", tmp.Name, err)
			continue
		}
		success++
	}
	return success
}

// RemoveLegacyTemps will remove capability definitions under `dir` but not included in `retainedTemps`.
func RemoveLegacyTemps(retainedTemps []types.Capability, dir string) int {
	success := 0
	var retainedFiles []string
	subDirs := []string{GetSubDir(dir, types.TypeComponentDefinition), GetSubDir(dir, types.TypeTrait)}
	for _, tmp := range retainedTemps {
		subDir := GetSubDir(dir, tmp.Type)
		tmpFilePath := filepath.Join(subDir, tmp.Name)
		retainedFiles = append(retainedFiles, tmpFilePath)
	}

	for _, subDir := range subDirs {
		if err := filepath.Walk(subDir, func(path string, info os.FileInfo, err error) error {
			if info == nil || info.IsDir() {
				// omit subDir or subDir not exist
				return nil
			}
			for _, retainedFile := range retainedFiles {
				if retainedFile == path {
					return nil
				}
			}
			if err := os.Remove(path); err != nil {
				fmt.Printf("remove legacy %s err: %v\n", path, err)
				return err
			}
			success++
			return nil
		}); err != nil {
			continue
		}
	}
	return success
}

// LoadCapabilityFromSyncedCenter will load capability from dir
func LoadCapabilityFromSyncedCenter(mapper discoverymapper.DiscoveryMapper, dir string) ([]types.Capability, error) {
	var tmps []types.Capability
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if strings.HasSuffix(f.Name(), ".cue") {
			continue
		}
		data, err := ioutil.ReadFile(filepath.Clean(filepath.Join(dir, f.Name())))
		if err != nil {
			fmt.Printf("read file %s err %v\n", f.Name(), err)
			continue
		}
		tmp, err := ParseAndSyncCapability(mapper, data)
		if err != nil {
			fmt.Printf("get definition of %s err %v\n", f.Name(), err)
			continue
		}
		tmps = append(tmps, tmp)
	}
	return tmps, nil
}
