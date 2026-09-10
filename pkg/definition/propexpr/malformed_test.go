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
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestHasExpressionReportsAMalformedOne separates "there is no expression here"
// from "there is one and it will not parse".
//
// Callers use HasExpression to decide whether to do expression work at all -
// admission before validating, every render path before resolving. Answering
// false for a typo meant nothing looked at it: admission accepted the
// Application and the render wrote `$(source.cfg.host` into the object as text.
func TestHasExpressionReportsAMalformedOne(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   interface{}
		want bool
	}{
		{"a well-formed expression", "$(source.cfg.host)", true},
		{"plain text", "no expression here", false},
		{"a dollar with no opener", "cost is $5", false},

		{"unclosed", "$(source.cfg.host", true},
		{"just the opener", "$(", true},
		{"empty expression", "$()", true},
		{"unclosed after text", "prefix $(source.a.b", true},

		{"nested in a map", map[string]interface{}{"a": "$(source.a.b"}, true},
		{"nested in a list", []interface{}{"plain", "$(source.a.b"}, true},
		{"a map of plain text", map[string]interface{}{"a": "plain"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, HasExpression(tc.in))
		})
	}
}
