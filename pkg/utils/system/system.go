package system

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/cloud-native-application/rudrx/api/types"
)

const defaultVelaHome = ".vela"
const VelaHomeEnv = "VELA_HOME"

func GetVelaHomeDir() (string, error) {
	if custom := os.Getenv(VelaHomeEnv); custom != "" {
		return custom, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, defaultVelaHome), nil
}

func GetCapCenterDir() (string, error) {
	home, err := GetVelaHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "centers"), nil
}

func GetRepoConfig() (string, error) {
	home, err := GetCapCenterDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "config.yaml"), nil
}

func GetApplicationDir() (string, error) {
	home, err := GetVelaHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "applications"), nil
}

func GetCapabilityDir() (string, error) {
	home, err := GetVelaHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "capabilities"), nil
}

func GetEnvDir() (string, error) {
	homedir, err := GetVelaHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homedir, "envs"), nil
}

func GetCurrentEnvPath() (string, error) {
	homedir, err := GetVelaHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homedir, "curenv"), nil
}

func InitDirs() error {
	if err := InitCapabilityDir(); err != nil {
		return err
	}
	if err := InitApplicationDir(); err != nil {
		return err
	}
	if err := InitCapCenterDir(); err != nil {
		return err
	}
	return nil
}

func InitCapCenterDir() error {
	home, err := GetCapCenterDir()
	if err != nil {
		return err
	}
	return StatAndCreate(filepath.Join(home, ".tmp"))
}

func InitCapabilityDir() error {
	dir, err := GetCapabilityDir()
	if err != nil {
		return err
	}
	return StatAndCreate(dir)
}

func InitApplicationDir() error {
	dir, err := GetApplicationDir()
	if err != nil {
		return err
	}
	return StatAndCreate(dir)
}

func InitDefaultEnv() error {
	envDir, err := GetEnvDir()
	if err != nil {
		return err
	}
	StatAndCreate(envDir)
	data, _ := json.Marshal(&types.EnvMeta{Namespace: types.DefaultEnvName})
	if err = ioutil.WriteFile(filepath.Join(envDir, types.DefaultEnvName), data, 0644); err != nil {
		return err
	}
	curEnvPath, err := GetCurrentEnvPath()
	if err != nil {
		return err
	}
	if err = ioutil.WriteFile(curEnvPath, []byte(types.DefaultEnvName), 0644); err != nil {
		return err
	}
	return nil
}

func StatAndCreate(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, 0755)
	}
	return nil
}
