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

package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velacommon "github.com/oam-dev/kubevela/pkg/utils/common"
)

// nowhereNamespace is a namespace nobody is realistically working in, so a
// definition restricted to it is unusable whichever namespace the test process
// resolves to.
const nowhereNamespace = "no-such-namespace-for-tests"

func schematic() *oamcommon.Schematic {
	return &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: `output: {}`}}
}

func restrictedTo(ns string) *oamcommon.DefinitionRestrictions {
	return &oamcommon.DefinitionRestrictions{Namespaces: []string{ns}}
}

func usableCompDef(name string, restrictions *oamcommon.DefinitionRestrictions) *v1beta1.ComponentDefinition {
	return &v1beta1.ComponentDefinition{
		TypeMeta:   metav1.TypeMeta{Kind: "ComponentDefinition", APIVersion: "core.oam.dev/v1beta1"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "vela-system"},
		Spec: v1beta1.ComponentDefinitionSpec{
			// A workload type rather than a GVK, so parsing needs no RESTMapper.
			Workload:     oamcommon.WorkloadTypeDescriptor{Type: "deployments.apps"},
			Schematic:    schematic(),
			Restrictions: restrictions,
		},
	}
}

func usableTraitDef(name string, restrictions *oamcommon.DefinitionRestrictions) *v1beta1.TraitDefinition {
	return &v1beta1.TraitDefinition{
		TypeMeta:   metav1.TypeMeta{Kind: "TraitDefinition", APIVersion: "core.oam.dev/v1beta1"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "vela-system"},
		Spec: v1beta1.TraitDefinitionSpec{
			Schematic:    schematic(),
			Restrictions: restrictions,
		},
	}
}

func listingArgs(t *testing.T, objs ...runtime.Object) (velacommon.Args, *bytes.Buffer) {
	t.Helper()
	sc := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(sc))
	builder := fake.NewClientBuilder().WithScheme(sc)
	for _, o := range objs {
		builder = builder.WithRuntimeObjects(o)
	}
	args := velacommon.Args{}
	args.SetClient(builder.Build())
	return args, &bytes.Buffer{}
}

// vela components must not offer a component type the current namespace cannot
// use.
func TestPrintInstalledCompDefOmitsUnusable(t *testing.T) {
	args, out := listingArgs(t,
		usableCompDef("open-comp", nil),
		usableCompDef("closed-comp", restrictedTo(nowhereNamespace)),
	)

	require.NoError(t, PrintInstalledCompDef(args, cmdutil.IOStreams{Out: out, ErrOut: out}, nil))
	assert.Contains(t, out.String(), "open-comp")
	assert.NotContains(t, out.String(), "closed-comp")
}

// vela traits, the same.
func TestPrintInstalledTraitDefOmitsUnusable(t *testing.T) {
	args, out := listingArgs(t,
		usableTraitDef("open-trait", nil),
		usableTraitDef("closed-trait", restrictedTo(nowhereNamespace)),
	)

	require.NoError(t, PrintInstalledTraitDef(args, cmdutil.IOStreams{Out: out, ErrOut: out}, nil))
	assert.Contains(t, out.String(), "open-trait")
	assert.NotContains(t, out.String(), "closed-trait")
}

// Where nothing declares a restriction, both listings are unchanged.
func TestListingsUnchangedWithoutRestrictions(t *testing.T) {
	args, out := listingArgs(t,
		usableCompDef("plain-comp", nil),
	)
	require.NoError(t, PrintInstalledCompDef(args, cmdutil.IOStreams{Out: out, ErrOut: out}, nil))
	assert.Contains(t, out.String(), "plain-comp")

	args, out = listingArgs(t,
		usableTraitDef("plain-trait", nil),
	)
	require.NoError(t, PrintInstalledTraitDef(args, cmdutil.IOStreams{Out: out, ErrOut: out}, nil))
	assert.Contains(t, out.String(), "plain-trait")
}
