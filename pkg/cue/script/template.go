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
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/kubevela/pkg/cue/cuex"
	"github.com/kubevela/workflow/pkg/cue/model/sets"
	"github.com/kubevela/workflow/pkg/cue/model/value"

	"github.com/oam-dev/kubevela/pkg/appfile"
	velacuex "github.com/oam-dev/kubevela/pkg/cue/cuex"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
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

// BaseTemplate include base info provided by KubeVela for CUE template
const BaseTemplate = `
context: {
  name: string
  config?: [...{
    name: string
    value: string
  }]
  ...
}
`

// BuildCUEScriptWithDefaultContext build a cue script instance from a byte array.
func BuildCUEScriptWithDefaultContext(defaultContext []byte, content []byte) CUE {
	return CUE(content) + "\n" + CUE(defaultContext)
}

// ParseToValue parse the cue script to cue.Value
func (c CUE) ParseToValue() (cue.Value, error) {
	// the cue script must be first, it could include the imports
	template := string(c) + "\n" + BaseTemplate
	v := cuecontext.New().CompileString(template)
	return v, v.Err()
}

// ParseToValueWithCueX parse the cue script to cue.Value
func (c CUE) ParseToValueWithCueX(ctx context.Context) (cue.Value, error) {
	// the cue script must be first, it could include the imports
	template := string(c) + "\n" + BaseTemplate
	val, err := velacuex.ConfigCompiler.Get().CompileStringWithOptions(ctx, template, cuex.DisableResolveProviderFunctions{})
	if err != nil {
		return cue.Value{}, fmt.Errorf("failed to compile config template: %w", err)
	}
	return val, nil
}

// ParseToTemplateValue parse the cue script to cue.Value. It must include a valid template.
func (c CUE) ParseToTemplateValue() (cue.Value, error) {
	// the cue script must be first, it could include the imports
	template := string(c) + "\n" + BaseTemplate
	v := cuecontext.New().CompileString(template)
	if v.Err() != nil {
		return cue.Value{}, fmt.Errorf("fail to parse the template:%w", v.Err())
	}
	res := v.LookupPath(value.FieldPath("template"))
	if res.Err() != nil {
		return cue.Value{}, fmt.Errorf("the template cue is invalid:%w", res.Err())
	}
	parameter := v.LookupPath(value.FieldPath("template", "parameter"))
	if parameter.Err() != nil {
		return cue.Value{}, fmt.Errorf("the template cue must include the template.parameter field")
	}
	return v, nil
}

// ParseToTemplateValueWithCueX parse the cue script to cue.Value. It must include a valid template.
func (c CUE) ParseToTemplateValueWithCueX(ctx context.Context) (cue.Value, error) {
	val, err := c.ParseToValueWithCueX(ctx)
	if err != nil {
		return cue.Value{}, err
	}
	templateValue := val.LookupPath(cue.ParsePath("template"))
	if !templateValue.Exists() {
		return cue.Value{}, fmt.Errorf("the template cue must include the template field")
	}
	tmplParamValue := val.LookupPath(cue.ParsePath("template.parameter"))
	if !tmplParamValue.Exists() {
		return cue.Value{}, fmt.Errorf("the template cue must include the template.parameter field")
	}
	return val, nil
}

// MergeValues merge the input values to the cue script
// The context variables could be referenced in all fields.
// The parameter only could be referenced in the template area.
func (c CUE) MergeValues(context interface{}, properties map[string]interface{}) (cue.Value, error) {
	parameterByte, err := json.Marshal(properties)
	if err != nil {
		return cue.Value{}, fmt.Errorf("the parameter is invalid %w", err)
	}
	contextByte, err := json.Marshal(context)
	if err != nil {
		return cue.Value{}, fmt.Errorf("the context is invalid %w", err)
	}
	var script = strings.Builder{}
	_, err = script.WriteString(string(c) + "\n")
	if err != nil {
		return cue.Value{}, err
	}
	if properties != nil {
		_, err = script.WriteString(fmt.Sprintf("template: parameter: %s \n", string(parameterByte)))
		if err != nil {
			return cue.Value{}, err
		}
	}
	if context != nil {
		_, err = script.WriteString(fmt.Sprintf("context: %s \n", string(contextByte)))
		if err != nil {
			return cue.Value{}, err
		}
	}
	mergeValue := cuecontext.New().CompileString(script.String())
	if mergeValue.Err() != nil {
		return cue.Value{}, mergeValue.Err()
	}
	if err := mergeValue.Validate(); err != nil {
		return cue.Value{}, fmt.Errorf("fail to validate the merged value %w", err)
	}
	return mergeValue, nil
}

// RunAndOutput run the cue script and return the values of the specified field.
// The output field must be under the template field.
func (c CUE) RunAndOutput(context interface{}, properties map[string]interface{}, outputField ...string) (cue.Value, error) {
	// Validate the properties
	if err := c.ValidateProperties(properties); err != nil {
		return cue.Value{}, err
	}
	render, err := c.MergeValues(context, properties)
	if err != nil {
		return cue.Value{}, fmt.Errorf("fail to merge the properties to template %w", err)
	}
	if render.Err() != nil {
		return cue.Value{}, fmt.Errorf("fail to merge the properties to template %w", render.Err())
	}
	if len(outputField) == 0 {
		outputField = []string{"template", "output"}
	}
	lookup := render.LookupPath(value.FieldPath(outputField...))
	return lookup, lookup.Err()
}

