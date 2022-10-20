/*
Copyright 2022 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package script

import (
	"encoding/json"
	"fmt"
	"strings"

	"cuelang.org/go/cue/errors"

	"github.com/kubevela/workflow/pkg/cue/model/value"

	"github.com/oam-dev/kubevela/pkg/cue"
)

// CUE the cue script with the template format
// Like this:
// ------------
// metadata: {}
//
//	template: {
//		parameter: {}
//		output: {}
//	}
//
// ------------
type CUE string

// BuildCUEScriptWithDefaultContext build a cue script instance from a byte array.
func BuildCUEScriptWithDefaultContext(defaultContext []byte, content []byte) CUE {
	return CUE(content) + "\n" + CUE(defaultContext)
}

// ParseToValue parse the cue script to cue.Value
func (c CUE) ParseToValue() (*value.Value, error) {
	// the cue script must be first, it could include the imports
	template := string(c) + "\n" + cue.BaseTemplate
	v, err := value.NewValue(template, nil, "")
	if err != nil {
		return nil, fmt.Errorf("fail to parse the template:%w", err)
	}
	return v, nil
}

// ParseToTemplateValue parse the cue script to cue.Value. It must include a valid template.
func (c CUE) ParseToTemplateValue() (*value.Value, error) {
	// the cue script must be first, it could include the imports
	template := string(c) + "\n" + cue.BaseTemplate
	v, err := value.NewValue(template, nil, "")
	if err != nil {
		return nil, fmt.Errorf("fail to parse the template:%w", err)
	}
	_, err = v.LookupValue("template")
	if err != nil {
		if v.Error() != nil {
			return nil, fmt.Errorf("the template cue is invalid:%w", v.Error())
		}
		return nil, fmt.Errorf("the template cue must include the template field:%w", err)
	}
	_, err = v.LookupValue("template", "parameter")
	if err != nil {
		return nil, fmt.Errorf("the template cue must include the template.parameter field")
	}
	return v, nil
}

// MergeValues merge the input values to the cue script
// The context variables could be referenced in all fields.
// The parameter only could be referenced in the template area.
func (c CUE) MergeValues(context interface{}, properties map[string]interface{}) (*value.Value, error) {
	parameterByte, err := json.Marshal(properties)
	if err != nil {
		return nil, fmt.Errorf("the parameter is invalid %w", err)
	}
	contextByte, err := json.Marshal(context)
	if err != nil {
		return nil, fmt.Errorf("the context is invalid %w", err)
	}
	var script = strings.Builder{}
	_, err = script.WriteString(string(c) + "\n")
	if err != nil {
		return nil, err
	}
	if properties != nil {
		_, err = script.WriteString(fmt.Sprintf("template: parameter: %s \n", string(parameterByte)))
		if err != nil {
			return nil, err
		}
	}
	if context != nil {
		_, err = script.WriteString(fmt.Sprintf("context: %s \n", string(contextByte)))
		if err != nil {
			return nil, err
		}
	}
	mergeValue, err := value.NewValue(script.String(), nil, "")
	if err != nil {
		return nil, err
	}
	if err := mergeValue.CueValue().Validate(); err != nil {
		return nil, fmt.Errorf("fail to validate the merged value %w", err)
	}
	return mergeValue, nil
}

// RunAndOutput run the cue script and return the values of the specified field.
// The output field must be under the template field.
func (c CUE) RunAndOutput(context interface{}, properties map[string]interface{}, outputField ...string) (*value.Value, error) {
	// Validate the properties
	if err := c.ValidateProperties(properties); err != nil {
		return nil, err
	}
	render, err := c.MergeValues(context, properties)
	if err != nil {
		return nil, fmt.Errorf("fail to merge the properties to template %w", err)
	}
	if render.Error() != nil {
		return nil, fmt.Errorf("fail to merge the properties to template %w", render.Error())
	}
	if len(outputField) == 0 {
		outputField = []string{"template", "output"}
	}
	return render.LookupValue(outputField...)
}

// ValidateProperties validate the input properties by the template
func (c CUE) ValidateProperties(properties map[string]interface{}) error {
	template, err := c.ParseToTemplateValue()
	if err != nil {
		return err
	}
	parameter, err := template.LookupValue("template", "parameter")
	if err != nil {
		return err
	}
	parameterStr, err := parameter.String()
	if err != nil {
		return fmt.Errorf("the parameter is invalid %w", err)
	}
	propertiesByte, err := json.Marshal(properties)
	if err != nil {
		return fmt.Errorf("the properties is invalid %w", err)
	}
	newCue := strings.Builder{}
	newCue.WriteString(parameterStr + "\n")
	newCue.WriteString(string(propertiesByte) + "\n")
	value, err := value.NewValue(newCue.String(), nil, "")
	if err != nil {
		return ConvertFieldError(err)
	}
	if err := value.CueValue().Validate(); err != nil {
		return ConvertFieldError(err)
	}
	_, err = value.CueValue().MarshalJSON()
	if err != nil {
		return ConvertFieldError(err)
	}
	return nil
}

// ParameterError the error report of the parameter field validation
type ParameterError struct {
	Name    string
	Message string
}

// Error return the error message
func (e *ParameterError) Error() string {
	return fmt.Sprintf("Field: %s Message: %s", e.Name, e.Message)
}

// ConvertFieldError convert the cue error to the field error
func ConvertFieldError(err error) error {
	var cueErr errors.Error
	if errors.As(err, &cueErr) {
		path := cueErr.Path()
		fieldName := path[len(path)-1]
		format, args := cueErr.Msg()
		message := fmt.Sprintf(format, args...)
		if strings.Contains(message, "cannot convert incomplete value") {
			message = "This parameter is required"
		}
		return &ParameterError{
			Name:    fieldName,
			Message: message,
		}
	}
	return err
}
