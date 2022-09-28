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

const colorFmt = "%s [%s::b]%s[::]"

// EmojiFormat format the name with the emoji
func EmojiFormat(name string, kind string) string {
	switch kind {
	case "app":
		return fmt.Sprintf(colorFmt, app, "red", name)
	case "workflow":
		return fmt.Sprintf(colorFmt, workflow, "yellow", name)
	case "component":
		return fmt.Sprintf(colorFmt, component, "green", name)
	case "policy":
		return fmt.Sprintf(colorFmt, policy, "orange", name)
	case "trait":
		return fmt.Sprintf(colorFmt, trait, "lightseagreen", name)
	default:
		return fmt.Sprintf(colorFmt, other, "blue", name)
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
func ColorizeStatus(status types.HealthStatusCode) string {
	switch status {
	case types.HealthStatusHealthy:
		return fmt.Sprintf(statusColorFmt, "green", status)
	case types.HealthStatusUnHealthy:
		return fmt.Sprintf(statusColorFmt, "red", status)
	case types.HealthStatusProgressing:
		return fmt.Sprintf(statusColorFmt, "orange", status)
	default:
		return fmt.Sprintf(statusColorFmt, "gray", status)
	}
}

const kindColorFmt = "[%s::b]%s[::]"

// ColorizeKind colorize the kind text
func ColorizeKind(kind string) string {
	return fmt.Sprintf(kindColorFmt, "orange", kind)
}
