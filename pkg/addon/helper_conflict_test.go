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

package addon

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractDefinitionNameFromFile(t *testing.T) {
	assert.Equal(t, "component", extractDefinitionNameFromFile(ElementFile{Name: "definitions/component.cue"}))
	assert.Equal(t, "trait", extractDefinitionNameFromFile(ElementFile{Name: "trait.yaml"}))
	assert.Equal(t, "my-policy", extractDefinitionNameFromFile(ElementFile{Name: "/tmp/my-policy.json"}))
}

func TestRemoveConflictingDefinitions(t *testing.T) {
	defs := []ElementFile{
		{Name: "definitions/comp.cue", Data: "comp"},
		{Name: "definitions/trait.cue", Data: "trait"},
		{Name: "definitions/policy.cue", Data: "policy"},
	}

	t.Run("removes all conflicting names", func(t *testing.T) {
		filtered := removeConflictingDefinitions(defs, []string{"trait", "policy"})
		assert.Equal(t, []ElementFile{{Name: "definitions/comp.cue", Data: "comp"}}, filtered)
	})

	t.Run("no conflicts keeps all definitions", func(t *testing.T) {
		filtered := removeConflictingDefinitions(defs, []string{"non-existent"})
		assert.Equal(t, defs, filtered)
	})

	t.Run("empty conflict list keeps all definitions", func(t *testing.T) {
		filtered := removeConflictingDefinitions(defs, nil)
		assert.Equal(t, defs, filtered)
	})
}
