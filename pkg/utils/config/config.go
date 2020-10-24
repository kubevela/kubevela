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

func GetConfigsDir(envName string) (string, error) {
	cfgDir := filepath.Join(env.GetEnvDirByName(envName), "configs")
	err := os.MkdirAll(cfgDir, 0700)
	return cfgDir, err
}

func DeleteConfig(envName, configName string) error {
	d, err := GetConfigsDir(envName)
	if err != nil {
		return err
	}
	cfgFile := filepath.Join(d, configName)
	return os.RemoveAll(cfgFile)
}

func ReadConfig(envName, configName string) ([]byte, error) {
	d, err := GetConfigsDir(envName)
	if err != nil {
		return nil, err
	}
	cfgFile := filepath.Join(d, configName)
	b, err := ioutil.ReadFile(cfgFile)
	if os.IsNotExist(err) {
		return []byte{}, nil
	}
	return b, err
}

func WriteConfig(envName, configName string, data []byte) error {
	d, err := GetConfigsDir(envName)
	if err != nil {
		return err
	}
	cfgFile := filepath.Join(d, configName)
	return ioutil.WriteFile(cfgFile, data, 0600)
}
