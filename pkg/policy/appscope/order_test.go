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

package appscope

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func names(refs []Ref) []string {
	out := make([]string, len(refs))
	for i, r := range refs {
		out[i] = r.Name
	}
	return out
}

func TestLess(t *testing.T) {
	cases := map[string]struct {
		in   []Ref
		want []string
	}{
		"lower priority first": {
			in:   []Ref{{Name: "b", Priority: 10, Global: true}, {Name: "a", Priority: 5, Global: true}},
			want: []string{"a", "b"},
		},
		"globals by name at a priority": {
			in:   []Ref{{Name: "z", Global: true}, {Name: "a", Global: true}, {Name: "m", Global: true}},
			want: []string{"a", "m", "z"},
		},
		"globals before explicit at a priority": {
			in:   []Ref{{Name: "explicit", SpecOrder: 0}, {Name: "global", Global: true}},
			want: []string{"global", "explicit"},
		},
		"explicit in spec order, not by name": {
			in:   []Ref{{Name: "b", SpecOrder: 1}, {Name: "a", SpecOrder: 2}, {Name: "c", SpecOrder: 0}},
			want: []string{"c", "b", "a"},
		},
		"a global with a negative priority runs before explicit ones": {
			in:   []Ref{{Name: "explicit"}, {Name: "early", Priority: -1, Global: true}},
			want: []string{"early", "explicit"},
		},
		"a global with a positive priority runs after explicit ones": {
			in:   []Ref{{Name: "late", Priority: 1, Global: true}, {Name: "explicit"}},
			want: []string{"explicit", "late"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			sort.SliceStable(tc.in, func(i, j int) bool { return Less(tc.in[i], tc.in[j]) })
			require.Equal(t, tc.want, names(tc.in))
		})
	}
}

func TestAsksToSkipGlobals(t *testing.T) {
	app := func(v string) *v1beta1.Application {
		a := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}}}
		if v != "" {
			a.Annotations[oam.AnnotationSkipGlobalPolicies] = v
		}
		return a
	}
	require.True(t, AsksToSkipGlobals(app("true")))
	require.False(t, AsksToSkipGlobals(app("false")))
	require.False(t, AsksToSkipGlobals(app("")))
	require.False(t, AsksToSkipGlobals(app("yes")))
}
