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

package hooks

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	wfTypes "github.com/oam-dev/kubevela/pkg/workflow/types"
)

// Input set data to parameter.
func Input(ctx wfContext.Context, paramValue *value.Value, step v1beta1.WorkflowStep) error {
	for _, input := range step.Inputs {
		inputValue, err := ctx.GetVar(strings.Split(input.From, ".")...)
		if err != nil {
			return errors.WithMessagef(err, "get input from [%s]", input.From)
		}
		if input.ParameterKey != "" {
			if err := paramValue.FillValueByScript(inputValue, input.ParameterKey); err != nil {
				return err
			}
		}
	}
	return nil
}

// Output get data from task value.
func Output(ctx wfContext.Context, taskValue *value.Value, step v1beta1.WorkflowStep, status common.StepStatus, stepStatus map[string]common.StepStatus) error {
	errMsg := ""
	if wfTypes.IsStepFinish(status.Phase, status.Reason) {
		SetAdditionalNameInStatus(stepStatus, step.Name, step.Properties, status)
		for _, output := range step.Outputs {
			v, err := taskValue.LookupByScript(output.ValueFrom)
			// if the error is not nil and the step is not skipped, return the error
			if err != nil && status.Phase != common.WorkflowStepPhaseSkipped {
				errMsg += fmt.Sprintf("failed to get output from %s: %s\n", output.ValueFrom, err.Error())
			}
			// if the error is not nil, set the value to null
			if err != nil || v.Error() != nil {
				v, _ = taskValue.MakeValue("null")
			}
			if err := ctx.SetVar(v, output.Name); err != nil {
				errMsg += fmt.Sprintf("failed to set output %s: %s\n", output.Name, err.Error())
			}
		}
	}

	if errMsg != "" {
		return errors.New(errMsg)
	}
	return nil
}

// SetAdditionalNameInStatus sets additional name from properties to status map
func SetAdditionalNameInStatus(stepStatus map[string]common.StepStatus, name string, properties *runtime.RawExtension, status common.StepStatus) {
	if stepStatus == nil || properties == nil {
		return
	}
	o := struct {
		Name      string `json:"name"`
		Component string `json:"component"`
	}{}
	js, err := properties.MarshalJSON()
	if err != nil {
		return
	}
	if err := json.Unmarshal(js, &o); err != nil {
		return
	}
	additionalName := ""
	switch {
	case o.Name != "":
		additionalName = o.Name
	case o.Component != "":
		additionalName = o.Component
	default:
		return
	}
	if _, ok := stepStatus[additionalName]; !ok {
		stepStatus[additionalName] = status
		return
	}
}
