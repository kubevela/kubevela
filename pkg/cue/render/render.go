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

// Package render holds the CUE template helpers shared by the workload/trait
// render engine and by source resolution.
//
// It exists because those two are separate packages that need the same four
// functions. Go's internal rules put a package under pkg/cue/definition out of
// reach of a sibling, so the shared parts live one level up rather than being
// duplicated - which is how they would drift.
package render

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/kubevela/workflow/pkg/cue/model/value"
	"k8s.io/klog/v2"

	apitypes "github.com/oam-dev/kubevela/apis/types"
)

// ErrsFieldName is the field a template writes authored errors to.
const ErrsFieldName = "errs"

// MaxAnnotationValueLen is the budget for everything recorded on one object.
//
// Kubernetes allows 256KB across all annotations on an object; these are
// diagnostic, so they take a small slice and leave the rest to whatever else
// annotates the entry. Contrast MaxPropertyValueLen, which caps one value
// within this budget.
const MaxAnnotationValueLen = 4096

// MaxPropertyValueLen caps one property within that budget, so a single large
// value cannot crowd out every other property.
const MaxPropertyValueLen = 512

// Template closes a definition's CUE over the fields a render supplies, so it
// compiles on its own.
//
// `context` and `parameter` are open here rather than declared: the caller
// unifies real values in afterwards, and declaring shapes this package cannot
// know would reject templates it has no business judging.
func Template(templ string) string {
	return templ + `
context: _
parameter: _
`
}

// UserErrors reads the authored `errs:` field from a compiled CUE value and
// returns its non-empty entries.
//
// A definition uses `errs:` to say why it refused, in its own words, rather than
// leaving a reader to infer it from a unification failure. A malformed field is
// logged and treated as empty, so a mistake in error reporting never masks the
// result it was reporting on.
func UserErrors(val cue.Value, entityType, entityName string) []string {
	errs := val.LookupPath(value.FieldPath(ErrsFieldName))
	if !errs.Exists() {
		return nil
	}
	var userErrors []string
	if err := errs.Decode(&userErrors); err != nil {
		klog.Warningf("%s '%s' has malformed 'errs' field (expected []string): %v. Custom error reporting will be skipped.", entityType, entityName, err)
		return nil
	}
	filtered := userErrors[:0]
	for _, e := range userErrors {
		if strings.TrimSpace(e) != "" {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// Properties marshals a binding's properties for the annotation, replacing any
// value too large to record with a placeholder.
//
// Clamping happens per value rather than on the finished JSON, because clipping
// a JSON document mid-string leaves something no reader can parse - and an
// annotation that has to be parsed to be useful is worth keeping valid. The
// placeholder keeps the shape intact and says what was dropped, so a reader
// still learns which properties distinguish this entry from its neighbours.
func Properties(props map[string]interface{}) (string, bool, error) {
	out := make(map[string]interface{}, len(props))
	truncated := false

	for name, value := range props {
		raw, err := json.Marshal(value)
		if err != nil {
			out[name] = "<unrepresentable>"
			truncated = true
			continue
		}
		if len(raw) > MaxPropertyValueLen {
			out[name] = fmt.Sprintf("<omitted: %d bytes>", len(raw))
			truncated = true
			continue
		}
		out[name] = value
	}

	raw, err := json.Marshal(out)
	if err != nil {
		return "", false, err
	}
	// Still over budget, which takes a great many properties rather than one
	// large one. Record the names alone: valid JSON, and enough to see what the
	// binding passed.
	if len(raw) > MaxAnnotationValueLen {
		all := make([]string, 0, len(props))
		for name := range props {
			all = append(all, name)
		}
		sort.Strings(all)

		// Sorted, then filled to the cap: enough names to be useful, in an order
		// that is stable across writes so two entries can be compared. Marshalled
		// each time rather than length-counted, so the result is valid JSON by
		// construction rather than by arithmetic about quoting and commas.
		names := []string{}
		for _, name := range all {
			candidate, cerr := json.Marshal(append(names, name))
			if cerr != nil || len(candidate) > MaxAnnotationValueLen {
				break
			}
			names = append(names, name)
		}
		raw, err = json.Marshal(names)
		if err != nil {
			return "", false, err
		}
		truncated = true
	}
	return string(raw), truncated, nil
}

// ContextLabels renders the identity's context values as labels, so entries can
// be selected on them.
//
// A value is emitted only when both halves are legal: the field name (with any
// index folded in) must be a valid label key, and the value a valid label value.
// Neither is guaranteed - an index like "example.org/service-name" would put a
// second slash in the key. Whatever is skipped is still recorded whole in
// AnnotationSourceContext, so only selectability is lost.
func ContextLabels(ctx map[string]interface{}) map[string]string {
	out := map[string]string{}
	for field, value := range ctx {
		switch v := value.(type) {
		case map[string]interface{}:
			for index, indexed := range v {
				addContextLabel(out, field+"."+index, indexed)
			}
		default:
			addContextLabel(out, field, value)
		}
	}
	return out
}

func addContextLabel(out map[string]string, name string, value interface{}) {
	text, ok := value.(string)
	if !ok || text == "" {
		// A struct cannot be a label value, and an empty one carries nothing a
		// selector could use.
		return
	}
	key := apitypes.LabelSourceContextPrefix + name
	if len(validation.IsQualifiedName(key)) > 0 || len(validation.IsValidLabelValue(text)) > 0 {
		return
	}
	out[key] = text
}

// shouldTouchSourceCache throttles last-accessed updates: it returns true only
// when no marker exists yet or the existing one is older than half the entry's
// TTL, so a hot stale entry is not rewritten on every reconcile. The TTL is read
// from the entry's own annotation, defaulting to sourceCacheTTL.
