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
	"encoding/json"
	"strings"

	"cuelang.org/go/cue/cuecontext"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	monitorContext "github.com/kubevela/pkg/monitor/context"
	wfContext "github.com/kubevela/workflow/pkg/context"
	"github.com/kubevela/workflow/pkg/cue/model/sets"
	"github.com/kubevela/workflow/pkg/cue/model/value"
	wfTypes "github.com/kubevela/workflow/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/oam"
)

const (
	// ProviderName is provider name for install.
	ProviderName = "oam"
)

// ComponentApply apply oam component.
type ComponentApply func(ctx context.Context, comp common.ApplicationComponent, patcher *value.Value, clusterName string, overrideNamespace string, env string) (*unstructured.Unstructured, []*unstructured.Unstructured, bool, error)

// ComponentRender render oam component.
type ComponentRender func(ctx context.Context, comp common.ApplicationComponent, patcher *value.Value, clusterName string, overrideNamespace string, env string) (*unstructured.Unstructured, []*unstructured.Unstructured, error)

// ComponentHealthCheck health check oam component.
type ComponentHealthCheck func(ctx context.Context, comp common.ApplicationComponent, patcher *value.Value, clusterName string, overrideNamespace string, env string) (bool, *unstructured.Unstructured, []*unstructured.Unstructured, error)

// WorkloadRenderer renderer to render application component into workload
type WorkloadRenderer func(ctx context.Context, comp common.ApplicationComponent) (*appfile.Workload, error)

type provider struct {
	render ComponentRender
	apply  ComponentApply
	app    *v1beta1.Application
	af     *appfile.Appfile
	cli    client.Client
}

// RenderComponent render component
func (p *provider) RenderComponent(ctx monitorContext.Context, wfCtx wfContext.Context, v *value.Value, act wfTypes.Action) error {
	comp, patcher, clusterName, overrideNamespace, env, err := lookUpCompInfo(v)
	if err != nil {
		return err
	}
	workload, traits, err := p.render(ctx, *comp, patcher, clusterName, overrideNamespace, env)
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
func (p *provider) ApplyComponent(ctx monitorContext.Context, wfCtx wfContext.Context, v *value.Value, act wfTypes.Action) error {
	comp, patcher, clusterName, overrideNamespace, env, err := lookUpCompInfo(v)
	if err != nil {
		return err
	}
	workload, traits, healthy, err := p.apply(ctx, *comp, patcher, clusterName, overrideNamespace, env)
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

	waitHealthy, err := v.GetBool("waitHealthy")
	if err != nil {
		waitHealthy = true
	}

	if waitHealthy && !healthy {
		act.Wait("wait healthy")
	}
	return nil
}

func lookUpCompInfo(v *value.Value) (*common.ApplicationComponent, *value.Value, string, string, string, error) {
	compSettings, err := v.LookupValue("value")
	if err != nil {
		return nil, nil, "", "", "", err
	}
	comp := &common.ApplicationComponent{}

	if err := compSettings.UnmarshalTo(comp); err != nil {
		return nil, nil, "", "", "", err
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
	env, err := v.GetString("env")
	if err != nil {
		env = ""
	}
	return comp, patcher, clusterName, overrideNamespace, env, nil
}

// LoadComponent load component describe info in application.
func (p *provider) LoadComponent(ctx monitorContext.Context, wfCtx wfContext.Context, v *value.Value, act wfTypes.Action) error {
	app := &v1beta1.Application{}
	// if specify `app`, use specified application otherwise use default application from provider
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
	for _, _comp := range app.Spec.Components {
		comp, err := p.af.LoadDynamicComponent(ctx, p.cli, _comp.DeepCopy())
		if err != nil {
			return err
		}
		comp.Inputs = nil
		comp.Outputs = nil
		jt, err := json.Marshal(comp)
		if err != nil {
			return err
		}
		vs := string(jt)
		cuectx := cuecontext.New()
		val := cuectx.CompileString(vs)
		if s, err := sets.OpenBaiscLit(val); err == nil {
			v := cuectx.BuildFile(s)
			str, err := sets.ToString(v)
			if err != nil {
				return err
			}
			vs = str
		}
		if err := v.FillRaw(vs, "value", comp.Name); err != nil {
			return err
		}
	}
	return nil
}

// LoadComponentInOrder load component describe info in application output will be a list with order defined in application.
func (p *provider) LoadComponentInOrder(ctx monitorContext.Context, wfCtx wfContext.Context, v *value.Value, act wfTypes.Action) error {
	app := &v1beta1.Application{}
	// if specify `app`, use specified application otherwise use default application from provider
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
	comps := make([]common.ApplicationComponent, len(app.Spec.Components))
	for idx, _comp := range app.Spec.Components {
		comp, err := p.af.LoadDynamicComponent(ctx, p.cli, _comp.DeepCopy())
		if err != nil {
			return err
		}
		comp.Inputs = nil
		comp.Outputs = nil
		comps[idx] = *comp
	}
	if err := v.FillObject(comps, "value"); err != nil {
		return err
	}
	return nil
}

// LoadPolicies load policy describe info in application.
func (p *provider) LoadPolicies(ctx monitorContext.Context, wfCtx wfContext.Context, v *value.Value, act wfTypes.Action) error {
	for _, po := range p.app.Spec.Policies {
		if err := v.FillObject(po, "value", po.Name); err != nil {
			return err
		}
	}
	return nil
}

// Install register handlers to provider discover.
func Install(p wfTypes.Providers, app *v1beta1.Application, af *appfile.Appfile, cli client.Client, apply ComponentApply, render ComponentRender) {
	prd := &provider{
		render: render,
		apply:  apply,
		app:    app.DeepCopy(),
		af:     af,
		cli:    cli,
	}
	p.Register(ProviderName, map[string]wfTypes.Handler{
		"component-render":    prd.RenderComponent,
		"component-apply":     prd.ApplyComponent,
		"load":                prd.LoadComponent,
		"load-policies":       prd.LoadPolicies,
		"load-comps-in-order": prd.LoadComponentInOrder,
	})
}
