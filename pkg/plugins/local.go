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
)

func GetDefFromLocal(dir string, defType types.DefinitionType) ([]types.Capability, error) {
	temps, err := LoadTempFromLocal(dir)
	if err != nil {
		return nil, err
	}
	var defs []types.Capability
	for _, t := range temps {
		if t.Type != defType {
			continue
		}
		defs = append(defs, t)
	}
	return defs, nil
}

func SinkTemp2Local(templates []types.Capability, dir string) int {
	success := 0
	for _, tmp := range templates {
		data, err := json.Marshal(tmp)
		if err != nil {
			fmt.Printf("sync %s err: %v\n", tmp.Name, err)
			continue
		}
		err = ioutil.WriteFile(filepath.Join(dir, tmp.Name), data, 0644)
		if err != nil {
			fmt.Printf("sync %s err: %v\n", tmp.Name, err)
			continue
		}
		success++
	}
	return success
}

func LoadCapabilityFromLocal(dir string) ([]types.Capability, error) {
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

func LoadTempFromLocal(dir string) ([]types.Capability, error) {
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
