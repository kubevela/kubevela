/*
Copyright 2026 The KubeVela Authors.

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

package module

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/module/naming"
)

func TestValidateModuleNameMustBeADNSLabel(t *testing.T) {
	for _, name := range []string{"Widget", "widget_kit", "widget.kit", "-widget", strings.Repeat("w", naming.MaxLabelValueLen+1)} {
		err := validateModuleName(name, "_module.cue")
		require.Error(t, err, "%q", name)
		assert.Contains(t, err.Error(), `module name "`+name+`" in _module.cue is invalid`)
	}
	assert.NoError(t, validateModuleName("widget-kit", "_module.cue"))
}

func TestValidateAPIVersionLength(t *testing.T) {
	long := "v" + strings.Repeat("1", naming.MaxLabelValueLen)
	err := validateAPIVersion(long, "v1/_version.cue")
	require.Error(t, err, "a well-formed version can still be too long to label with")
	assert.Contains(t, err.Error(), "is too long")

	assert.NoError(t, validateAPIVersion("v1beta2", "v1/_version.cue"))
}

func TestValidateDefinitionNameShapeAndLength(t *testing.T) {
	def := func(name string) map[string]interface{} {
		return map[string]interface{}{"metadata": map[string]interface{}{"name": name}}
	}

	err := validateDefinitionName(def("Bad_Name"), "v1/definitions/bad.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `has invalid metadata.name "Bad_Name"`)

	long := strings.Repeat("a", naming.MaxLabelValueLen+1)
	err = validateDefinitionName(def(long), "v1/definitions/long.yaml")
	require.Error(t, err, "a valid subdomain can still exceed what a label value holds")
	assert.Contains(t, err.Error(), "module references use it as a label value")

	assert.NoError(t, validateDefinitionName(def("widget.v2"), "v1/definitions/ok.yaml"), "a dotted subdomain is a valid definition name")
}
