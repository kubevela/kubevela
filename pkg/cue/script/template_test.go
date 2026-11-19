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
	"fmt"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/stretchr/testify/assert"
)

var templateScript = `
metadata: {
	name:      "helm-repository"
	alias:     "Helm Repository"
	scope:     "system"
	sensitive: false
}
template: {
	output: {
		url: parameter.url
	}
	parameter: {
		// +usage=The public url of the helm chart repository.
		url: string
		// +usage=The username of basic auth repo.
		username: string
		// +usage=The password of basic auth repo.
		password?: string
		// +usage=The ca certificate of helm repository. Please encode this data with base64.
		caFile?: string

		options: "o1" | "o2"
	}
}
`

var templateWithContextScript = `
import (
	"strconv"
  )
metadata: {
	name:      "helm-repository"
	alias:     "Helm Repository"
	scope:     "system"
	sensitive: false
}
template: {
	output: {
		url: parameter.url
		name: context.name
		namespace: context.namespace
		sensitive: strconv.FormatBool(metadata.sensitive)
	}
	parameter: {
		// +usage=The public url of the helm chart repository.
		url: string
		// +usage=The username of basic auth repo.
		username: string
		// +usage=The password of basic auth repo.
		password?: string
		// +usage=The ca certificate of helm repository. Please encode this data with base64.
		caFile?: string
	}
}
`

var withPackage = `

package main

const: {
	// +usage=The name of the addon application
	name: "addon-loki"
}

outputs: {
	a: context.appName
}

parameter: {

	// global parameters

	// +usage=The namespace of the loki to be installed
	namespace: *"o11y-system" | string
	// +usage=The clusters to install
	clusters?: [...string]

	// loki parameters

	// +usage=Specify the image of loki
	image: *"grafana/loki" | string
	// +usage=Specify the imagePullPolicy of the image
	imagePullPolicy: *"IfNotPresent" | "Never" | "Always"
	// +usage=Specify the service type for expose loki. If empty, it will be not exposed.
	serviceType: *"ClusterIP" | "NodePort" | "LoadBalancer" | ""
	// +usage=Specify the storage size to use. If empty, emptyDir will be used. Otherwise pvc will be used.
	storage?: =~"^([1-9][0-9]{0,63})(E|P|T|G|M|K|Ei|Pi|Ti|Gi|Mi|Ki)$"
	// +usage=Specify the storage class to use.
	storageClassName?: string

	// agent parameters

	// +usage=Specify the type of log agents, if empty, no agent will be installed
	agent: *"" | "vector" | "promtail"
	// +usage=Specify the image of promtail
	promtailImage: *"grafana/promtail" | string
	// +usage=Specify the image of vector
	vectorImage: *"timberio/vector:0.24.0-distroless-libc" | string
}
`

var withTemplate = `
metadata: {
	name: "xxx"
}
template: {
	output: {
		name:    metadata.name
		appName: context.appName
		value:   parameter.value
	}
	parameter: {
		// +usage=Specify the value of the object
		value: {...}
		// +usage=Specify the cluster of the object
		cluster: *"" | string
	}
}
`

func TestMergeValues(t *testing.T) {
	var cueScript = CUE(templateScript)
	v, err := cueScript.MergeValues(nil, map[string]interface{}{
		"url":      "hub.docker.com",
		"username": "name",
	})
	assert.Equal(t, err, nil)
	output := v.LookupPath(cue.ParsePath("template.output"))
	assert.Equal(t, output.Err(), nil)
	var data = map[string]interface{}{}
	err = value.UnmarshalTo(output, &data)
	assert.Equal(t, err, nil)
	assert.Equal(t, data["url"], "hub.docker.com")
}

func TestRunAndOutput(t *testing.T) {
	var cueScript = BuildCUEScriptWithDefaultContext([]byte("context:{namespace:string \n name:string}"), []byte(templateWithContextScript))
	output, err := cueScript.RunAndOutput(map[string]interface{}{
		"name":      "nnn",
		"namespace": "ns",
	}, map[string]interface{}{
		"url":      "hub.docker.com",
		"username": "test",
		"password": "test",
		"caFile":   "test ca",
	})
	assert.Equal(t, err, nil)
	var data = map[string]interface{}{}
	err = value.UnmarshalTo(output, &data)
	assert.Equal(t, err, nil)
	assert.Equal(t, data["name"], "nnn")
	assert.Equal(t, data["namespace"], "ns")
	assert.Equal(t, data["url"], "hub.docker.com")
}

func TestRunAndOutputWithCueX(t *testing.T) {
	var cueScript = BuildCUEScriptWithDefaultContext([]byte("context:{namespace:string \n name:string}"), []byte(templateWithContextScript))
	output, err := cueScript.RunAndOutputWithCueX(context.Background(), map[string]interface{}{
		"name":      "nnn",
		"namespace": "ns",
	}, map[string]interface{}{
		"url":      "hub.docker.com",
		"username": "test",
		"password": "test",
		"caFile":   "test ca",
	}, "template", "output")
	assert.Equal(t, err, nil)
	var data = map[string]interface{}{}
	err = output.Decode(&data)
	assert.Equal(t, err, nil)
	assert.Equal(t, data["name"], "nnn")
	assert.Equal(t, data["namespace"], "ns")
	assert.Equal(t, data["url"], "hub.docker.com")
}

