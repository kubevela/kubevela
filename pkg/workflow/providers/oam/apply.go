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

	"github.com/pkg/errors"
	errors2 "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/model/sets"
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
type ComponentApply func(comp common.ApplicationComponent, patcher *value.Value, clusterName string, overrideNamespace string, env string) (*unstructured.Unstructured, []*unstructured.Unstructured, bool, error)

// ComponentRender render oam component.
type ComponentRender func(comp common.ApplicationComponent, patcher *value.Value, clusterName string, overrideNamespace string, env string) (*unstructured.Unstructured, []*unstructured.Unstructured, error)

type provider struct {
	render ComponentRender
	apply  ComponentApply
	app    *v1beta1.Application
	cli    client.Client
}

// RenderComponent render component
func (p *provider) RenderComponent(ctx wfContext.Context, v *value.Value, act wfTypes.Action) error {
	comp, patcher, clusterName, overrideNamespace, env, err := lookUpValues(v)
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

// ApplyComponent apply component.
func (p *provider) ApplyComponent(ctx wfContext.Context, v *value.Value, act wfTypes.Action) error {
	comp, patcher, clusterName, overrideNamespace, env, err := lookUpValues(v)
	if err != nil {
		return err
	}
	workload, traits, healthy, err := p.apply(*comp, patcher, clusterName, overrideNamespace, env)
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

func lookUpValues(v *value.Value) (*common.ApplicationComponent, *value.Value, string, string, string, error) {
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
	if comp.Type == "ref-objects" {
		_comp := comp.DeepCopy()
		props := &struct {
			Objects []struct {
				APIVersion string            `json:"apiVersion"`
				Kind       string            `json:"kind"`
				Name       string            `json:"name,omitempty"`
				Selector   map[string]string `json:"selector,omitempty"`
			} `json:"objects"`
		}{}
		if err := json.Unmarshal(_comp.Properties.Raw, props); err != nil {
			return nil, errors.Wrapf(err, "invalid properties for ref-objects")
		}
		var objects []*unstructured.Unstructured
		addObj := func(un *unstructured.Unstructured) {
			un.SetResourceVersion("")
			un.SetGeneration(0)
			un.SetOwnerReferences(nil)
			un.SetDeletionTimestamp(nil)
			un.SetUID("")
			objects = append(objects, un)
		}
		for _, obj := range props.Objects {
			if obj.Name != "" && obj.Selector != nil {
				return nil, errors.Errorf("invalid properties for ref-objects, name and selector cannot be both set")
			}
			if obj.Name == "" && obj.Selector != nil {
				uns := &unstructured.UnstructuredList{}
				uns.SetAPIVersion(obj.APIVersion)
				uns.SetKind(obj.Kind)
				if err := p.cli.List(context.Background(), uns, client.InNamespace(p.app.GetNamespace()), client.MatchingLabels(obj.Selector)); err != nil {
					return nil, errors.Wrapf(err, "failed to load ref object %s with selector", obj.Kind)
				}
				for _, _un := range uns.Items {
					addObj(_un.DeepCopy())
				}
			} else if obj.Selector == nil {
				un := &unstructured.Unstructured{}
				un.SetAPIVersion(obj.APIVersion)
				un.SetKind(obj.Kind)
				un.SetName(obj.Name)
				un.SetNamespace(p.app.GetNamespace())
				if obj.Name == "" {
					un.SetName(comp.Name)
				}
				if err := p.cli.Get(context.Background(), client.ObjectKeyFromObject(un), un); err != nil {
					return nil, errors.Wrapf(err, "failed to load ref object %s %s/%s", un.GetKind(), un.GetNamespace(), un.GetName())
				}
				addObj(un)
			}
		}
		bs, err := json.Marshal(map[string]interface{}{"objects": objects})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to marshal loaded ref-objects")
		}
		_comp.Properties.Raw = bs
		return _comp, nil
	}
	return comp, nil
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

func (p *provider) LoadPoliciesInOrder(ctx wfContext.Context, v *value.Value, act wfTypes.Action) error {
	policyMap := map[string]v1beta1.AppPolicy{}
	var specifiedPolicyNames []string
	specifiedPolicyNamesRaw, err := v.LookupValue("input")
	if err != nil {
		return err
	}
	if specifiedPolicyNamesRaw == nil {
		for _, policy := range p.app.Spec.Policies {
			specifiedPolicyNames = append(specifiedPolicyNames, policy.Name)
		}
	}
	if err = specifiedPolicyNamesRaw.UnmarshalTo(&specifiedPolicyNames); err != nil {
		return errors.Wrapf(err, "failed to parse specified policy names")
	}
	for _, policy := range p.app.Spec.Policies {
		policyMap[policy.Name] = policy
	}
	var specifiedPolicies []v1beta1.AppPolicy
	for _, policyName := range specifiedPolicyNames {
		if policy, found := policyMap[policyName]; found {
			specifiedPolicies = append(specifiedPolicies, policy)
		} else {
			po := &v1alpha1.Policy{}
			if err = p.cli.Get(context.Background(), types.NamespacedName{Namespace: p.app.GetNamespace(), Name: policyName}, po); err != nil {
				if errors2.IsNotFound(err) {
					return errors.Errorf("policy %s not found", policyName)
				}
				return errors.Wrapf(err, "failed to search policy %s in namespace %s", policyName, p.app.GetNamespace())
			}
			specifiedPolicies = append(specifiedPolicies, v1beta1.AppPolicy{Name: policyName, Type: po.Type, Properties: po.Properties})
		}
	}
	return v.FillObject(specifiedPolicies, "output")
}

// Install register handlers to provider discover.
func Install(p providers.Providers, app *v1beta1.Application, cli client.Client, apply ComponentApply, render ComponentRender) {
	prd := &provider{
		render: render,
		apply:  apply,
		app:    app.DeepCopy(),
		cli:    cli,
	}
	p.Register(ProviderName, map[string]providers.Handler{
		"component-render":       prd.RenderComponent,
		"component-apply":        prd.ApplyComponent,
		"load":                   prd.LoadComponent,
		"load-policies":          prd.LoadPolicies,
		"load-policies-in-order": prd.LoadPoliciesInOrder,
		"load-comps-in-order":    prd.LoadComponentInOrder,
	})
}
