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
	"fmt"
	"strings"
	"testing"

	"gotest.tools/assert"
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

func TestMergeValues(t *testing.T) {
	var cueScript = CUE(templateScript)
	value, err := cueScript.MergeValues(nil, map[string]interface{}{
		"url":      "hub.docker.com",
		"username": "name",
	})
	assert.Equal(t, err, nil)
	output, err := value.LookupValue("template", "output")
	assert.Equal(t, err, nil)
	var data = map[string]interface{}{}
	err = output.UnmarshalTo(&data)
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
	err = output.UnmarshalTo(&data)
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
