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
	"context"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"gotest.tools/assert"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
)

func TestInput(t *testing.T) {
	wfCtx := mockContext(t)

	paramValue, err := wfCtx.MakeParameter(map[string]interface{}{
		"name": "foo",
	})
	assert.NilError(t, err)
	score, err := paramValue.MakeValue(`score: 99`)
	assert.NilError(t, err)
	wfCtx.SetVar(score, "foo")
	Input(wfCtx, paramValue, v1beta1.WorkflowStep{
		Inputs: common.StepInputs{{
			From:         "foo.score",
			ParameterKey: "myscore",
		}},
	})
	result, err := paramValue.LookupValue("myscore")
	assert.NilError(t, err)
	s, _ := result.String()
	assert.Equal(t, s, `99
`)
}

func TestOutput(t *testing.T) {
	wfCtx := mockContext(t)
	taskValue, err := value.NewValue(`
output: score: 99 
`, nil)
	assert.NilError(t, err)
	Output(wfCtx, taskValue, v1beta1.WorkflowStep{
		Outputs: common.StepOutputs{{
			ExportKey: "output.score",
			Name:      "myscore",
		}},
	}, common.WorkflowStepPhaseSucceeded)
	result, err := wfCtx.GetVar("myscore")
	assert.NilError(t, err)
	s, _ := result.String()
	assert.Equal(t, s, `99
`)
}

func mockContext(t *testing.T) wfContext.Context {
	cli := &test.MockClient{
		MockCreate: func(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
			return nil
		},
		MockUpdate: func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
			return nil
		},
		MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
			return nil
		},
	}
	wfCtx, err := wfContext.NewEmptyContext(cli, "default", "v1")
	assert.NilError(t, err)
	return wfCtx
}
