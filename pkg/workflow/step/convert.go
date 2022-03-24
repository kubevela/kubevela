/*
Copyright 2021 The KubeVela Authors.

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

package step

import (
	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// ConvertSteps convert common steps in to v1beta1 steps for compatibility
func ConvertSteps(steps []common.WorkflowStep) []v1beta1.WorkflowStep {
	var _steps []v1beta1.WorkflowStep
	for _, step := range steps {
		_steps = append(_steps, v1beta1.WorkflowStep(step))
	}
	return _steps
}
