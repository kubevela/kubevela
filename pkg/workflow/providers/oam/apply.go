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

package oam

import (
	"context"
	_ "embed"

	"cuelang.org/go/cue"
	"k8s.io/apimachinery/pkg/types"

	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/workflow/pkg/cue/model/value"
	workflowerrors "github.com/kubevela/workflow/pkg/errors"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	oamprovidertypes "github.com/oam-dev/kubevela/pkg/workflow/providers/types"
)

const (
	// ProviderName is provider name for install.
	ProviderName = "oam"
)

// RenderComponent render component
func RenderComponent(ctx context.Context, params *oamprovidertypes.Params[cue.Value]) (cue.Value, error) {
	v := params.Params

	comp, patcher, clusterName, overrideNamespace, err := lookUpCompInfo(v)
	if err != nil {
		return cue.Value{}, err
	}
	workload, traits, err := params.ComponentRender(ctx, *comp, patcher, clusterName, overrideNamespace)
	if err != nil {
		return cue.Value{}, err
	}

	if workload != nil {
		v = v.FillPath(value.FieldPath("$returns", "output"), workload.Object)
	}

	for _, trait := range traits {
		name := trait.GetLabels()[oam.TraitResource]
		if name != "" {
			v = v.FillPath(value.FieldPath("$returns", "outputs", name), workload.Object)
		}
	}

	return v, nil
}

// ApplyComponent apply component.
func ApplyComponent(ctx context.Context, params *oamprovidertypes.Params[cue.Value]) (cue.Value, error) {
	v := params.Params
	comp, patcher, clusterName, overrideNamespace, err := lookUpCompInfo(v)
	if err != nil {
		return cue.Value{}, err
	}
	workload, traits, healthy, err := params.ComponentApply(ctx, *comp, patcher, clusterName, overrideNamespace)
	if err != nil {
		return cue.Value{}, err
	}

	if workload != nil {
		v = v.FillPath(value.FieldPath("$returns", "output"), workload.Object)
	}

	for _, trait := range traits {
		name := trait.GetLabels()[oam.TraitResource]
		if name != "" {
			v = v.FillPath(value.FieldPath("$returns", "outputs", name), trait)
		}
	}

	waitHealthy, err := v.LookupPath(cue.ParsePath("waitHealthy")).Bool()
	if err != nil {
		waitHealthy = true
	}

	if waitHealthy && !healthy {
		params.Action.Wait("wait healthy")
	}
	return v, nil
}

func lookUpCompInfo(v cue.Value) (*common.ApplicationComponent, *cue.Value, string, string, error) {
	compSettings := v.LookupPath(cue.ParsePath("value"))
	if !compSettings.Exists() {
		return nil, nil, "", "", workflowerrors.LookUpNotFoundErr("value")
	}
	comp := &common.ApplicationComponent{}

	if err := value.UnmarshalTo(compSettings, comp); err != nil {
		return nil, nil, "", "", err
	}
	var patcherValue *cue.Value
	patcher := v.LookupPath(cue.ParsePath("patch"))
	if patcher.Exists() {
		patcherValue = &patcher
	}
	clusterName, err := v.LookupPath(cue.ParsePath("cluster")).String()
	if err != nil {
		clusterName = ""
	}
	overrideNamespace, err := v.LookupPath(cue.ParsePath("namespace")).String()
	if err != nil {
		overrideNamespace = ""
	}
	return comp, patcherValue, clusterName, overrideNamespace, nil
}

// LoadVars is the load provider vars.
type LoadVars struct {
	App string `json:"app,omitempty"`
}

// LoadReturnVars is the load provider return vars.
type LoadReturnVars struct {
	Value any `json:"value"`
}

// LoadParams is the load provider params.
type LoadParams = oamprovidertypes.Params[LoadVars]

// LoadReturns is the load provider returns.
type LoadReturns = oamprovidertypes.Returns[LoadReturnVars]

// LoadComponent load component describe info in application.
func LoadComponent(ctx context.Context, params *LoadParams) (*LoadReturns, error) {
	app := &v1beta1.Application{}
	cli := params.KubeClient
	// if specify `app`, use specified application otherwise use default application from provider
	appSettings := params.Params.App
	if appSettings == "" {
		app = params.App
	} else {
		if err := cli.Get(ctx, types.NamespacedName{Name: appSettings, Namespace: params.App.Namespace}, app); err != nil {
			return nil, err
		}
	}
	comps := make(map[string]*common.ApplicationComponent, 0)
	for _, _comp := range app.Spec.Components {
		comp, err := params.Appfile.LoadDynamicComponent(ctx, cli, _comp.DeepCopy())
		if err != nil {
			return nil, err
		}
		comp.Inputs = nil
		comp.Outputs = nil
		comps[_comp.Name] = comp
	}
	return &LoadReturns{Returns: LoadReturnVars{Value: comps}}, nil
}

// LoadComponentInOrder load component describe info in application output will be a list with order defined in application.
func LoadComponentInOrder(ctx context.Context, params *LoadParams) (*LoadReturns, error) {
	app := &v1beta1.Application{}
	cli := params.KubeClient
	// if specify `app`, use specified application otherwise use default application from provider
	appSettings := params.Params.App
	if appSettings == "" {
		app = params.App
	} else {
		if err := cli.Get(ctx, types.NamespacedName{Name: appSettings, Namespace: params.App.Namespace}, app); err != nil {
			return nil, err
		}
	}
	comps := make([]common.ApplicationComponent, len(app.Spec.Components))
	for idx, _comp := range app.Spec.Components {
		comp, err := params.Appfile.LoadDynamicComponent(ctx, cli, _comp.DeepCopy())
		if err != nil {
			return nil, err
		}
		comp.Inputs = nil
		comp.Outputs = nil
		comps[idx] = *comp
	}
	return &LoadReturns{Returns: LoadReturnVars{Value: comps}}, nil
}

// LoadPolicies load policy describe info in application.
func LoadPolicies(_ context.Context, params *LoadParams) (*LoadReturns, error) {
	app := params.App
	policies := make(map[string]v1beta1.AppPolicy, 0)
	for _, po := range app.Spec.Policies {
		policies[po.Name] = po
	}
	return &LoadReturns{Returns: LoadReturnVars{Value: policies}}, nil
}

//go:embed oam.cue
var template string

// GetTemplate returns the cue template.
func GetTemplate() string {
	return template
}

// GetProviders returns the cue providers.
func GetProviders() map[string]cuexruntime.ProviderFn {
	return map[string]cuexruntime.ProviderFn{
		"component-render":    oamprovidertypes.NativeProviderFn(RenderComponent),
		"component-apply":     oamprovidertypes.NativeProviderFn(ApplyComponent),
		"load":                oamprovidertypes.GenericProviderFn[LoadVars, LoadReturns](LoadComponent),
		"load-comps-in-order": oamprovidertypes.GenericProviderFn[LoadVars, LoadReturns](LoadComponentInOrder),
		"load-policies":       oamprovidertypes.GenericProviderFn[LoadVars, LoadReturns](LoadPolicies),
	}
}
