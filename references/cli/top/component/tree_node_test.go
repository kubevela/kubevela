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

package component

import (
	"testing"

	"github.com/kubevela/workflow/api/v1alpha1"
	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
)

func TestEmojiFormat(t *testing.T) {
	assert.Contains(t, EmojiFormat("app", "app"), "red")
	assert.Contains(t, EmojiFormat("app", "app"), "🎯")

	assert.Contains(t, EmojiFormat("workflow", "workflow"), "yellow")
	assert.Contains(t, EmojiFormat("workflow", "workflow"), "👟")

	assert.Contains(t, EmojiFormat("component", "component"), "green")
	assert.Contains(t, EmojiFormat("component", "component"), "🧩")

	assert.Contains(t, EmojiFormat("policy", "policy"), "orange")
	assert.Contains(t, EmojiFormat("policy", "policy"), "📜")

	assert.Contains(t, EmojiFormat("trait", "trait"), "lightseagreen")
	assert.Contains(t, EmojiFormat("trait", "trait"), "🔧")

	assert.Contains(t, EmojiFormat("service", "service"), "blue")
	assert.Contains(t, EmojiFormat("service", "service"), "🚓")
}

func TestColorizeKind(t *testing.T) {
	assert.Contains(t, ColorizeKind("Pod"), "orange")
}

func TestColorizeStatus(t *testing.T) {
	assert.Contains(t, ColorizeStatus(types.HealthStatusHealthy), "green")
	assert.Contains(t, ColorizeStatus(types.HealthStatusUnHealthy), "red")
	assert.Contains(t, ColorizeStatus(types.HealthStatusProgressing), "orange")
	assert.Contains(t, ColorizeStatus(types.HealthStatusUnKnown), "gray")
}

func TestWorkflowStepFormat(t *testing.T) {
	assert.Contains(t, WorkflowStepFormat("step1", v1alpha1.WorkflowStepPhaseSucceeded), "✅")
	assert.NotContains(t, WorkflowStepFormat("step2", v1alpha1.WorkflowStepPhaseFailed), "✅")
}
