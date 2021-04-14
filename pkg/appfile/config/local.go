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

package config

import (
	"bufio"
	"bytes"

	"github.com/oam-dev/kubevela/pkg/utils/config"
	env2 "github.com/oam-dev/kubevela/pkg/utils/env"
)

// TypeLocal defines the local config store type
const TypeLocal = "local"

// Local is the local implementation of config store
type Local struct{}

var _ Store = &Local{}

// GetConfigData will return config data from local
func (l *Local) GetConfigData(configName, envName string) ([]map[string]string, error) {
	cfgData, err := config.ReadConfig(envName, configName)
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(bytes.NewReader(cfgData))
	var data []map[string]string
	for scanner.Scan() {
		k, v, err := config.ReadConfigLine(scanner.Text())
		if err != nil {
			return nil, err
		}
		data = append(data, EncodeConfigFormat(k, v))
	}
	return data, nil
}

// Namespace return namespace from env
func (l *Local) Namespace(envName string) (string, error) {
	env, err := env2.GetEnvByName(envName)
	if err != nil {
		return "", err
	}
	return env.Namespace, nil
}

// Type returns the type of this config store implementation
func (l *Local) Type() string {
	return TypeLocal
}
