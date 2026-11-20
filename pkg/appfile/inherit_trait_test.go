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

package appfile

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func traitDef(name, extends, template string) *v1beta1.TraitDefinition {
	return &v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1beta1.TraitDefinitionSpec{
			Extends:   extends,
			Schematic: &common.Schematic{CUE: &common.CUE{Template: template}},
		},
	}
}

func traitFetcherOver(defs map[string]*v1beta1.TraitDefinition) traitFetcher {
	return func(_ context.Context, name string) (*v1beta1.TraitDefinition, error) {
		td, ok := defs[name]
		if !ok {
			return nil, fmt.Errorf("trait definition %s not found", name)
		}
		return td.DeepCopy(), nil
	}
}

func traitIn(namespace, name, extends, template string) *v1beta1.TraitDefinition {
	td := traitDef(name, extends, template)
	td.Namespace = namespace
	return td
}

func traitClient(objs ...*v1beta1.TraitDefinition) *fake.ClientBuilder {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	b := fake.NewClientBuilder().WithScheme(scheme)
	for _, o := range objs {
		b = b.WithObjects(o)
	}
	return b
}

func TestTraitChainIsResolvedAndRecorded(t *testing.T) {
	withInheritance(t)

	tmpl := &Template{}
	child := traitDef("tenant-gateway", "gateway", "$super: properties: {}")

	require.NoError(t, resolveTraitChain(context.Background(), tmpl, child,
		traitFetcherOver(map[string]*v1beta1.TraitDefinition{
			"gateway": traitDef("gateway", "", "patch: {}"),
		})))

	require.Len(t, tmpl.Ancestors, 1)
	require.Equal(t, "gateway", tmpl.Ancestors[0].Name)
	require.Contains(t, tmpl.AncestorTraitDefinitions, "gateway")
}

func TestTraitCycleIsNamedRatherThanLooped(t *testing.T) {
	withInheritance(t)

	defs := map[string]*v1beta1.TraitDefinition{
		"a": traitDef("a", "b", "$super: properties: {}"),
		"b": traitDef("b", "a", "$super: properties: {}"),
	}

	err := resolveTraitChain(context.Background(), &Template{}, defs["a"].DeepCopy(), traitFetcherOver(defs))
	require.ErrorContains(t, err, "already in its own inheritance chain")
}

func TestTraitDepthIsCapped(t *testing.T) {
	withInheritance(t)

	defs := map[string]*v1beta1.TraitDefinition{}
	for i := 0; i <= MaxInheritanceDepth+2; i++ {
		defs[fmt.Sprintf("t%d", i)] = traitDef(fmt.Sprintf("t%d", i), fmt.Sprintf("t%d", i+1), "$super: properties: {}")
	}

	err := resolveTraitChain(context.Background(), &Template{}, defs["t0"].DeepCopy(), traitFetcherOver(defs))
	require.ErrorContains(t, err, fmt.Sprintf("deeper than %d", MaxInheritanceDepth))
}

func TestTraitExtendsIsRefusedWhileTheGateIsOff(t *testing.T) {
	err := resolveTraitChain(context.Background(), &Template{},
		traitDef("tenant-gateway", "gateway", "$super: properties: {}"),
		traitFetcherOver(map[string]*v1beta1.TraitDefinition{
			"gateway": traitDef("gateway", "", "patch: {}"),
		}))

	require.ErrorContains(t, err, "EnableDefinitionInheritance")
}

