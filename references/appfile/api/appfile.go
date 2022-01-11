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

package api

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/builtin"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/appfile/template"
)

// error msg used in Appfile
var (
	ErrImageNotDefined = errors.New("image not defined")
)

// DefaultAppfilePath defines the default file path that used by `vela up` command
const (
	DefaultJSONAppfilePath         = "./vela.json"
	DefaultAppfilePath             = "./vela.yaml"
	DefaultUnknowFormatAppfilePath = "./Appfile"
)

const (
	// DefaultHealthScopeKey is the key in application for default health scope
	DefaultHealthScopeKey = "healthscopes.core.oam.dev"
)

// AppFile defines the spec of KubeVela Appfile
type AppFile struct {
	Name       string             `json:"name"`
	CreateTime time.Time          `json:"createTime,omitempty"`
	UpdateTime time.Time          `json:"updateTime,omitempty"`
	Services   map[string]Service `json:"services"`
	Secrets    map[string]string  `json:"secrets,omitempty"`

	initialized bool
}

// NewAppFile init an empty AppFile struct
func NewAppFile() *AppFile {
	return &AppFile{
		Services: make(map[string]Service),
		Secrets:  make(map[string]string),
	}
}

// Load will load appfile from default path
func Load() (*AppFile, error) {
	if _, err := os.Stat(DefaultAppfilePath); err == nil {
		return LoadFromFile(DefaultAppfilePath)
	}
	if _, err := os.Stat(DefaultJSONAppfilePath); err == nil {
		return LoadFromFile(DefaultJSONAppfilePath)
	}
	return LoadFromFile(DefaultUnknowFormatAppfilePath)
}

// JSONToYaml will convert JSON format appfile to yaml and load the AppFile struct
func JSONToYaml(data []byte, appFile *AppFile) (*AppFile, error) {
	j, e := yaml.JSONToYAML(data)
	if e != nil {
		return nil, e
	}
	err := yaml.Unmarshal(j, appFile)
	if err != nil {
		return nil, err
	}
	return appFile, nil
}

// LoadFromFile will read the file and load the AppFile struct
func LoadFromFile(filename string) (*AppFile, error) {
	b, err := os.ReadFile(filepath.Clean(filename))
	if err != nil {
		return nil, err
	}
	return LoadFromBytes(b)
}

// LoadFromBytes will load AppFile from bytes
func LoadFromBytes(b []byte) (*AppFile, error) {
	af := NewAppFile()
	var err error
	if json.Valid(b) {
		af, err = JSONToYaml(b, af)
	} else {
		err = yaml.Unmarshal(b, af)
	}
	if err != nil {
		return nil, err
	}
	return af, nil
}

// ExecuteAppfileTasks will execute built-in tasks(such as image builder, etc.) and generate locally executed application
func (app *AppFile) ExecuteAppfileTasks(io cmdutil.IOStreams) error {
	if app.initialized {
		return nil
	}
	for name, svc := range app.Services {
		newSvc, err := builtin.RunBuildInTasks(svc, io)
		if err != nil {
			return err
		}
		app.Services[name] = newSvc
	}
	app.initialized = true
	return nil
}

// ConvertToApplication renders Appfile into Application, Scopes and other K8s Resources.
func (app *AppFile) ConvertToApplication(namespace string, io cmdutil.IOStreams, tm template.Manager, silence bool) (*v1beta1.Application, error) {
	if err := app.ExecuteAppfileTasks(io); err != nil {
		if strings.Contains(err.Error(), "'image' : not found") {
			return nil, ErrImageNotDefined
		}
		return nil, err
	}
	// auxiliaryObjects currently include OAM Scope Custom Resources and ConfigMaps
	servApp := new(v1beta1.Application)
	servApp.SetNamespace(namespace)
	servApp.SetName(app.Name)
	servApp.Spec.Components = []common.ApplicationComponent{}
	if !silence {
		io.Infof("parsing application components")
	}
	for serviceName, svc := range app.GetServices() {
		comp, err := svc.RenderServiceToApplicationComponent(tm, serviceName)
		if err != nil {
			return nil, err
		}
		servApp.Spec.Components = append(servApp.Spec.Components, comp)
	}
	servApp.SetGroupVersionKind(v1beta1.SchemeGroupVersion.WithKind("Application"))
	return servApp, nil
}
