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
	"os"
	"strings"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	pkgtypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	monitorContext "github.com/kubevela/pkg/monitor/context"
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/kubevela/workflow/pkg/cue/packages"
	"github.com/kubevela/workflow/pkg/executor"
	"github.com/kubevela/workflow/pkg/generator"
	"github.com/kubevela/workflow/pkg/providers"
	"github.com/kubevela/workflow/pkg/providers/kube"
	wfTypes "github.com/kubevela/workflow/pkg/types"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/stdlib"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"github.com/oam-dev/kubevela/pkg/velaql/providers/query"
	"github.com/oam-dev/kubevela/pkg/workflow/template"
)

func init() {
	if err := stdlib.SetupBuiltinImports(); err != nil {
		klog.ErrorS(err, "Unable to set up builtin imports on package initialization")
		os.Exit(1)
	}
}

const (
	qlNs = "vela-system"

	// ViewTaskPhaseSucceeded means view task run succeeded.
	ViewTaskPhaseSucceeded = "succeeded"
)

// ViewHandler view handler
type ViewHandler struct {
	cli       client.Client
	cfg       *rest.Config
	viewTask  workflowv1alpha1.WorkflowStep
	dm        discoverymapper.DiscoveryMapper
	pd        *packages.PackageDiscover
	namespace string
}

// NewViewHandler new view handler
func NewViewHandler(cli client.Client, cfg *rest.Config, dm discoverymapper.DiscoveryMapper, pd *packages.PackageDiscover) *ViewHandler {
	return &ViewHandler{
		cli:       cli,
		cfg:       cfg,
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
		return nil, errors.Errorf("unmarhsal query template: %v", err)
	}

	handler.viewTask = workflowv1alpha1.WorkflowStep{
		WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
			Name:       fmt.Sprintf("%s-%s", qv.View, qv.Export),
			Type:       qv.View,
			Properties: oamutil.Object2RawExtension(qv.Parameter),
			Outputs:    queryKey.Outputs,
		},
	}

	instance := &wfTypes.WorkflowInstance{
		WorkflowMeta: wfTypes.WorkflowMeta{
			Name: fmt.Sprintf("%s-%s", qv.View, qv.Export),
		},
		Steps: []workflowv1alpha1.WorkflowStep{
			{
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       fmt.Sprintf("%s-%s", qv.View, qv.Export),
					Type:       qv.View,
					Properties: oamutil.Object2RawExtension(qv.Parameter),
					Outputs:    queryKey.Outputs,
				},
			},
		},
	}
	executor.InitializeWorkflowInstance(instance)
	handlerProviders := providers.NewProviders()
	kube.Install(handlerProviders, handler.cli, nil, &kube.Handlers{
		Apply:  handler.dispatch,
		Delete: handler.delete,
	})
	query.Install(handlerProviders, handler.cli, handler.cfg)
	loader := template.NewViewTemplateLoader(handler.cli, handler.namespace)
	if len(strings.Split(qv.View, "\n")) > 2 {
		loader = &template.EchoLoader{}
	}
	logCtx := monitorContext.NewTraceContext(ctx, "").AddTag("velaql")
	runners, err := generator.GenerateRunners(logCtx, instance, wfTypes.StepGeneratorOptions{
		Providers:       handlerProviders,
		PackageDiscover: handler.pd,
		ProcessCtx:      process.NewContext(process.ContextData{}),
		TemplateLoader:  loader,
		Client:          handler.cli,
		LogLevel:        3,
	})
	if err != nil {
		return nil, err
	}

	viewCtx, err := NewViewContext()
	if err != nil {
		return nil, errors.Errorf("new view context: %v", err)
	}
	for _, runner := range runners {
		status, _, err := runner.Run(viewCtx, &wfTypes.TaskRunOptions{})
		if err != nil {
			return nil, errors.Errorf("run query view: %v", err)
		}
		if string(status.Phase) != ViewTaskPhaseSucceeded {
			return nil, errors.Errorf("failed to query the view %s %s", status.Message, status.Reason)
		}
	}
	return viewCtx.GetVar(qv.Export)
}

