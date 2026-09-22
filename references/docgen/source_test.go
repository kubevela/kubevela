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

package docgen

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleSourceTemplate = `
import "strconv"

schema: {
	value:       int
	valueString: string
}

storage: {
	key:            "get-random-\(parameter.min)-\(parameter.max)"
	storageTTL:     "10s"
	onStaleFailure: "use-stale"
}

parameter: {
	min: *1 | int
	max: *5 | int
}

output: {
	value:       strconv.Atoi("1")
	valueString: "1"
}
`

func TestExtractSourceOutputs(t *testing.T) {
	outputs, err := extractSourceOutputs(sampleSourceTemplate)
	require.NoError(t, err)
	require.Len(t, outputs, 2)

	byName := map[string]string{}
	for _, o := range outputs {
		byName[o.Name] = o.Type.String()
	}
	assert.Equal(t, "int", byName["value"])
	assert.Equal(t, "string", byName["valueString"])
}

func TestExtractSourceOutputsNoSchema(t *testing.T) {
	outputs, err := extractSourceOutputs("parameter: {min: int}\noutput: {}")
	require.NoError(t, err)
	assert.Empty(t, outputs)
}

func TestExtractStorageFields(t *testing.T) {
	fields, err := extractStorageFields(sampleSourceTemplate)
	require.NoError(t, err)
	require.Len(t, fields, 3)

	byName := map[string]string{}
	var order []string
	for _, f := range fields {
		byName[f.Name] = f.Value
		order = append(order, f.Name)
	}
	// Order preserved from the template.
	assert.Equal(t, []string{"key", "storageTTL", "onStaleFailure"}, order)
	// Interpolation preserved verbatim, not evaluated; surrounding quotes stripped.
	assert.Contains(t, byName["key"], `\(parameter.min)`)
	assert.Equal(t, "10s", byName["storageTTL"])
	assert.Equal(t, "use-stale", byName["onStaleFailure"])
}

func TestExtractStorageFieldsMissing(t *testing.T) {
	fields, err := extractStorageFields("parameter: {min: int}\noutput: {}")
	require.NoError(t, err)
	assert.Empty(t, fields)
}