func TestValidateProperties(t *testing.T) {
	var cueScript = CUE(templateScript)
	// miss the required parameter
	err := cueScript.ValidateProperties(map[string]interface{}{
		"url": "hub.docker.com",
	})
	assert.Equal(t, err.(*ParameterError).Message, "This parameter is required")

	// wrong the parameter value type
	err = cueScript.ValidateProperties(map[string]interface{}{
		"url":      1,
		"username": "ddd",
	})
	assert.Equal(t, strings.Contains(err.(*ParameterError).Message, "conflicting values"), true)
	assert.Equal(t, strings.Contains(err.(*ParameterError).Name, "url"), true)

	// wrong the parameter value
	err = cueScript.ValidateProperties(map[string]interface{}{
		"url":      "ddd",
		"username": "ddd",
	})
	assert.Equal(t, strings.Contains(err.(*ParameterError).Message, "This parameter is required"), true)
	assert.Equal(t, strings.Contains(err.(*ParameterError).Name, "options"), true)

	// wrong the parameter value and no required value
	err = cueScript.ValidateProperties(map[string]interface{}{
		"url":      "ddd",
		"username": "ddd",
		"options":  "o3",
	})
	fmt.Println(err.(*ParameterError).Message)
	assert.Equal(t, strings.Contains(err.(*ParameterError).Name, "options"), true)
	assert.Equal(t, strings.Contains(err.(*ParameterError).Message, "2 errors in empty disjunction"), true)
}

func TestValidatePropertiesWithCueX(t *testing.T) {
	var cueScript = CUE(templateScript)
	ctx := context.Background()
	// miss the required parameter
	err := cueScript.ValidatePropertiesWithCueX(ctx, map[string]interface{}{
		"url": "hub.docker.com",
	})
	assert.Equal(t, err.(*ParameterError).Message, "This parameter is required")

	// wrong the parameter value type
	err = cueScript.ValidatePropertiesWithCueX(ctx, map[string]interface{}{
		"url":      1,
		"username": "ddd",
	})
	assert.Equal(t, strings.Contains(err.(*ParameterError).Message, "conflicting values"), true)
	assert.Equal(t, strings.Contains(err.(*ParameterError).Name, "url"), true)

	// wrong the parameter value
	err = cueScript.ValidatePropertiesWithCueX(ctx, map[string]interface{}{
		"url":      "ddd",
		"username": "ddd",
	})
	assert.Equal(t, strings.Contains(err.(*ParameterError).Message, "This parameter is required"), true)
	assert.Equal(t, strings.Contains(err.(*ParameterError).Name, "options"), true)

	// wrong the parameter value and no required value
	err = cueScript.ValidatePropertiesWithCueX(ctx, map[string]interface{}{
		"url":      "ddd",
		"username": "ddd",
		"options":  "o3",
	})
	fmt.Println(err.(*ParameterError).Message)
	assert.Equal(t, strings.Contains(err.(*ParameterError).Name, "options"), true)
	assert.Equal(t, strings.Contains(err.(*ParameterError).Message, "2 errors in empty disjunction"), true)
}

func TestParsePropertiesToSchemaWithCueX(t *testing.T) {
	cue := CUE([]byte(withPackage))
	ctx := context.Background()
	schema, err := cue.ParsePropertiesToSchemaWithCueX(ctx, "")
	assert.Equal(t, err, nil)
	assert.Equal(t, len(schema.Properties), 10)

	cue = CUE([]byte(withTemplate))
	schema, err = cue.ParsePropertiesToSchemaWithCueX(ctx, "template")
	assert.Equal(t, err, nil)
	assert.Equal(t, len(schema.Properties), 2)
}

func TestParseToValue(t *testing.T) {
	cases := map[string]struct {
		script    CUE
		expectErr bool
	}{
		"valid cue script": {
			script:    CUE(templateScript),
			expectErr: false,
		},
		"invalid cue script": {
			script: CUE(`
metadata: {
	name: "invalid"
	alias: "Invalid"
`),
			expectErr: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			v, err := tc.script.ParseToValue()
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.True(t, v.Exists())
			}
		})
	}
}

func TestParseToValueWithCueX(t *testing.T) {
	cases := map[string]struct {
		script    CUE
		expectErr bool
	}{
		"valid cue script": {
			script:    CUE(templateScript),
			expectErr: false,
		},
		"invalid cue script": {
			script: CUE(`
metadata: {
	name: "invalid"
	alias: "Invalid"
`),
			expectErr: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			v, err := tc.script.ParseToValueWithCueX(context.Background())
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.True(t, v.Exists())
			}
		})
	}
}

func TestParseToTemplateValue(t *testing.T) {
	cases := map[string]struct {
		script      CUE
		expectErr   bool
		errContains string
	}{
		"valid cue script": {
			script:    CUE(templateScript),
			expectErr: false,
		},
		"missing template field": {
			script: CUE(`
metadata: {
	name: "missing-template"
}
`),
			expectErr:   true,
			errContains: "the template cue is invalid",
		},
		"missing parameter field": {
			script: CUE(`
template: {
	output: {}
}
`),
			expectErr:   true,
			errContains: "the template cue must include the template.parameter field",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			v, err := tc.script.ParseToTemplateValue()
			if tc.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errContains)
			} else {
				assert.NoError(t, err)
				assert.True(t, v.Exists())
			}
		})
	}
}

func TestParseToTemplateValueWithCueX(t *testing.T) {
	cases := map[string]struct {
		script      CUE
		expectErr   bool
		errContains string
	}{
		"valid cue script": {
			script:    CUE(templateScript),
			expectErr: false,
		},
		"missing template field": {
			script: CUE(`
metadata: {
	name: "missing-template"
}
`),
			expectErr:   true,
			errContains: "the template cue must include the template field",
		},
		"missing parameter field": {
			script: CUE(`
template: {
	output: {}
}
`),
			expectErr:   true,
			errContains: "the template cue must include the template.parameter field",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			v, err := tc.script.ParseToTemplateValueWithCueX(context.Background())
			if tc.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errContains)
			} else {
				assert.NoError(t, err)
				assert.True(t, v.Exists())
			}
		})
	}
}
