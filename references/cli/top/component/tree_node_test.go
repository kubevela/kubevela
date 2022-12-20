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

func TestTopologyTreeNodeFormatter(t *testing.T) {
	topology := NewTopologyTreeNodeFormatter(&themeConfig)
	t.Run("test emoji format", func(t *testing.T) {
		assert.Contains(t, topology.EmojiFormat("app", "app"), themeConfig.Topology.App.String())
		assert.Contains(t, topology.EmojiFormat("app", "app"), "🎯")

		assert.Contains(t, topology.EmojiFormat("workflow", "workflow"), themeConfig.Topology.Workflow.String())
		assert.Contains(t, topology.EmojiFormat("workflow", "workflow"), "👟")

		assert.Contains(t, topology.EmojiFormat("component", "component"), themeConfig.Topology.Component.String())
		assert.Contains(t, topology.EmojiFormat("component", "component"), "🧩")

		assert.Contains(t, topology.EmojiFormat("policy", "policy"), themeConfig.Topology.Policy.String())
		assert.Contains(t, topology.EmojiFormat("policy", "policy"), "📜")

		assert.Contains(t, topology.EmojiFormat("trait", "trait"), themeConfig.Topology.Trait.String())
		assert.Contains(t, topology.EmojiFormat("trait", "trait"), "🔧")

		assert.Contains(t, topology.EmojiFormat("service", "service"), themeConfig.Topology.Kind.String())
		assert.Contains(t, topology.EmojiFormat("service", "service"), "🚓")
	})

	t.Run("colorize kind", func(t *testing.T) {
		assert.Contains(t, topology.ColorizeKind("Pod"), themeConfig.Topology.Kind.String())
	})

	t.Run("colorize status", func(t *testing.T) {
		assert.Contains(t, topology.ColorizeStatus(types.HealthStatusHealthy), themeConfig.Status.Healthy.String())
		assert.Contains(t, topology.ColorizeStatus(types.HealthStatusUnHealthy), themeConfig.Status.UnHealthy.String())
		assert.Contains(t, topology.ColorizeStatus(types.HealthStatusProgressing), themeConfig.Status.Waiting.String())
		assert.Contains(t, topology.ColorizeStatus(types.HealthStatusUnKnown), themeConfig.Status.Unknown.String())
	})
}

func TestWorkflowStepFormat(t *testing.T) {
	assert.Contains(t, WorkflowStepFormat("step1", v1alpha1.WorkflowStepPhaseSucceeded), "✅")
	assert.NotContains(t, WorkflowStepFormat("step2", v1alpha1.WorkflowStepPhaseFailed), "✅")
}