func (handler *ViewHandler) dispatch(ctx context.Context, cluster string, owner string, manifests ...*unstructured.Unstructured) error {
	ctx = multicluster.ContextWithClusterName(ctx, cluster)
	applicator := apply.NewAPIApplicator(handler.cli)
	for _, manifest := range manifests {
		if err := applicator.Apply(ctx, manifest); err != nil {
			return err
		}
	}
	return nil
}

func (handler *ViewHandler) delete(ctx context.Context, cluster string, owner string, manifest *unstructured.Unstructured) error {
	return handler.cli.Delete(ctx, manifest)
}

// ValidateView makes sure the cue provided can use as view.
//
// For now, we only check 1. cue is valid 2. `status` or `view` field exists
func ValidateView(viewStr string) error {
	val, err := value.NewValue(viewStr, nil, "")
	if err != nil {
		return errors.Errorf("error when parsing view: %v", err)
	}

	// Make sure `status` or `export` field exists
	vStatus, errStatus := val.LookupValue(DefaultExportValue)
	vExport, errExport := val.LookupValue(KeyWordExport)
	if errStatus != nil && errExport != nil {
		return errors.Errorf("no `status` or `export` field found in view: %v, %v", errStatus, errExport)
	}
	if errStatus == nil {
		_, errStatus = vStatus.String()
	}
	if errExport == nil {
		_, errExport = vExport.String()
	}
	if errStatus != nil && errExport != nil {
		return errors.Errorf("connot get string from` status` or `export`: %v, %v", errStatus, errExport)
	}

	return nil
}

// ParseViewIntoConfigMap parses a CUE string (representing a view) into a ConfigMap
// ready to be stored into etcd.
func ParseViewIntoConfigMap(viewStr, name string) (*v1.ConfigMap, error) {
	err := ValidateView(viewStr)
	if err != nil {
		return nil, err
	}

	cm := &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: types.DefaultKubeVelaNS,
			// TODO(charlie0129): add a label to ConfigMap to identify itself as a view
			// It is useful when searching for views through all other ConfigMaps (when listing views).
		},
		Data: map[string]string{
			types.VelaQLConfigmapKey: viewStr,
		},
	}

	return cm, nil
}

// StoreViewFromFile reads a view from the specified CUE file, and stores into a ConfigMap in vela-system namespace.
// So the user can use the view in VelaQL later.
//
// By saying file, it can actually be a file, URL, or stdin (-).
func StoreViewFromFile(ctx context.Context, c client.Client, path, viewName string) error {
	content, err := utils.ReadRemoteOrLocalPath(path, false)
	if err != nil {
		return errors.Errorf("cannot load cue file: %v", err)
	}

	cm, err := ParseViewIntoConfigMap(string(content), viewName)
	if err != nil {
		return err
	}

	// Create or Update ConfigMap
	oldCm := cm.DeepCopy()
	err = c.Get(ctx, pkgtypes.NamespacedName{
		Namespace: oldCm.GetNamespace(),
		Name:      oldCm.GetName(),
	}, oldCm)

	if err != nil {
		// No previous ConfigMap found, create one.
		if apierrors.IsNotFound(err) {
			err = c.Create(ctx, cm)
			if err != nil {
				return errors.Errorf("cannot create ConfigMap %s: %v", viewName, err)
			}
			return nil
		}
		return err
	}

	// Previous ConfigMap found, update it.
	if err = c.Update(ctx, cm); err != nil {
		return errors.Errorf("cannot update ConfigMap %s: %v", viewName, err)
	}

	return nil
}

// QueryParameterKey query parameter key
type QueryParameterKey struct {
	Outputs workflowv1alpha1.StepOutputs `json:"outputs"`
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
