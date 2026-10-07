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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func def(restrictions map[string]interface{}) client.Object {
	spec := map[string]interface{}{}
	if restrictions != nil {
		spec["restrictions"] = restrictions
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{"spec": spec}}
}

// typedDef is the shape vela components and vela traits pass: a typed object,
// not unstructured. Both go through the same helper.
func typedDef(patterns ...string) client.Object {
	return &v1beta1.ComponentDefinition{
		Spec: v1beta1.ComponentDefinitionSpec{
			Restrictions: &oamcommon.DefinitionRestrictions{Namespaces: patterns},
		},
	}
}

func namespaceClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	sc := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(sc))
	return fake.NewClientBuilder().WithScheme(sc).WithObjects(objs...).Build()
}

// The list is what you can build with, so a definition reserved for elsewhere is
// left out of it entirely.
func TestUsabilityCounts(t *testing.T) {
	byName := def(map[string]interface{}{"namespaces": []interface{}{"tenant-*"}})
	defs := []client.Object{def(nil), byName, byName}

	usable, err := judgeUsability(context.Background(), namespaceClient(t), defs, "default")
	require.NoError(t, err)

	kept, hidden := 0, 0
	for _, ok := range usable {
		if ok {
			kept++
		} else {
			hidden++
		}
	}
	assert.Equal(t, 1, kept)
	assert.Equal(t, 2, hidden)

	// From an allowed namespace nothing is left out.
	usable, err = judgeUsability(context.Background(), namespaceClient(t), defs, "tenant-a")
	require.NoError(t, err)
	assert.Equal(t, []bool{true, true, true}, usable)
}

// vela components and vela traits hand typed objects to the same helper that
// vela def list hands unstructured ones, so both shapes have to decide alike.
func TestJudgeUsabilityAcceptsTypedDefinitions(t *testing.T) {
	defs := []client.Object{
		typedDef("tenant-*"),
		def(map[string]interface{}{"namespaces": []interface{}{"tenant-*"}}),
	}
	usable, err := judgeUsability(context.Background(), namespaceClient(t), defs, "default")
	require.NoError(t, err)
	assert.Equal(t, []bool{false, false}, usable, "typed and unstructured must agree")

	usable, err = judgeUsability(context.Background(), namespaceClient(t), defs, "tenant-a")
	require.NoError(t, err)
	assert.Equal(t, []bool{true, true}, usable)
}

func TestJudgeUsability(t *testing.T) {
	unrestricted := def(nil)
	byName := def(map[string]interface{}{"namespaces": []interface{}{"tenant-*"}})
	bySelector := def(map[string]interface{}{
		"namespaceSelector": map[string]interface{}{
			"matchLabels": map[string]interface{}{"tier": "gold"},
		},
	})

	t.Run("names only", func(t *testing.T) {
		usable, err := judgeUsability(context.Background(),
			namespaceClient(t), []client.Object{unrestricted, byName}, "tenant-a")
		require.NoError(t, err)
		assert.Equal(t, []bool{true, true}, usable)
	})

	t.Run("a name that does not match is reported", func(t *testing.T) {
		usable, err := judgeUsability(context.Background(),
			namespaceClient(t), []client.Object{unrestricted, byName}, "default")
		require.NoError(t, err)
		assert.Equal(t, []bool{true, false}, usable)
	})

	t.Run("a selector reads the namespace's labels", func(t *testing.T) {
		c := namespaceClient(t, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "goldilocks", Labels: map[string]string{"tier": "gold"}},
		})
		usable, err := judgeUsability(context.Background(), c,
			[]client.Object{bySelector}, "goldilocks")
		require.NoError(t, err)
		assert.Equal(t, []bool{true}, usable)
	})

	// A namespace that does not exist has no labels, so a selector matches nothing.
	t.Run("a missing namespace is not an error", func(t *testing.T) {
		usable, err := judgeUsability(context.Background(),
			namespaceClient(t), []client.Object{bySelector}, "gone")
		require.NoError(t, err)
		assert.Equal(t, []bool{false}, usable)
	})

	// Restrictions by name alone must not cost a namespace read.
	t.Run("the namespace is read only when a selector needs it", func(t *testing.T) {
		reads := 0
		sc := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(sc))
		counting := fake.NewClientBuilder().WithScheme(sc).WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*corev1.Namespace); ok {
					reads++
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}).Build()

		_, err := judgeUsability(context.Background(), counting,
			[]client.Object{unrestricted, byName}, "default")
		require.NoError(t, err)
		assert.Zero(t, reads, "a name-only restriction should not read the namespace")

		_, err = judgeUsability(context.Background(), counting,
			[]client.Object{bySelector}, "default")
		require.NoError(t, err)
		assert.Equal(t, 1, reads, "a selector should read the namespace exactly once")
	})

	// A real failure has to surface rather than read as "not usable".
	t.Run("a read failure is returned", func(t *testing.T) {
		sc := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(sc))
		broken := fake.NewClientBuilder().WithScheme(sc).WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				return assert.AnError
			},
		}).Build()
		_, err := judgeUsability(context.Background(), broken,
			[]client.Object{bySelector}, "default")
		require.Error(t, err)
	})
}
