/*Copyright 2021 The KubeVela Authors.

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

package workflow

import (
	"time"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	monitorContext "github.com/oam-dev/kubevela/pkg/monitor/context"
	"github.com/oam-dev/kubevela/pkg/workflow/types"
)

// Workflow is used to execute the workflow steps of Application.
type Workflow interface {
	// ExecuteSteps executes the steps of an Application with given steps of rendered resources.
	// It returns done=true only if all steps are executed and succeeded.
	ExecuteSteps(ctx monitorContext.Context, appRev *v1beta1.ApplicationRevision, taskRunners []types.TaskRunner) (state common.WorkflowState, err error)

	// Trace record workflow state in controllerRevision.
	Trace() error

	// GetBackoffWaitTime returns the wait time for next retry.
	GetBackoffWaitTime() time.Duration

	HandleSuspendWait(ctx monitorContext.Context) (bool, time.Duration, error)

	GetSuspendBackoffWaitTime() time.Duration
}
