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

package cli

import (
	"strings"
	"testing"

	"gotest.tools/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestFormatApplicationString(t *testing.T) {
	var (
		str string
		err error
	)

	app := &v1beta1.Application{}
	app.SetGroupVersionKind(v1beta1.ApplicationKindVersionKind)
	// This should not be preset in the formatted string
	app.ManagedFields = []v1.ManagedFieldsEntry{
		{
			Manager:     "",
			Operation:   "",
			APIVersion:  "",
			Time:        nil,
			FieldsType:  "",
			FieldsV1:    nil,
			Subresource: "",
		},
	}
	app.SetName("app-name")

	_, err = formatApplicationString("", app)
	assert.ErrorContains(t, err, "no format", "no format provided, should error out")

	_, err = formatApplicationString("invalid", app)
	assert.ErrorContains(t, err, "not supported", "invalid format provided, should error out")

	str, err = formatApplicationString("yaml", app)
	assert.NilError(t, err)
	assert.Equal(t, true, strings.Contains(str, `apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  creationTimestamp: null
  name: app-name
spec:
  components: null
status: {}
`), "formatted yaml is not correct")

	str, err = formatApplicationString("json", app)
	assert.NilError(t, err)
	assert.Equal(t, true, strings.Contains(str, `{
  "kind": "Application",
  "apiVersion": "core.oam.dev/v1beta1",
  "metadata": {
    "name": "app-name",
    "creationTimestamp": null
  },
  "spec": {
    "components": null
  },
  "status": {}
}`), "formatted json is not correct")

	_, err = formatApplicationString("jsonpath", app)
	assert.ErrorContains(t, err, "jsonpath template", "no jsonpath template provided, should not pass")

	str, err = formatApplicationString("jsonpath={.apiVersion}", app)
	assert.NilError(t, err)
	assert.Equal(t, str, "core.oam.dev/v1beta1")
}
