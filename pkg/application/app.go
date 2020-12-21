package application

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/appfile/storage"
	"github.com/oam-dev/kubevela/pkg/appfile/storage/driver"
	"github.com/oam-dev/kubevela/pkg/appfile/template"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/oam"
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

// NewEmptyApplication new empty application, only set tm
func NewEmptyApplication() (*Application, error) {
	tm, err := template.Load()
	if err != nil {
		return nil, err
	}
	return newApplication(nil, tm), nil
}

// IsNotFound is application not found error
func IsNotFound(appName string, err error) bool {
	return err != nil && err.Error() == fmt.Sprintf(`application "%s" not found`, appName)
}

// Load will load application with env and name from default vela home dir.
func Load(envName, appName string) (*Application, error) {
	respApp, err := storage.Store.Get(envName, appName)
	if err != nil {
		return nil, err
	}
	app := newApplication(respApp.AppFile, respApp.Tm)
	err = app.Validate()
	return app, err
}

// Delete will delete an app along with it's appfile.
func Delete(envName, appName string) error {
	return storage.Store.Delete(envName, appName)
}

// List will list all apps
func List(envName string) ([]*Application, error) {
	respApps, err := storage.Store.List(envName)
	if err != nil {
		return nil, err
	}
	var apps []*Application
	for _, resp := range respApps {
		app := newApplication(resp.AppFile, resp.Tm)
		err := app.Validate()
		if err != nil {
			return nil, err
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
	return storage.Store.Save(&driver.RespApplication{AppFile: app.AppFile, Tm: app.tm}, envName)
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
	comps, appConfig, scopes, err := app.BuildOAM(env.Namespace, io, app.tm, silence)
	if err != nil {
		return nil, nil, nil, err
	}
	return comps, appConfig, scopes, nil
}

// GetAppConfig will get AppConfig from K8s cluster.
func GetAppConfig(ctx context.Context, c client.Client, app *Application, env *types.EnvMeta) (*v1alpha2.ApplicationConfiguration, error) {
	appConfig := &v1alpha2.ApplicationConfiguration{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: env.Namespace, Name: app.Name}, appConfig); err != nil {
		return nil, err
	}
	return appConfig, nil
}
