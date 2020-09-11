package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"

	"cuelang.org/go/cue"

	"k8s.io/apimachinery/pkg/runtime"

	mycue "github.com/oam-dev/kubevela/pkg/cue"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/pkg/plugins"

	"github.com/oam-dev/kubevela/api/types"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

	"github.com/oam-dev/kubevela/pkg/utils/system"
	"github.com/ghodss/yaml"
)

const (
	Traits = "traits"
	Scopes = "scopes"
)

type Application struct {
	Name string `json:"name"`
	// key of map is component name
	Components map[string]map[string]interface{} `json:"components"`
	Secrets    map[string]map[string]interface{} `json:"secrets"`
	Scopes     map[string]map[string]interface{} `json:"globalScopes"`
	CreateTime time.Time                         `json:"createTime,omitempty"`
	UpdateTime time.Time                         `json:"updateTime,omitempty"`
}

func LoadFromFile(fileName string) (*Application, error) {
	var app = &Application{}
	data, err := ioutil.ReadFile(fileName)
	if err != nil {
		if os.IsNotExist(err) {
			return app, nil
		}
		return nil, err
	}
	err = yaml.Unmarshal(data, app)
	if err != nil {
		return nil, err
	}
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
	return ioutil.WriteFile(filepath.Join(appDir, app.Name+".yaml"), out, 0644)
}

func (app *Application) Validate() error {
	if app == nil {
		return errors.New("app is nil")
	}
	if app.Name == "" {
		return errors.New("name is required")
	}
	if len(app.Components) == 0 {
		return errors.New("at least one component is required")
	}
	for name, comp := range app.Components {
		lenth := len(comp)
		if traits, ok := comp[Traits]; ok {
			lenth--
			switch trs := traits.(type) {
			case map[string]map[string]interface{}:
			case map[string]interface{}:
				for traitName, tr := range trs {
					_, ok := tr.(map[string]interface{})
					if !ok {
						return fmt.Errorf("trait %s in '%s' must be map", traitName, name)
					}
				}
			default:
				return fmt.Errorf("format of traits in '%s' must be nested map instead of %v", name, reflect.TypeOf(traits))
			}
		}
		if scopes, ok := comp[Scopes]; ok {
			lenth--
			_, ok := scopes.([]string)
			if !ok {
				return fmt.Errorf("format of scopes in '%s' must be string array", name)
			}
			//TODO(wonderflow) check scope exist
		}
		if lenth != 1 {
			return fmt.Errorf("you must have only one workload in component '%s'", name)
		}
		for workloadType, workload := range comp {
			if NotWorkload(workloadType) {
				continue
			}
			_, ok := workload.(map[string]interface{})
			if !ok {
				return fmt.Errorf("format of workload in %s must be map", name)
			}
			//TODO(wonderflow) check workload type exists
			//TODO(wonderflow) check arguments of workload is valid
		}
	}
	//TODO(wonderflow) check scope types
	return nil
}

func NotWorkload(tp string) bool {
	if tp == Scopes || tp == Traits {
		return true
	}
	return false
}

func (app *Application) GetComponents() []string {
	var components []string
	for name := range app.Components {
		components = append(components, name)
	}
	sort.Strings(components)
	return components
}

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
	traits, err := app.GetTraits(componentName)
	if err != nil {
		return nil, err
	}
	for t, tt := range traits {
		if t == traitType {
			return tt, nil
		}
	}
	return make(map[string]interface{}), nil
}

func (app *Application) GetWorkloadObject(componentName string) (*unstructured.Unstructured, string, error) {
	workloadType, workloadData := app.GetWorkload(componentName)
	if workloadType == "" {
		return nil, workloadType, errors.New(componentName + " workload not exist")
	}
	obj, err := EvalToObject(workloadType, workloadData)
	if err != nil {
		return nil, "", err
	}
	return obj, workloadType, nil
}

