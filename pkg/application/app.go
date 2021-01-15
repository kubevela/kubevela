package application

import (
	"context"
	"fmt"
	"reflect"
	"sort"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile/storage"
	"github.com/oam-dev/kubevela/pkg/appfile/storage/driver"
	"github.com/oam-dev/kubevela/pkg/appfile/template"
)

// NewEmptyApplication new empty application, only set tm
func NewEmptyApplication() (*driver.Application, error) {
	tm, err := template.Load()
	if err != nil {
		return nil, err
	}
	return driver.NewApplication(nil, tm), nil
}

// IsNotFound is application not found error
func IsNotFound(appName string, err error) bool {
	return err != nil && err.Error() == fmt.Sprintf(`application "%s" not found`, appName)
}

// Load will load application with env and name from default vela home dir.
func Load(envName, appName string) (*driver.Application, error) {
	app, err := storage.Store.Get(envName, appName)
	if err != nil {
		return nil, err
	}
	err = app.Validate()
	return app, err
}

// Delete will delete an app along with it's appfile.
func Delete(envName, appName string) error {
	return storage.Store.Delete(envName, appName)
}

// List will list all apps
func List(envName string) ([]*driver.Application, error) {
	respApps, err := storage.Store.List(envName)
	if err != nil {
		return nil, err
	}
	var apps []*driver.Application
	for _, resp := range respApps {
		app := driver.NewApplication(resp.AppFile, resp.Tm)
		err := app.Validate()
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}
	return apps, nil
}

// MatchAppByComp will get application with componentName without AppName.
func MatchAppByComp(envName, compName string) (*driver.Application, error) {
	apps, err := List(envName)
	if err != nil {
		return nil, err
	}
	for _, subapp := range apps {
		for _, v := range GetComponents(subapp) {
			if v == compName {
				return subapp, nil
			}
		}
	}
	return nil, fmt.Errorf("no app found contains %s in env %s", compName, envName)
}

// Save will save appfile into default dir.
func Save(app *driver.Application, envName string) error {
	return storage.Store.Save(app, envName)
}

// GetComponents will get oam components from Appfile.
func GetComponents(app *driver.Application) []string {
	var components []string
	for name := range app.Services {
		components = append(components, name)
	}
	sort.Strings(components)
	return components
}

// GetServiceConfig will get service type and it's configuration
func GetServiceConfig(app *driver.Application, componentName string) (string, map[string]interface{}) {
	svc, ok := app.Services[componentName]
	if !ok {
		return "", make(map[string]interface{})
	}
	return svc.GetType(), svc.GetApplicationConfig()
}

// GetWorkload will get workload type and it's configuration
func GetWorkload(app *driver.Application, componentName string) (string, map[string]interface{}) {
	svcType, config := GetServiceConfig(app, componentName)
	if svcType == "" {
		return "", make(map[string]interface{})
	}
	workloadData := make(map[string]interface{})
	for k, v := range config {
		if app.Tm.IsTrait(k) {
			continue
		}
		workloadData[k] = v
	}
	return svcType, workloadData
}

// GetTraits will list all traits and it's configurations attached to the specified component.
func GetTraits(app *driver.Application, componentName string) (map[string]map[string]interface{}, error) {
	_, config := GetServiceConfig(app, componentName)
	traitsData := make(map[string]map[string]interface{})
	for k, v := range config {
		if !app.Tm.IsTrait(k) {
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
func GetTraitsByType(app *driver.Application, componentName, traitType string) (map[string]interface{}, error) {
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

// GetAppConfig will get AppConfig from K8s cluster.
func GetAppConfig(ctx context.Context, c client.Client, app *driver.Application, env *types.EnvMeta) (*v1alpha2.ApplicationConfiguration, error) {
	appConfig := &v1alpha2.ApplicationConfiguration{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: env.Namespace, Name: app.Name}, appConfig); err != nil {
		return nil, err
	}
	return appConfig, nil
}

// GetApplication will get Application from K8s cluster.
func GetApplication(ctx context.Context, c client.Client, app *driver.Application, env *types.EnvMeta) (*v1alpha2.Application, error) {
	appl := &v1alpha2.Application{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: env.Namespace, Name: app.Name}, appl); err != nil {
		return nil, err
	}
	return appl, nil
}
