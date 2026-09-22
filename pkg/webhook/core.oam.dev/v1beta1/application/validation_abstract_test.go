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
	"testing"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func componentDef(namespace, name, extends string) *v1beta1.ComponentDefinition {
	return &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: v1beta1.ComponentDefinitionSpec{
			Extends:   extends,
			Schematic: &common.Schematic{CUE: &common.CUE{Template: "output: {}"}},
		},
	}
}

func handlerWith(defs ...*v1beta1.ComponentDefinition) *ValidatingHandler {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	b := fake.NewClientBuilder().WithScheme(scheme)
	for _, d := range defs {
		b = b.WithObjects(d)
	}
	return &ValidatingHandler{Client: b.Build()}
}

func abstractDef(namespace, name string) *v1beta1.ComponentDefinition {
	cd := componentDef(namespace, name, "")
	cd.Spec.Abstract = true
	return cd
}

func appNaming(namespace, typ string) *v1beta1.Application {
	return &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: namespace},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{{Name: "c", Type: typ}},
		},
	}
}

func TestAbstractTypeCannotBeNamedDirectly(t *testing.T) {
	h := handlerWith(abstractDef("team-a", "base"))

	errs := h.ValidateAbstractTypes(context.Background(), appNaming("team-a", "base"))
	require.Len(t, errs, 1)
	require.Contains(t, errs[0].Error(), "abstract and cannot be used directly")
	require.Contains(t, errs[0].Error(), "spec.components[0].type")
}

// The whole point of marking one: what extends it is still usable.
func TestExtendingAnAbstractTypeIsAllowed(t *testing.T) {
	h := handlerWith(
		abstractDef("team-a", "base"),
		componentDef("team-a", "paved", "base"),
	)

	require.Empty(t, h.ValidateAbstractTypes(context.Background(), appNaming("team-a", "paved")))
}

// A child is concrete unless it says otherwise, or a chain of them would all be
// unusable the moment its root was marked.
func TestAbstractIsNotInherited(t *testing.T) {
	h := handlerWith(
		abstractDef("team-a", "base"),
		componentDef("team-a", "middle", "base"),
		componentDef("team-a", "leaf", "middle"),
	)

	require.Empty(t, h.ValidateAbstractTypes(context.Background(), appNaming("team-a", "leaf")))
}

// Definitions normally live in vela-system, so that is the second place to look.
func TestAbstractIsFoundInTheSystemNamespace(t *testing.T) {
	h := handlerWith(abstractDef(oam.SystemDefinitionNamespace, "webservice"))

	require.Len(t, h.ValidateAbstractTypes(context.Background(), appNaming("team-a", "webservice")), 1)
}

// A type that does not exist is not abstract. It fails elsewhere, with a message
// that says what is actually wrong.
func TestAnUnknownTypeIsNotAbstract(t *testing.T) {
	h := handlerWith()

	require.Empty(t, h.ValidateAbstractTypes(context.Background(), appNaming("team-a", "nowhere")))
}

func TestAbstractTraitCannotBeNamedDirectly(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	td := &v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "base-gateway", Namespace: "team-a"},
		Spec:       v1beta1.TraitDefinitionSpec{Abstract: true},
	}
	h := &ValidatingHandler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(td).Build()}

	app := appNaming("team-a", "webservice")
	app.Spec.Components[0].Traits = []common.ApplicationTrait{{Type: "base-gateway"}}

	errs := h.ValidateAbstractTypes(context.Background(), app)
	require.Len(t, errs, 1)
	require.Contains(t, errs[0].Error(), "spec.components[0].traits[0].type")
	require.Contains(t, errs[0].Error(), "name a trait that extends it")
}

// A lookup that fails for any reason other than "not there" must not be read as
// "not abstract". A transient API error or an RBAC denial would otherwise admit
// exactly the direct use the marking exists to refuse.
func TestALookupFailureDoesNotAdmitAnAbstractType(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	base := abstractDef("team-a", "base")
	failing := &erroringClient{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(base).Build(),
		err:    apierrors.NewServiceUnavailable("apiserver is having a moment"),
	}
	h := &ValidatingHandler{Client: failing}

	errs := h.ValidateAbstractTypes(context.Background(), appNaming("team-a", "base"))
	require.Len(t, errs, 1, "a failed lookup must not be taken as permission")
	require.Contains(t, errs[0].Error(), "could not be read")
}

// Pinning a revision must not shake the marking off. The revision carries its
// own copy of the spec, so that copy is what has to be judged: the live
// definition may since have been unmarked, or not exist at all.
func TestAPinnedRevisionOfAnAbstractTypeIsStillAbstract(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)

	// base-v3 is abstract; the live `base` is not.
	rev := &v1beta1.DefinitionRevision{
		ObjectMeta: metav1.ObjectMeta{Name: "base-v3", Namespace: "team-a"},
		Spec: v1beta1.DefinitionRevisionSpec{
			Revision:            3,
			DefinitionType:      common.ComponentType,
			ComponentDefinition: *abstractDef("team-a", "base"),
		},
	}
	live := componentDef("team-a", "base", "")

	h := &ValidatingHandler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(rev, live).Build(),
	}

	errs := h.ValidateAbstractTypes(context.Background(), appNaming("team-a", "base@v3"))
	require.Len(t, errs, 1, "the pinned revision is abstract, whatever the live definition says")

	require.Empty(t, h.ValidateAbstractTypes(context.Background(), appNaming("team-a", "base")),
		"and the live one, which is not marked, stays usable")
}

// erroringClient fails every Get, which is what a busy or restrictive API server
// looks like from here.
type erroringClient struct {
	client.Client
	err error
}

func (c *erroringClient) Get(_ context.Context, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
	return c.err
}
