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

package schema

import (
	"encoding/json"
	"testing"

	"cuelang.org/go/cue"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func openAPI(t *testing.T, doc string) *openapi3.Schema {
	t.Helper()
	s := &openapi3.Schema{}
	require.NoError(t, json.Unmarshal([]byte(doc), s))
	return s
}

func TestKindAt(t *testing.T) {
	deployment := openAPI(t, `{"type":"object","properties":{
		"apiVersion":{"type":"string"},
		"metadata":{"type":"object","properties":{"name":{"type":"string"},
			"labels":{"type":"object","additionalProperties":{"type":"string"}}}},
		"spec":{"type":"object","properties":{
			"replicas":{"type":"integer"},
			"ratio":{"type":"number"},
			"paused":{"type":"boolean"},
			"ports":{"type":"array","items":{"type":"object","properties":{"port":{"type":"integer"}}}},
			"raw":{"type":"object","x-kubernetes-preserve-unknown-fields":true},
			"anything":{}}},
		"status":{}}}`)

	for name, tc := range map[string]struct {
		path []string
		kind cue.Kind
		ok   bool
	}{
		"the whole object":                    {nil, cue.StructKind, true},
		"a string field":                      {[]string{"apiVersion"}, cue.StringKind, true},
		"a nested string":                     {[]string{"metadata", "name"}, cue.StringKind, true},
		"an integer":                          {[]string{"spec", "replicas"}, cue.IntKind, true},
		"a number":                            {[]string{"spec", "ratio"}, cue.NumberKind, true},
		"a boolean":                           {[]string{"spec", "paused"}, cue.BoolKind, true},
		"a list":                              {[]string{"spec", "ports"}, cue.ListKind, true},
		"a list element":                      {[]string{"spec", "ports", "0"}, cue.StructKind, true},
		"a field of a list element":           {[]string{"spec", "ports", "3", "port"}, cue.IntKind, true},
		"a list read by a field name":         {[]string{"spec", "ports", "port"}, cue.BottomKind, false},
		"a map":                               {[]string{"metadata", "labels"}, cue.StructKind, true},
		"a map's value":                       {[]string{"metadata", "labels", "app.kubernetes.io/name"}, cue.StringKind, true},
		"a field the template does not write": {[]string{"metadata", "uid"}, cue.BottomKind, false},
		"below a scalar":                      {[]string{"spec", "replicas", "x"}, cue.BottomKind, false},
		"status, of any type":                 {[]string{"status"}, cue.BottomKind, false},
		"inside status":                       {[]string{"status", "readyReplicas"}, cue.BottomKind, false},
		"a value kept as written":             {[]string{"spec", "raw", "x"}, cue.BottomKind, false},
		"the kept value itself":               {[]string{"spec", "raw"}, cue.BottomKind, false},
		"a value of any type":                 {[]string{"spec", "anything"}, cue.BottomKind, false},
	} {
		t.Run(name, func(t *testing.T) {
			kind, ok := KindAt(deployment, tc.path)
			require.Equal(t, tc.ok, ok)
			if ok {
				require.Equal(t, tc.kind, kind)
			}
		})
	}

	t.Run("no schema", func(t *testing.T) {
		_, ok := KindAt(nil, []string{"a"})
		require.False(t, ok)
	})
}

func TestLiteralSchema(t *testing.T) {
	kindOf := func(t *testing.T, literal string, path ...string) (cue.Kind, bool) {
		t.Helper()
		var v interface{}
		require.NoError(t, json.Unmarshal([]byte(literal), &v))
		return KindAt(LiteralSchema(v), path)
	}
	for name, tc := range map[string]struct {
		literal string
		path    []string
		kind    cue.Kind
		ok      bool
	}{
		"whole numbers are integers":            {`[1, 2]`, []string{"0"}, cue.IntKind, true},
		"a fraction makes the element a number": {`[1, 2.5]`, []string{"0"}, cue.NumberKind, true},
		"strings":                               {`["eu", "us"]`, []string{"1"}, cue.StringKind, true},
		"booleans":                              {`[true, false]`, []string{"0"}, cue.BoolKind, true},
		"elements of two kinds have none":       {`[1, "a"]`, []string{"0"}, cue.BottomKind, false},
		"the list itself":                       {`[1, "a"]`, nil, cue.ListKind, true},
		"an empty list has no element kind":     {`[]`, []string{"0"}, cue.BottomKind, false},
		"object fields":                         {`[{"region":"eu","wave":1},{"region":"us","wave":2}]`, []string{"0", "wave"}, cue.IntKind, true},
		"a field some elements leave out":       {`[{"region":"eu","wave":1},{"region":"us"}]`, []string{"0", "wave"}, cue.IntKind, true},
		"a field of two kinds has none":         {`[{"wave":1},{"wave":"2"}]`, []string{"0", "wave"}, cue.BottomKind, false},
		"other fields keep theirs":              {`[{"wave":1,"region":"eu"},{"wave":"2","region":"us"}]`, []string{"0", "region"}, cue.StringKind, true},
		"a field no element has":                {`[{"region":"eu"}]`, []string{"0", "zone"}, cue.BottomKind, false},
		"an object beside a scalar has none":    {`[{"region":"eu"}, "us"]`, []string{"0"}, cue.BottomKind, false},
		"nested lists":                          {`[[1, 2], [3]]`, []string{"0", "1"}, cue.IntKind, true},
		"an expression is of any type":          {`["$(source.cfg.region)", "us"]`, []string{"0"}, cue.BottomKind, false},
		"an expression in text is text":         {`["eu-$(source.cfg.zone)", "us"]`, []string{"0"}, cue.StringKind, true},
		"a field holding an expression":         {`[{"region":"$(source.cfg.region)"}]`, []string{"0", "region"}, cue.BottomKind, false},
		"null is of any type":                   {`[null]`, []string{"0"}, cue.BottomKind, false},
	} {
		t.Run(name, func(t *testing.T) {
			kind, ok := kindOf(t, tc.literal, tc.path...)
			require.Equal(t, tc.ok, ok)
			if ok {
				require.Equal(t, tc.kind, kind)
			}
		})
	}
}
