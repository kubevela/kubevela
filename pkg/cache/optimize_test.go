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

package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestIndexDefinitionUsage(t *testing.T) {
	testCases := map[string]struct {
		optimize bool
		gate     bool
		expected bool
	}{
		"both on, so the quota check has its index": {true, true, true},
		"the feature is off, so nothing reads it":   {true, false, false},
		"no informer indexes at all":                {false, true, false},
		"neither":                                   {false, false, false},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			withFlags(t, tc.optimize, tc.gate)
			assert.Equal(t, tc.expected, indexDefinitionUsage())
		})
	}
}

func TestBaseTypeName(t *testing.T) {
	testCases := map[string]string{
		"webservice":     "webservice",
		"webservice@v1":  "webservice",
		"webservice@v10": "webservice",
		"":               "",
		"@v1":            "",
		"a@b@c":          "a",
	}
	for in, want := range testCases {
		assert.Equal(t, want, BaseTypeName(in), "input %q", in)
	}
}

func TestDefinitionUsageOf(t *testing.T) {
	comp := func(t string, traits ...string) common.ApplicationComponent {
		c := common.ApplicationComponent{Type: t}
		for _, tr := range traits {
			c.Traits = append(c.Traits, common.ApplicationTrait{Type: tr})
		}
		return c
	}
	appWith := func(comps ...common.ApplicationComponent) *v1beta1.Application {
		return &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Components: comps}}
	}

	testCases := map[string]struct {
		app      *v1beta1.Application
		expected []string
	}{
		"each type once": {appWith(comp("webservice"), comp("worker")),
			[]string{"component/webservice", "component/worker"}},
		"repeats collapse": {appWith(comp("webservice"), comp("webservice")),
			[]string{"component/webservice"}},
		"a pin buckets with its base": {appWith(comp("webservice@v1"), comp("webservice")),
			[]string{"component/webservice"}},
		"traits index beside their components": {appWith(comp("webservice", "gateway", "scaler")),
			[]string{"component/webservice", "trait/gateway", "trait/scaler"}},
		"a trait on two components collapses": {appWith(comp("webservice", "gateway"), comp("worker", "gateway")),
			[]string{"component/webservice", "component/worker", "trait/gateway"}},
		"a pinned trait buckets with its base": {appWith(comp("webservice", "gateway@v1"), comp("worker", "gateway")),
			[]string{"component/webservice", "component/worker", "trait/gateway"}},
		"a name shared by both kinds stays apart": {appWith(comp("gateway", "gateway")),
			[]string{"component/gateway", "trait/gateway"}},
		"nothing indexes under none": {appWith(), []string{}},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			assert.ElementsMatch(t, tc.expected, DefinitionUsageOf(tc.app))
		})
	}
}
