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
	// key identifies the definition in the module, by kind and unprefixed name.
	key string
	// parentKey is the key of the definition it extends, when that is in the
	// module too.
	parentKey string
	// extendsRev is the revision pinned on the parent, as in "@v3", or "".
	extendsRev string
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
			md.key = definitionKey(md.def.GetKind(), md.def.GetName())
		} else if kind := kindForType(result.Definition.Type); kind != "" && result.Definition.Name != "" {
			// Keyed from the loader's metadata, so a child of a broken parent is
			// held back rather than applied against whatever is installed.
			md.key = definitionKey(kind, result.Definition.Name)
		}
		if md.key != "" {
			byKey[md.key] = len(defs)
		}
		defs = append(defs, md)
	}
	for i := range defs {
		if defs[i].err != nil {
			continue
		}
		extends := definitionExtends(defs[i].def)
		parentKey := definitionKey(defs[i].def.GetKind(), extendsBase(extends))
		if _, ok := byKey[parentKey]; ok && extends != "" {
			defs[i].parentKey = parentKey
			defs[i].extendsRev = strings.TrimPrefix(extends, extendsBase(extends))
		}
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
		if defs[i].parentKey != "" {
			if err := visit(byKey[defs[i].parentKey], append(path, defs[i].def.GetName())); err != nil {
				return err
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
			if ordered[i].parentKey != "" {
				if err := unstructured.SetNestedField(def.Object, prefix+definitionExtends(*def), "spec", "extends"); err != nil {
					return nil, err
				}
			}
			def.SetName(prefix + def.GetName())
		}
	}
	return ordered, nil
}

// linkToAppliedParent points a definition at the name its in-module parent was
// applied under, which --conflict=rename may have changed. It refuses when the
// parent was not applied, so the child never lands on a stale or missing one.
func linkToAppliedParent(md *moduleDefinition, applied map[string]string) error {
	if md.parentKey == "" {
		return nil
	}
	name, ok := applied[md.parentKey]
	if !ok {
		return fmt.Errorf("not applied, because %s, which it extends, was not applied", strings.TrimPrefix(md.parentKey, md.def.GetKind()+"/"))
	}
	return unstructured.SetNestedField(md.def.Object, name+md.extendsRev, "spec", "extends")
}

// kindForType maps a loader definition type to the kinds that can be extended.
func kindForType(defType string) string {
	switch defType {
	case "component":
		return "ComponentDefinition"
	case "trait":
		return "TraitDefinition"
	}
	return ""
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
