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
	"errors"

	"github.com/oam-dev/kubevela/references/appfile/api"
)

// ConfigMapDriverName is local storage driver name
const ConfigMapDriverName = "ConfigMap"

// ConfigMap Storage
type ConfigMap struct {
	api.Driver
}

// NewConfigMapStorage get storage client of ConfigMap type
func NewConfigMapStorage() *ConfigMap {
	return &ConfigMap{}
}

// Name of local storage
func (c *ConfigMap) Name() string {
	return ConfigMapDriverName
}

// List applications from configmap storage
func (c *ConfigMap) List(envName string) ([]*api.Application, error) {
	// TODO support configmap storage
	return nil, errors.New("not implement")
}

// Save applications from configmap storage
func (c *ConfigMap) Save(app *api.Application, envName string) error {
	// TODO support configmap storage
	return errors.New("not implement")
}

// Delete applications from configmap storage
func (c *ConfigMap) Delete(envName, appName string) error {
	// TODO support configmap storage
	return errors.New("not implement")
}

// Get applications from configmap storage
func (c *ConfigMap) Get(envName, appName string) (*api.Application, error) {
	// TODO support configmap storage
	return nil, errors.New("not implement")
}
