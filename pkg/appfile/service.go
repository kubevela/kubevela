package appfile

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/oam-dev/kubevela/pkg/appfile/config"

	"cuelang.org/go/cue"
	cueJson "cuelang.org/go/pkg/encoding/json"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/appfile/template"
	mycue "github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// Service defines the service spec for AppFile, it will contain all information a service realted including OAM component, traits, source to image, etc...
type Service map[string]interface{}

// DefaultWorkloadType defines the default service type if no type specified in Appfile
const DefaultWorkloadType = "webservice"

// GetType get type from AppFile
func (s Service) GetType() string {
	t, ok := s["type"]
	if !ok {
		return DefaultWorkloadType
	}
	return t.(string)
}

// GetUserConfigName get user config from AppFile, it will contain config file in it.
func (s Service) GetUserConfigName() string {
	t, ok := s["config"]
	if !ok {
		return ""
	}
	return t.(string)
}

// GetConfig will get OAM workload and trait information exclude inner section('build','type' and 'config')
func (s Service) GetConfig() map[string]interface{} {
	config := make(map[string]interface{})
outerLoop:
	for k, v := range s {
		switch k {
		case "build", "type", "config": // skip
			continue outerLoop
		}
		config[k] = v
	}
	return config
}

// GetBuild will get source to image build info
func (s Service) GetBuild() *Build {
	v, ok := s["build"]
	if !ok {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	build := &Build{}
	err = json.Unmarshal(b, build)
	if err != nil {
		panic(err)
	}
	return build
}

func (s Service) RenderServiceToApplicationComponent(tm template.Manager, serviceName string) (v1alpha2.ApplicationComponent, error) {

	// sort out configs by workload/trait
	workloadKeys := map[string]interface{}{}
	var traits []v1alpha2.ApplicationTrait

	wtype := s.GetType()

	comp := v1alpha2.ApplicationComponent{
		Name:         serviceName,
		WorkloadType: wtype,
	}

	for k, v := range s.GetConfig() {
		if tm.IsTrait(k) {
			trait := v1alpha2.ApplicationTrait{
				Name: k,
			}
			pts := &runtime.RawExtension{}
			jt, err := json.Marshal(v)
			if err != nil {
				return comp, err
			}
			if err := pts.UnmarshalJSON(jt); err != nil {
				return comp, err
			}
			trait.Properties = *pts
			traits = append(traits, trait)
			continue
		}
		workloadKeys[k] = v
	}

	// Handle workloadKeys to settings
	settings := &runtime.RawExtension{}
	pt, err := json.Marshal(workloadKeys)
	if err != nil {
		return comp, err
	}
	if err := settings.UnmarshalJSON(pt); err != nil {
		return comp, err
	}
	comp.Settings = *settings

	if len(traits) > 0 {
		comp.Traits = traits
	}

	return comp, nil
}

// RenderService render all capabilities of a service to CUE values of a Component.
// It outputs a Component which will be marshaled as standalone Component and also returned AppConfig Component section.
func (s Service) RenderService(tm template.Manager, name, ns string, cg config.Store) (*v1alpha2.ApplicationConfigurationComponent, *v1alpha2.Component, error) {

	// sort out configs by workload/trait
	workloadKeys := map[string]interface{}{}
	traitKeys := map[string]interface{}{}

	wtype := s.GetType()

	for k, v := range s.GetConfig() {
		if tm.IsTrait(k) {
			traitKeys[k] = v
			continue
		}
		workloadKeys[k] = v
	}

	// render component
	component := &v1alpha2.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}

	ctxData := map[string]interface{}{
		"name": name,
	}
	if cn := s.GetUserConfigName(); cn != "" {
		data, err := cg.GetConfigData(cn)
		if err != nil {
			return nil, nil, err
		}
		ctxData["config"] = data
	}

	// = ======= 以上 Merged ==================

	u, err := evalComponent(tm, wtype, ctxData, intifyValues(workloadKeys))
	if err != nil {
		return nil, nil, fmt.Errorf("eval service failed: %w", err)
	}
	component.Spec.Workload.Object = u

	// render traits
	traits := make([]v1alpha2.ComponentTrait, 0)
	for traitType, traitData := range traitKeys {
		ts, err := evalTraits(tm.LoadTemplate(traitType), ctxData, intifyValues(traitData))
		if err != nil {
			return nil, nil, fmt.Errorf("eval traits failed: %w", err)
		}
		// one capability corresponds to one trait only
		if len(ts) == 1 {
			ts[0].SetLabels(map[string]string{oam.TraitTypeLabel: traitType})
		}
		for _, t := range ts {
			traits = append(traits, v1alpha2.ComponentTrait{
				Trait: runtime.RawExtension{
					Object: t,
				}},
			)
		}
	}

	acComp := &v1alpha2.ApplicationConfigurationComponent{
		ComponentName: component.Name,
		Traits:        traits,
	}
	return acComp, component, nil
}

