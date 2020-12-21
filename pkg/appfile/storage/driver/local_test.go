package driver

import (
	"io/ioutil"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ghodss/yaml"

	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/appfile/template"
)

var dir string
var tm template.Manager
var afile *appfile.AppFile
var appName = "testsvc"
var envName = "default"

func init() {
	dir, _ = getApplicationDir(envName)
	tm, _ = template.Load()
	afile = appfile.NewAppFile()
	afile.Name = appName
	svcs := make(map[string]appfile.Service, 0)
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

func TestLocal_Get(t *testing.T) {
	type args struct {
		envName string
		appName string
	}
	tests := []struct {
		name    string
		args    args
		want    *RespApplication
		wantErr bool
	}{
		{"TestLocal_Get1", args{envName: envName, appName: appName}, &RespApplication{AppFile: afile, Tm: tm}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &Local{}
			got, err := l.Get(tt.args.envName, tt.args.appName)
			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Get() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLocal_Delete(t *testing.T) {
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

func TestLocal_Save(t *testing.T) {
	type args struct {
		app     *RespApplication
		envName string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"TestLocal_Save1", args{&RespApplication{AppFile: afile, Tm: nil}, envName}, false},
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

func TestLocal_List(t *testing.T) {
	type args struct {
		envName string
	}
	want := make([]*RespApplication, 0)
	want = append(want, &RespApplication{afile, tm})
	tests := []struct {
		name    string
		args    args
		want    []*RespApplication
		wantErr bool
	}{
		{"TestLocal_List1", args{envName}, want, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &Local{}
			got, err := l.List(tt.args.envName)
			if (err != nil) != tt.wantErr {
				t.Errorf("List() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(got) == 0 {
				t.Errorf("List() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLocal_Name(t *testing.T) {
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

func Test_getApplicationDir(t *testing.T) {
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

func Test_loadFromFile(t *testing.T) {
	type args struct {
		fileName string
	}
	tests := []struct {
		name    string
		args    args
		want    *RespApplication
		wantErr bool
	}{
		{"testRespApp", args{fileName: filepath.Join(dir, appName+".yaml")}, &RespApplication{AppFile: afile, Tm: tm}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := loadFromFile(tt.args.fileName)
			if (err != nil) != tt.wantErr {
				t.Errorf("loadFromFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got.Name != tt.want.Name {
				t.Errorf("loadFromFile() got = %v, want %v", got, tt.want)
			}
		})
	}
}
