package application

import (
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
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/utils/system"
)

const (
	Traits = "traits"
	Scopes = "scopes"
)

type Application struct {
	*appfile.AppFile `json:",inline"`
}

func LoadFromFile(fileName string) (*Application, error) {
	_, err := ioutil.ReadFile(fileName)
	if err != nil {
		if os.IsNotExist(err) {
			return &Application{AppFile: appfile.NewAppFile()}, nil
		}
		return nil, err
	}

	f, err := appfile.LoadFromFile(fileName)
	if err != nil {
		return nil, err
	}
	return &Application{AppFile: f}, nil
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
	return ioutil.WriteFile(filepath.Join(appDir, app.Name+".yaml"), out, 0644)
}

func NotWorkload(tp string) bool {
	if tp == Scopes || tp == Traits {
		return true
	}
	return false
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

// TODO: replace this with GetServiceConfig()
func (app *Application) GetWorkload(componentName string) (string, map[string]interface{}) {
	comp, ok := app.Components[componentName]
	if !ok {
		return "", make(map[string]interface{})
	}
	for tp, workload := range comp {
		if NotWorkload(tp) {
			continue
		}
		return tp, workload.(map[string]interface{})
	}
	return "", make(map[string]interface{})
}

// TODO: replace this with GetServiceConfig() or use AppConfig after RenderOAM()
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

// TODO: replace this with GetServiceConfig() or use AppConfig after RenderOAM()
func (app *Application) GetTraits(componentName string) (map[string]map[string]interface{}, error) {
	comp, ok := app.Components[componentName]
	if !ok {
		return nil, fmt.Errorf("%s not exist", componentName)
	}
	t, ok := comp[Traits]
	if !ok {
		return make(map[string]map[string]interface{}), nil
	}
	// assume it's valid, use Validate() to check
	switch trs := t.(type) {
	case map[string]interface{}:
		traits := make(map[string]map[string]interface{})
		for k, v := range trs {
			traits[k] = v.(map[string]interface{})
		}
		return traits, nil
	case map[string]map[string]interface{}:
		return trs, nil
	}
	return nil, fmt.Errorf("invalid traits data format in %s, expect nested map but got %v", componentName, reflect.TypeOf(t))
}

func (app *Application) GetTraitsByType(componentName, traitType string) (map[string]interface{}, error) {
	service, ok := app.Services[componentName]
	if !ok {
		return nil, fmt.Errorf("component name (%s) doesn't exist", componentName)
	}
	t, ok := service[traitType]
	if !ok {
		return nil, fmt.Errorf("trait type (%s:%s) doesn't exist", componentName, traitType)
	}
	return t.(map[string]interface{}), nil
}

func FormatDefaultHealthScopeName(appName string) string {
	return appName + "-default-health"
}

//TODO(wonderflow) add scope support here
func (app *Application) OAM(env *types.EnvMeta, io cmdutil.IOStreams) ([]*v1alpha2.Component, *v1alpha2.ApplicationConfiguration, []oam.Object, error) {
	comps, appConfig, err := app.RenderOAM(env.Name, io)
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