// GetServices will get all services defined in AppFile
func (af *AppFile) GetServices() map[string]Service {
	return af.Services
}

func getValueStruct(raw string, ctxValues, userValues interface{}) (*cue.Struct, error) {
	r := &cue.Runtime{}
	template, err := r.Compile("", raw+mycue.BaseTemplate)
	if err != nil {
		return nil, fmt.Errorf("compile CUE template failed: %w", err)
	}
	// fill values
	rootValue := template.Value()
	rootValue = rootValue.Fill(ctxValues, "context")
	rootValue = rootValue.Fill(intifyValues(userValues), "parameter")
	appValue, err := rootValue.Eval().Struct()
	if err != nil {
		return nil, fmt.Errorf("eval CUE template failed: %w", err)
	}
	return appValue, nil
}

func renderOneOutput(appValue *cue.Struct) (*unstructured.Unstructured, error) {
	outputField, err := appValue.FieldByName("output", true)
	if err != nil {
		return nil, fmt.Errorf("FieldByName('output'): %w", err)
	}

	final := outputField.Value
	data, err := cueJson.Marshal(final)
	if err != nil {
		return nil, fmt.Errorf("marshal final value failed: %w", err)
	}
	obj := make(map[string]interface{})
	if err = json.Unmarshal([]byte(data), &obj); err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{
		Object: obj,
	}, nil
}

func renderAllOutputs(field cue.FieldInfo) ([]*unstructured.Unstructured, error) {
	iter, err := field.Value.Fields()
	if err != nil {
		return nil, err
	}
	us := []*unstructured.Unstructured{}
	for iter.Next() {
		final := iter.Value()
		data, err := cueJson.Marshal(final)
		if err != nil {
			return nil, fmt.Errorf("marshal final value err %w", err)
		}
		// need to unmarshal it to a map to get rid of the outer spec name
		obj := make(map[string]interface{})
		if err = json.Unmarshal([]byte(data), &obj); err != nil {
			return nil, err
		}
		u := &unstructured.Unstructured{Object: obj}
		us = append(us, u)
	}
	return us, nil
}

func evalComponent(tm template.Manager, wtype string, ctxValues, userValues interface{}) (*unstructured.Unstructured, error) {
	workloadCueTemplate := tm.LoadTemplate(wtype)
	if workloadCueTemplate == "" {
		return nil, fmt.Errorf("no template found in capability %s", wtype)
	}
	appValue, err := getValueStruct(workloadCueTemplate, ctxValues, userValues)
	if err != nil {
		return nil, err
	}
	return renderOneOutput(appValue)
}

func evalTraits(raw string, ctxValues, userValues interface{}) ([]*unstructured.Unstructured, error) {
	appValue, err := getValueStruct(raw, ctxValues, userValues)
	if err != nil {
		return nil, err
	}

	_, err = appValue.FieldByName("output", true)
	if err != nil {
		outputField, err := appValue.FieldByName("outputs", true)
		if err != nil {
			return nil, errors.New("both output and outputs fields not found")
		}
		return renderAllOutputs(outputField)
	}
	u, err := renderOneOutput(appValue)
	if err != nil {
		return nil, err
	}
	return []*unstructured.Unstructured{u}, nil
}
