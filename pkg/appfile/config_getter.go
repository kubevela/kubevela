package appfile

import (
	"bufio"
	"bytes"

	"github.com/oam-dev/kubevela/pkg/utils/config"
	"github.com/oam-dev/kubevela/pkg/utils/env"
)

type configGetter interface {
	GetConfigData(configName string) ([]map[string]string, error)
}

type defaultConfigGetter struct{}

func (defaultConfigGetter) GetConfigData(configName string) ([]map[string]string, error) {
	envName, err := env.GetCurrentEnvName()
	if err != nil {
		return nil, err
	}
	cfgData, err := config.ReadConfig(envName, configName)
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(bytes.NewReader(cfgData))
	data := []map[string]string{}
	for scanner.Scan() {
		k, v, err := config.ReadConfigLine(scanner.Text())
		if err != nil {
			return nil, err
		}
		data = append(data, map[string]string{
			"name":  k,
			"value": v,
		})
	}
	return data, nil
}

type fakeConfigGetter struct {
	Data []map[string]string
}

func (f *fakeConfigGetter) GetConfigData(_ string) ([]map[string]string, error) {
	return f.Data, nil
}
