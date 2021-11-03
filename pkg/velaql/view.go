/*
 Copyright 2021. The KubeVela Authors.

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

package velaql

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"github.com/oam-dev/kubevela/pkg/workflow/tasks"
	wfTypes "github.com/oam-dev/kubevela/pkg/workflow/types"
)

const (
	qlNs = "vela-system"

	// ViewTaskPhaseSucceeded means view task run succeeded.
	ViewTaskPhaseSucceeded = "succeeded"
)

// ViewHandler view handler
type ViewHandler struct {
	cli       client.Client
	viewTask  v1beta1.WorkflowStep
	dm        discoverymapper.DiscoveryMapper
	pd        *packages.PackageDiscover
	namespace string
}

// NewViewHandler new view handler
func NewViewHandler(cli client.Client, dm discoverymapper.DiscoveryMapper, pd *packages.PackageDiscover) *ViewHandler {
	return &ViewHandler{
		cli:       cli,
		dm:        dm,
		pd:        pd,
		namespace: qlNs,
	}
}

// QueryView generate view step
func (handler *ViewHandler) QueryView(ctx context.Context, qv QueryView) (*value.Value, error) {
	outputsTemplate := fmt.Sprintf(OutputsTemplate, qv.Export, qv.Export)
	queryKey := QueryParameterKey{}
	if err := json.Unmarshal([]byte(outputsTemplate), &queryKey); err != nil {
		return nil, err
	}

	handler.viewTask = v1beta1.WorkflowStep{
		Name:       fmt.Sprintf("%s-%s", qv.View, qv.Export),
		Type:       qv.View,
		Properties: oamutil.Object2RawExtension(qv.Parameter),
		Outputs:    queryKey.Outputs,
	}

	taskDiscover := tasks.NewViewTaskDiscover(handler.pd, handler.cli, handler.dispatch, handler.delete, handler.namespace)
	genTask, err := taskDiscover.GetTaskGenerator(ctx, handler.viewTask.Type)
	if err != nil {
		return nil, err
	}

	runner, err := genTask(handler.viewTask, &wfTypes.GeneratorOptions{ID: utils.RandomString(10)})
	if err != nil {
		return nil, err
	}

	viewCtx, err := NewViewContext()
	if err != nil {
		return nil, err
	}
	status, _, err := runner.Run(viewCtx, &wfTypes.TaskRunOptions{})
	if err != nil {
		return nil, err
	}
	if string(status.Phase) != ViewTaskPhaseSucceeded {
		return nil, errors.Errorf("failed to query the view %s", status.Message)
	}
	return viewCtx.GetVar(qv.Export)
}

func (handler *ViewHandler) dispatch(ctx context.Context, cluster string, owner common.ResourceCreatorRole, manifests ...*unstructured.Unstructured) error {
	ctx = multicluster.ContextWithClusterName(ctx, cluster)
	applicator := apply.NewAPIApplicator(handler.cli)
	for _, manifest := range manifests {
		if err := applicator.Apply(ctx, manifest); err != nil {
			return err
		}
	}
	return nil
}

func (handler *ViewHandler) delete(ctx context.Context, cluster string, owner common.ResourceCreatorRole, manifest *unstructured.Unstructured) error {
	return handler.cli.Delete(ctx, manifest)
}

// QueryParameterKey query parameter key
type QueryParameterKey struct {
	Outputs common.StepOutputs `json:"outputs"`
}

// OutputsTemplate output template
var OutputsTemplate = `
{
    "outputs": [
        {
            "valueFrom": "%s",
            "name": "%s"
        }
    ]
}
`
