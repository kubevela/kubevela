/*
Copyright 2023 The KubeVela Authors.

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

package cuegen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegisterAny(t *testing.T) {
	tests := []struct {
		name string
		typ  []string
		want map[string]struct{}
	}{
		{
			name: "builtin",
			typ:  []string{"int", "string", "bool"},
			want: map[string]struct{}{"int": {}, "string": {}, "bool": {}},
		},
		{
			name: "map",
			typ:  []string{"map[string]interface{}"},
			want: map[string]struct{}{"map[string]interface{}": {}},
		},
		{
			name: "external",
			typ:  []string{"http.Header"},
			want: map[string]struct{}{"http.Header": {}},
		},
		{
			name: "external2",
			typ:  []string{"*k8s.io/apimachinery/pkg/apis/meta/v1/unstructured.Unstructured", "http.Header"},
			want: map[string]struct{}{
				"*k8s.io/apimachinery/pkg/apis/meta/v1/unstructured.Unstructured": {},
				"http.Header": {},
			},
		},
	}

	for _, tt := range tests {
		g := Generator{anyTypes: map[string]struct{}{}}
		g.RegisterAny(tt.typ...)
		assert.Equal(t, tt.want, g.anyTypes, tt.name)
	}
}
