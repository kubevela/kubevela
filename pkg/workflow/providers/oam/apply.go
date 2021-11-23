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
	"encoding/json"
	"strings"

	"github.com/oam-dev/kubevela/pkg/cue/model/sets"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/oam"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	wfTypes "github.com/oam-dev/kubevela/pkg/workflow/types"
)

const (
	// ProviderName is provider name for install.
	ProviderName = "oam"
)

// ComponentApply apply oam component.
type ComponentApply func(comp common.ApplicationComponent, patcher *value.Value, clusterName string, overrideNamespace string) (*unstructured.Unstructured, []*unstructured.Unstructured, bool, error)

// ComponentRender render oam component.
type ComponentRender func(comp common.ApplicationComponent, patcher *value.Value, clusterName string, overrideNamespace string) (*unstructured.Unstructured, []*unstructured.Unstructured, error)

type provider struct {
	render ComponentRender
	apply  ComponentApply
	app    *v1beta1.Application
}

// RenderComponent render component
func (p *provider) RenderComponent(ctx wfContext.Context, v *value.Value, act wfTypes.Action) error {
	comp, patcher, clusterName, overrideNamespace, err := lookUpValues(v)
	if err != nil {
		return err
	}
	workload, traits, err := p.render(*comp, patcher, clusterName, overrideNamespace)
	if err != nil {
		return err
	}

	if workload != nil {
		if err := v.FillObject(workload.Object, "output"); err != nil {
			return errors.WithMessage(err, "FillOutput")
		}
	}

	for _, trait := range traits {
		name := trait.GetLabels()[oam.TraitResource]
		if name != "" {
			if err := v.FillObject(trait.Object, "outputs", name); err != nil {
				return errors.WithMessage(err, "FillOutputs")
			}
		}
	}

	return nil
}

// ApplyComponent apply component.
func (p *provider) ApplyComponent(ctx wfContext.Context, v *value.Value, act wfTypes.Action) error {
	comp, patcher, clusterName, overrideNamespace, err := lookUpValues(v)
	if err != nil {
		return err
	}
	workload, traits, healthy, err := p.apply(*comp, patcher, clusterName, overrideNamespace)
	if err != nil {
		return err
	}

	if workload != nil {
		if err := v.FillObject(workload.Object, "output"); err != nil {
			return errors.WithMessage(err, "FillOutput")
		}
	}

	for _, trait := range traits {
		name := trait.GetLabels()[oam.TraitResource]
		if name != "" {
			if err := v.FillObject(trait.Object, "outputs", name); err != nil {
				return errors.WithMessage(err, "FillOutputs")
			}
		}
	}

	if !healthy {
		act.Wait("wait healthy")
	}

	return nil
}

func lookUpValues(v *value.Value) (*common.ApplicationComponent, *value.Value, string, string, error) {
	compSettings, err := v.LookupValue("value")
	if err != nil {
		return nil, nil, "", "", err
	}
	comp := &common.ApplicationComponent{}

	if err := compSettings.UnmarshalTo(comp); err != nil {
		return nil, nil, "", "", err
	}
	patcher, err := v.LookupValue("patch")
	if err != nil {
		patcher = nil
	}
	clusterName, err := v.GetString("cluster")
	if err != nil {
		clusterName = ""
	}
	overrideNamespace, err := v.GetString("namespace")
	if err != nil {
		overrideNamespace = ""
	}
	return comp, patcher, clusterName, overrideNamespace, nil
}

// LoadComponent load component describe info in application.
func (p *provider) LoadComponent(ctx wfContext.Context, v *value.Value, act wfTypes.Action) error {
	app := &v1beta1.Application{}
	// if specify `app`, use specified application otherwise use default application fron provider
	appSettings, err := v.LookupValue("app")
	if err != nil {
		if strings.Contains(err.Error(), "not exist") {
			app = p.app
		} else {
			return err
		}
	} else {
		if err := appSettings.UnmarshalTo(app); err != nil {
			return err
		}
	}
	for _, comp := range app.Spec.Components {
		comp.Inputs = nil
		comp.Outputs = nil
		jt, err := json.Marshal(comp)
		if err != nil {
			return err
		}
		vs := string(jt)
		if s, err := sets.OpenBaiscLit(vs); err == nil {
			vs = s
		}
		if err := v.FillRaw(vs, "value", comp.Name); err != nil {
			return err
		}
	}
	return nil
}

// LoadComponentInOrder load component describe info in application output will be a list with order defined in application.
func (p *provider) LoadComponentInOrder(ctx wfContext.Context, v *value.Value, act wfTypes.Action) error {
	app := &v1beta1.Application{}
	// if specify `app`, use specified application otherwise use default application fron provider
	appSettings, err := v.LookupValue("app")
	if err != nil {
		if strings.Contains(err.Error(), "not exist") {
			app = p.app
		} else {
			return err
		}
	} else {
		if err := appSettings.UnmarshalTo(app); err != nil {
			return err
		}
	}
	if err := v.FillObject(app.Spec.Components, "value"); err != nil {
		return err
	}
	return nil
}

// LoadPolicies load policy describe info in application.
func (p *provider) LoadPolicies(ctx wfContext.Context, v *value.Value, act wfTypes.Action) error {
	for _, po := range p.app.Spec.Policies {
		if err := v.FillObject(po, "value", po.Name); err != nil {
			return err
		}
	}
	return nil
}

// Install register handlers to provider discover.
func Install(p providers.Providers, app *v1beta1.Application, apply ComponentApply, render ComponentRender) {
	prd := &provider{
		render: render,
		apply:  apply,
		app:    app.DeepCopy(),
	}
	p.Register(ProviderName, map[string]providers.Handler{
		"component-render":    prd.RenderComponent,
		"component-apply":     prd.ApplyComponent,
		"load":                prd.LoadComponent,
		"load-policies":       prd.LoadPolicies,
		"load-comps-in-order": prd.LoadComponentInOrder,
	})
}
