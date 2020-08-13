package plugins

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloud-native-application/rudrx/api/types"
	"github.com/cloud-native-application/rudrx/pkg/utils/system"
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

// leave dir as argument for test convenience
func loadInstalledCapabilityWithType(dir string, capT types.CapType) ([]types.Capability, error) {
	dir = GetSubDir(dir, capT)
	return loadInstalledCapability(dir)
}

func loadInstalledCapability(dir string) ([]types.Capability, error) {
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
		tmps = append(tmps, tmp)
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
		system.CreateIfNotExist(subDir)
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
