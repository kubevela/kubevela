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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/module/naming"
	"github.com/oam-dev/kubevela/pkg/oam"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

// These tests drive ResolveModuleType against a fake API server. The
// parse-only cases live in type_resolution_test.go.

const appNS = "team-a"

// stampedLabels are what the module render service stamps on every definition
// it installs for module/apiVersion/name.
func stampedLabels(module, apiVersion, name string) map[string]string {
	return map[string]string{
		types.LabelDefinitionModule:           module,
		types.LabelDefinitionModuleAPIVersion: apiVersion,
		types.LabelDefinitionName:             name,
	}
}

// definitionOf returns a definition object of the kind capType resolves, so
// one table can cover every list and get branch.
func definitionOf(capType types.CapType, ns, name string, labels map[string]string) client.Object {
	meta := metav1.ObjectMeta{Name: name, Namespace: ns, Labels: labels}
	switch capType {
	case types.TypeTrait:
		return &v1beta1.TraitDefinition{ObjectMeta: meta}
	case types.TypePolicy:
		return &v1beta1.PolicyDefinition{ObjectMeta: meta}
	case types.TypeWorkflowStep:
		return &v1beta1.WorkflowStepDefinition{ObjectMeta: meta}
	case types.TypeWorkload:
		return &v1beta1.WorkloadDefinition{ObjectMeta: meta}
	case types.TypeSource:
		return &v1beta1.SourceDefinition{ObjectMeta: meta}
	default:
		return &v1beta1.ComponentDefinition{ObjectMeta: meta}
	}
}

func resolutionClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func appCtx() context.Context {
	return oamutil.SetNamespaceInCtx(context.Background(), appNS)
}

// faultyReader fails the calls a test says to fail and delegates the rest.
type faultyReader struct {
	client.Reader
	getErr error
	// listErr is consulted per List call with its options, so a test can fail
	// only the namespaced listings or only the cluster-wide one.
	listErr func(opts []client.ListOption) error
}

func (f faultyReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if f.getErr != nil {
		return f.getErr
	}
	return f.Reader.Get(ctx, key, obj, opts...)
}

func (f faultyReader) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if f.listErr != nil {
		if err := f.listErr(opts); err != nil {
			return err
		}
	}
	return f.Reader.List(ctx, list, opts...)
}

func isNamespaced(opts []client.ListOption) bool {
	for _, o := range opts {
		if _, ok := o.(client.InNamespace); ok {
			return true
		}
	}
	return false
}

