/*
Copyright 2024 The KubeVela Authors.

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

package schema

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/kubevela/pkg/cue/cuex"
	"github.com/kubevela/workflow/pkg/cue/model/value"

	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
)

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

// ErrGenerateOpenAPIV2JSONSchemaForCapability is the error while generating OpenAPI v3 schema
const ErrGenerateOpenAPIV2JSONSchemaForCapability = "cannot generate OpenAPI v3 JSON schema for capability %s: %v"

// ParsePropertiesToSchema parse the properties in cue script to the openapi schema
func ParsePropertiesToSchema(ctx context.Context, s string, templateFieldPath ...string) (*openapi3.Schema, error) {
	t := s + "\n" + BaseTemplate
	val, err := providers.DefaultCompiler.Get().CompileStringWithOptions(ctx, t, cuex.DisableResolveProviderFunctions{})
	if err != nil {
		return nil, err
	}
	var template cue.Value
	if len(templateFieldPath) == 0 {
		template = val
	} else {
		template = val.LookupPath(value.FieldPath(templateFieldPath...))
		if template.Err() != nil {
			return nil, fmt.Errorf("%w cue script: %s", template.Err(), s)
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

// ConvertOpenAPISchema2SwaggerObject converts OpenAPI v2 JSON schema to Swagger Object
func ConvertOpenAPISchema2SwaggerObject(data []byte) (*openapi3.Schema, error) {
	swagger, err := openapi3.NewLoader().LoadFromData(data)
	if err != nil {
		return nil, err
	}

	schemaRef, ok := swagger.Components.Schemas["parameter"]
	if !ok {
		return nil, errors.New(util.ErrGenerateOpenAPIV2JSONSchemaForCapability)
	}
	return schemaRef.Value, nil
}

// FixOpenAPISchema fixes tainted `description` filed, missing of title `field`.
func FixOpenAPISchema(name string, schema *openapi3.Schema) {
	t := schema.Type

	if t.Is("object") {
		for k, v := range schema.Properties {
			s := v.Value
			FixOpenAPISchema(k, s)
		}
	} else if t.Is("array") {
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
