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
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/definition/health"
	"github.com/oam-dev/kubevela/pkg/definition/inherit"
	"github.com/oam-dev/kubevela/pkg/features"
)

func compDef(name, extends, template string) *v1beta1.ComponentDefinition {
	return &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1beta1.ComponentDefinitionSpec{
			Extends: extends,
			Schematic: &common.Schematic{
				CUE: &common.CUE{Template: template},
			},
		},
	}
}

// fetcherOver serves definitions from a map and records what was asked for.
func fetcherOver(defs map[string]*v1beta1.ComponentDefinition) componentFetcher {
	return func(_ context.Context, name string) (*v1beta1.ComponentDefinition, error) {
		cd, ok := defs[name]
		if !ok {
			return nil, fmt.Errorf("component definition %s not found", name)
		}
		return cd.DeepCopy(), nil
	}
}

func withInheritance(t *testing.T) {
	t.Helper()
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableDefinitionInheritance, true)
}

// Off by default, and refused rather than ignored: ignoring the field would
// render the child's template with an unresolved `$super` and fail somewhere
// that says nothing about why.
func TestExtendsIsRefusedWhileTheGateIsOff(t *testing.T) {
	// Pinned rather than assumed: relying on the process-wide default means the
	// test passes for the wrong reason the moment anything else turns it on.
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate,
		features.EnableDefinitionInheritance, false)

	tmpl := &Template{}
	err := resolveComponentChain(context.Background(), tmpl,
		compDef("tenant-webservice", "webservice", ""),
		fetcherOver(map[string]*v1beta1.ComponentDefinition{"webservice": compDef("webservice", "", "output: {}")}))

	require.ErrorContains(t, err, "definition inheritance is disabled")
	require.ErrorContains(t, err, "EnableDefinitionInheritance")
}

func TestChainIsResolvedAndRecorded(t *testing.T) {
	withInheritance(t)

	defs := map[string]*v1beta1.ComponentDefinition{
		"tiered-webservice": compDef("tiered-webservice", "webservice", "output: tier: true"),
		"webservice":        compDef("webservice", "", "output: base: true"),
	}
	tmpl := &Template{}
	require.NoError(t, resolveComponentChain(context.Background(), tmpl,
		compDef("tenant-webservice", "tiered-webservice", "output: tenant: true"), fetcherOver(defs)))

	require.Len(t, tmpl.Ancestors, 2)
	require.Equal(t, "tiered-webservice", tmpl.Ancestors[0].Name, "nearest parent first")
	require.Equal(t, "webservice", tmpl.Ancestors[1].Name)
	require.Equal(t, "output: base: true", tmpl.Ancestors[1].Template)

	// Both are recorded as objects, which is what reaches the ApplicationRevision.
	require.Len(t, tmpl.AncestorComponentDefinitions, 2)
	require.Contains(t, tmpl.AncestorComponentDefinitions, "tiered-webservice")
	require.Contains(t, tmpl.AncestorComponentDefinitions, "webservice")
}

// A pinned parent is keyed by the name that was written, so it cannot collide
// with a sibling component using the same definition unpinned.
func TestPinnedAncestorKeepsItsOwnKey(t *testing.T) {
	withInheritance(t)

	defs := map[string]*v1beta1.ComponentDefinition{
		"webservice@v3": compDef("webservice", "", "output: pinned: true"),
	}
	tmpl := &Template{}
	require.NoError(t, resolveComponentChain(context.Background(), tmpl,
		compDef("tenant-webservice", "webservice@v3", ""), fetcherOver(defs)))

	require.Contains(t, tmpl.AncestorComponentDefinitions, "webservice@v3")
	require.NotContains(t, tmpl.AncestorComponentDefinitions, "webservice")
}

func TestCycleIsNamedRatherThanLooped(t *testing.T) {
	withInheritance(t)

	defs := map[string]*v1beta1.ComponentDefinition{
		"a": compDef("a", "b", "output: {}"),
		"b": compDef("b", "a", "output: {}"),
	}
	err := resolveComponentChain(context.Background(), &Template{}, defs["a"].DeepCopy(), fetcherOver(defs))
	require.ErrorContains(t, err, "already in its own inheritance chain")
	require.ErrorContains(t, err, "a -> b -> a")
}

