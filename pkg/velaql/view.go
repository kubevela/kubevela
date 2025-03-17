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
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	pkgtypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/cue/cuex"
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	"github.com/kubevela/workflow/pkg/cue/model/value"
	providertypes "github.com/kubevela/workflow/pkg/providers/types"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	oamprovidertypes "github.com/oam-dev/kubevela/pkg/workflow/providers/types"
	"github.com/oam-dev/kubevela/pkg/workflow/template"
)

const (
	qlNs = "vela-system"

	// ViewTaskPhaseSucceeded means view task run succeeded.
	ViewTaskPhaseSucceeded = "succeeded"
)

// ViewHandler view handler
type ViewHandler struct {
	cli       client.Client
	cfg       *rest.Config
	namespace string
}

// NewViewHandler new view handler
func NewViewHandler(cli client.Client, cfg *rest.Config) *ViewHandler {
	return &ViewHandler{
		cli:       cli,
		cfg:       cfg,
		namespace: qlNs,
	}
}

// QueryView generate view step
func (handler *ViewHandler) QueryView(ctx context.Context, qv QueryView) (cue.Value, error) {
	outputsTemplate := fmt.Sprintf(OutputsTemplate, qv.Export, qv.Export)
	queryKey := QueryParameterKey{}
	if err := json.Unmarshal([]byte(outputsTemplate), &queryKey); err != nil {
		return cue.Value{}, errors.Errorf("unmarhsal query template: %v", err)
	}
	ctx = oamprovidertypes.WithRuntimeParams(ctx, oamprovidertypes.RuntimeParams{
		KubeClient:   handler.cli,
		KubeConfig:   handler.cfg,
		KubeHandlers: &providertypes.KubeHandlers{Apply: handler.dispatch, Delete: handler.delete},
	})
	loader := template.NewViewTemplateLoader(handler.cli, handler.namespace)
	if len(strings.Split(qv.View, "\n")) > 2 {
		loader = &template.EchoLoader{}
	}
	temp, err := loader.LoadTemplate(ctx, qv.View)
	if err != nil {
		return cue.Value{}, fmt.Errorf("failed to load query templates: %w", err)
	}
	v, err := providers.DefaultCompiler.Get().CompileStringWithOptions(ctx, temp, cuex.WithExtraData("parameter", qv.Parameter))
	if err != nil {
		return cue.Value{}, fmt.Errorf("failed to compile query: %w", err)
	}
	res := v.LookupPath(value.FieldPath(qv.Export))
	if !res.Exists() {
		return cuecontext.New().CompileString("null"), nil
	}
	return res, res.Err()
}

func (handler *ViewHandler) dispatch(ctx context.Context, _ client.Client, cluster string, _ string, manifests ...*unstructured.Unstructured) error {
	ctx = multicluster.ContextWithClusterName(ctx, cluster)
	applicator := apply.NewAPIApplicator(handler.cli)
	for _, manifest := range manifests {
		if err := applicator.Apply(ctx, manifest); err != nil {
			return err
		}
	}
	return nil
}

func (handler *ViewHandler) delete(ctx context.Context, _ client.Client, _ string, _ string, manifest *unstructured.Unstructured) error {
	return handler.cli.Delete(ctx, manifest)
}

// ValidateView makes sure the cue provided can use as view.
//
// For now, we only check 1. cue is valid 2. `status` or `view` field exists
func ValidateView(ctx context.Context, viewStr string) error {
	val, err := providers.DefaultCompiler.Get().CompileStringWithOptions(ctx, viewStr, cuex.DisableResolveProviderFunctions{})
	if err != nil {
		return errors.Errorf("error when parsing view: %v", err)
	}

	// Make sure `status` or `export` field exists
	vStatus := val.LookupPath(cue.ParsePath(DefaultExportValue))
	vExport := val.LookupPath(cue.ParsePath(KeyWordExport))
	if !vStatus.Exists() && !vExport.Exists() {
		return errors.Errorf("no `status` or `export` field found in view")
	}

	return nil
}

// ParseViewIntoConfigMap parses a CUE string (representing a view) into a ConfigMap
// ready to be stored into etcd.
func ParseViewIntoConfigMap(ctx context.Context, viewStr, name string) (*v1.ConfigMap, error) {
	err := ValidateView(ctx, viewStr)
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

	cm, err := ParseViewIntoConfigMap(ctx, string(content), viewName)
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
