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
