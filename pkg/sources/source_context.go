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

package sources

import (
	"encoding/json"
	"fmt"
	"strings"

	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/definition/cachekey"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
)

// sourceContextFile renders the `context:` a source template is compiled against.
//
// A source does not get the component's context. It gets exactly the fields the
// cache-key rules make readable, which is what keeps inference and the runtime
// from disagreeing: a field that would not contribute to the key is not present
// to be read. Admission rejects a template that reaches for one, and this makes
// the rejection unnecessary rather than merely correct - a definition applied
// with the webhook disabled still cannot depend on something the key ignores.
//
// context.name is the binding - the spec.sources[] entry this resolution is for -
// not the consuming component. That is the sense context.name carries everywhere
// else in KubeVela: the instance being rendered, rather than the definition it
// instantiates.
func sourceContextFile(values map[string]interface{}, bindingName string, fields []string) (string, error) {
	data := map[string]interface{}{}
	for _, field := range fields {
		if field == velaprocess.ContextName {
			continue // supplied below, from the binding rather than the component
		}
		if v := values[field]; v != nil {
			data[field] = v
		}
	}
	data[velaprocess.ContextName] = bindingName

	raw, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("rendering source context: %w", err)
	}
	return fmt.Sprintf("context: %s", string(raw)), nil
}

// sourceContext renders the context for a binding, using the current rules
// narrowed to what the calling surface actually offers.
//
// A source's context has never been simply its caller's: the rules decide what a
// source may read at all, and context.name is overridden to the binding. The
// surface is the second half of that projection. Today it removes nothing - every
// keyed field is offered by every surface that resolves a source, which
// TestKeyedFieldsExistInTheContextRegistry enforces - so this is the machinery
// for fields that are surface-specific, not a change in behaviour.
//
// An unrecognised surface is treated as offering everything the rules allow,
// rather than nothing. Failing open matters here: a caller that forgot to name
// its surface should behave as it did before, not silently lose its context.
func sourceContext(values map[string]interface{}, bindingName, surface string) (string, error) {
	rules, err := cachekey.LoadRules()
	if err != nil {
		return "", err
	}
	return sourceContextFile(values, bindingName, availableFields(rules.Fields(), surface))
}

// availableFields narrows the rules' field list to those the surface offers.
func availableFields(fields []string, surface string) []string {
	if surface == "" || !propexpr.SurfaceDeclared(surface) {
		return fields
	}
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		// context.name is supplied from the binding rather than the caller, so it
		// is available wherever a source resolves regardless of the surface.
		if field == velaprocess.ContextName || propexpr.SurfaceOffers(surface, field) {
			out = append(out, field)
		}
	}
	return out
}

// identityContext gathers the values named by the generated keyInputs list.
//
// Exactly those, and no more: hashing every label on the object would change the
// identity whenever GitOps stamped an unrelated annotation, leaving the cache
// permanently cold. A field that is absent contributes nil and one that is
// present but empty contributes "", because a template may branch on the
// difference and the identity has to draw it too.
func identityContext(values map[string]interface{}, bindingName string, inputs []string) map[string]interface{} {
	if len(inputs) == 0 {
		return nil
	}
	out := map[string]interface{}{}
	for _, in := range inputs {
		field, index, indexed := splitIndexed(in)

		var value interface{}
		switch {
		case field == velaprocess.ContextName:
			value = bindingName
		case indexed:
			value = lookupIndex(values[field], index)
		default:
			value = values[field]
		}

		if indexed {
			nested, ok := out[field].(map[string]interface{})
			if !ok {
				// Either nothing yet, or a bare read of the same field got here
				// first. Keep it under a reserved key rather than dropping it:
				// this map decides the cache key, so losing a contribution would
				// silently widen what an entry is shared by.
				nested = map[string]interface{}{}
				if prev, seen := out[field]; seen {
					nested[wholeFieldKey] = prev
				}
				out[field] = nested
			}
			nested[index] = value
			continue
		}
		if nested, ok := out[field].(map[string]interface{}); ok {
			// An indexed read of the same field got here first.
			nested[wholeFieldKey] = value
			continue
		}
		out[field] = value
	}
	return out
}

// wholeFieldKey holds a bare read of a field that is also read by index.
//
// The rules make a field one or the other, so this is unreachable today. It
// exists because the alternative is silent: whichever read arrived second would
// replace the first, and the identity would stop distinguishing entries it
// should.
const wholeFieldKey = "\x00whole"

// splitIndexed parses "appLabels[team]" into its field and index.
func splitIndexed(input string) (field, index string, indexed bool) {
	open := strings.Index(input, "[")
	if open < 0 || !strings.HasSuffix(input, "]") {
		return input, "", false
	}
	return input[:open], input[open+1 : len(input)-1], true
}

// lookupIndex reads one entry from a context map, returning nil when it is
// absent - which is distinct from an entry present with an empty value.
func lookupIndex(container interface{}, index string) interface{} {
	switch m := container.(type) {
	case map[string]string:
		if v, ok := m[index]; ok {
			return v
		}
	case map[string]interface{}:
		if v, ok := m[index]; ok {
			return v
		}
	}
	return nil
}
