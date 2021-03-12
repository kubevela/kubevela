package driver

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/ghodss/yaml"

	"github.com/oam-dev/kubevela/pkg/utils/env"
	"github.com/oam-dev/kubevela/pkg/utils/system"
	"github.com/oam-dev/kubevela/references/appfile/api"
)

// LocalDriverName is local storage driver name
const LocalDriverName = "Local"

// Local Storage
type Local struct {
	api.Driver
}

// NewLocalStorage get storage client of Local type
func NewLocalStorage() *Local {
	return &Local{}
}

// Name  is local storage driver name
func (l *Local) Name() string {
	return LocalDriverName
}

// Save application from local storage
func (l *Local) Save(app *api.Application, envName string) error {
	appDir, err := getApplicationDir(envName)
	if err != nil {
		return err
	}
	if app.CreateTime.IsZero() {
		app.CreateTime = time.Now()
	}
	app.UpdateTime = time.Now()
	out, err := yaml.Marshal(app)
	if err != nil {
		return err
	}
	//nolint:gosec
	return ioutil.WriteFile(filepath.Join(appDir, app.Name+".yaml"), out, 0644)
}

// Delete application from local storage
func (l *Local) Delete(envName, appName string) error {
	appDir, err := getApplicationDir(envName)
	if err != nil {
		return err
	}
	return os.Remove(filepath.Join(appDir, appName+".yaml"))
}

func getApplicationDir(envName string) (string, error) {
	appDir := filepath.Join(env.GetEnvDirByName(envName), "applications")
	_, err := system.CreateIfNotExist(appDir)
	if err != nil {
		err = fmt.Errorf("getting application directory from env %s failed, error: %w ", envName, err)
	}
	return appDir, err
}
