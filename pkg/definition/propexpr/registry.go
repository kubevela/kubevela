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

package propexpr

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	cueparser "cuelang.org/go/cue/parser"
)

// The context registry is loaded from context.cue and read directly - the types
// a surface offers are the CUE types declared there, not a Go restatement of
// them.
//
// That is the point rather than a convenience. When admission checked one
// hand-written table and render used another, the two disagreed in both
// directions at once: context.appRevisionNum passed the check and failed at
// render, while context.policyName was supplied at render and refused by the
// check. One declaration cannot do that.

//go:embed context.cue
var contextRegistrySource string

// contextRegistry is the parsed registry: one composed type per surface, plus
// the reason each excluded field is unavailable.
type contextRegistry struct {
	surfaces map[string]cue.Value
	labels   map[string]string
	plurals  map[string]string
	excluded map[string]string
}

// registryContext is long-lived: a cue.Value belongs to the context that made
// it, and the surface values are held for the process's lifetime.
var registryContext = cuecontext.New()

var registry = mustLoadContextRegistry()

func mustLoadContextRegistry() contextRegistry {
	r, err := loadContextRegistry(contextRegistrySource)
	if err != nil {
		// The file is embedded, so a failure here is a build-time mistake that
		// every surface depends on. There is no degraded mode worth running in.
		panic(fmt.Sprintf("loading the context registry: %v", err))
	}
	return r
}

func loadContextRegistry(source string) (contextRegistry, error) {
	v := registryContext.CompileString(source)
	if v.Err() != nil {
		return contextRegistry{}, v.Err()
	}

	surfaces := map[string]cue.Value{}
	iter, err := v.LookupPath(cue.ParsePath("surfaces")).Fields()
	if err != nil {
		return contextRegistry{}, fmt.Errorf("reading surfaces: %w", err)
	}
	for iter.Next() {
		surfaces[iter.Selector().Unquoted()] = iter.Value()
	}
	if len(surfaces) == 0 {
		return contextRegistry{}, fmt.Errorf("the registry declares no surfaces")
	}

	labels, err := stringMapAt(v, "labels")
	if err != nil {
		return contextRegistry{}, err
	}
	plurals, err := stringMapAt(v, "plurals")
	if err != nil {
		return contextRegistry{}, err
	}
	for name := range surfaces {
		if labels[name] == "" || plurals[name] == "" {
			return contextRegistry{}, fmt.Errorf(
				"surface %q needs both a label and a plural; every surface must name itself for error messages", name)
		}
	}

	excluded, err := excludedReasons(source)
	if err != nil {
		return contextRegistry{}, err
	}
	return contextRegistry{surfaces: surfaces, labels: labels, plurals: plurals, excluded: excluded}, nil
}

// stringMapAt decodes a top-level struct of strings from the registry.
func stringMapAt(v cue.Value, path string) (map[string]string, error) {
	out := map[string]string{}
	field := v.LookupPath(cue.ParsePath(path))
	if !field.Exists() {
		return out, nil
	}
	iter, err := field.Fields()
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	for iter.Next() {
		s, serr := iter.Value().String()
		if serr != nil {
			return nil, fmt.Errorf("%s.%s: %w", path, iter.Selector().Unquoted(), serr)
		}
		out[iter.Selector().Unquoted()] = s
	}
	return out, nil
}

// SurfacesOffering names, in the plural, every surface that offers a field.
func SurfacesOffering(field string) []string {
	var out []string
	for name, v := range registry.surfaces {
		if v.LookupPath(cue.MakePath(cue.Str(field))).Exists() {
			out = append(out, registry.plurals[name])
		}
	}
	sort.Strings(out)
	return out
}

// SurfacePlural names a surface in the plural, for a message that reads
// "unavailable in workflow steps" rather than naming one instance.
func SurfacePlural(surface string) string {
	if p := registry.plurals[surface]; p != "" {
		return p
	}
	return surface
}

