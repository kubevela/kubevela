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

package helm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/encoding/openapi"
	"cuelang.org/go/encoding/yaml"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
)

var (
	getters = getter.Providers{
		getter.Provider{
			Schemes: []string{"http", "https"},
			New:     getter.NewHTTPGetter,
		},
	}
)

// GetChartValuesJSONSchema fetched the Chart bundle and get JSON schema of Values
// file.  If the Chart provides a 'values.json.schema' file, use it directly.
// Otherwise, try to generate a JSON schema based on the Values file.
func GetChartValuesJSONSchema(ctx context.Context, h *common.Helm) ([]byte, error) {
	releaseSpec, repoSpec, err := decodeHelmSpec(h)
	if err != nil {
		return nil, errors.WithMessage(err, "Helm spec is invalid")
	}
	chartSpec := releaseSpec.Chart.Spec
	files, err := loadChartFiles(ctx, repoSpec.URL, chartSpec.Chart, chartSpec.Version)
	if err != nil {
		return nil, errors.WithMessage(err, "cannot load Chart files")
	}
	var values *loader.BufferedFile
	for _, f := range files {
		switch f.Name {
		case "values.yaml", "values.yml":
			values = f
		case "values.schema.json":
			// use the JSON schema file if exists
			return f.Data, nil
		default:
			continue
		}
	}
	if values == nil {
		return nil, errors.New("cannot find 'values.schema.json' or 'values.yaml' file in the Chart")
	}
	// try to generate a schema based on Values file
	generatedSchema, err := generateSchemaFromValues(values.Data)
	if err != nil {
		return nil, errors.WithMessage(err, "cannot generate schema from Values file")
	}
	return generatedSchema, nil
}

// generateSchemaFromValues generate OpenAPIv3 schema based on Chart Values
// file.
func generateSchemaFromValues(values []byte) ([]byte, error) {
	valuesIdentifier := "values"
	cuectx := cuecontext.New()
	// convert Values yaml to CUE
	file, err := yaml.Extract("", string(values))
	if err != nil {
		return nil, errors.Wrap(err, "cannot extract Values.yaml to CUE")
	}
	ins := cuectx.BuildFile(file)
	// get the streamed CUE including the comments which will be used as
	// 'description' in the schema
	c, err := format.Node(ins.Value().Syntax(cue.Docs(true)), format.Simplify())
	if err != nil {
		return nil, errors.Wrap(err, "cannot format CUE generated from Values.yaml")
	}
	// cue openapi encoder only works on top-level identifier, we have to add
	// an identifier manually
	valuesStr := fmt.Sprintf("#%s:{\n%s\n}", valuesIdentifier, string(c))

	val := cuecontext.New().CompileString(valuesStr)
	if val.Err() != nil {
		return nil, errors.Wrap(val.Err(), "cannot compile CUE generated from Values.yaml")
	}
	// generate OpenAPIv3 schema through cue openapi encoder
	rawSchema, err := openapi.Gen(val, &openapi.Config{})
	if err != nil {
		return nil, errors.Wrap(err, "cannot generate OpenAPIv3 schema")
	}
	rawSchema, err = makeSwaggerCompatible(rawSchema)
	if err != nil {
		return nil, errors.WithMessage(err, "cannot make CUE-generated schema compatible with Swagger")
	}

	var out = &bytes.Buffer{}
	_ = json.Indent(out, rawSchema, "", "   ")
	// load schema into Swagger to validate it compatible with Swagger OpenAPIv3
	fullSchemaBySwagger, err := openapi3.NewLoader().LoadFromData(out.Bytes())
	if err != nil {
		return nil, errors.Wrap(err, "cannot load schema by SwaggerLoader")
	}
	valuesSchema := fullSchemaBySwagger.Components.Schemas[valuesIdentifier].Value
	changeEnumToDefault(valuesSchema)

	b, err := valuesSchema.MarshalJSON()
	if err != nil {
		return nil, errors.Wrap(err, "cannot marshall Values schema")
	}
	return b, nil
}

func loadChartFiles(ctx context.Context, repoURL, chart, version string) ([]*loader.BufferedFile, error) {
	url, err := repo.FindChartInRepoURL(repoURL, chart, version, "", "", "", getters)
	if err != nil {
		return nil, errors.Wrap(err, "cannot find Chart URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot fetch Chart from remote URL:%s", url)
	}
	//nolint:errcheck
	defer resp.Body.Close()
	files, err := loader.LoadArchiveFiles(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "cannot load Chart files")
	}
	return files, nil
}

// cue openapi encoder converts default in Chart Values as enum in schema
// changing enum to default makes the schema consistent with Chart Values
func changeEnumToDefault(schema *openapi3.Schema) {
	t := schema.Type
	switch t {
	case "object":
		for _, v := range schema.Properties {
			s := v.Value
			changeEnumToDefault(s)
		}
	case "array":
		if schema.Items != nil {
			changeEnumToDefault(schema.Items.Value)
		}
	}
	// change enum to default
	if len(schema.Enum) > 0 {
		schema.Default = schema.Enum[0]
		schema.Enum = nil
	}
	// remove all required fields, because fields in Values.yml are all optional
	schema.Required = nil
}

// cue openapi encoder converts 'items' field in an array type field into array,
// that's not compatible with OpenAPIv3. 'items' field should be an object.
func makeSwaggerCompatible(d []byte) ([]byte, error) {
	m := map[string]interface{}{}
	err := json.Unmarshal(d, &m)
	if err != nil {
		return nil, errors.Wrap(err, "cannot unmarshall schema")
	}
	handleItemsOfArrayType(m)
	b, err := json.Marshal(m)
	if err != nil {
		return nil, errors.Wrap(err, "cannot marshall schema")
	}
	return b, nil
}

// handleItemsOfArrayType will convert all 'items' of array type from array to object
// and remove enum in the items
func handleItemsOfArrayType(t map[string]interface{}) {
	for _, v := range t {
		if next, ok := v.(map[string]interface{}); ok {
			handleItemsOfArrayType(next)
		}
	}
	if t["type"] == "array" {
		if i, ok := t["items"].([]interface{}); ok {
			if len(i) > 0 {
				if itemSpec, ok := i[0].(map[string]interface{}); ok {
					handleItemsOfArrayType(itemSpec)
					itemSpec["enum"] = nil
					t["items"] = itemSpec
				}
			}
		}
	}
}
