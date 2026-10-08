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

func TestGenerateIgnoredParameter(t *testing.T) {
	ps := generate(t, `parameter: {
	// +ignore
	internal: string
	// +ignore
	// +usage=Shown in the form
	described: string
}`)
	assert.Equal(t, "", uiParam(t, ps.UI, "internal").Description, "+ignore is CLI metadata, not prose")
	assert.Equal(t, "Shown in the form", uiParam(t, ps.UI, "described").Description)
	assert.Equal(t, "keep +ignore here", parseDoc("keep +ignore here").description)
}
