package appfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"cuelang.org/go/cue"
	cueJson "cuelang.org/go/pkg/encoding/json"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/ghodss/yaml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/pkg/appfile/template"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	mycue "github.com/oam-dev/kubevela/pkg/cue"
)

type Context struct {
	Data      map[string]interface{}
	Namespace string
	IO        cmdutil.IOStreams
}

type Service map[string]interface{}

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
func (s Service) RenderService(ctx *Context, tm template.Manager, cfg *bytes.Buffer) (
	*v1alpha2.ApplicationConfigurationComponent, error) {

	// sort out configs by workload/trait
	workloadKeys := map[string]interface{}{}
	traitKeys := map[string]interface{}{}

outerLoop:
	for k, v := range s {
		switch k {
		case "build": // skip
			continue outerLoop
		}

		if tm.IsTrait(k) {
			traitKeys[k] = v
		} else if tm.IsWorkload(k) {
			workloadKeys[k] = v
		}
	}

	// render component
	component := &v1alpha2.Component{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha2.ComponentGroupVersionKind.GroupVersion().String(),
			Kind:       v1alpha2.ComponentKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ctx.Data["name"].(string),
			Namespace: ctx.Namespace,
		},
	}

	// only render webservice workload for now.
	u, err := evalComponent(tm.LoadTemplate("webservice"), ctx.Data, intifyValues(workloadKeys))
	if err != nil {
		return nil, fmt.Errorf("eval component failed: %w", err)
	}
	component.Spec.Workload.Object = u

	// render traits
	traits := []v1alpha2.ComponentTrait{}
	for k, v := range traitKeys {
		ts, err := evalTraits(tm.LoadTemplate(k), ctx.Data, intifyValues(v))
		if err != nil {
			return nil, fmt.Errorf("eval traits failed: %w", err)
		}
		for _, t := range ts {
			traits = append(traits, v1alpha2.ComponentTrait{
				Trait: runtime.RawExtension{
					Object: t,
				}},
			)
		}
	}

	// write component yaml
	b, err := yaml.Marshal(component)
	if err != nil {
		return nil, err
	}
	cfg.Write(b)
	cfg.WriteString("\n---\n")

	acComp := &v1alpha2.ApplicationConfigurationComponent{
		ComponentName: component.Name,
		Traits:        traits,
	}
	return acComp, nil
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
		return nil, fmt.Errorf("marshal final value err %v", err)
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
