package plugins

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/pkg/utils/system"
)

func LoadCapabilityByName(name string) (types.Capability, error) {
	caps, err := LoadAllInstalledCapability()
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

func LoadAllInstalledCapability() ([]types.Capability, error) {
	workloads, err := LoadInstalledCapabilityWithType(types.TypeWorkload)
	if err != nil {
		return nil, err
	}
	traits, err := LoadInstalledCapabilityWithType(types.TypeTrait)
	if err != nil {
		return nil, err
	}
	workloads = append(workloads, traits...)
	return workloads, nil
}

func LoadInstalledCapabilityWithType(capT types.CapType) ([]types.Capability, error) {
	dir, _ := system.GetCapabilityDir()
	return loadInstalledCapabilityWithType(dir, capT)
}

func GetInstalledCapabilityWithCapAlias(capT types.CapType, capAlias string) (types.Capability, error) {
	dir, _ := system.GetCapabilityDir()
	return loadInstalledCapabilityWithCapAlias(dir, capT, capAlias)
}

// leave dir as argument for test convenience
func loadInstalledCapabilityWithType(dir string, capT types.CapType) ([]types.Capability, error) {
	dir = GetSubDir(dir, capT)
	return loadInstalledCapability(dir, "")
}

func loadInstalledCapabilityWithCapAlias(dir string, capT types.CapType, capAlias string) (types.Capability, error) {
	var cap types.Capability
	dir = GetSubDir(dir, capT)
	capList, err := loadInstalledCapability(dir, capAlias)
	if err != nil {
		return cap, err
	} else if len(capList) != 1 {
		return cap, fmt.Errorf("could not get installed capability by %s", capAlias)
	}
	return capList[0], nil
}

func loadInstalledCapability(dir string, capAlias string) ([]types.Capability, error) {
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
		data, err := ioutil.ReadFile(filepath.Join(dir, f.Name()))
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
		// Get the specified installed capability: a WorkoadDefinition or a TraitDefinition
		if capAlias != "" {
			if capAlias == f.Name() {
				tmps = append(tmps, tmp)
				break
			}
			continue
		} else {
			tmps = append(tmps, tmp)
		}
	}
	return tmps, nil
}

func GetSubDir(dir string, capT types.CapType) string {
	switch capT {
	case types.TypeWorkload:
		return filepath.Join(dir, "workloads")
	case types.TypeTrait:
		return filepath.Join(dir, "traits")
	}
	return dir
}

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
	retainedFiles := []string{}
	subDirs := []string{GetSubDir(dir, types.TypeWorkload), GetSubDir(dir, types.TypeTrait)}
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

func LoadCapabilityFromSyncedCenter(dir string) ([]types.Capability, error) {
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
		data, err := ioutil.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			fmt.Printf("read file %s err %v\n", f.Name(), err)
			continue
		}
		tmp, err := ParseAndSyncCapability(data, filepath.Join(dir, ".tmp"))
		if err != nil {
			fmt.Printf("get definition of %s err %v\n", f.Name(), err)
			continue
		}
		tmps = append(tmps, tmp)
	}
	return tmps, nil
}
