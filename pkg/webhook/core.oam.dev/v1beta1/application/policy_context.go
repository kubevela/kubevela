/*
Copyright 2026 The KubeVela Authors.

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

package application

import (
	"context"
	"sort"

	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	common2 "github.com/oam-dev/kubevela/pkg/controller/common"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/policy/appscope"
	"github.com/oam-dev/kubevela/pkg/policy/ctxschema"
)

// policyContextView returns context.custom as the Application-scoped policies
// that apply to app will supply it, built from their schemas, for components
// to be validated against. Nil validates without a policy context: when the policies
// are off, when no template reads the context, or when they cannot be listed,
// in which case the render still refuses what does not match.
//
// The policies are chosen and ordered as the controller does, with the same
// flags and appscope.Less: the last to set a key wins, so an order that
// differed would judge the Application by a context it never gets.
func (h *ValidatingHandler) policyContextView(ctx context.Context, app *v1beta1.Application, af *appfile.Appfile) *ctxschema.View {
	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableApplicationScopedPolicies) || !appfile.ReadsPolicyContext(af) {
		return nil
	}
	type entry struct {
		ref    appscope.Ref
		policy ctxschema.Policy
	}
	var entries []entry

	if utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableGlobalPolicies) &&
		!(common2.AllowSkipGlobalPolicies && appscope.AsksToSkipGlobals(app)) {
		globals, err := h.globalPolicies(ctx, app.Namespace)
		if err != nil {
			klog.ErrorS(err, "Skipping the policy context check: cannot list global policies", "app", client.ObjectKeyFromObject(app))
			return nil
		}
		for _, pd := range globals {
			entries = append(entries, entry{
				ref:    appscope.Ref{Name: pd.Name, Priority: pd.Spec.Priority, Global: true},
				policy: ctxschema.Policy{Name: pd.Name, Template: cueTemplate(pd)},
			})
		}
	}

	for i, p := range app.Spec.Policies {
		ref := appscope.Ref{Name: p.Name, SpecOrder: i}
		tmpl, err := appfile.LoadTemplate(ctx, h.Client, p.Type, types.TypePolicy, app.Annotations)
		if err != nil {
			entries = append(entries, entry{ref: ref, policy: ctxschema.Policy{Name: p.Type, Unknown: true}})
			continue
		}
		pd := tmpl.PolicyDefinition
		if pd == nil || pd.Spec.Scope != v1beta1.ApplicationScope {
			continue
		}
		entries = append(entries, entry{ref: ref, policy: ctxschema.Policy{Name: pd.Name, Template: cueTemplate(pd)}})
	}

	sort.SliceStable(entries, func(i, j int) bool { return appscope.Less(entries[i].ref, entries[j].ref) })
	policies := make([]ctxschema.Policy, len(entries))
	for i, e := range entries {
		policies[i] = e.policy
	}
	view := ctxschema.Build(policies)
	for _, w := range view.Warnings {
		klog.V(2).InfoS("Policy context schema not read", "app", client.ObjectKeyFromObject(app), "warning", w)
	}
	return view
}

// globalPolicies returns the global Application-scoped policies for a
// namespace as the controller's index does: those of the namespace itself,
// when namespace globals are allowed, then vela-system's not of the same name.
func (h *ValidatingHandler) globalPolicies(ctx context.Context, namespace string) ([]*v1beta1.PolicyDefinition, error) {
	namespaces := []string{oam.SystemDefinitionNamespace}
	if namespace != oam.SystemDefinitionNamespace && common2.AllowNamespaceGlobalPolicies {
		namespaces = []string{namespace, oam.SystemDefinitionNamespace}
	}
	var out []*v1beta1.PolicyDefinition
	seen := map[string]bool{}
	for _, ns := range namespaces {
		list := &v1beta1.PolicyDefinitionList{}
		if err := h.Client.List(ctx, list, client.InNamespace(ns)); err != nil {
			return nil, err
		}
		for i := range list.Items {
			pd := &list.Items[i]
			if !pd.Spec.Global || pd.Spec.Scope != v1beta1.ApplicationScope || seen[pd.Name] {
				continue
			}
			seen[pd.Name] = true
			out = append(out, pd)
		}
	}
	return out, nil
}

func cueTemplate(pd *v1beta1.PolicyDefinition) string {
	if pd.Spec.Schematic == nil || pd.Spec.Schematic.CUE == nil {
		return ""
	}
	return pd.Spec.Schematic.CUE.Template
}
