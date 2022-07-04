/*
Copyright 2022 The KubeVela Authors.

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

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetFilenameFromLocalOrRemote(t *testing.T) {
	cases := []struct {
		path     string
		filename string
	}{
		{
			path:     "./name",
			filename: "name",
		},
		{
			path:     "name",
			filename: "name",
		},
		{
			path:     "../name.js",
			filename: "name",
		},
		{
			path:     "../name.tar.zst",
			filename: "name.tar",
		},
		{
			path:     "https://some.com/file.js",
			filename: "file",
		},
		{
			path:     "https://some.com/file",
			filename: "file",
		},
	}

	for _, c := range cases {
		n, _ := GetFilenameFromLocalOrRemote(c.path)
		assert.Equal(t, c.filename, n)
	}
}