// A trait's attributes are inherited where the child is silent, and the booleans
// are OR'd: those fields cannot tell unset from false, so a child cannot turn a
// parent's off. Documented rather than worked around, since making them pointers
// is an API change to fields every consumer reads.
func TestTraitAttributesAreInheritedWhereTheChildIsSilent(t *testing.T) {
	withInheritance(t)

	parent := traitDef("gateway", "", "patch: {}")
	parent.Spec.AppliesToWorkloads = []string{"deployments.apps"}
	parent.Spec.ConflictsWith = []string{"ingress"}
	parent.Spec.WorkloadRefPath = "spec.workloadRef"
	parent.Spec.Stage = v1beta1.PostDispatch
	parent.Spec.PodDisruptive = true
	parent.Spec.RevisionEnabled = true

	t.Run("silent child inherits", func(t *testing.T) {
		tmpl := &Template{}
		child := traitDef("tenant-gateway", "gateway", "$super: properties: {}")

		require.NoError(t, resolveTraitChain(context.Background(), tmpl, child,
			traitFetcherOver(map[string]*v1beta1.TraitDefinition{"gateway": parent})))

		require.Equal(t, []string{"deployments.apps"}, child.Spec.AppliesToWorkloads)
		require.Equal(t, []string{"ingress"}, child.Spec.ConflictsWith)
		require.Equal(t, "spec.workloadRef", child.Spec.WorkloadRefPath)
		require.Equal(t, v1beta1.PostDispatch, child.Spec.Stage)
		require.True(t, child.Spec.PodDisruptive)
		require.True(t, child.Spec.RevisionEnabled)
	})

	t.Run("a child that speaks keeps its own", func(t *testing.T) {
		tmpl := &Template{}
		child := traitDef("tenant-gateway", "gateway", "$super: properties: {}")
		child.Spec.AppliesToWorkloads = []string{"statefulsets.apps"}
		child.Spec.WorkloadRefPath = "spec.other"

		require.NoError(t, resolveTraitChain(context.Background(), tmpl, child,
			traitFetcherOver(map[string]*v1beta1.TraitDefinition{"gateway": parent})))

		require.Equal(t, []string{"statefulsets.apps"}, child.Spec.AppliesToWorkloads)
		require.Equal(t, "spec.other", child.Spec.WorkloadRefPath)
	})

	t.Run("a bool is OR'd and cannot be turned off", func(t *testing.T) {
		tmpl := &Template{}
		child := traitDef("tenant-gateway", "gateway", "$super: properties: {}")
		child.Spec.PodDisruptive = false

		require.NoError(t, resolveTraitChain(context.Background(), tmpl, child,
			traitFetcherOver(map[string]*v1beta1.TraitDefinition{"gateway": parent})))

		require.True(t, child.Spec.PodDisruptive, "OR'd with the parent's, which is set")
	})
}

// A parent's status CUE is carried rather than folded in, so a trait's health
// policy composes with the one it extends at evaluation time.
func TestTraitAncestorStatusIsCarried(t *testing.T) {
	withInheritance(t)

	parent := traitDef("gateway", "", "patch: {}")
	parent.Spec.Status = &common.Status{
		HealthPolicy: "isHealth: true",
		CustomStatus: `message: "from the gateway"`,
	}

	tmpl := &Template{}
	require.NoError(t, resolveTraitChain(context.Background(), tmpl,
		traitDef("tenant-gateway", "gateway", "$super: properties: {}"),
		traitFetcherOver(map[string]*v1beta1.TraitDefinition{"gateway": parent})))

	require.Len(t, tmpl.AncestorStatus, 1)
	require.Equal(t, "isHealth: true", tmpl.AncestorStatus[0].Health)
	require.Equal(t, `message: "from the gateway"`, tmpl.AncestorStatus[0].Custom)
}

// The two exported helpers admission and the definition controllers call.
func TestAncestorsHelpersResolveFromTheCluster(t *testing.T) {
	withInheritance(t)

	t.Run("component", func(t *testing.T) {
		child := definitionIn("team-a", "child", "base", "$super: properties: {}")
		cli := fakeClientWith(
			definitionIn("team-a", "base", "", "output: {}"),
			child,
		).Build()

		got, err := ComponentAncestors(context.Background(), cli, child)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "base", got[0].Name)
	})

	t.Run("trait", func(t *testing.T) {
		child := traitIn("team-a", "child", "base", "$super: properties: {}")
		cli := traitClient(
			traitIn("team-a", "base", "", "patch: {}"),
			child,
		).Build()

		got, err := TraitAncestors(context.Background(), cli, child)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "base", got[0].Name)
	})

	t.Run("a parent that is not there is reported", func(t *testing.T) {
		child := traitIn("team-a", "orphan", "gone", "$super: properties: {}")

		_, err := TraitAncestors(context.Background(), traitClient(child).Build(), child)
		require.Error(t, err)
	})
}
