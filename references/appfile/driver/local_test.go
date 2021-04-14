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
	"io/ioutil"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ghodss/yaml"

	"github.com/oam-dev/kubevela/references/appfile/api"
)

var dir string
var afile *api.AppFile
var appName = "testsvc"
var envName = "default"

func init() {
	dir, _ = getApplicationDir(envName)
	afile = api.NewAppFile()
	afile.Name = appName
	svcs := make(map[string]api.Service)
	svcs["wordpress"] = map[string]interface{}{
		"type":  "webservice",
		"image": "wordpress:php7.4-apache",
		"port":  "80",
		"cpu":   "1",
		"route": "",
	}
	afile.Services = svcs
	out, _ := yaml.Marshal(afile)
	_ = ioutil.WriteFile(filepath.Join(dir, appName+".yaml"), out, 0644)
}

func TestLocalDelete(t *testing.T) {
	type args struct {
		envName string
		appName string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"testLocalDelete1", args{envName: envName, appName: appName}, false},
		{"testLocalDelete2", args{envName: envName, appName: "test"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &Local{}
			if err := l.Delete(tt.args.envName, tt.args.appName); (err != nil) != tt.wantErr {
				t.Errorf("Delete() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLocalSave(t *testing.T) {
	type args struct {
		app     *api.Application
		envName string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"TestLocal_Save1", args{&api.Application{AppFile: afile, Tm: nil}, envName}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &Local{}
			if err := l.Save(tt.args.app, tt.args.envName); (err != nil) != tt.wantErr {
				t.Errorf("Save() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLocalName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"testDriverName", "Local"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &Local{}
			if got := l.Name(); got != tt.want {
				t.Errorf("Name() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewLocalStorage(t *testing.T) {
	tests := []struct {
		name string
		want *Local
	}{
		{"testNewLocal", &Local{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewLocalStorage(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewLocalStorage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetApplicationDir(t *testing.T) {
	type args struct {
		envName string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{"testGetApplicationDir", args{envName: envName}, "/envs/default/applications", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getApplicationDir(tt.args.envName)
			if (err != nil) != tt.wantErr {
				t.Errorf("getApplicationDir() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("getApplicationDir() got = %v, want %v", got, tt.want)
			}
		})
	}
}