// Pinning a revision does not disguise a cycle.
func TestPinnedCycleIsStillACycle(t *testing.T) {
	withInheritance(t)

	defs := map[string]*v1beta1.ComponentDefinition{
		"a":    compDef("a", "b", "output: {}"),
		"b":    compDef("b", "a@v2", "output: {}"),
		"a@v2": compDef("a", "b", "output: {}"),
	}
	err := resolveComponentChain(context.Background(), &Template{}, defs["a"].DeepCopy(), fetcherOver(defs))
	require.ErrorContains(t, err, "already in its own inheritance chain")
}

func TestDepthIsCapped(t *testing.T) {
	withInheritance(t)

	defs := map[string]*v1beta1.ComponentDefinition{}
	for i := 0; i <= MaxInheritanceDepth+2; i++ {
		defs[fmt.Sprintf("d%d", i)] = compDef(fmt.Sprintf("d%d", i), fmt.Sprintf("d%d", i+1), "output: {}")
	}
	err := resolveComponentChain(context.Background(), &Template{}, defs["d0"].DeepCopy(), fetcherOver(defs))
	require.ErrorContains(t, err, fmt.Sprintf("deeper than %d", MaxInheritanceDepth))
}

func TestExtendingANonCUEDefinitionIsRefused(t *testing.T) {
	withInheritance(t)

	terraform := compDef("cloud-resource", "", "")
	terraform.Spec.Schematic = &common.Schematic{Terraform: &common.Terraform{}}

	err := resolveComponentChain(context.Background(), &Template{},
		compDef("my-bucket", "cloud-resource", ""),
		fetcherOver(map[string]*v1beta1.ComponentDefinition{"cloud-resource": terraform}))
	require.ErrorContains(t, err, "has no CUE template")
}

// The template is composed by rendering; the rest of the spec describes the
// workload and is inherited only where the child said nothing.
func TestAttributesAreInheritedOnlyWhereTheChildIsSilent(t *testing.T) {
	withInheritance(t)

	parent := compDef("webservice", "", "output: {}")
	parent.Spec.Workload = common.WorkloadTypeDescriptor{
		Definition: common.WorkloadGVK{APIVersion: "apps/v1", Kind: "Deployment"},
	}
	parent.Spec.PodSpecPath = "spec.template.spec"
	parent.Spec.Status = &common.Status{
		HealthPolicy: "isHealth: true",
		CustomStatus: "message: \"from parent\"",
	}

	t.Run("silent child inherits", func(t *testing.T) {
		tmpl := &Template{}
		child := compDef("tenant-webservice", "webservice", "")
		require.NoError(t, resolveComponentChain(context.Background(), tmpl, child,
			fetcherOver(map[string]*v1beta1.ComponentDefinition{"webservice": parent})))

		require.Equal(t, "Deployment", child.Spec.Workload.Definition.Kind)
		require.Equal(t, "Deployment", tmpl.Reference.Definition.Kind)
		require.Equal(t, "spec.template.spec", child.Spec.PodSpecPath)
	})

	t.Run("a child that speaks keeps its own", func(t *testing.T) {
		tmpl := &Template{}
		child := compDef("tenant-webservice", "webservice", "")
		child.Spec.PodSpecPath = "spec.jobTemplate.spec.template.spec"
		require.NoError(t, resolveComponentChain(context.Background(), tmpl, child,
			fetcherOver(map[string]*v1beta1.ComponentDefinition{"webservice": parent})))

		require.Equal(t, "spec.jobTemplate.spec.template.spec", child.Spec.PodSpecPath)
	})
}

