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
"context"
"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
"github.com/oam-dev/kubevela/pkg/appfile"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Workflow is used to execute the workflow steps of Application.
type Workflow interface {
	// ExecuteSteps executes the steps of an Application with given steps of rendered resources.
	// It returns done=true only if all steps are executed and succeeded.
	ExecuteSteps(ctx context.Context, appRevName string, steps []*unstructured.Unstructured) (done bool, err error)
}

// SucceededMessage is the data json-marshalled into the message of `workflow-progress` condition
// when its reason is `succeeded`.
type SucceededMessage struct {
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

type TaskRunner func(ctx wfContext.Context)(common.WorkflowStepStatus,*Operation,error)

type inputItem struct {
	ParameterKey string
	From string
}

type StepInput []inputItem

type outputItem struct {
	ExportKey string
	Name string
}

type StepOutput []outputItem

type Operation struct {
	Suspend bool
	Terminated bool
}

type StepWorkload struct {
	workload *appfile.Workload
	spec *v1aplha2.WorkloadStep
}


type Action interface {
	Suspend()
	Terminated()
	Wait()
	Message(msg string)
	Reason(reason string)
}




