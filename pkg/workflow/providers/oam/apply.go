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
	"sync"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/component"
	"github.com/oam-dev/kubevela/pkg/cue/model/sets"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/oam"
	velaerrors "github.com/oam-dev/kubevela/pkg/utils/errors"
	"github.com/oam-dev/kubevela/pkg/utils/parallel"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	wfTypes "github.com/oam-dev/kubevela/pkg/workflow/types"
)

const (
	// ProviderName is provider name for install.
	ProviderName = "oam"
)

// ComponentApply apply oam component.
type ComponentApply func(comp common.ApplicationComponent, patcher *value.Value, clusterName string, overrideNamespace string, env string) (*unstructured.Unstructured, []*unstructured.Unstructured, bool, error)

// ComponentRender render oam component.
type ComponentRender func(comp common.ApplicationComponent, patcher *value.Value, clusterName string, overrideNamespace string, env string) (*unstructured.Unstructured, []*unstructured.Unstructured, error)

type provider struct {
	render ComponentRender
	apply  ComponentApply
	app    *v1beta1.Application
	af     *appfile.Appfile
	cli    client.Client
}

// RenderComponent render component
func (p *provider) RenderComponent(ctx wfContext.Context, v *value.Value, act wfTypes.Action) error {
	comp, patcher, clusterName, overrideNamespace, env, err := lookUpValues(v, nil)
	if err != nil {
		return err
	}
	workload, traits, err := p.render(*comp, patcher, clusterName, overrideNamespace, env)
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

func (p *provider) applyComponent(_ wfContext.Context, v *value.Value, act wfTypes.Action, mu *sync.Mutex) error {
	comp, patcher, clusterName, overrideNamespace, env, err := lookUpValues(v, mu)
	if err != nil {
		return err
	}
	workload, traits, healthy, err := p.apply(*comp, patcher, clusterName, overrideNamespace, env)
	if err != nil {
		return err
	}

	if mu != nil {
		mu.Lock()
		defer mu.Unlock()
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

// ApplyComponent apply component.
func (p *provider) ApplyComponent(ctx wfContext.Context, v *value.Value, act wfTypes.Action) error {
	return p.applyComponent(ctx, v, act, nil)
}

// ApplyComponents apply components in parallel.
func (p *provider) ApplyComponents(ctx wfContext.Context, v *value.Value, act wfTypes.Action) error {
	components, err := v.LookupValue("components")
	if err != nil {
		return err
	}
	parallelism, err := v.GetInt64("parallelism")
	if err != nil {
		return err
	}
	if parallelism <= 0 {
		return errors.Errorf("parallelism cannot be smaller than 1")
	}
	// prepare parallel execution args
	mu := &sync.Mutex{}
	var parInputs [][]interface{}
	if err = components.StepByFields(func(name string, in *value.Value) (bool, error) {
		parInputs = append(parInputs, []interface{}{name, ctx, in, act, mu})
		return false, nil
	}); err != nil {
		return errors.Wrapf(err, "failed to looping over components")
	}
	// parallel execution
	outputs := parallel.Run(func(name string, ctx wfContext.Context, v *value.Value, act wfTypes.Action, mu *sync.Mutex) error {
		if err := p.applyComponent(ctx, v, act, mu); err != nil {
			return errors.Wrapf(err, "failed to apply component %s", name)
		}
		return nil
	}, parInputs, int(parallelism))
	// aggregate errors
	return velaerrors.AggregateErrors(outputs.([]error))
}

func lookUpValues(v *value.Value, mu *sync.Mutex) (*common.ApplicationComponent, *value.Value, string, string, string, error) {
	if mu != nil {
		mu.Lock()
		defer mu.Unlock()
	}
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

func (p *provider) loadDynamicComponent(comp *common.ApplicationComponent) (*common.ApplicationComponent, error) {
	if comp.Type != v1alpha1.RefObjectsComponentType {
		return comp, nil
	}
	_comp := comp.DeepCopy()
	spec := &v1alpha1.RefObjectsComponentSpec{}
	if err := json.Unmarshal(comp.Properties.Raw, spec); err != nil {
		return nil, errors.Wrapf(err, "invalid ref-object component properties")
	}
	var uns []*unstructured.Unstructured
	for _, selector := range spec.Objects {
		objs, err := component.SelectRefObjectsForDispatch(context.Background(), component.ReferredObjectsDelegatingClient(p.cli, p.af.ReferredObjects), p.af.Namespace, comp.Name, selector)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to select objects from referred objects in revision storage")
		}
		uns = component.AppendUnstructuredObjects(uns, objs...)
	}
	refObjs, err := component.ConvertUnstructuredsToReferredObjects(uns)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal referred object")
	}
	bs, err := json.Marshal(&common.ReferredObjectList{Objects: refObjs})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal loaded ref-objects")
	}
	_comp.Properties = &runtime.RawExtension{Raw: bs}
	return _comp, nil
}

// LoadComponent load component describe info in application.
func (p *provider) LoadComponent(ctx wfContext.Context, v *value.Value, act wfTypes.Action) error {
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
		comp, err := p.loadDynamicComponent(_comp.DeepCopy())
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
		comp, err := p.loadDynamicComponent(_comp.DeepCopy())
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
func (p *provider) LoadPolicies(ctx wfContext.Context, v *value.Value, act wfTypes.Action) error {
	for _, po := range p.app.Spec.Policies {
		if err := v.FillObject(po, "value", po.Name); err != nil {
			return err
		}
	}
	return nil
}

func (p *provider) LoadPoliciesInOrder(ctx wfContext.Context, v *value.Value, act wfTypes.Action) error {
	policyMap := map[string]v1beta1.AppPolicy{}
	var specifiedPolicyNames []string
	specifiedPolicyNamesRaw, err := v.LookupValue("input")
	if err != nil || specifiedPolicyNamesRaw == nil {
		for _, policy := range p.app.Spec.Policies {
			specifiedPolicyNames = append(specifiedPolicyNames, policy.Name)
		}
	} else if err = specifiedPolicyNamesRaw.UnmarshalTo(&specifiedPolicyNames); err != nil {
		return errors.Wrapf(err, "failed to parse specified policy names")
	}
	for _, policy := range p.af.Policies {
		policyMap[policy.Name] = policy
	}
	var specifiedPolicies []v1beta1.AppPolicy
	for _, policyName := range specifiedPolicyNames {
		if policy, found := policyMap[policyName]; found {
			specifiedPolicies = append(specifiedPolicies, policy)
		} else {
			return errors.Errorf("policy %s not found", policyName)
		}
	}
	return v.FillObject(specifiedPolicies, "output")
}

// Install register handlers to provider discover.
func Install(p providers.Providers, app *v1beta1.Application, af *appfile.Appfile, cli client.Client, apply ComponentApply, render ComponentRender) {
	prd := &provider{
		render: render,
		apply:  apply,
		app:    app.DeepCopy(),
		af:     af,
		cli:    cli,
	}
	p.Register(ProviderName, map[string]providers.Handler{
		"component-render":       prd.RenderComponent,
		"component-apply":        prd.ApplyComponent,
		"components-apply":       prd.ApplyComponents,
		"load":                   prd.LoadComponent,
		"load-policies":          prd.LoadPolicies,
		"load-policies-in-order": prd.LoadPoliciesInOrder,
		"load-comps-in-order":    prd.LoadComponentInOrder,
	})
}
