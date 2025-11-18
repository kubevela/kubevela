/*
Copyright 2025 The KubeVela Authors.

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

package definition

import (
	"context"
	"cuelang.org/go/cue"
	_ "embed"
	"encoding/json"
	"fmt"
	compilercontext "github.com/kubevela/pkg/cue/cuex/context"
	"github.com/kubevela/pkg/cue/cuex/providers"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/runtime"
	"github.com/kubevela/pkg/util/singleton"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/pkg/errors"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type RenderInput struct {
	Definition string      `json:"definition"`
	Properties interface{} `json:"properties"`
}

type RenderOutput struct {
	Output  interface{} `json:"output"`
	Outputs interface{} `json:"outputs"`
}

type RenderInputParams providers.Params[RenderInput]
type RenderOutputParams providers.Returns[RenderOutput]

func RenderComponent(ctx context.Context, value cue.Value) (cue.Value, error) {
	definition, _ := value.LookupPath(cue.ParsePath("definition")).String()
	propertiesVal := value.LookupPath(cue.ParsePath("properties")).Value()
	baseCtx := ctx.Value("base")

	var properties map[string]interface{}
	if propertiesVal.Exists() {
		err := propertiesVal.Decode(&properties)
		if err != nil {
			return cue.Value{}, err
		}
	}

	namespace := "vela-system"
	klog.V(4).Infof("Rendering component definition %s in namespace %s", definition, namespace)

	kubeClient := singleton.KubeClient.Get()
	if kubeClient == nil {
		return cue.Value{}, errors.New("KubeClient not available")
	}

	// Fetch ComponentDefinition
	compDef := &v1beta1.ComponentDefinition{}
	if err := kubeClient.Get(ctx, client.ObjectKey{
		Name:      definition,
		Namespace: namespace,
	}, compDef); err != nil {
		return cue.Value{}, errors.Wrapf(err, "failed to get ComponentDefinition %s in namespace %s", definition, namespace)
	}

	// Prepare the CUE template with parameters
	template := compDef.Spec.Schematic.CUE.Template
	if template == "" {
		return cue.Value{}, errors.Errorf("ComponentDefinition %s has empty template", definition)
	}

	// Prepare parameters as CUE
	paramStr := "parameter: {}"
	if properties != nil {
		paramBytes, err := json.Marshal(properties)
		if err != nil {
			return cue.Value{}, errors.Wrap(err, "failed to marshal parameters")
		}
		paramStr = fmt.Sprintf("parameter: %s", string(paramBytes))
	}

	// Compile the template with parameters
	fullTemplate := fmt.Sprintf("%s\n%s\n%s", template, paramStr, baseCtx)
	compiler := compilercontext.GetCompiler(ctx)
	v, err := compiler.CompileString(ctx, fullTemplate)

	if err != nil {
		return cue.Value{}, errors.Wrapf(err, "template validation failed for definition %s", definition)
	}

	if err := v.Validate(); err != nil {
		return cue.Value{}, errors.Wrapf(err, "template validation failed for definition %s", definition)
	}

	klog.Infof("Successfully rendered ComponentDefinition %s", definition)
	return v, nil
}

// ProviderName .
const ProviderName = "def"

//go:embed definition.cue
var template string

// Package .
var Package = runtime.Must(cuexruntime.NewInternalPackage(ProviderName, template, map[string]cuexruntime.ProviderFn{
	"RenderComponent": cuexruntime.NativeProviderFn(RenderComponent),
}))
