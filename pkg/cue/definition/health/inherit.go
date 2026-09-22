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

package health

import (
	"fmt"
	"strings"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
)

// InheritField is the directive a snippet uses to take a concern over from the
// definition it extends.
const InheritField = "$inherit"

// SuperField is where a snippet reads what its parent decided.
const SuperField = "$super"

// Snippets are one definition's status CUE.
type Snippets struct {
	Health  string
	Custom  string
	Details string
}

// inherits reports whether a snippet composes with its parent's or replaces it.
// Silence composes; `$inherit: false` takes it over.
func inherits(snippet string) bool {
	file, err := parser.ParseFile("-", snippet, parser.ParseComments)
	if err != nil {
		return true
	}
	for _, decl := range file.Decls {
		field, ok := decl.(*ast.Field)
		if !ok || labelOf(field.Label) != InheritField {
			continue
		}
		if lit, ok := field.Value.(*ast.BasicLit); ok && lit.Kind == token.FALSE {
			return false
		}
	}
	return true
}

// stripDirectives removes `$inherit` from a snippet. Every top-level field of a
// details snippet becomes a published status entry, directives included.
//
// `$inherit` is a field, not a line: a snippet may put it beside another
// declaration, so the field is removed rather than the text around it. A snippet
// that does not parse is returned as it stands, since reporting the syntax error
// is the compiler's job and mangling it first only obscures that.
func stripDirectives(snippet string) string {
	file, err := parser.ParseFile("-", snippet, parser.ParseComments)
	if err != nil {
		return snippet
	}

	kept := make([]ast.Decl, 0, len(file.Decls))
	found := false
	for _, decl := range file.Decls {
		if field, ok := decl.(*ast.Field); ok && labelOf(field.Label) == InheritField {
			found = true
			continue
		}
		kept = append(kept, decl)
	}
	if !found {
		return snippet
	}

	file.Decls = kept
	out, err := format.Node(file)
	if err != nil {
		return snippet
	}
	return string(out)
}

func labelOf(label ast.Label) string {
	if id, ok := label.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// withSuper adds what a level sees of its parent, after the snippet rather than
// before it: a snippet may open with `import`, which the CUE upgrade pass adds
// when it rewrites list arithmetic, and an import has to lead the file.
func withSuper(snippet, super string) string {
	return snippet + "\n" + super
}

// superHealth is what a snippet sees of its parent's verdict.
func superHealth(healthy bool) string {
	return fmt.Sprintf("%s: isHealth: %t\n", SuperField, healthy)
}

// superMessage is what a snippet sees of its parent's status line.
func superMessage(message string) string {
	return fmt.Sprintf("%s: message: %q\n", SuperField, message)
}

// checkHealthChain evaluates a chain's health policies, root first.
//
// A level that says nothing about health takes its parent's verdict whole. One
// that states `isHealth` adds to it: both must hold. One that declares
// `$inherit: false` decides alone, and may still read `$super.isHealth` to build
// on what its parent concluded.
func checkHealthChain(templateContext map[string]interface{}, snippets []Snippets, parameter interface{}) (bool, error) {
	healthy := true
	seen := false

	if !anyHealth(snippets) {
		// Nothing declares a policy, so there is nothing to evaluate and no
		// reason to marshal the rendered workload to find that out.
		return true, nil
	}

	// Formatted once and shared: it holds the whole rendered workload.
	runtimeContextBuff, err := formatRuntimeContext(templateContext, parameter)
	if err != nil {
		return false, err
	}

	for i := len(snippets) - 1; i >= 0; i-- {
		policy := snippets[i].Health
		if strings.TrimSpace(policy) == "" {
			continue
		}
		own := stripDirectives(policy)
		if seen {
			own = withSuper(own, superHealth(healthy))
		}

		got, err := checkHealthWith(runtimeContextBuff, own)
		if err != nil {
			return false, err
		}
		switch {
		case !seen:
			healthy = got
		case inherits(policy):
			healthy = healthy && got
		default:
			healthy = got
		}
		seen = true
	}
	return healthy, nil
}

// statusMessageChain evaluates a chain's custom status, root first.
//
// A level that states a message replaces its parent's, since a status line is
// one line. `$super.message` is there for one that would rather extend it.
func statusMessageChain(templateContext map[string]interface{}, snippets []Snippets, parameter interface{}) (string, error) {
	message := ""
	seen := false

	if !anyCustom(snippets) {
		return "", nil
	}

	runtimeContextBuff, err := formatRuntimeContext(templateContext, parameter)
	if err != nil {
		return "", err
	}

	for i := len(snippets) - 1; i >= 0; i-- {
		custom := snippets[i].Custom
		if strings.TrimSpace(custom) == "" {
			continue
		}
		own := stripDirectives(custom)
		if seen {
			own = withSuper(own, superMessage(message))
		}

		got, err := statusMessageWith(runtimeContextBuff, own)
		if err != nil {
			return message, err
		}
		message = got
		seen = true
	}
	return message, nil
}

// anyHealth and anyCustom report whether a chain declares anything at all, so a
// definition with no policy costs no marshalling of the workload it rendered.
func anyHealth(snippets []Snippets) bool {
	for _, s := range snippets {
		if strings.TrimSpace(s.Health) != "" {
			return true
		}
	}
	return false
}

func anyCustom(snippets []Snippets) bool {
	for _, s := range snippets {
		if strings.TrimSpace(s.Custom) != "" {
			return true
		}
	}
	return false
}

// detailsChain joins a chain's details snippets, root first.
//
// Details are a flat map of fields, so concatenation is the merge: a child's
// entries join its parent's and CUE unifies any they share. A level declaring
// `$inherit: false` starts the map again from itself.
func detailsChain(snippets []Snippets) string {
	var parts []string
	for i := len(snippets) - 1; i >= 0; i-- {
		details := snippets[i].Details
		if strings.TrimSpace(details) == "" {
			continue
		}
		if !inherits(details) {
			parts = nil
		}
		parts = append(parts, stripDirectives(details))
	}
	return joinSnippets(parts)
}

// joinSnippets concatenates CUE snippets, lifting every import to the top.
//
// Any level may open with `import`, which the CUE upgrade pass adds when it
// rewrites list arithmetic, and an import has to lead the file. Concatenating as
// text would leave a later snippet's import below an earlier one's fields.
func joinSnippets(parts []string) string {
	if len(parts) < 2 {
		return strings.Join(parts, "\n")
	}

	seen := map[string]bool{}
	var imports, bodies []string

	for _, part := range parts {
		file, err := parser.ParseFile("-", part, parser.ParseComments)
		if err != nil {
			// Not parseable on its own; leave it as written and let the compiler
			// report it.
			bodies = append(bodies, part)
			continue
		}

		var body []ast.Decl
		for _, decl := range file.Decls {
			imp, ok := decl.(*ast.ImportDecl)
			if !ok {
				body = append(body, decl)
				continue
			}
			for _, spec := range imp.Specs {
				if text := specText(spec); text != "" && !seen[text] {
					seen[text] = true
					imports = append(imports, "import "+text)
				}
			}
		}

		file.Decls = body
		out, err := format.Node(file)
		if err != nil {
			bodies = append(bodies, part)
			continue
		}
		bodies = append(bodies, string(out))
	}

	return strings.Join(append(imports, bodies...), "\n")
}

func specText(spec *ast.ImportSpec) string {
	if spec == nil || spec.Path == nil {
		return ""
	}
	if spec.Name != nil {
		return spec.Name.Name + " " + spec.Path.Value
	}
	return spec.Path.Value
}
