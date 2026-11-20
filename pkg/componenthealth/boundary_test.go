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

package componenthealth

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// Component health is evaluated wherever a component runs: in the Application controller
// today, in vela-agent for dispatched Components. So this package may know nothing of the
// Application, and its KubeVela dependencies are listed rather than open-ended.

const module = "github.com/oam-dev/kubevela/"

var allowedImports = map[string]string{
	"apis/core.oam.dev/common":  "the component and trait status types",
	"pkg/cue/definition/health": "evaluates the health, customStatus and details CUE",
	"pkg/oam/util":              "reads the rendered resources the CUE refers to",
}

func TestImportsAreListed(t *testing.T) {
	bp, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range bp.Imports { // non-test imports only
		if rel, ok := strings.CutPrefix(imp, module); ok {
			if _, listed := allowedImports[rel]; !listed {
				t.Errorf("new dependency on %s: component health must stay usable outside the Application controller; list it here only if that still holds", rel)
			}
		}
	}
}

// applicationAgnosticExceptions are names the API gave these types before components
// existed outside Applications. Nothing in them is Application-specific.
var applicationAgnosticExceptions = map[string]string{
	"ApplicationComponentStatus": "component status (Application.status.services, and the Component status to come)",
	"ApplicationTraitStatus":     "trait status within a component's status",
}

func TestIsApplicationAgnostic(t *testing.T) {
	bp, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range bp.GoFiles {
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			if _, allowed := applicationAgnosticExceptions[id.Name]; allowed {
				return true
			}
			if strings.Contains(id.Name, "Application") || strings.HasPrefix(id.Name, "LabelApp") || strings.HasPrefix(id.Name, "AnnotationApp") {
				t.Errorf("%s: %s names the Application; component health must work without one", fset.Position(id.Pos()), id.Name)
			}
			return true
		})
	}
}
