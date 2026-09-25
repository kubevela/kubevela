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
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func definitionIn(namespace, name, extends, template string) *v1beta1.ComponentDefinition {
	cd := compDef(name, extends, template)
	cd.Namespace = namespace
	return cd
}

func fakeClientWith(objs ...*v1beta1.ComponentDefinition) *fake.ClientBuilder {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	b := fake.NewClientBuilder().WithScheme(scheme)
	for _, o := range objs {
		b = b.WithObjects(o)
	}
	return b
}

// A parent in the same namespace resolves.
func TestParentInTheSameNamespaceResolves(t *testing.T) {
	withInheritance(t)

	cli := fakeClientWith(
		definitionIn("team-a", "base", "", "output: base: true"),
	).Build()

	child := definitionIn("team-a", "child", "base", "$super: properties: {}")
	got, err := clusterComponentFetcher(cli, "team-a", nil)(context.Background(), "base")
	require.NoError(t, err)
	require.Equal(t, "team-a", got.Namespace)

	tmpl := &Template{}
	require.NoError(t, resolveComponentChain(context.Background(), tmpl, child,
		clusterComponentFetcher(cli, "team-a", nil)))
	require.Len(t, tmpl.Ancestors, 1)
}

// The escalation this closes: a definition in a user's namespace reaching a
// privileged one in vela-system. The lookup finds it, because capability
// resolution falls back to the system namespace, and it is refused by name.
func TestParentInAnotherNamespaceIsRefused(t *testing.T) {
	withInheritance(t)

	cli := fakeClientWith(
		definitionIn("vela-system", "webservice", "", "output: privileged: true"),
	).Build()

	child := definitionIn("team-a", "sneaky", "webservice", "$super: properties: {}")
	err := resolveComponentChain(context.Background(), &Template{}, child,
		clusterComponentFetcher(cli, "team-a", nil))

	require.ErrorContains(t, err, "vela-system")
	require.ErrorContains(t, err, "its own namespace")
}

// A definition in vela-system extending another in vela-system is the supported
// way to build on a builtin, and is unaffected.
func TestSystemNamespaceChainResolves(t *testing.T) {
	withInheritance(t)

	cli := fakeClientWith(
		definitionIn("vela-system", "webservice", "", "output: base: true"),
	).Build()

	child := definitionIn("vela-system", "tenant-webservice", "webservice", "$super: properties: {}")
	tmpl := &Template{}
	require.NoError(t, resolveComponentChain(context.Background(), tmpl, child,
		clusterComponentFetcher(cli, "vela-system", nil)))
	require.Len(t, tmpl.Ancestors, 1)
	require.Equal(t, "webservice", tmpl.Ancestors[0].Name)
}

// A cluster-scoped definition, from an install predating namespaced ones, is
// global and admin-installed. Letting a definition in a team's namespace extend
// it is the same reach across the boundary the check exists to stop, so the
// absence of a namespace is not an exemption.
func TestANamespacedChildCannotExtendAClusterScopedParent(t *testing.T) {
	err := sameNamespace("component", "legacy-base", "", "team-a")
	require.Error(t, err)
	require.Contains(t, err.Error(), "its own namespace")
}

// A definition read from a file carries no namespace of its own and is not being
// read into one either, which is what `vela dry-run` and `vela show` do.
func TestAFileHasNoNamespaceToConfine(t *testing.T) {
	require.NoError(t, sameNamespace("component", "from-a-file", "", ""))
}

// Definitions handed to a dry run are the user's own files, but one that does
// carry a namespace is still held to it: a dry run that admits a chain the
// cluster would refuse is worse than useless.
func TestALocallySuppliedParentIsHeldToItsNamespace(t *testing.T) {
	withInheritance(t)

	elsewhere := definitionIn("vela-system", "base", "", "output: base: true")
	elsewhere.TypeMeta = metav1.TypeMeta{Kind: v1beta1.ComponentDefinitionKind, APIVersion: v1beta1.SchemeGroupVersion.String()}
	local, err := runtime.DefaultUnstructuredConverter.ToUnstructured(elsewhere)
	require.NoError(t, err)

	fetch := localComponentFetcher(
		[]*unstructured.Unstructured{{Object: local}},
		fakeClientWith().Build(), "team-a", nil)

	_, err = fetch(context.Background(), "base")
	require.Error(t, err, "a dry run must refuse what the cluster would refuse")
	require.Contains(t, err.Error(), "its own namespace")
}

// And one with no namespace at all is the ordinary dry-run case, which works.
func TestALocallySuppliedParentWithoutANamespaceResolves(t *testing.T) {
	withInheritance(t)

	fromFile := compDef("base", "", "output: base: true")
	fromFile.TypeMeta = metav1.TypeMeta{Kind: v1beta1.ComponentDefinitionKind, APIVersion: v1beta1.SchemeGroupVersion.String()}
	local, err := runtime.DefaultUnstructuredConverter.ToUnstructured(fromFile)
	require.NoError(t, err)

	fetch := localComponentFetcher(
		[]*unstructured.Unstructured{{Object: local}},
		fakeClientWith().Build(), "team-a", nil)

	got, err := fetch(context.Background(), "base")
	require.NoError(t, err)
	require.Equal(t, "base", got.Name)
}
