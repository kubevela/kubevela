package application

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
	"github.com/ghodss/yaml"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/appfile/template"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/utils/env"
	"github.com/oam-dev/kubevela/pkg/utils/system"
)

// Application is an implementation level object for Appfile, all vela commands will access AppFile from Appliction struct here.
type Application struct {
	*appfile.AppFile `json:",inline"`
	tm               template.Manager
}

func newApplication(f *appfile.AppFile, tm template.Manager) *Application {
	if f == nil {
		f = appfile.NewAppFile()
	}
	return &Application{AppFile: f, tm: tm}
}

// LoadFromFile will load application from file
func LoadFromFile(fileName string) (*Application, error) {
	tm, err := template.Load()
	if err != nil {
		return nil, err
	}
	_, err = os.Stat(fileName)
	if err != nil {
		if os.IsNotExist(err) {
			return newApplication(nil, tm), nil
		}
		return nil, err
	}

	f, err := appfile.LoadFromFile(fileName)
	if err != nil {
		return nil, err
	}
	app := newApplication(f, tm)
	return app, app.Validate()
}

// Load will load application with env and name from default vela home dir.
func Load(envName, appName string) (*Application, error) {
	appDir, err := getApplicationDir(envName)
	if err != nil {
		return nil, fmt.Errorf("get app dir from env %s err %w", envName, err)
	}
	return LoadFromFile(filepath.Join(appDir, appName+".yaml"))
}

// Delete will delete an app along with it's appfile.
func Delete(envName, appName string) error {
	appDir, err := getApplicationDir(envName)
	if err != nil {
		return fmt.Errorf("get app dir from env %s err %w", envName, err)
	}
	return os.Remove(filepath.Join(appDir, appName+".yaml"))
}

// List will list all apps
func List(envName string) ([]*Application, error) {
	appDir, err := getApplicationDir(envName)
	if err != nil {
		return nil, fmt.Errorf("get app dir from env %s err %w", envName, err)
	}
	files, err := ioutil.ReadDir(appDir)
	if err != nil {
		return nil, fmt.Errorf("list apps from %s err %w", appDir, err)
	}
	var apps []*Application
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if !strings.HasSuffix(f.Name(), ".yaml") {
			continue
		}
		app, err := LoadFromFile(filepath.Join(appDir, f.Name()))
		if err != nil {
			return nil, fmt.Errorf("load application err %w", err)
		}
		apps = append(apps, app)
	}
	return apps, nil
}

// MatchAppByComp will get application with componentName without AppName.
func MatchAppByComp(envName, compName string) (*Application, error) {
	apps, err := List(envName)
	if err != nil {
		return nil, err
	}
	for _, subapp := range apps {
		for _, v := range subapp.GetComponents() {
			if v == compName {
				return subapp, nil
			}
		}
	}
	return nil, fmt.Errorf("no app found contains %s in env %s", compName, envName)
}

// Save will save appfile into default dir.
func (app *Application) Save(envName string) error {
	appDir, err := getApplicationDir(envName)
	if err != nil {
		return fmt.Errorf("get app dir from env %s err %w", envName, err)
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

// Validate will validate whether an Appfile is valid.
func (app *Application) Validate() error {
	if app.Name == "" {
		return errors.New("name is required")
	}
	if len(app.Services) == 0 {
		return errors.New("at least one service is required")
	}
	for name, svc := range app.Services {
		for traitName, traitData := range svc.GetConfig() {
			if app.tm.IsTrait(traitName) {
				if _, ok := traitData.(map[string]interface{}); !ok {
					return fmt.Errorf("trait %s in '%s' must be map", traitName, name)
				}
			}
		}
	}
	return nil
}

// GetComponents will get oam components from Appfile.
func (app *Application) GetComponents() []string {
	var components []string
	for name := range app.Services {
		components = append(components, name)
	}
	sort.Strings(components)
	return components
}

// GetServiceConfig will get service type and it's configuration
func (app *Application) GetServiceConfig(componentName string) (string, map[string]interface{}) {
	svc, ok := app.Services[componentName]
	if !ok {
		return "", make(map[string]interface{})
	}
	return svc.GetType(), svc.GetConfig()
}

// GetWorkload will get workload type and it's configuration
func (app *Application) GetWorkload(componentName string) (string, map[string]interface{}) {
	svcType, config := app.GetServiceConfig(componentName)
	if svcType == "" {
		return "", make(map[string]interface{})
	}
	workloadData := make(map[string]interface{})
	for k, v := range config {
		if app.tm.IsTrait(k) {
			continue
		}
		workloadData[k] = v
	}
	return svcType, workloadData
}

// GetTraitNames will list all traits attached to the specified component.
func (app *Application) GetTraitNames(componentName string) ([]string, error) {
	tt, err := app.GetTraits(componentName)
	if err != nil {
		return nil, err
	}
	var names []string
	for k := range tt {
		names = append(names, k)
	}
	return names, nil
}

// GetTraits will list all traits and it's configurations attached to the specified component.
func (app *Application) GetTraits(componentName string) (map[string]map[string]interface{}, error) {
	_, config := app.GetServiceConfig(componentName)
	traitsData := make(map[string]map[string]interface{})
	for k, v := range config {
		if !app.tm.IsTrait(k) {
			continue
		}
		newV, ok := v.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%s is trait, but with invalid format %s, should be map[string]interface{}", k, reflect.TypeOf(v))
		}
		traitsData[k] = newV
	}
	return traitsData, nil
}

// GetTraitsByType will get trait configuration with specified component and trait type, we assume one type of trait can only attach to a component once.
func (app *Application) GetTraitsByType(componentName, traitType string) (map[string]interface{}, error) {
	service, ok := app.Services[componentName]
	if !ok {
		return nil, fmt.Errorf("service name (%s) doesn't exist", componentName)
	}
	t, ok := service[traitType]
	if !ok {
		return make(map[string]interface{}), nil
	}
	return t.(map[string]interface{}), nil
}

// OAM will convert an AppFile to OAM objects
// TODO(wonderflow) add scope support here
func (app *Application) OAM(env *types.EnvMeta, io cmdutil.IOStreams, silence bool) ([]*v1alpha2.Component, *v1alpha2.ApplicationConfiguration, []oam.Object, error) {
	comps, appConfig, scopes, err := app.RenderOAM(env.Namespace, io, app.tm, silence)
	if err != nil {
		return nil, nil, nil, err
	}
	return comps, appConfig, scopes, nil
}

func getApplicationDir(envName string) (string, error) {
	appDir := filepath.Join(env.GetEnvDirByName(envName), "applications")
	_, err := system.CreateIfNotExist(appDir)
	return appDir, err
}

// GetAppConfig will get AppConfig from K8s cluster.
func GetAppConfig(ctx context.Context, c client.Client, app *Application, env *types.EnvMeta) (*v1alpha2.ApplicationConfiguration, error) {
	appConfig := &v1alpha2.ApplicationConfiguration{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: env.Namespace, Name: app.Name}, appConfig); err != nil {
		return nil, err
	}
	return appConfig, nil
}
