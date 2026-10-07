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

package inherit

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// shippedWebservice is the path to the definition the fixture is taken from.
const shippedWebservice = "../../../vela-templates/definitions/internal/component/webservice.cue"

// ShippedTemplate extracts the `template:` body of a shipped definition, with
// the file's imports put back on top, which is what a template is to the render
// path: a file in its own right.
func ShippedTemplate(t *testing.T, path string) string {
	t.Helper()

	b, err := os.ReadFile(path)
	require.NoError(t, err)
	src := string(b)

	// Imports come grouped or singly, and a grouped block runs to its closing
	// paren.
	var imports []string
	lines := strings.Split(src, "\n")
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		switch {
		case strings.HasPrefix(trimmed, "import ("):
			imports = append(imports, lines[i])
			for i++; i < len(lines) && strings.TrimSpace(lines[i]) != ")"; i++ {
				imports = append(imports, lines[i])
			}
			if i < len(lines) {
				imports = append(imports, lines[i])
			}
		case strings.HasPrefix(trimmed, "import "):
			imports = append(imports, lines[i])
		}
	}

	i := strings.Index(src, "\ntemplate: {")
	require.GreaterOrEqual(t, i, 0, "no template block in %s", path)
	body := src[i+len("\ntemplate: {"):]

	// Take to the closing brace of the block, tracking depth.
	depth := 1
	end := -1
	for j, r := range body {
		switch r {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = j
			}
		}
		if end >= 0 {
			break
		}
	}
	require.GreaterOrEqual(t, end, 0, "unbalanced template block in %s", path)

	var deindented []string
	for _, line := range strings.Split(body[:end], "\n") {
		deindented = append(deindented, strings.TrimPrefix(line, "\t"))
	}

	return strings.TrimSpace(strings.Join(imports, "\n")+"\n"+strings.Join(deindented, "\n")) + "\n"
}

// The fixture is the shipped webservice, and stays that way.
//
// Testing against a real definition is the point: it exercises file-level
// imports, a `parameter` block declared through `#HealthProbe`, comprehensions
// over optional parameters and a conditional `outputs`. A copy that drifts tests
// none of that, and drifts quietly.
func TestTheFixtureIsTheShippedWebservice(t *testing.T) {
	fixture, err := os.ReadFile("testdata/webservice.cue")
	require.NoError(t, err)

	require.Equal(t, ShippedTemplate(t, shippedWebservice), string(fixture),
		"testdata/webservice.cue has drifted from the shipped definition; "+
			"regenerate it with `go test ./pkg/definition/inherit/ -run TestTheFixtureIsTheShippedWebservice -update`")
}
