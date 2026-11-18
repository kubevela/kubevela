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
	"context"
	"encoding/json"
	"fmt"

	"cuelang.org/go/cue"
)

// AncestorProperties is what each ancestor of chain[0] receives as its
// parameter when chain[0] is given paramFile: the `$super: properties` each
// level passes up, as the render passes them. The i-th entry is for
// chain[i+1], as JSON of its concrete fields; a field that depends on the
// render's context is left out.
//
// It lets a check that belongs to an ancestor's parameters, such as
// +immutable, judge them where they arrive rather than where the Application
// wrote them.
func AncestorProperties(ctx context.Context, chain []Level, paramFile, contextFile string, compile CompileFunc) ([][]byte, error) {
	out := make([][]byte, 0, len(chain))
	for i := 0; i < len(chain)-1; i++ {
		staged, err := compile(ctx, join(chain[i].Template, paramFile, contextFile))
		if err != nil {
			return nil, fmt.Errorf("compile %s: %w", chain[i].Name, err)
		}
		super := staged.LookupPath(propertiesPath())
		raw, err := json.Marshal(concreteOf(super))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", chain[i].Name, err)
		}
		out = append(out, raw)
		paramFile = paramsFile(super, staged.LookupPath(cue.ParsePath(ParameterField)))
	}
	return out, nil
}

// concreteOf is the concrete part of a value as plain data, a default standing
// in for a disjunction that has one.
func concreteOf(v cue.Value) any {
	if !v.Exists() {
		return map[string]any{}
	}
	if d, ok := v.Default(); ok {
		v = d
	}
	if v.IncompleteKind() == cue.StructKind {
		out := map[string]any{}
		iter, err := v.Fields()
		if err != nil {
			return out
		}
		for iter.Next() {
			if c := concreteOf(iter.Value()); c != nil {
				out[iter.Selector().Unquoted()] = c
			}
		}
		return out
	}
	if !v.IsConcrete() {
		return nil
	}
	var data any
	if err := v.Decode(&data); err != nil {
		return nil
	}
	return data
}
