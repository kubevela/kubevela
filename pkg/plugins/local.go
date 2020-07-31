package plugins

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"

	"github.com/cloud-native-application/rudrx/api/types"
)

func GetDefFromLocal(dir string, defType types.DefinitionType) ([]types.Template, error) {
	temps, err := LoadTempFromLocal(dir)
	if err != nil {
		return nil, err
	}
	var defs []types.Template
	for _, t := range temps {
		if t.Type != defType {
			continue
		}
		defs = append(defs, t)
	}
	return defs, nil
}

func SinkTemp2Local(templates []types.Template, dir string) error {
	for _, tmp := range templates {
		data, err := json.Marshal(tmp)
		if err != nil {
			return err
		}
		err = ioutil.WriteFile(filepath.Join(dir, tmp.Name), data, 0644)
		if err != nil {
			return err
		}
	}
	return nil
}

func LoadTempFromLocal(dir string) ([]types.Template, error) {
	var tmps []types.Template
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		data, err := ioutil.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			return nil, err
		}
		var tmp types.Template
		if err = json.Unmarshal(data, &tmp); err != nil {
			return nil, err
		}
		tmps = append(tmps, tmp)
	}
	return tmps, nil
}