// excludedReasons reads the +reason= annotation off each field in `excluded`.
//
// Comments are not part of a compiled cue.Value, so this walks the AST - the
// same approach the +sensitive marker uses in pkg/appfile.
func excludedReasons(source string) (map[string]string, error) {
	file, err := cueparser.ParseFile("-", source, cueparser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing the registry: %w", err)
	}
	out := map[string]string{}
	for _, decl := range file.Decls {
		field, ok := decl.(*ast.Field)
		if !ok {
			continue
		}
		if name, _, lerr := ast.LabelName(field.Label); lerr != nil || name != "excluded" {
			continue
		}
		st, ok := field.Value.(*ast.StructLit)
		if !ok {
			continue
		}
		for _, elt := range st.Elts {
			f, ok := elt.(*ast.Field)
			if !ok {
				continue
			}
			name, _, lerr := ast.LabelName(f.Label)
			if lerr != nil {
				continue
			}
			reason := annotationValue(f, "+reason=")
			if reason == "" {
				return nil, fmt.Errorf("excluded field %q has no +reason=; every exclusion must say why", name)
			}
			out[name] = reason
		}
	}
	return out, nil
}

// annotationValue reads a `// +tag=value` comment off a field, joining a
// continuation line so a long reason can wrap.
func annotationValue(field *ast.Field, tag string) string {
	var value string
	for _, cg := range field.Comments() {
		for _, c := range cg.List {
			line := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(c.Text), "//"))
			switch {
			case strings.HasPrefix(line, tag):
				value = strings.TrimSpace(strings.TrimPrefix(line, tag))
			case value != "" && line != "" && !strings.HasPrefix(line, "+"):
				value += " " + line
			}
		}
	}
	return value
}

// surfaceSchema returns the readable context for a named surface.
//
// An unknown surface yields a schema with no readable fields, which reports
// every read as unavailable rather than silently accepting it.
func surfaceSchema(surface string) ContextSchema {
	label := registry.labels[surface]
	if label == "" {
		label = surface
	}
	return ContextSchema{
		Surface:  label,
		key:      surface,
		value:    registry.surfaces[surface],
		excluded: registry.excluded,
	}
}

// knownField reports whether the registry accounts for a field at all - offered
// by some surface, or explicitly excluded from every one.
//
// This is what the drift tests assert: a field the render context carries and the
// registry has never heard of is the thing that must not happen. A field offered
// elsewhere but not here is accounted for - `why` says where it is available.
func knownField(name string) bool {
	if _, ok := registry.excluded[name]; ok {
		return true
	}
	for _, v := range registry.surfaces {
		if v.LookupPath(cue.MakePath(cue.Str(name))).Exists() {
			return true
		}
	}
	return false
}

// SurfaceOffers reports whether a surface offers a context field.
//
// Exported for the cache-key rules, which may only key on a field every surface
// that resolves a source can supply - otherwise a source would resolve from one
// call site and fail from another.
func SurfaceOffers(surface, field string) bool {
	v, ok := registry.surfaces[surface]
	if !ok {
		return false
	}
	return v.LookupPath(cue.MakePath(cue.Str(field))).Exists()
}

// ContextFor returns the readable context for a surface by name.
//
// An unrecognised surface falls back to the component's context, which is what
// every caller used before surfaces existed. Failing open here matters: a render
// path not yet taught to name itself should behave as it did, not lose its
// context and start rejecting valid expressions.
func ContextFor(surface string) ContextSchema {
	if !SurfaceDeclared(surface) {
		return ComponentContext
	}
	return surfaceSchema(surface)
}

// SurfaceDeclared reports whether the registry knows a surface, so a caller can
// distinguish "offers nothing" from "never heard of it".
func SurfaceDeclared(surface string) bool {
	_, ok := registry.surfaces[surface]
	return ok
}

// SurfaceNames lists the declared surfaces, for error messages and tests.
func SurfaceNames() []string {
	out := make([]string, 0, len(registry.surfaces))
	for name := range registry.surfaces {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