// ConvertDataByType will fix int become float after yaml.unmarshal
func ConvertDataByType(val interface{}, tp cue.Kind) interface{} {
	switch tp {
	case cue.FloatKind:
		switch rv := val.(type) {
		case int64:
			return float64(rv)
		case int:
			return float64(rv)
		}
	case cue.IntKind:
		switch rv := val.(type) {
		case float64:
			return int64(rv)
		}
	}
	return val
}

func EvalToObject(capName string, data map[string]interface{}) (*unstructured.Unstructured, error) {
	cap, err := plugins.LoadCapabilityByName(capName)
	if err != nil {
		return nil, err
	}
	for _, v := range cap.Parameters {
		val, ok := data[v.Name]
		if ok {
			data[v.Name] = ConvertDataByType(val, v.Type)
		}
	}
	jsondata, err := mycue.Eval(cap.DefinitionPath, capName, data)
	if err != nil {
		return nil, err
	}
	var obj = make(map[string]interface{})
	if err = json.Unmarshal([]byte(jsondata), &obj); err != nil {
		return nil, err
	}
	u := &unstructured.Unstructured{Object: obj}
	if cap.CrdInfo != nil {
		u.SetAPIVersion(cap.CrdInfo.APIVersion)
		u.SetKind(cap.CrdInfo.Kind)
	}
	return u, nil
}

func (app *Application) GetComponentTraits(componentName string) ([]v1alpha2.ComponentTrait, error) {
	var traits []v1alpha2.ComponentTrait
	rawTraits, err := app.GetTraits(componentName)
	if err != nil {
		return nil, err
	}
	for traitType, traitData := range rawTraits {
		obj, err := EvalToObject(traitType, traitData)
		if err != nil {
			return nil, err
		}
		//TODO(wonderflow): handle trait data input/output here
		obj.SetAnnotations(map[string]string{types.AnnTraitDef: traitType})
		traits = append(traits, v1alpha2.ComponentTrait{Trait: runtime.RawExtension{Object: obj}})
	}
	return traits, nil
}

func FormatDefaultHealthScopeName(appName string) string {
	return appName + "-default-health"
}

//TODO(wonderflow) add scope support here
func (app *Application) OAM(env *types.EnvMeta) ([]v1alpha2.Component, v1alpha2.ApplicationConfiguration, []oam.Object, error) {
	var appConfig v1alpha2.ApplicationConfiguration
	if err := app.Validate(); err != nil {
		return nil, appConfig, nil, err
	}
	appConfig.Name = app.Name
	appConfig.Namespace = env.Namespace

	var health v1alpha2.HealthScope
	health.Name = FormatDefaultHealthScopeName(app.Name)
	health.Namespace = env.Namespace
	health.Spec.WorkloadReferences = make([]v1alpha1.TypedReference, 0)

	var components []v1alpha2.Component
	for name := range app.Components {
		// fulfill component
		var component v1alpha2.Component
		component.Name = name
		component.Namespace = env.Namespace
		obj, workloadType, err := app.GetWorkloadObject(name)
		if err != nil {
			return nil, v1alpha2.ApplicationConfiguration{}, nil, err
		}
		anns := component.Annotations
		if anns == nil {
			anns = map[string]string{types.AnnWorkloadDef: workloadType}
		} else {
			anns[types.AnnWorkloadDef] = workloadType
		}
		component.Annotations = anns
		component.Spec.Workload.Object = obj
		components = append(components, component)

		var appConfigComp v1alpha2.ApplicationConfigurationComponent
		appConfigComp.ComponentName = name

		//TODO(wonderflow): Temporarily we add health scope here, should change to use scope framework
		appConfigComp.Scopes = append(appConfigComp.Scopes, v1alpha2.ComponentScope{ScopeReference: v1alpha1.TypedReference{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.HealthScopeKind,
			Name:       health.Name,
		}})

		//TODO(wonderflow): handle component data input/output here
		compTraits, err := app.GetComponentTraits(name)
		if err != nil {
			return nil, v1alpha2.ApplicationConfiguration{}, nil, err
		}
		appConfigComp.Traits = compTraits
		appConfig.Spec.Components = append(appConfig.Spec.Components, appConfigComp)
	}

	return components, appConfig, []oam.Object{&health}, nil
}