func TestParseTypeRef_Form3InvalidAPIVersion(t *testing.T) {
	_, _, _, _, err := parseTypeRef("s3/latest/bucket")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"latest" is not a valid API version`)
}

func TestResolveModuleTypeRejectsAnUnparsableReference(t *testing.T) {
	_, err := ResolveModuleType(appCtx(), resolutionClient(t), "a/b/c/d", types.TypeComponentDefinition)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "3 segments")
}

func TestResolveModuleTypeForm3IsPureStringMath(t *testing.T) {
	// No definition exists; Form 3 does not look.
	got, err := ResolveModuleType(appCtx(), resolutionClient(t), "s3/v1/bucket", types.TypeComponentDefinition)
	require.NoError(t, err)
	assert.Equal(t, naming.DefinitionName("s3", "v1", "bucket"), got)
}

func TestResolveModuleTypeForm2(t *testing.T) {
	capType := types.TypeComponentDefinition

	t.Run("one match in the app namespace", func(t *testing.T) {
		cli := resolutionClient(t, definitionOf(capType, appNS, "s3-v1-bucket", stampedLabels("s3", "v1", "bucket")))
		got, err := ResolveModuleType(appCtx(), cli, "v1/bucket", capType)
		require.NoError(t, err)
		assert.Equal(t, "s3-v1-bucket", got)
	})

	t.Run("one match in vela-system", func(t *testing.T) {
		cli := resolutionClient(t, definitionOf(capType, oam.SystemDefinitionNamespace, "s3-v1-bucket", stampedLabels("s3", "v1", "bucket")))
		got, err := ResolveModuleType(appCtx(), cli, "v1/bucket", capType)
		require.NoError(t, err)
		assert.Equal(t, "s3-v1-bucket", got)
	})

	t.Run("the API line is part of the match", func(t *testing.T) {
		cli := resolutionClient(t,
			definitionOf(capType, appNS, "s3-v1-bucket", stampedLabels("s3", "v1", "bucket")),
			definitionOf(capType, appNS, "s3-v2-bucket", stampedLabels("s3", "v2", "bucket")),
		)
		got, err := ResolveModuleType(appCtx(), cli, "v2/bucket", capType)
		require.NoError(t, err)
		assert.Equal(t, "s3-v2-bucket", got, "two API lines of one name are not ambiguous when the line is given")
	})

	t.Run("no match", func(t *testing.T) {
		_, err := ResolveModuleType(appCtx(), resolutionClient(t), "v1/bucket", capType)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `no definition found for type "v1/bucket"`)
	})

	t.Run("ambiguous across modules", func(t *testing.T) {
		cli := resolutionClient(t,
			definitionOf(capType, appNS, "s3-v1-bucket", stampedLabels("s3", "v1", "bucket")),
			definitionOf(capType, oam.SystemDefinitionNamespace, "gcs-v1-bucket", stampedLabels("gcs", "v1", "bucket")),
		)
		_, err := ResolveModuleType(appCtx(), cli, "v1/bucket", capType)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `type "v1/bucket" is ambiguous`)
		assert.Contains(t, err.Error(), "s3")
		assert.Contains(t, err.Error(), "gcs")
		assert.Contains(t, err.Error(), "use a fully qualified type ({module}/{apiVersion}/{name})")
	})

	t.Run("listing fails", func(t *testing.T) {
		boom := errors.New("apiserver unavailable")
		cli := faultyReader{Reader: resolutionClient(t), listErr: func([]client.ListOption) error { return boom }}
		_, err := ResolveModuleType(appCtx(), cli, "v1/bucket", capType)
		require.ErrorIs(t, err, boom)
		assert.Contains(t, err.Error(), `resolving type "v1/bucket"`)
	})

	t.Run("unsupported capability type", func(t *testing.T) {
		_, err := ResolveModuleType(appCtx(), resolutionClient(t), "v1/bucket", types.CapType("scope"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `unsupported capType "scope"`)
	})
}

func TestResolveModuleTypeForm1(t *testing.T) {
	capType := types.TypeComponentDefinition

	t.Run("a legacy definition wins and is returned unchanged", func(t *testing.T) {
		cli := resolutionClient(t,
			definitionOf(capType, oam.SystemDefinitionNamespace, "bucket", nil),
			// A module also installs a "bucket"; the plain name must keep
			// meaning what it meant before modules existed.
			definitionOf(capType, appNS, "s3-v1-bucket", stampedLabels("s3", "v1", "bucket")),
		)
		got, err := ResolveModuleType(appCtx(), cli, "bucket", capType)
		require.NoError(t, err)
		assert.Equal(t, "bucket", got)
	})

	t.Run("falls back to the module label", func(t *testing.T) {
		cli := resolutionClient(t, definitionOf(capType, appNS, "s3-v1-bucket", stampedLabels("s3", "v1", "bucket")))
		got, err := ResolveModuleType(appCtx(), cli, "bucket", capType)
		require.NoError(t, err)
		assert.Equal(t, "s3-v1-bucket", got)
	})

	t.Run("no definition at all returns the name for the caller to report", func(t *testing.T) {
		got, err := ResolveModuleType(appCtx(), resolutionClient(t), "bucket", capType)
		require.NoError(t, err)
		assert.Equal(t, "bucket", got, "the caller's own lookup reports the authoritative not-found")
	})

	t.Run("ambiguous across modules", func(t *testing.T) {
		cli := resolutionClient(t,
			definitionOf(capType, appNS, "s3-v1-bucket", stampedLabels("s3", "v1", "bucket")),
			definitionOf(capType, appNS, "gcs-v1-bucket", stampedLabels("gcs", "v1", "bucket")),
		)
		_, err := ResolveModuleType(appCtx(), cli, "bucket", capType)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `type "bucket" is ambiguous`)
		assert.Contains(t, err.Error(), "use {apiVersion}/{name} (Form 2) or {module}/{apiVersion}/{name} (Form 3)")
	})

	t.Run("a failed get other than not-found is reported", func(t *testing.T) {
		boom := errors.New("apiserver unavailable")
		cli := faultyReader{Reader: resolutionClient(t), getErr: boom}
		_, err := ResolveModuleType(appCtx(), cli, "bucket", capType)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("a failed label search is not an error", func(t *testing.T) {
		cli := faultyReader{Reader: resolutionClient(t), listErr: func([]client.ListOption) error { return errors.New("list refused") }}
		got, err := ResolveModuleType(appCtx(), cli, "bucket", capType)
		require.NoError(t, err, "a bare name resolved fine before modules existed; the search is best-effort")
		assert.Equal(t, "bucket", got)
	})
}

// TestResolveModuleTypeCoversEveryDefinitionKind runs the Form 1 legacy get
// and the Form 2 label listing for each capability type, so the per-kind
// branches of definitionObjectFor and listModuleDefinitions are all taken.
func TestResolveModuleTypeCoversEveryDefinitionKind(t *testing.T) {
	for _, capType := range []types.CapType{
		types.TypeComponentDefinition, types.TypeWorkload, types.TypeTrait,
		types.TypePolicy, types.TypeWorkflowStep, types.TypeSource,
	} {
		t.Run(string(capType), func(t *testing.T) {
			cli := resolutionClient(t,
				definitionOf(capType, oam.SystemDefinitionNamespace, "legacy", nil),
				definitionOf(capType, appNS, "kit-v1-thing", stampedLabels("kit", "v1", "thing")),
			)

			got, err := ResolveModuleType(appCtx(), cli, "legacy", capType)
			require.NoError(t, err)
			assert.Equal(t, "legacy", got)

			got, err = ResolveModuleType(appCtx(), cli, "v1/thing", capType)
			require.NoError(t, err)
			assert.Equal(t, "kit-v1-thing", got)

			got, err = ResolveModuleType(appCtx(), cli, "thing", capType)
			require.NoError(t, err)
			assert.Equal(t, "kit-v1-thing", got)

			require.NoError(t, DefinitionExists(appCtx(), cli, "kit-v1-thing", capType))

			boom := errors.New("list refused")
			faulty := faultyReader{Reader: cli, listErr: func([]client.ListOption) error { return boom }}
			_, err = ResolveModuleType(appCtx(), faulty, "v1/thing", capType)
			assert.ErrorIs(t, err, boom, "a failed listing of this kind is reported")
		})
	}
}

func TestListModuleDefinitionsSearchesClusterWideAsALastResort(t *testing.T) {
	capType := types.TypeComponentDefinition
	// Installed into an operator-chosen namespace that is neither the app
	// namespace nor vela-system.
	cli := resolutionClient(t, definitionOf(capType, "platform-modules", "s3-v1-bucket", stampedLabels("s3", "v1", "bucket")))

	got, err := ResolveModuleType(appCtx(), cli, "v1/bucket", capType)
	require.NoError(t, err)
	assert.Equal(t, "s3-v1-bucket", got)

	t.Run("the cluster-wide listing can fail on its own", func(t *testing.T) {
		boom := errors.New("forbidden at cluster scope")
		faulty := faultyReader{Reader: cli, listErr: func(opts []client.ListOption) error {
			if isNamespaced(opts) {
				return nil
			}
			return boom
		}}
		_, err := ResolveModuleType(appCtx(), faulty, "v1/bucket", capType)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("a namespaced hit skips the cluster-wide listing", func(t *testing.T) {
		withLocal := resolutionClient(t,
			definitionOf(capType, appNS, "gcs-v1-bucket", stampedLabels("gcs", "v1", "bucket")),
			definitionOf(capType, "platform-modules", "s3-v1-bucket", stampedLabels("s3", "v1", "bucket")),
		)
		faulty := faultyReader{Reader: withLocal, listErr: func(opts []client.ListOption) error {
			if isNamespaced(opts) {
				return nil
			}
			return errors.New("must not be called")
		}}
		got, err := ResolveModuleType(appCtx(), faulty, "v1/bucket", capType)
		require.NoError(t, err)
		assert.Equal(t, "gcs-v1-bucket", got, "the definition in a searched namespace wins without widening the search")
	})
}

func TestListModuleDefinitionsDeduplicatesTheSystemNamespace(t *testing.T) {
	capType := types.TypeComponentDefinition
	cli := resolutionClient(t, definitionOf(capType, oam.SystemDefinitionNamespace, "s3-v1-bucket", stampedLabels("s3", "v1", "bucket")))

	// No app namespace in the context means the app namespace is vela-system
	// itself; the one definition must be listed once, not reported ambiguous.
	got, err := ResolveModuleType(context.Background(), cli, "v1/bucket", capType)
	require.NoError(t, err)
	assert.Equal(t, "s3-v1-bucket", got)
}

func TestResolveUniqueNamesEachModuleOnce(t *testing.T) {
	matches := []client.Object{
		definitionOf(types.TypeComponentDefinition, appNS, "s3-v1-bucket", stampedLabels("s3", "v1", "bucket")),
		definitionOf(types.TypeComponentDefinition, oam.SystemDefinitionNamespace, "s3-v1-bucket", stampedLabels("s3", "v1", "bucket")),
		// A definition without a module label is named by itself.
		definitionOf(types.TypeComponentDefinition, appNS, "hand-made-bucket", map[string]string{types.LabelDefinitionName: "bucket"}),
	}
	_, err := resolveUnique("bucket", matches, "pick one")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "definitions from modules [s3, hand-made-bucket] all match; pick one",
		"the same module in two namespaces is listed once; an unlabelled definition is listed by name")

	got, err := resolveUnique("bucket", matches[:1], "pick one")
	require.NoError(t, err)
	assert.Equal(t, "s3-v1-bucket", got)

	_, err = resolveUnique("bucket", nil, "pick one")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `no definition found for type "bucket"`)
}

func TestDefinitionExists(t *testing.T) {
	capType := types.TypeTrait
	cli := resolutionClient(t, definitionOf(capType, oam.SystemDefinitionNamespace, "s3-v1-note", stampedLabels("s3", "v1", "note")))

	assert.NoError(t, DefinitionExists(appCtx(), cli, "s3-v1-note", capType))

	err := DefinitionExists(appCtx(), cli, "s3-v1-gauge", capType)
	require.Error(t, err)
	assert.Equal(t, `definition "s3-v1-gauge" not found: ensure the module is installed`, err.Error())

	boom := errors.New("apiserver unavailable")
	err = DefinitionExists(appCtx(), faultyReader{Reader: cli, getErr: boom}, "s3-v1-note", capType)
	assert.ErrorIs(t, err, boom, "only not-found gets the friendly wording")
}
