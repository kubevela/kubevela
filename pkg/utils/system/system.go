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

func GetRepoDir() (string, error) {
	home, err := GetVelaHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "repositories"), nil
}

func GetRepoConfig() (string, error) {
	home, err := GetRepoDir()
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

func GetDefinitionDir() (string, error) {
	home, err := GetVelaHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "definitions"), nil
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
	if err := InitDefinitionDir(); err != nil {
		return err
	}
	if err := InitApplicationDir(); err != nil {
		return err
	}
	if err := InitRepositoryDir(); err != nil {
		return err
	}
	return nil
}

func InitRepositoryDir() error {
	home, err := GetRepoDir()
	if err != nil {
		return err
	}
	return StatAndCreate(filepath.Join(home, ".tmp"))
}

func InitDefinitionDir() error {
	dir, err := GetDefinitionDir()
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
