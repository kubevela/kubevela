package application

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
	"github.com/ghodss/yaml"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/appfile/template"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/utils/system"
)

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

func LoadFromFile(fileName string) (*Application, error) {
	tm, err := template.Load()
	if err != nil {
		return nil, err
	}
	_, err = ioutil.ReadFile(fileName)
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

func Load(envName, appName string) (*Application, error) {
	appDir, err := system.GetApplicationDir(envName)
	if err != nil {
		return nil, fmt.Errorf("get app dir from env %s err %v", envName, err)
	}
	return LoadFromFile(filepath.Join(appDir, appName+".yaml"))
}

func Delete(envName, appName string) error {
	appDir, err := system.GetApplicationDir(envName)
	if err != nil {
		return fmt.Errorf("get app dir from env %s err %v", envName, err)
	}
	return os.Remove(filepath.Join(appDir, appName+".yaml"))
}

func List(envName string) ([]*Application, error) {
	appDir, err := system.GetApplicationDir(envName)
	if err != nil {
		return nil, fmt.Errorf("get app dir from env %s err %v", envName, err)
	}
	files, err := ioutil.ReadDir(appDir)
	if err != nil {
		return nil, fmt.Errorf("list apps from %s err %v", appDir, err)
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
			return nil, fmt.Errorf("load application err %v", err)
		}
		apps = append(apps, app)
	}
	return apps, nil
}

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

func (app *Application) Save(envName string) error {
	appDir, err := system.GetApplicationDir(envName)
	if err != nil {
		return fmt.Errorf("get app dir from env %s err %v", envName, err)
	}
	if app.CreateTime.IsZero() {
		app.CreateTime = time.Now()
	}
	app.UpdateTime = time.Now()
	out, err := yaml.Marshal(app)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filepath.Join(appDir, app.Name+".yaml"), out, 0o644)
}

func (app *Application) Validate() error {
	if app.Name == "" {
		return errors.New("name is required")
	}
	if len(app.Services) == 0 {
		return errors.New("at least one service component is required")
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

func (app *Application) GetComponents() []string {
	var components []string
	for name := range app.Services {
		components = append(components, name)
	}
	sort.Strings(components)
	return components
}

func (app *Application) GetServiceConfig(componentName string) (string, map[string]interface{}) {
	svc, ok := app.Services[componentName]
	if !ok {
		return "", make(map[string]interface{})
	}
	return svc.GetType(), svc.GetConfig()
}

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

func (app *Application) GetTraitsByType(componentName, traitType string) (map[string]interface{}, error) {
	service, ok := app.Services[componentName]
	if !ok {
		return nil, fmt.Errorf("component name (%s) doesn't exist", componentName)
	}
	t, ok := service[traitType]
	if !ok {
		return make(map[string]interface{}), nil
	}
	return t.(map[string]interface{}), nil
}

func FormatDefaultHealthScopeName(appName string) string {
	return appName + "-default-health"
}

// TODO(wonderflow) add scope support here
func (app *Application) OAM(env *types.EnvMeta, io cmdutil.IOStreams) ([]*v1alpha2.Component, *v1alpha2.ApplicationConfiguration, []oam.Object, error) {
	comps, appConfig, err := app.RenderOAM(env.Namespace, io)
	if err != nil {
		return nil, nil, nil, err
	}
	addWorkloadTypeLabel(comps, app.Services)
	health := addHealthScope(appConfig)
	return comps, appConfig, []oam.Object{health}, nil
}

func addWorkloadTypeLabel(comps []*v1alpha2.Component, services map[string]appfile.Service) {
	for _, comp := range comps {
		workloadType := services[comp.Name].GetType()
		workloadObject := comp.Spec.Workload.Object.(*unstructured.Unstructured)
		labels := workloadObject.GetLabels()
		if labels == nil {
			labels = map[string]string{oam.WorkloadTypeLabel: workloadType}
		} else {
			labels[oam.WorkloadTypeLabel] = workloadType
		}
		workloadObject.SetLabels(labels)
	}
}

func addHealthScope(appConfig *v1alpha2.ApplicationConfiguration) *v1alpha2.HealthScope {
	health := &v1alpha2.HealthScope{}
	health.Name = FormatDefaultHealthScopeName(appConfig.Name)
	health.Namespace = appConfig.Namespace
	health.Spec.WorkloadReferences = make([]v1alpha1.TypedReference, 0)
	for i := range appConfig.Spec.Components {
		// TODO(wonderflow): Temporarily we add health scope here, should change to use scope framework
		appConfig.Spec.Components[i].Scopes = append(appConfig.Spec.Components[i].Scopes, v1alpha2.ComponentScope{
			ScopeReference: v1alpha1.TypedReference{
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
				Kind:       v1alpha2.HealthScopeKind,
				Name:       health.Name,
			},
		})
	}
	return health
}
