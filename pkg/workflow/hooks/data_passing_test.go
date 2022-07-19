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
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
)

func TestInput(t *testing.T) {
	wfCtx := mockContext(t)
	r := require.New(t)
	paramValue, err := wfCtx.MakeParameter(map[string]interface{}{
		"name": "foo",
	})
	r.NoError(err)
	score, err := paramValue.MakeValue(`score: 99`)
	r.NoError(err)
	err = wfCtx.SetVar(score, "foo")
	r.NoError(err)
	err = Input(wfCtx, paramValue, v1beta1.WorkflowStep{
		DependsOn: []string{"mystep"},
		Inputs: common.StepInputs{{
			From:         "foo.score",
			ParameterKey: "myscore",
		}},
	})
	r.NoError(err)
	result, err := paramValue.LookupValue("myscore")
	r.NoError(err)
	s, err := result.String()
	r.NoError(err)
	r.Equal(s, `99
`)
}

func TestOutput(t *testing.T) {
	wfCtx := mockContext(t)
	r := require.New(t)
	taskValue, err := value.NewValue(`
output: score: 99 
`, nil, "")
	r.NoError(err)
	stepStatus := make(map[string]common.StepStatus)
	err = Output(wfCtx, taskValue, v1beta1.WorkflowStep{
		Properties: &runtime.RawExtension{
			Raw: []byte("{\"name\":\"mystep\"}"),
		},
		Outputs: common.StepOutputs{{
			ValueFrom: "output.score",
			Name:      "myscore",
		}},
	}, common.StepStatus{
		Phase: common.WorkflowStepPhaseSucceeded,
	}, stepStatus)
	r.NoError(err)
	result, err := wfCtx.GetVar("myscore")
	r.NoError(err)
	s, err := result.String()
	r.NoError(err)
	r.Equal(s, `99
`)
	r.Equal(stepStatus["mystep"].Phase, common.WorkflowStepPhaseSucceeded)
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
	wfCtx, err := wfContext.NewContext(cli, "default", "v1", "testuid")
	require.NoError(t, err)
	return wfCtx
}
