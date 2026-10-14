/*
Copyright 2025 The KubeVela Authors.

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
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	pkgdef "github.com/oam-dev/kubevela/pkg/definition"
	"github.com/oam-dev/kubevela/pkg/definition/goloader"
)

// moduleDefinition is one definition of a module, parsed and ready to apply.
type moduleDefinition struct {
	result goloader.LoadResult
	def    pkgdef.Definition
	// err is why the definition could not be loaded or parsed.
	err error
}

// prepareModuleDefinitions parses a module's definitions, orders them so each
// is applied after the definition it extends within the module, and applies
// the name prefix. Admission resolves a parent live, so a child applied first
// is refused.
func prepareModuleDefinitions(results []goloader.LoadResult, prefix string) ([]moduleDefinition, error) {
	defs := make([]moduleDefinition, 0, len(results))
	byKey := map[string]int{}
	for _, result := range results {
		md := moduleDefinition{result: result, err: result.Error}
		if md.err == nil {
			md.err = md.def.FromCUEString(result.CUE, nil)
		}
		if md.err == nil {
			byKey[definitionKey(md.def.GetKind(), md.def.GetName())] = len(defs)
		}
		defs = append(defs, md)
	}

	ordered := make([]moduleDefinition, 0, len(defs))
	const (
		unvisited = iota
		visiting
		done
	)
	state := make([]int, len(defs))
	var visit func(i int, path []string) error
	visit = func(i int, path []string) error {
		switch state[i] {
		case done:
			return nil
		case visiting:
			cycle := append(append([]string{}, path...), defs[i].def.GetName())
			return fmt.Errorf("%s %s extends %s, so none of them can be applied",
				defs[i].def.GetKind(), cycle[0], strings.Join(cycle[1:], ", which extends "))
		}
		state[i] = visiting
		if defs[i].err == nil {
			if parent, ok := byKey[definitionKey(defs[i].def.GetKind(), extendsBase(definitionExtends(defs[i].def)))]; ok {
				if err := visit(parent, append(path, defs[i].def.GetName())); err != nil {
					return err
				}
			}
		}
		state[i] = done
		ordered = append(ordered, defs[i])
		return nil
	}
	for i := range defs {
		if err := visit(i, nil); err != nil {
			return nil, err
		}
	}

	if prefix != "" {
		for i := range ordered {
			if ordered[i].err != nil {
				continue
			}
			def := &ordered[i].def
			if extends := definitionExtends(*def); extends != "" {
				if _, ok := byKey[definitionKey(def.GetKind(), extendsBase(extends))]; ok {
					if err := unstructured.SetNestedField(def.Object, prefix+extends, "spec", "extends"); err != nil {
						return nil, err
					}
				}
			}
			def.SetName(prefix + def.GetName())
		}
	}
	return ordered, nil
}

// definitionExtends returns the spec.extends of a definition, or "".
func definitionExtends(def pkgdef.Definition) string {
	extends, _, _ := unstructured.NestedString(def.Object, "spec", "extends")
	return extends
}

// extendsBase strips a pinned revision, as in "webservice@v3".
func extendsBase(extends string) string {
	name, _, _ := strings.Cut(extends, "@")
	return name
}

func definitionKey(kind, name string) string { return kind + "/" + name }