// RunAndOutputWithCueX run the cue script and return the values of the specified field.
// The output field must be under the template field.
func (c CUE) RunAndOutputWithCueX(ctx context.Context, context interface{}, properties map[string]interface{}, outputField ...string) (cue.Value, error) {
	// Validate the properties
	if err := c.ValidatePropertiesWithCueX(ctx, properties); err != nil {
		return cue.Value{}, err
	}
	contextOption := cuex.WithExtraData("context", context)
	parameterOption := cuex.WithExtraData("template.parameter", properties)
	val, err := velacuex.ConfigCompiler.Get().CompileStringWithOptions(ctx, string(c), contextOption, parameterOption)
	if !val.Exists() {
		return cue.Value{}, fmt.Errorf("failed to compile config template")
	}
	if err != nil {
		return cue.Value{}, fmt.Errorf("failed to compile config template: %w", err)
	}
	if Error(val) != nil {
		return cue.Value{}, fmt.Errorf("failed to compile config template: %w", Error(val))
	}
	if len(outputField) == 0 {
		return val, nil
	}
	outputFieldVal := val.LookupPath(cue.ParsePath(strings.Join(outputField, ".")))
	if !outputFieldVal.Exists() {
		return cue.Value{}, fmt.Errorf("failed to lookup value: var(path=%s) not exist", strings.Join(outputField, "."))
	}
	return outputFieldVal, nil
}

// ValidateProperties validate the input properties by the template
func (c CUE) ValidateProperties(properties map[string]interface{}) error {
	template, err := c.ParseToTemplateValue()
	if err != nil {
		return err
	}
	parameter := template.LookupPath(value.FieldPath("template", "parameter"))
	if parameter.Err() != nil {
		return parameter.Err()
	}
	parameterStr, err := sets.ToString(parameter)
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
	newValue := cuecontext.New().CompileString(newCue.String())
	if newValue.Err() != nil {
		return ConvertFieldError(newValue.Err())
	}
	if err := newValue.Validate(); err != nil {
		return ConvertFieldError(err)
	}
	_, err = newValue.MarshalJSON()
	if err != nil {
		return ConvertFieldError(err)
	}
	return nil
}

// ValidatePropertiesWithCueX validate the input properties by the template
func (c CUE) ValidatePropertiesWithCueX(ctx context.Context, properties map[string]interface{}) error {
	template, err := c.ParseToTemplateValueWithCueX(ctx)
	if err != nil {
		return err
	}
	paramPath := cue.ParsePath("template.parameter")
	parameter := template.LookupPath(paramPath)
	if !parameter.Exists() {
		return fmt.Errorf("failed to lookup value: var(path=template.parameter) not exist")
	}
	props := parameter.FillPath(cue.ParsePath(""), properties)
	if props.Err() != nil {
		return ConvertFieldError(props.Err())
	}
	if err := props.Validate(); err != nil {
		return ConvertFieldError(err)
	}
	_, err = props.MarshalJSON()
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

// Error return value's error information.
func Error(val cue.Value) error {
	if !val.Exists() {
		return errors.New("empty value")
	}
	if err := val.Err(); err != nil {
		return err
	}
	var gerr error
	val.Walk(func(value cue.Value) bool {
		if err := value.Eval().Err(); err != nil {
			gerr = err
			return false
		}
		return true
	}, nil)
	return gerr
}

// ParsePropertiesToSchemaWithCueX parse the properties in cue script to the openapi schema
// Read the template.parameter field
func (c CUE) ParsePropertiesToSchemaWithCueX(ctx context.Context, templateFieldPath string) (*openapi3.Schema, error) {
	val, err := c.ParseToValueWithCueX(ctx)
	if err != nil {
		return nil, err
	}
	var template cue.Value
	if len(templateFieldPath) == 0 {
		template = val
	} else {
		template = val.LookupPath(cue.ParsePath(templateFieldPath))
		if !template.Exists() {
			return nil, fmt.Errorf("failed to lookup value: var(path=%s) not exist, cue script: %s", templateFieldPath, c)
		}
	}
	data, err := common.GenOpenAPIWithCueX(template)
	if err != nil {
		return nil, err
	}
	schema, err := ConvertOpenAPISchema2SwaggerObject(data)
	if err != nil {
		return nil, err
	}
	FixOpenAPISchema("", schema)
	return schema, nil
}

// FIXME: double code with pkg/schema/schema.go to avoid import cycle

// FixOpenAPISchema fixes tainted `description` filed, missing of title `field`.
func FixOpenAPISchema(name string, schema *openapi3.Schema) {
	t := schema.Type
	switch t {
	case  &openapi3.Types{openapi3.TypeObject}:
		for k, v := range schema.Properties {
			s := v.Value
			FixOpenAPISchema(k, s)
		}
	case  &openapi3.Types{openapi3.TypeArray}:
		if schema.Items != nil {
			FixOpenAPISchema("", schema.Items.Value)
		}
	}
	if name != "" {
		schema.Title = name
	}

	description := schema.Description
	if strings.Contains(description, appfile.UsageTag) {
		description = strings.Split(description, appfile.UsageTag)[1]
	}
	if strings.Contains(description, appfile.ShortTag) {
		description = strings.Split(description, appfile.ShortTag)[0]
		description = strings.TrimSpace(description)
	}
	schema.Description = description
}

// ConvertOpenAPISchema2SwaggerObject converts OpenAPI v2 JSON schema to Swagger Object
func ConvertOpenAPISchema2SwaggerObject(data []byte) (*openapi3.Schema, error) {
	swagger, err := openapi3.NewLoader().LoadFromData(data)
	if err != nil {
		return nil, err
	}

	schemaRef, ok := swagger.Components.Schemas[process.ParameterFieldName]
	if !ok {
		return nil, errors.New(util.ErrGenerateOpenAPIV2JSONSchemaForCapability)
	}
	return schemaRef.Value, nil
}
