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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestImmutableFieldsFromTemplate(t *testing.T) {
	cases := []struct {
		name     string
		template string
		want     map[string]bool
	}{
		{
			name:     "empty template",
			template: "",
			want:     nil,
		},
		{
			name: "no parameter block",
			template: `
output: {}`,
			want: nil,
		},
		{
			name: "parameter block with no immutable fields",
			template: `
parameter: {
	// +usage=The image to use
	image: string
	replicas: int
}`,
			want: nil,
		},
		{
			name: "parameter with one immutable field",
			template: `
parameter: {
	// +immutable
	image: string
	replicas: int
}`,
			want: map[string]bool{"image": true},
		},
		{
			name: "multiple immutable fields",
			template: `
parameter: {
	// +immutable
	image: string
	// +immutable
	storageClass: string
	replicas: int
}`,
			want: map[string]bool{"image": true, "storageClass": true},
		},
		{
			name: "immutable with usage and short tags on same field",
			template: `
parameter: {
	// +usage=The image to use
	// +short=i
	// +immutable
	image: string
}`,
			want: map[string]bool{"image": true},
		},
		{
			name: "nested immutable field returns dotted path",
			template: `
parameter: {
	governance: {
		// +immutable
		tenant: string
		region: string
	}
	image: string
}`,
			want: map[string]bool{"governance.tenant": true},
		},
		{
			name: "deeply nested immutable field",
			template: `
parameter: {
	config: {
		db: {
			// +immutable
			name: string
		}
	}
}`,
			want: map[string]bool{"config.db.name": true},
		},
		{
			name: "mixed top-level and nested immutable fields",
			template: `
parameter: {
	// +immutable
	image: string
	governance: {
		// +immutable
		tenant: string
		region: string
	}
}`,
			want: map[string]bool{"image": true, "governance.tenant": true},
		},
		{
			name:     "invalid CUE returns nil",
			template: `parameter: { image: }`,
			want:     nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ImmutableFieldsFromTemplate(tc.template)
			assert.Equal(t, tc.want, got)
		})
	}
}
