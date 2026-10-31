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

package mcp

import (
	"sort"
	"strconv"
	"strings"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
)

// ProducedResource is a Kubernetes kind a definition's template emits.
type ProducedResource struct {
	// Path is where the template declares it: output, outputs.<name> or
	// patch for a trait that modifies the workload.
	Path       string `json:"path"`
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
}

// ParameterUsage lists the template fields one parameter flows into.
type ParameterUsage struct {
	Parameter string   `json:"parameter"`
	Fields    []string `json:"fields"`
}

// templateFacts is what static inspection of a CUE template yields. It is
// syntactic: it reads literals and references, it does not evaluate, so it
// works for a template whose required parameters have no values.
type templateFacts struct {
	Produces []ProducedResource
	Usage    []ParameterUsage
	Err      string
}

// inspectTemplate parses a definition template and reports the kinds it
// produces and which fields each parameter reaches. A parse failure is
// reported in Err rather than returned, since the rest of an explanation
// is still useful.
func inspectTemplate(template string) templateFacts {
	var facts templateFacts
	if strings.TrimSpace(template) == "" {
		return facts
	}
	f, err := parser.ParseFile("definition.cue", template)
	if err != nil {
		facts.Err = err.Error()
		return facts
	}
	usage := map[string]map[string]bool{}
	var path []string
	ast.Walk(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Field:
			name, _, err := ast.LabelName(x.Label)
			if err != nil || name == "" {
				name = "?"
			}
			path = append(path, name)
			if res, ok := producedResource(path, x.Value); ok {
				facts.Produces = append(facts.Produces, res)
			}
		case *ast.SelectorExpr, *ast.IndexExpr:
			expr, _ := n.(ast.Expr)
			if param, ok := parameterReference(expr); ok && len(path) > 0 && path[0] != "parameter" {
				field := strings.Join(path, ".")
				if usage[param] == nil {
					usage[param] = map[string]bool{}
				}
				usage[param][field] = true
			}
		}
		return true
	}, func(n ast.Node) {
		if _, ok := n.(*ast.Field); ok {
			path = path[:len(path)-1]
		}
	})
	for param, fields := range usage {
		u := ParameterUsage{Parameter: param}
		for field := range fields {
			u.Fields = append(u.Fields, field)
		}
		sort.Strings(u.Fields)
		facts.Usage = append(facts.Usage, u)
	}
	sort.Slice(facts.Usage, func(i, j int) bool { return facts.Usage[i].Parameter < facts.Usage[j].Parameter })
	return facts
}

// producedResource recognises output, outputs.<name> and patch structs and
// reads their literal apiVersion and kind.
func producedResource(path []string, value ast.Expr) (ProducedResource, bool) {
	var where string
	switch {
	case len(path) == 1 && (path[0] == "output" || path[0] == "patch"):
		where = path[0]
	case len(path) == 2 && path[0] == "outputs":
		where = "outputs." + path[1]
	default:
		return ProducedResource{}, false
	}
	st, ok := value.(*ast.StructLit)
	if !ok {
		return ProducedResource{}, false
	}
	res := ProducedResource{Path: where}
	for _, decl := range st.Elts {
		fld, ok := decl.(*ast.Field)
		if !ok {
			continue
		}
		name, _, err := ast.LabelName(fld.Label)
		if err != nil {
			continue
		}
		switch name {
		case "apiVersion":
			res.APIVersion = stringLiteral(fld.Value)
		case "kind":
			res.Kind = stringLiteral(fld.Value)
		}
	}
	if res.Kind == "" && res.APIVersion == "" && where == "patch" {
		// A patch without a kind still produces something: it edits the workload.
		res.Kind = "(workload patch)"
	}
	return res, true
}

// parameterReference returns the top-level parameter name a selector chain
// such as parameter.image or parameter["image"].size refers to.
func parameterReference(expr ast.Expr) (string, bool) {
	for {
		switch x := expr.(type) {
		case *ast.SelectorExpr:
			if id, ok := x.X.(*ast.Ident); ok && id.Name == "parameter" {
				name, _, err := ast.LabelName(x.Sel)
				return name, err == nil && name != ""
			}
			expr = x.X
		case *ast.IndexExpr:
			if id, ok := x.X.(*ast.Ident); ok && id.Name == "parameter" {
				return stringLiteral(x.Index), stringLiteral(x.Index) != ""
			}
			expr = x.X
		default:
			return "", false
		}
	}
}

func stringLiteral(expr ast.Expr) string {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return strings.Trim(lit.Value, `"`)
	}
	return s
}
