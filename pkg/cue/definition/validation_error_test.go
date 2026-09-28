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

package definition

import (
	"errors"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/cue/process"
)

func TestValidationError(t *testing.T) {
	const wl = `
output: {kind: "X", spec: replicas: parameter.replicas}
parameter: replicas: int
`
	const paramErr = "parameter.replicas: conflicting values int and \"x\" (mismatched types int and string)"
	const templateErr = "output.spec.paused: conflicting values false and true"
	cases := map[string]struct {
		template string
		params   map[string]any
		message  string
		want     ValidationError
	}{
		"user": {
			template: wl + "\nerrs: [\"boom\"]",
			params:   map[string]any{"replicas": 1},
			message:  "validation failed for workload wl:\n\nUser Errors:\n  boom",
			want:     ValidationError{Kind: "workload", Name: "wl", User: []string{"boom"}},
		},
		"parameter": {
			template: wl,
			params:   map[string]any{"replicas": "x"},
			message:  "validation failed for workload wl:\n\nParameter errors:\n  " + paramErr,
			want:     ValidationError{Kind: "workload", Name: "wl", Parameter: []string{paramErr}},
		},
		"template": {
			template: wl + "\noutput: spec: paused: true & false",
			params:   map[string]any{"replicas": 1},
			message:  "validation failed for workload wl:\n\nTemplate errors:\n  " + templateErr,
			want:     ValidationError{Kind: "workload", Name: "wl", Template: []string{templateErr}},
		},
		"all three": {
			template: wl + "\nerrs: [\"boom\", \"bang\"]\noutput: spec: paused: true & false",
			params:   map[string]any{"replicas": "x"},
			message: "validation failed for workload wl:\n\nUser Errors:\n  boom\n  bang\n\n\nParameter errors:\n  " + paramErr +
				"\n\n\nTemplate errors:\n  " + templateErr,
			want: ValidationError{Kind: "workload", Name: "wl", User: []string{"boom", "bang"}, Parameter: []string{paramErr}, Template: []string{templateErr}},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := process.NewContext(process.ContextData{AppName: "a", CompName: "c", Namespace: "default"})
			err := NewWorkloadAbstractEngine("wl").Complete(ctx, tc.template, tc.params)
			require.EqualError(t, err, tc.message, "the message is unchanged")
			var verr *ValidationError
			require.True(t, errors.As(err, &verr))
			require.Equal(t, tc.want, *verr)
		})
	}
}

func TestValidationErrorTrait(t *testing.T) {
	ctx := process.NewContext(process.ContextData{AppName: "a", CompName: "c", Namespace: "default"})
	require.NoError(t, NewWorkloadAbstractEngine("wl").Complete(ctx, "output: {kind: \"X\"}", map[string]any{}))
	err := NewTraitAbstractEngine("tr").Complete(ctx, "errs: [\"boom\"]\npatch: spec: x: parameter.x\nparameter: x: int\noutputs: o: {a: true & false}", map[string]any{"x": "y"})
	require.EqualError(t, err, "validation failed for trait tr:\n\nUser Errors:\n  boom\n\n\nParameter errors:\n  "+
		"parameter.x: conflicting values int and \"y\" (mismatched types int and string)\n\n\nTemplate errors:\n  outputs.o.a: conflicting values false and true")
	var verr *ValidationError
	require.True(t, errors.As(err, &verr))
	require.Equal(t, ValidationError{
		Kind: "trait", Name: "tr", User: []string{"boom"},
		Parameter: []string{"parameter.x: conflicting values int and \"y\" (mismatched types int and string)"},
		Template:  []string{"outputs.o.a: conflicting values false and true"},
	}, *verr)
}

// FormatCUEError's error keeps its message and carries its sections by kind,
// as a render's own ValidationError does.
func TestFormatCUEErrorIsTyped(t *testing.T) {
	v := cuecontext.New().CompileString(`
parameter: image: string
output: image: parameter.image
`)
	err := FormatCUEError(v.Validate(cue.Concrete(true)), "cannot generate manifests from", "component", "web", &v)
	require.Error(t, err)
	require.True(t, strings.HasPrefix(err.Error(), "cannot generate manifests from component web:"), err.Error())
	var verr *ValidationError
	require.True(t, errors.As(err, &verr), "typed, so a caller can read its sections")
	require.NotEmpty(t, verr.Parameter)
	require.Contains(t, verr.Parameter[0], "parameter.image")
}
