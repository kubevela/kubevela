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

package policy

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils"
)

// ParsePolicyToSteps parses policy to steps
func ParsePolicyToSteps(policies []v1beta1.AppPolicy) ([]v1beta1.WorkflowStep, error) {
	steps := make([]v1beta1.WorkflowStep, 0)
	for _, policy := range policies {
		switch policy.Type {
		case v1alpha1.PostStopHookPolicyType:
			postStopHookSpec := &v1alpha1.PostStopHookPolicySpec{}
			if err := utils.StrictUnmarshal(policy.Properties.Raw, postStopHookSpec); err != nil {
				return nil, errors.Wrapf(err, "failed to parse post stop hook policy %s", policy.Name)
			}
			if postStopHookSpec.Webhook != nil {
				steps = append(steps, v1beta1.WorkflowStep{
					Name:       fmt.Sprintf("%s-webhook", policy.Name),
					Type:       "webhook",
					Properties: postStopHookSpec.Webhook,
				})
			}
			if postStopHookSpec.Notification != nil {
				steps = append(steps, v1beta1.WorkflowStep{
					Name:       fmt.Sprintf("%s-notification", policy.Name),
					Type:       "notification",
					Properties: postStopHookSpec.Notification,
				})
			}
		default:
		}
	}
	return steps, nil
}

// ParsePolicyStepStatus parses policy status from steps
func ParsePolicyStepStatus(steps []common.WorkflowStepStatus, app *v1beta1.Application) error {
	for _, step := range steps {
		for _, policy := range app.Spec.Policies {
			switch policy.Type {
			case v1alpha1.PostStopHookPolicyType:
				if strings.Contains(step.ID, policy.Name) {
					policyStatus := common.PolicyStatus{
						Name: policy.Name,
						Type: policy.Type,
					}
					hookStatus := &v1alpha1.PostStopHookPolicyStatus{}
					index := -1
					for i, status := range app.Status.PolicyStatus {
						if status.Name == policy.Name {
							index = i
							if err := utils.StrictUnmarshal(status.Status.Raw, hookStatus); err != nil {
								return errors.Wrapf(err, "failed to parse post stop hook policy status %s", policy.Name)
							}
							policyStatus = app.Status.PolicyStatus[i]
							break
						}
					}
					stepStatus := &common.WorkflowStepStatus{
						Name:  step.Name,
						ID:    step.ID,
						Type:  step.Type,
						Phase: step.Phase,
					}
					switch step.Type {
					case "webhook":
						hookStatus.Webhook = stepStatus
					case "notification":
						hookStatus.Notification = stepStatus
					default:
					}
					policyStatus.Status = util.Object2RawExtension(hookStatus)
					if index > -1 {
						app.Status.PolicyStatus[index] = policyStatus
					} else {
						app.Status.PolicyStatus = append(app.Status.PolicyStatus, policyStatus)
					}
				}
			default:
				continue
			}
		}
	}
	return nil
}

// HandleOptions is the options for handle
type HandleOptions struct {
	GCFinished bool
}

// Handle checks whether the policy is ready to be handled
func Handle(app *v1beta1.Application, options HandleOptions) (bool, error) {
	handle := false
	for _, policy := range app.Spec.Policies {
		switch policy.Type {
		case v1alpha1.PostStopHookPolicyType:
			handle = true
			postStopHookSpec := &v1alpha1.PostStopHookPolicySpec{}
			if err := utils.StrictUnmarshal(policy.Properties.Raw, postStopHookSpec); err != nil {
				return false, errors.Wrapf(err, "failed to parse post stop hook policy %s", policy.Name)
			}

			if len(postStopHookSpec.Phases) == 0 {
				return false, fmt.Errorf("post stop hook policy %s has no phases", policy.Name)
			}
			for _, phase := range postStopHookSpec.Phases {
				switch phase {
				case v1alpha1.OnErrorPhase:
					if !app.Status.Error {
						return false, nil
					}
				case v1alpha1.OnGarbageCollectFinishPhase:
					if !options.GCFinished {
						return false, nil
					}
				default:
					return false, nil
				}
			}
		default:
			continue
		}
	}
	return handle, nil
}
