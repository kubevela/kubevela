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

package workflow

import (
	"fmt"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam"
)

var (
	// DisableRecorder optimize workflow by disable recorder
	DisableRecorder = false
)

const (
	// MessageTerminatedFailedAfterRetries is the message of failed after retries
	MessageTerminatedFailedAfterRetries = "The workflow terminates automatically because the failed times of steps have reached the limit"
	// MessageSuspendFailedAfterRetries is the message of failed after retries
	MessageSuspendFailedAfterRetries = "The workflow suspends automatically because the failed times of steps have reached the limit"
)

// ComputeWorkflowRevisionHash compute workflow revision.
func ComputeWorkflowRevisionHash(rev string, app *oamcore.Application) (string, error) {
	version := ""
	if annos := app.Annotations; annos != nil {
		version = annos[oam.AnnotationPublishVersion]
	}
	if version == "" {
		specHash, err := utils.ComputeSpecHash(app.Spec)
		if err != nil {
			return "", err
		}
		version = fmt.Sprintf("%s:%s", rev, specHash)
	}
	return version, nil
}

// IsFailedAfterRetry check if application is hang due to FailedAfterRetry
func IsFailedAfterRetry(app *oamcore.Application) bool {
	return app.Status.Workflow != nil && (app.Status.Workflow.Message == MessageTerminatedFailedAfterRetries || app.Status.Workflow.Message == MessageSuspendFailedAfterRetries)
}

// ConvertWorkflowStatus convert workflow run status to workflow status
func ConvertWorkflowStatus(status workflowv1alpha1.WorkflowRunStatus, revision string) *common.WorkflowStatus {
	return &common.WorkflowStatus{
		AppRevision:    revision,
		Mode:           fmt.Sprintf("%s-%s", status.Mode.Steps, status.Mode.SubSteps),
		Phase:          status.Phase,
		Message:        status.Message,
		Suspend:        status.Suspend,
		SuspendState:   status.SuspendState,
		Terminated:     status.Terminated,
		Finished:       status.Finished,
		ContextBackend: status.ContextBackend,
		Steps:          status.Steps,
		StartTime:      status.StartTime,
		EndTime:        status.EndTime,
	}
}
