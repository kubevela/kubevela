/*
 Copyright 2026. The KubeVela Authors.

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

package upgrade

import (
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/cue/nesting"
)

// A stored template nested too deeply, written while admission was not checking, is never
// parsed: the controller renders a template that fails with the reason instead of crashing.
func TestATemplateNestedTooDeeplyIsNeverParsed(t *testing.T) {
	deep := "output: " + strings.Repeat("[", nesting.MaxDepth+1)

	got, upgraded := EnsureCueVersionCompatibility(deep, "poisoned", ComponentKind, TemplateAreaMain)
	require.False(t, upgraded)
	v := cuecontext.New().CompileString(got)
	require.ErrorContains(t, v.Validate(), "poisoned: CUE is nested deeper than 1000 levels")

	_, err := Upgrade(deep)
	require.ErrorContains(t, err, "nested deeper than 1000 levels")
	_, _, err = RequiresUpgrade(deep)
	require.ErrorContains(t, err, "nested deeper than 1000 levels")

	fine := "output: {a: [1]}"
	got, _ = EnsureCueVersionCompatibility(fine, "fine", ComponentKind, TemplateAreaMain)
	require.Contains(t, got, "output")
}