// Status is carried rather than folded into the child, so a health policy can
// compose with the one it extends at evaluation time.
func TestAncestorStatusIsCarriedNotFolded(t *testing.T) {
	withInheritance(t)

	parent := compDef("webservice", "", "output: {}")
	parent.Spec.Status = &common.Status{
		HealthPolicy: "isHealth: true",
		CustomStatus: "message: \"from parent\"",
	}

	tmpl := &Template{Health: "isHealth: false"}
	child := compDef("tenant-webservice", "webservice", "")
	require.NoError(t, resolveComponentChain(context.Background(), tmpl, child,
		fetcherOver(map[string]*v1beta1.ComponentDefinition{"webservice": parent})))

	require.Equal(t, "isHealth: false", tmpl.Health, "the child's own snippet is untouched")
	require.Len(t, tmpl.AncestorStatus, 1)
	require.Equal(t, "isHealth: true", tmpl.AncestorStatus[0].Health)
	require.Equal(t, "message: \"from parent\"", tmpl.AncestorStatus[0].Custom)

	req := tmpl.AsStatusRequest(nil)
	require.Len(t, req.Ancestors, 1, "and reaches the status request that evaluates it")
}

// A revision resolves from itself and never from the cluster, or replaying it
// would pick up whatever the parent has since become.
func TestRevisionFetcherDoesNotFallBackToTheCluster(t *testing.T) {
	apprev := &v1beta1.ApplicationRevision{
		ObjectMeta: metav1.ObjectMeta{Name: "acme-billing-v1"},
		Spec: v1beta1.ApplicationRevisionSpec{
			ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
				ComponentDefinitions: map[string]*v1beta1.ComponentDefinition{
					"webservice": compDef("webservice", "", "output: recorded: true"),
				},
			},
		},
	}

	got, err := revisionComponentFetcher(apprev)(context.Background(), "webservice")
	require.NoError(t, err)
	require.Equal(t, "output: recorded: true", got.Spec.Schematic.CUE.Template)

	_, err = revisionComponentFetcher(apprev)(context.Background(), "not-recorded")
	require.ErrorContains(t, err, "not found in app revision")
	require.ErrorContains(t, err, "ancestors were not recorded")
}

// The undeclared-parameter check reads a component's declaration on its own. A
// child inherits half of its, so judging it by its own template alone would
// call every parameter it takes from its parent undeclared.
func TestInheritedParametersAreNotUndeclared(t *testing.T) {
	withInheritance(t)

	parent := `
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	spec: replicas: parameter.replicas
}

parameter: {
	image:    string
	replicas: *1 | int
}
`
	child := `
$super: properties: {image: parameter.image}

parameter: $super.parameter & {
	tenant: string
}
`

	wl := &Component{
		Name: "tenant-app",
		FullTemplate: &Template{
			TemplateStr: child,
			Ancestors:   []inherit.Level{{Name: "webservice", Template: parent}},
		},
	}

	schema, err := componentParameterSchema(context.Background(), wl, child, "context: _")
	require.NoError(t, err)

	declared := getDeclaredFieldNames(schema)
	require.True(t, declared["tenant"], "the child's own")
	require.True(t, declared["image"], "and its parent's, or every app using it is refused")
	require.True(t, declared["replicas"])

	params := map[string]any{"image": "nginx:1.27", "tenant": "acme"}
	require.Empty(t, findUndeclaredFields(schema, params, ""))
}

// A parent's status CUE goes through the same compatibility upgrade the child's
// does. It was written against whatever CUE shipped when it was authored, and
// inheriting it must not mean inheriting a syntax error.
func TestAncestorStatusIsUpgradedToo(t *testing.T) {
	legacy := `baseTags: ["a"]
extraTags: ["b"]
allTags: baseTags + extraTags
isHealth: len(allTags) == 2
`

	tmpl := &Template{
		Health: legacy,
		AncestorStatus: []health.Snippets{
			{Health: legacy, Custom: legacy, Details: legacy},
		},
	}

	req := tmpl.AsStatusRequest(nil)

	require.NotContains(t, req.Health, "baseTags + extraTags", "the child's own is upgraded")
	require.Len(t, req.Ancestors, 1)
	require.NotContains(t, req.Ancestors[0].Health, "baseTags + extraTags",
		"and so is what it inherits, or a legacy parent breaks its children")
	require.NotContains(t, req.Ancestors[0].Custom, "baseTags + extraTags")
	require.NotContains(t, req.Ancestors[0].Details, "baseTags + extraTags")
}
