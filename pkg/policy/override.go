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

package policy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	errors2 "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// ParseOverridePolicyRelatedDefinitions get definitions inside override policy
func ParseOverridePolicyRelatedDefinitions(ctx context.Context, cli client.Client, app *v1beta1.Application, policy v1beta1.AppPolicy) (compDefs []*v1beta1.ComponentDefinition, traitDefs []*v1beta1.TraitDefinition, err error) {
	if policy.Properties == nil {
		return compDefs, traitDefs, fmt.Errorf("override policy %s must not have empty properties", policy.Name)
	}
	spec := &v1alpha1.OverridePolicySpec{}
	if err = json.Unmarshal(policy.Properties.Raw, spec); err != nil {
		return nil, nil, errors.Wrapf(err, "invalid override policy spec")
	}
	componentTypes := map[string]struct{}{}
	traitTypes := map[string]struct{}{}
	for _, comp := range spec.Components {
		if comp.Type != "" {
			componentTypes[comp.Type] = struct{}{}
		}
		for _, trait := range comp.Traits {
			if trait.Type != "" {
				traitTypes[trait.Type] = struct{}{}
			}
		}
	}
	getDef := func(name string, _type string, obj client.Object) error {
		err = cli.Get(ctx, types.NamespacedName{Namespace: oam.SystemDefinitionNamespace, Name: name}, obj)
		if err != nil && errors2.IsNotFound(err) {
			err = cli.Get(ctx, types.NamespacedName{Namespace: app.Namespace, Name: name}, obj)
		}
		if err != nil {
			return errors.Wrapf(err, "failed to get %s definition %s for override policy %s", _type, name, policy.Name)
		}
		return nil
	}
	for compDefName := range componentTypes {
		def := &v1beta1.ComponentDefinition{}
		if err = getDef(compDefName, "component", def); err != nil {
			return nil, nil, err
		}
		compDefs = append(compDefs, def)
	}
	for traitDefName := range traitTypes {
		def := &v1beta1.TraitDefinition{}
		if err = getDef(traitDefName, "trait", def); err != nil {
			return nil, nil, err
		}
		traitDefs = append(traitDefs, def)
	}
	return compDefs, traitDefs, nil
}
