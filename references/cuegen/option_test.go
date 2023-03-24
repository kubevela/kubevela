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

func TestWithAnyTypes(t *testing.T) {
	tests := []struct {
		name  string
		opts  []Option
		extra map[string]struct{}
	}{
		{
			name:  "default",
			opts:  nil,
			extra: map[string]struct{}{},
		},
		{
			name:  "single",
			opts:  []Option{WithAnyTypes("foo", "bar")},
			extra: map[string]struct{}{"foo": {}, "bar": {}},
		},
		{
			name: "multiple",
			opts: []Option{WithAnyTypes("foo", "bar"),
				WithAnyTypes("baz", "qux")},
			extra: map[string]struct{}{"foo": {}, "bar": {}, "baz": {}, "qux": {}},
		},
	}

	for _, tt := range tests {
		opts := options{map[string]struct{}{}}
		for _, opt := range tt.opts {
			opt(&opts)
		}
		assert.Equal(t, opts.anyTypes, tt.extra, tt.name)
	}
}
