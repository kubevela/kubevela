/*
Copyright 2021 The KubeVela Authors.

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

package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/encoding/openapi"
	"github.com/getkin/kin-openapi/openapi3"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	oamcue "github.com/oam-dev/kubevela/pkg/cue"
)

// ErrNoSectionParameterInCue means there is not parameter section in Cue template of a workload
const ErrNoSectionParameterInCue = "capability %s doesn't contain section `parameter`"

// GenerateCUETemplateProperties get all properties of a capability
func (p *ParseReference) GenerateCUETemplateProperties(capability *types.Capability) (string, error) {
	t, err := prepareParameterCue(capability.Name, capability.CueTemplate)
	if err != nil {
		return "", err
	}

	r := cue.Runtime{}
	inst, err := r.Compile("", t+oamcue.BaseTemplate)
	if err != nil {
		return "", err
	}

	b, err := openapi.Gen(inst, nil)
	if err != nil {
		return "", err
	}

	var out bytes.Buffer
	err = json.Indent(&out, b, "", "   ")
	if err != nil {
		return "", err
	}

	schema, err := utils.ConvertOpenAPISchema2SwaggerObject(out.Bytes())
	if err != nil {
		return "", err
	}

	fixOpenAPISchema("", schema)

	jsonSchema, err := schema.MarshalJSON()
	if err != nil {
		return "", err
	}

	return string(jsonSchema), nil
}

// Int64Type is int64 type
type Int64Type = int64

// StringType is string type
type StringType = string

// BoolType is bool type
type BoolType = bool

// prepareParameterCue cuts `parameter` section form definition .cue file
func prepareParameterCue(capabilityName, capabilityTemplate string) (string, error) {
	var template string
	var withParameterFlag bool
	r := regexp.MustCompile("[[:space:]]*parameter:[[:space:]]*{.*")

	for _, text := range strings.Split(capabilityTemplate, "\n") {
		if r.MatchString(text) {
			// a variable has to be refined as a definition which starts with "#"
			text = fmt.Sprintf("parameter: #parameter\n#%s", text)
			withParameterFlag = true
		}
		template += fmt.Sprintf("%s\n", text)
	}

	if !withParameterFlag {
		return "", fmt.Errorf(ErrNoSectionParameterInCue, capabilityName)
	}
	return template, nil
}

// fixOpenAPISchema fixes tainted `description` filed, missing of title `field`.
func fixOpenAPISchema(name string, schema *openapi3.Schema) {
	t := schema.Type
	switch t {
	case "object":
		for k, v := range schema.Properties {
			s := v.Value
			fixOpenAPISchema(k, s)
		}
	case "array":
		fixOpenAPISchema("", schema.Items.Value)
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
