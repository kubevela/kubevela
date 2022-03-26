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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
)

const (
	// ReadyComponent is the key for depends on in workflow context
	ReadyComponent = "readyComponent__"
)

// Input set data to parameter.
func Input(ctx wfContext.Context, paramValue *value.Value, step v1beta1.WorkflowStep) error {
	for _, depend := range step.DependsOn {
		if _, err := ctx.GetVar(ReadyComponent, depend); err != nil {
			return errors.WithMessagef(err, "the depends on component [%s] is not ready", depend)
		}
	}
	for _, input := range step.Inputs {
		inputValue, err := ctx.GetVar(strings.Split(input.From, ".")...)
		if err != nil {
			return errors.WithMessagef(err, "get input from [%s]", input.From)
		}
		if err := paramValue.FillValueByScript(inputValue, input.ParameterKey); err != nil {
			return err
		}
	}
	return nil
}

// Output get data from task value.
func Output(ctx wfContext.Context, taskValue *value.Value, step v1beta1.WorkflowStep, phase common.WorkflowStepPhase) error {
	if phase == common.WorkflowStepPhaseSucceeded {
		if step.Properties != nil {
			o := struct {
				Name string `json:"name"`
			}{}
			js, err := common.RawExtensionPointer{RawExtension: step.Properties}.MarshalJSON()
			if err != nil {
				return err
			}
			if err := json.Unmarshal(js, &o); err != nil {
				return err
			}
			ready, err := value.NewValue(`true`, nil, "")
			if err != nil {
				return err
			}
			if err := ctx.SetVar(ready, ReadyComponent, o.Name); err != nil {
				return err
			}
		}

		for _, output := range step.Outputs {
			v, err := taskValue.LookupByScript(output.ValueFrom)
			if err != nil {
				return err
			}
			if err := ctx.SetVar(v, output.Name); err != nil {
				if errv, _ := taskValue.LookupValue("output", "err"); errv != nil {
					return fmt.Errorf("%w: %v", err, errv.CueValue())
				}
				return err
			}
		}
	}

	return nil
}
