package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	"k8s.io/apimachinery/pkg/runtime"

	mycue "github.com/cloud-native-application/rudrx/pkg/cue"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/cloud-native-application/rudrx/pkg/plugins"

	"github.com/cloud-native-application/rudrx/api/types"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

	"github.com/cloud-native-application/rudrx/pkg/utils/system"
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
	Scopes     map[string]map[string]interface{} `json:"appScopes"`
}

func Load(envName, appName string) (*Application, error) {
	appDir, err := system.GetApplicationDir(envName)
	if err != nil {
		return nil, fmt.Errorf("get app dir from env %s err %v", envName, err)
	}
	app := &Application{Name: appName}
	data, err := ioutil.ReadFile(filepath.Join(appDir, appName+".yaml"))
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
	return app, nil
}

func (app *Application) Save(envName, appName string) error {
	appDir, err := system.GetApplicationDir(envName)
	if err != nil {
		return fmt.Errorf("get app dir from env %s err %v", envName, err)
	}
	out, err := yaml.Marshal(app)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filepath.Join(appDir, appName+".yaml"), out, 0644)
}

func (app *Application) Validate() error {
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
			trs, ok := traits.(map[string]interface{})
			if !ok {
				return fmt.Errorf("format of traits in %s must be map", name)
			}
			for traitName, tr := range trs {
				_, ok := tr.(map[string]interface{})
				if !ok {
					return fmt.Errorf("trait %s in %s must be map", traitName, name)
				}
			}
		}
		if scopes, ok := comp[Scopes]; ok {
			lenth--
			_, ok := scopes.([]string)
			if !ok {
				return fmt.Errorf("format of scopes in %s must be string array", name)
			}
		}
		if lenth != 1 {
			return fmt.Errorf("you must have only one workload in component %s", name)
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

func (app *Application) GetWorkload(componentName string) (string, map[string]interface{}, error) {
	comp, ok := app.Components[componentName]
	if !ok {
		return "", nil, fmt.Errorf("%s not exist", componentName)
	}
	for tp, workload := range comp {
		if NotWorkload(tp) {
			continue
		}
		return tp, workload.(map[string]interface{}), nil
	}
	return "", nil, fmt.Errorf("workload not exist in %s", componentName)
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
	trs := t.(map[string]interface{})
	traits := make(map[string]map[string]interface{})
	for k, v := range trs {
		traits[k] = v.(map[string]interface{})
	}
	return traits, nil
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

func (app *Application) GetWorkloadObject(componentName string) (*unstructured.Unstructured, error) {
	workloadType, workloadData, err := app.GetWorkload(componentName)
	if err != nil {
		return nil, err
	}
	return EvalToObject(workloadType, workloadData)
}

func EvalToObject(capName string, data map[string]interface{}) (*unstructured.Unstructured, error) {
	cap, err := plugins.LoadCapabilityByName(capName)
	if err != nil {
		return nil, err
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
		u.SetAPIVersion(cap.CrdInfo.ApiVersion)
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
		obj.SetAnnotations(map[string]string{types.TraitDefLabel: traitType})
		traits = append(traits, v1alpha2.ComponentTrait{Trait: runtime.RawExtension{Object: obj}})
	}
	return traits, nil
}

//TODO(wonderflow) add scope support here
func (app *Application) OAM(env *types.EnvMeta) ([]v1alpha2.Component, v1alpha2.ApplicationConfiguration, error) {
	var appConfig v1alpha2.ApplicationConfiguration
	if err := app.Validate(); err != nil {
		return nil, appConfig, err
	}
	appConfig.Name = app.Name
	appConfig.Namespace = env.Namespace

	var components []v1alpha2.Component

	for name := range app.Components {
		// fulfill component
		var component v1alpha2.Component
		component.Name = name
		component.Namespace = env.Namespace
		obj, err := app.GetWorkloadObject(name)
		if err != nil {
			return nil, v1alpha2.ApplicationConfiguration{}, err
		}
		labels := component.Labels
		if labels == nil {
			labels = map[string]string{types.ComponentWorkloadDefLabel: name}
		} else {
			labels[types.ComponentWorkloadDefLabel] = name
		}
		component.Labels = labels
		component.Spec.Workload.Object = obj
		components = append(components, component)

		var appConfigComp v1alpha2.ApplicationConfigurationComponent
		appConfigComp.ComponentName = name
		//TODO(wonderflow): handle component data input/output here
		compTraits, err := app.GetComponentTraits(name)
		if err != nil {
			return nil, v1alpha2.ApplicationConfiguration{}, err
		}
		appConfigComp.Traits = compTraits
		appConfig.Spec.Components = append(appConfig.Spec.Components, appConfigComp)
	}
	return components, appConfig, nil
}
