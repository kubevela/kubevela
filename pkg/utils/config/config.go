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
	b64 "encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/oam-dev/kubevela/pkg/utils/env"
)

// ReadConfigLine will read config from line
func ReadConfigLine(line string) (string, string, error) {
	ss := strings.SplitN(line, ":", 2)
	if len(ss) != 2 {
		return "", "", fmt.Errorf("config data is malformed: %s", line)
	}
	for i := range ss {
		ss[i] = strings.TrimSpace(ss[i])
	}
	vDec, err := b64.StdEncoding.DecodeString(ss[1])
	if err != nil {
		return "", "", err
	}
	return ss[0], string(vDec), nil
}

// GetConfigsDir will get config from dir
func GetConfigsDir(envName string) (string, error) {
	cfgDir := filepath.Join(env.GetEnvDirByName(envName), "configs")
	err := os.MkdirAll(cfgDir, 0700)
	return cfgDir, err
}

// DeleteConfig will delete local config file
func DeleteConfig(envName, configName string) error {
	d, err := GetConfigsDir(envName)
	if err != nil {
		return err
	}
	cfgFile := filepath.Join(d, configName)
	return os.RemoveAll(cfgFile)
}

// ReadConfig will read the config data from local
func ReadConfig(envName, configName string) ([]byte, error) {
	d, err := GetConfigsDir(envName)
	if err != nil {
		return nil, err
	}
	cfgFile := filepath.Join(d, configName)
	b, err := ioutil.ReadFile(filepath.Clean(cfgFile))
	if os.IsNotExist(err) {
		return []byte{}, nil
	}
	return b, err
}

// WriteConfig will write data into local config
func WriteConfig(envName, configName string, data []byte) error {
	d, err := GetConfigsDir(envName)
	if err != nil {
		return err
	}
	cfgFile := filepath.Join(d, configName)
	return ioutil.WriteFile(cfgFile, data, 0600)
}
