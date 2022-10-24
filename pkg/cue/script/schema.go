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
	"errors"
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/kubevela/workflow/pkg/cue/model/value"

	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// ParsePropertiesToSchema parse the properties in cue script to the openapi schema
// Read the template.parameter field
func (c CUE) ParsePropertiesToSchema(templateFieldPath ...string) (*openapi3.Schema, error) {
	val, err := c.ParseToValue()
	if err != nil {
		return nil, err
	}
	var template *value.Value
	if len(templateFieldPath) == 0 {
		template = val
	} else {
		template, err = val.LookupValue(templateFieldPath...)
		if err != nil {
			return nil, fmt.Errorf("%w cue script: %s", err, c)
		}
	}
	data, err := common.GenOpenAPI(template)
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

// FixOpenAPISchema fixes tainted `description` filed, missing of title `field`.
func FixOpenAPISchema(name string, schema *openapi3.Schema) {
	t := schema.Type
	switch t {
	case "object":
		for k, v := range schema.Properties {
			s := v.Value
			FixOpenAPISchema(k, s)
		}
	case "array":
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
