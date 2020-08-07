package types

import (
	"errors"
	"fmt"
	"sort"
)

const (
	DefaultOAMNS          = "oam-system"
	DefaultOAMReleaseName = "core-runtime"
	DefaultOAMChartName   = "crossplane-master/oam-kubernetes-runtime"
	DefaultOAMRuntimeName = "oam-kubernetes-runtime"
	DefaultOAMRepoName    = "crossplane-master"
	DefaultOAMRepoUrl     = "https://charts.crossplane.io/master"
	DefaultOAMVersion     = ">0.0.0-0"

	DefaultEnvName = "default"
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

func (app *Application) Valid() error {
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

func (app *Application) GetTraits(componentName string) (map[string]interface{}, error) {
	comp, ok := app.Components[componentName]
	if !ok {
		return nil, fmt.Errorf("%s not exist", componentName)
	}
	t, ok := comp[Traits]
	if !ok {
		return map[string]interface{}{}, nil
	}
	// assume it's valid, use Valid() to check
	traits := t.(map[string]interface{})
	return traits, nil
}

type EnvMeta struct {
	Namespace string `json:"namespace"`
}

const (
	TagCommandType = "commandType"

	TypeStart     = "Getting Started"
	TypeApp       = "Applications"
	TypeWorkloads = "Workloads"
	TypeTraits    = "Traits"
	TypeRelease   = "Release"
	TypeOthers    = "Others"
	TypeSystem    = "System"
)
