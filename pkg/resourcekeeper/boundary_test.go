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

package resourcekeeper

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The apply / ResourceTracker / GC library is headed for its own repo, imported by
// kubevela and vela-agent (design/vela-core/agent-dispatch.md, phase 1b). It may depend
// on its own packages and on the API types, which move with it; nothing else.
//
// knownDebt is a ratchet: it lists dependencies still to remove, and the test fails both on
// a new one and on a listed one that has gone, so the list can only shrink. It is empty.

const module = "github.com/oam-dev/kubevela/"

var libraryPackages = []string{
	"pkg/resourcekeeper",
	"pkg/kubeutil",
	"pkg/resourcetracker",
	"pkg/utils/apply",
}

// apiPackages travel with the library in phase 1b. Besides the API types, pkg/oam (label
// and annotation keys: the on-cluster contract) and pkg/utils/errors count as API:
// kubevela-core-api already ships both next to apis/.
var apiPackages = []string{
	"apis/core.oam.dev/common",
	"apis/core.oam.dev/condition",
	"apis/core.oam.dev/v1alpha1",
	"apis/core.oam.dev/v1beta1",
	"pkg/oam",
	"pkg/utils/errors",
}

var allowed = append(append([]string{}, apiPackages...), libraryPackages...)

var knownDebt = map[string][]string{} // paid off; keep the ratchet as the guard

func TestLibraryImportsStayInsideTheBoundary(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	isAllowed := map[string]bool{}
	for _, p := range allowed {
		isAllowed[p] = true
	}
	for _, pkg := range libraryPackages {
		bp, err := build.ImportDir(filepath.Join(root, pkg), 0)
		if err != nil {
			t.Fatalf("%s: %v", pkg, err)
		}
		var outside []string
		for _, imp := range bp.Imports { // the package itself; its tests may reach wider
			if rel, ok := strings.CutPrefix(imp, module); ok && !isAllowed[rel] {
				outside = append(outside, rel)
			}
		}
		sort.Strings(outside)

		debt := map[string]bool{}
		for _, d := range knownDebt[pkg] {
			debt[d] = true
		}
		for _, d := range outside {
			if !debt[d] {
				t.Errorf("%s: new dependency on %s crosses the library boundary", pkg, d)
			}
			delete(debt, d)
		}
		for d := range debt {
			t.Errorf("%s no longer imports %s: remove it from knownDebt", pkg, d)
		}
	}
}

// Allowing an API package is only safe while it stays light: whatever it imports, the
// library imports too. So the non-apis entries may themselves import only apis/ packages.
func TestAPIPackagesStayLight(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range apiPackages {
		if strings.HasPrefix(pkg, "apis/") {
			continue
		}
		bp, err := build.ImportDir(filepath.Join(root, pkg), 0)
		if err != nil {
			t.Fatalf("%s: %v", pkg, err)
		}
		for _, imp := range bp.Imports {
			if rel, ok := strings.CutPrefix(imp, module); ok && !strings.HasPrefix(rel, "apis/") {
				t.Errorf("%s is allowed in the library as API, but now imports %s; move that dependency out or stop treating %s as API", pkg, rel, pkg)
			}
		}
	}
}

// The library knows no owner kind. Kinds implement resourcetracker.Tracked and supply
// Options (pkg/appkeeper for Applications), so no library code may name the Application
// API or its labels. The import checks above cannot catch this: those names live in
// allowed API packages. Comments are not checked.
var applicationAgnosticExceptions = map[string]string{
	"ApplicationGeneration": "ResourceTracker.spec field; renamed with the v1alpha1 API work",
	"AnnotationAppSharedBy": "the shared-by annotation's name on clusters; renaming needs a migration",
}

func TestLibraryIsApplicationAgnostic(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range libraryPackages {
		dir := filepath.Join(root, pkg)
		bp, err := build.ImportDir(dir, 0)
		if err != nil {
			t.Fatalf("%s: %v", pkg, err)
		}
		fset := token.NewFileSet()
		for _, name := range bp.GoFiles {
			f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
			if err != nil {
				t.Fatalf("%s/%s: %v", pkg, name, err)
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
					t.Errorf("%s: %s names the Application; move it to pkg/appkeeper or make it kind-agnostic", fset.Position(id.Pos()), id.Name)
				}
				return true
			})
		}
	}
}
