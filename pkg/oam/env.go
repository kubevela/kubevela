package oam

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"

	"github.com/cloud-native-application/rudrx/api/types"
	"github.com/cloud-native-application/rudrx/pkg/utils/system"
)

func GetEnvByName(name string) (*types.EnvMeta, error) {
	envdir, err := system.GetEnvDir()
	if err != nil {
		return nil, err
	}
	data, err := ioutil.ReadFile(filepath.Join(envdir, name))
	if err != nil {
		return nil, err
	}
	var meta types.EnvMeta
	if err = json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}
