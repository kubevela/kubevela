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

package celexpr

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common"
	celengine "github.com/kubevela/pkg/cel"
	"github.com/kubevela/pkg/cel/template"
)

// hyphenatedComponentRead is a component read with a dot whose name has a
// hyphen. Component names are Kubernetes names, so a hyphen is common, and CEL
// parses `component.my-db` as `component.my - db`. A name may start with a
// digit and hold a run of hyphens.
var hyphenatedComponentRead = regexp.MustCompile(`\bcomponent\.([A-Za-z0-9_][A-Za-z0-9_-]*-[A-Za-z0-9_-]*[A-Za-z0-9])`)

// Explain adds to an expression's compile error the index form to write where
// the error falls on a hyphenated component name read with a dot. The component
// root is a map, so component["my-db"] reads it; the suggestion is the whole
// expression with each such read rewritten. Any other error is returned as is.
//
// The engine reports CEL's own error, so the expression is compiled again here
// for the error's location; that happens only for an expression holding such a
// read.
func Explain(expr string, err error) error {
	if err == nil || !hyphenatedComponentRead.MatchString(expr) {
		return err
	}
	_, iss := Vela.DynEnv().Compile(expr)
	if iss == nil || iss.Err() == nil {
		return err
	}
	src := common.NewTextSource(expr)
	var fixed strings.Builder
	last, found := 0, false
	for _, m := range hyphenatedComponentRead.FindAllStringSubmatchIndex(expr, -1) {
		if !errorWithin(src, iss, utf8.RuneCountInString(expr[:m[0]]), utf8.RuneCountInString(expr[:m[1]])) {
			continue
		}
		fmt.Fprintf(&fixed, "%scomponent[%q]", expr[last:m[0]], expr[m[2]:m[3]])
		last, found = m[1], true
	}
	if !found {
		return err
	}
	fixed.WriteString(expr[last:])
	return fmt.Errorf("%w; a component whose name has a hyphen is read by index: write %s", err, fixed.String())
}

// ExplainFaults applies Explain to each fault of a CheckErrors, which is how a
// plan reports the expressions that will not compile. Any other error,
// including one wrapping a CheckErrors, is returned as is.
func ExplainFaults(err error) error {
	faults, ok := err.(celengine.CheckErrors) //nolint:errorlint // a wrapper's context must not be dropped
	if !ok {
		return err
	}
	out := make(celengine.CheckErrors, len(faults))
	for i, f := range faults {
		f.Err = Explain(f.Expr, f.Err)
		out[i] = f
	}
	return out
}

// errorWithin reports whether a compile error falls in the runes [start, end)
// of the source.
func errorWithin(src common.Source, iss *cel.Issues, start, end int) bool {
	for _, e := range iss.Errors() {
		if off, ok := src.LocationOffset(e.Location); ok && int(off) >= start && int(off) < end {
			return true
		}
	}
	return false
}

// OutputType is the engine's OutputType, with Explain applied to its error.
func OutputType(env *cel.Env, expr string) (*cel.Type, error) {
	t, err := Vela.OutputType(env, expr)
	return t, Explain(expr, err)
}

// PropertyReferences is the engine's PropertyReferences, with Explain applied
// to its error.
func PropertyReferences(expr string) ([]template.Reference, error) {
	refs, err := Vela.PropertyReferences(expr)
	return refs, Explain(expr, err)
}
