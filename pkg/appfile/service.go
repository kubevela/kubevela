package appfile

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"

	"cuelang.org/go/cue"
	cueJson "cuelang.org/go/pkg/encoding/json"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/pkg/appfile/template"
	mycue "github.com/oam-dev/kubevela/pkg/cue"
)

type Service map[string]interface{}

const DefaultWorkloadType = "webservice"

func (s Service) GetType() string {
	t, ok := s["type"]
	if !ok {
		return DefaultWorkloadType
	}
	return t.(string)
}

func (s Service) GetUserConfigName() string {
	t, ok := s["config"]
	if !ok {
		return ""
	}
	return t.(string)
}

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

// RenderService render all capabilities of a service to CUE values of a Component.
// It outputs a Component which will be marshaled as standalone Component and also returned AppConfig Component section.
func (s Service) RenderService(tm template.Manager, name, ns string, cg configGetter) (*v1alpha2.ApplicationConfigurationComponent, *v1alpha2.Component, error) {

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
	_, rawTmpl := tm.LoadTemplate(wtype)
	u, err := evalComponent(rawTmpl, ctxData, intifyValues(workloadKeys))
	if err != nil {
		return nil, nil, fmt.Errorf("eval component failed: %w", err)
	}
	component.Spec.Workload.Object = u

	// render traits
	traits := make([]v1alpha2.ComponentTrait, 0)
	for traitType, traitData := range traitKeys {
		defName, rawTmpl := tm.LoadTemplate(traitType)
		ts, err := evalTraits(rawTmpl, ctxData, intifyValues(traitData))
		if err != nil {
			return nil, nil, fmt.Errorf("eval traits failed: %w", err)
		}
		// one capability corresponds to one trait only
		if len(ts) == 1 {
			ts[0].SetLabels(map[string]string{oam.TraitTypeLabel: defName})
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

func (af *AppFile) GetServices() map[string]Service {
	return af.Services
}

func isIntegral(val float64) bool {
	return val == float64(int(val))
}

// JSON marshalling of user values will put integer into float,
// we have to change it back so that CUE check will succeed.
func intifyValues(raw interface{}) interface{} {
	switch v := raw.(type) {
	case map[string]interface{}:
		return intifyMap(v)
	case []interface{}:
		return intifyList(v)
	case float64:
		if isIntegral(v) {
			return int(v)
		}
		return v
	default:
		return raw
	}
}

func intifyList(l []interface{}) interface{} {
	l2 := make([]interface{}, 0, len(l))
	for _, v := range l {
		l2 = append(l2, intifyValues(v))
	}
	return l2
}

func intifyMap(m map[string]interface{}) interface{} {
	m2 := make(map[string]interface{}, len(m))
	for k, v := range m {
		m2[k] = intifyValues(v)
	}
	return m2
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
		return nil, fmt.Errorf("marshal final value failed: %v", err)
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
			return nil, fmt.Errorf("marshal final value err %v", err)
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

func evalComponent(raw string, ctxValues, userValues interface{}) (*unstructured.Unstructured, error) {
	appValue, err := getValueStruct(raw, ctxValues, userValues)
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
