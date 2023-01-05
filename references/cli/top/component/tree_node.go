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
	"fmt"

	"github.com/kubevela/workflow/api/v1alpha1"

	"github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
	"github.com/oam-dev/kubevela/references/cli/top/config"
)

const (
	component           = "🧩"
	workflow            = "👟"
	policy              = "📜"
	trait               = "🔧"
	app                 = "🎯"
	other               = "🚓"
	workflowStepSucceed = "✅"
)

// TopologyTreeNodeFormatter is the formatter for the topology tree node
type TopologyTreeNodeFormatter struct {
	style *config.ThemeConfig
}

// NewTopologyTreeNodeFormatter create a new topology tree node formatter
func NewTopologyTreeNodeFormatter(style *config.ThemeConfig) *TopologyTreeNodeFormatter {
	return &TopologyTreeNodeFormatter{
		style: style,
	}
}

const colorFmt = "%s [%s::b]%s[::]"

// EmojiFormat format the name with the emoji
func (t TopologyTreeNodeFormatter) EmojiFormat(name string, kind string) string {
	switch kind {
	case "app":
		return fmt.Sprintf(colorFmt, app, t.style.Topology.App.String(), name)
	case "workflow":
		return fmt.Sprintf(colorFmt, workflow, t.style.Topology.Workflow.String(), name)
	case "component":
		return fmt.Sprintf(colorFmt, component, t.style.Topology.Component.String(), name)
	case "policy":
		return fmt.Sprintf(colorFmt, policy, t.style.Topology.Policy.String(), name)
	case "trait":
		return fmt.Sprintf(colorFmt, trait, t.style.Topology.Trait.String(), name)
	default:
		return fmt.Sprintf(colorFmt, other, t.style.Topology.Kind.String(), name)
	}
}

const workflowStepFmt = "[::b]%s %s[::]"

// WorkflowStepFormat format the workflow step text with the emoji
func WorkflowStepFormat(name string, status v1alpha1.WorkflowStepPhase) string {
	switch status {
	case v1alpha1.WorkflowStepPhaseSucceeded:
		return fmt.Sprintf(workflowStepFmt, name, workflowStepSucceed)
	default:
		return name
	}
}

const statusColorFmt = "[%s::b]%s[::]"

// ColorizeStatus colorize the status text
func (t TopologyTreeNodeFormatter) ColorizeStatus(status types.HealthStatusCode) string {
	switch status {
	case types.HealthStatusHealthy:
		return fmt.Sprintf(statusColorFmt, t.style.Status.Healthy.String(), status)
	case types.HealthStatusUnHealthy:
		return fmt.Sprintf(statusColorFmt, t.style.Status.UnHealthy.String(), status)
	case types.HealthStatusProgressing:
		return fmt.Sprintf(statusColorFmt, t.style.Status.Waiting.String(), status)
	default:
		return fmt.Sprintf(statusColorFmt, t.style.Status.Unknown.String(), status)
	}
}

const kindColorFmt = "[%s::b]%s[::]"

// ColorizeKind colorize the kind text
func (t TopologyTreeNodeFormatter) ColorizeKind(kind string) string {
	return fmt.Sprintf(kindColorFmt, t.style.Topology.Kind, kind)
}
