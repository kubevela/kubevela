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
	"testing"

	"github.com/stretchr/testify/assert"

	wfprocess "github.com/kubevela/workflow/pkg/cue/process"

	"github.com/oam-dev/kubevela/pkg/cue/process"
)

const optionalFieldTemplate = `
$internal: {key: "demo", keyInputs: []}
schema: {meta: {region?: string, zone: string}, labels: [string]: string, extra?: {tier: string}}
output: {meta: {region: "eu-west-1", zone: "a"}}
`

func optionalFieldContext(t *testing.T) wfprocess.Context {
	t.Helper()
	pCtx := process.NewContext(process.ContextData{
		Namespace: "default", CompName: "web", AppName: "app",
	})
	pCtx.PushData(process.ContextAppSources, map[string]map[string]interface{}{"cfg": {}})
	pCtx.PushData(process.ContextAppSourceTypes, map[string]string{"cfg": "demo"})
	pCtx.PushData(process.ContextAppSourceTemplates, map[string]string{"demo": optionalFieldTemplate})
	return pCtx
}

// A required parameter is only explained by an optional source field when that
// is actually why it is absent, so each way of not being that case has to
// report false. Explaining one wrongly points the author at the wrong file.
func TestOptionalSourceField(t *testing.T) {
	pCtx := optionalFieldContext(t)
	for _, tc := range []struct {
		name    string
		params  map[string]interface{}
		missing string
		binding string
		field   string
		ok      bool
	}{
		{
			name:    "a whole read of a struct with an optional field",
			params:  map[string]interface{}{"meta": "$(source.cfg.meta)"},
			missing: "meta.region",
			binding: "cfg", field: "meta.region", ok: true,
		},
		{
			name:    "the binding read entire",
			params:  map[string]interface{}{"all": "$(source.cfg)"},
			missing: "all.meta.region",
			binding: "cfg", field: "meta.region", ok: true,
		},
		{
			// The outermost optional field is the one to name: making `tier`
			// required changes nothing while `extra` may be absent.
			name:    "a required field inside an optional parent",
			params:  map[string]interface{}{"extra": "$(source.cfg.extra)"},
			missing: "extra.tier",
			binding: "cfg", field: "extra", ok: true,
		},
		{
			name:    "a field the schema requires",
			params:  map[string]interface{}{"meta": "$(source.cfg.meta)"},
			missing: "meta.zone",
		},
		{
			name:    "a key of an open map, absent but not declared optional",
			params:  map[string]interface{}{"labels": "$(source.cfg.labels)"},
			missing: "labels.team",
		},
		{
			name:    "an expression that computes, so the schema's fields are not what lands",
			params:  map[string]interface{}{"meta": `$(source.cfg.meta.zone == "a" ? source.cfg.meta : source.cfg.meta)`},
			missing: "meta.region",
		},
		{
			name:    "a read interpolated into text, which is a string whatever it reads",
			params:  map[string]interface{}{"meta": "prefix-$(source.cfg.meta)"},
			missing: "meta.region",
		},
		{
			name:    "a parameter holding no expression at all",
			params:  map[string]interface{}{"meta": "literal"},
			missing: "meta.region",
		},
		{
			name:    "a parameter the properties never set",
			params:  map[string]interface{}{},
			missing: "meta.region",
		},
		{
			name:    "a top-level parameter, which no ancestor read can supply",
			params:  map[string]interface{}{"image": "$(source.cfg.meta)"},
			missing: "image",
		},
		{
			name:    "a read of a binding that is not declared",
			params:  map[string]interface{}{"meta": "$(source.other.meta)"},
			missing: "meta.region",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binding, field, ok := OptionalSourceField(pCtx, tc.params, tc.missing)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.binding, binding)
			assert.Equal(t, tc.field, field)
		})
	}

	// No context is no schemas, which is the admission path before sources are
	// pushed rather than an error.
	binding, field, ok := OptionalSourceField(nil, map[string]interface{}{"meta": "$(source.cfg.meta)"}, "meta.region")
	assert.False(t, ok)
	assert.Empty(t, binding)
	assert.Empty(t, field)
}
