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

package common

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

func TestInstallComponentDefinition(t *testing.T) {
	s := runtime.NewScheme()
	assert.NoError(t, v1beta1.AddToScheme(s))

	validComponentData := []byte(`
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: test-component
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
`)

	existingComponent := &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-component",
			Namespace: types.DefaultKubeVelaNS,
		},
	}

	t.Run("success", func(t *testing.T) {
		k8sClient := fake.NewClientBuilder().WithScheme(s).Build()
		var out bytes.Buffer
		ioStreams := cmdutil.IOStreams{In: os.Stdin, Out: &out, ErrOut: &out}

		err := InstallComponentDefinition(k8sClient, validComponentData, ioStreams)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), "Installing component: test-component")

		var cd v1beta1.ComponentDefinition
		err = k8sClient.Get(context.Background(), client.ObjectKey{Name: "test-component", Namespace: types.DefaultKubeVelaNS}, &cd)
		assert.NoError(t, err)
		assert.Equal(t, "test-component", cd.Name)
	})

	t.Run("already exists", func(t *testing.T) {
		k8sClient := fake.NewClientBuilder().WithScheme(s).WithObjects(existingComponent).Build()
		var out bytes.Buffer
		ioStreams := cmdutil.IOStreams{In: os.Stdin, Out: &out, ErrOut: &out}

		err := InstallComponentDefinition(k8sClient, validComponentData, ioStreams)
		assert.NoError(t, err)
	})

	t.Run("client error", func(t *testing.T) {
		k8sClient := fake.NewClientBuilder().WithScheme(s).WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				return errors.New("client create error")
			},
		}).Build()
		var out bytes.Buffer
		ioStreams := cmdutil.IOStreams{In: os.Stdin, Out: &out, ErrOut: &out}

		err := InstallComponentDefinition(k8sClient, validComponentData, ioStreams)
		assert.Error(t, err)
		assert.EqualError(t, err, "client create error")
	})

	t.Run("invalid data", func(t *testing.T) {
		k8sClient := fake.NewClientBuilder().WithScheme(s).Build()
		var out bytes.Buffer
		ioStreams := cmdutil.IOStreams{In: os.Stdin, Out: &out, ErrOut: &out}
		invalidData := []byte(`{"invalid": "yaml"`)

		err := InstallComponentDefinition(k8sClient, invalidData, ioStreams)
		assert.Error(t, err)
	})

	t.Run("nil data", func(t *testing.T) {
		k8sClient := fake.NewClientBuilder().WithScheme(s).Build()
		var out bytes.Buffer
		ioStreams := cmdutil.IOStreams{In: os.Stdin, Out: &out, ErrOut: &out}

		err := InstallComponentDefinition(k8sClient, nil, ioStreams)
		assert.Error(t, err)
		assert.Equal(t, "componentData is nil", err.Error())
	})
}

func TestInstallTraitDefinition(t *testing.T) {
	s := runtime.NewScheme()
	v1beta1.AddToScheme(s)

	validTraitData := []byte(`
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: test-trait
spec:
  appliesToWorkloads:
    - deployments.apps
`)

	existingTrait := &v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-trait",
			Namespace: types.DefaultKubeVelaNS,
		},
	}

	t.Run("success", func(t *testing.T) {
		k8sClient := fake.NewClientBuilder().WithScheme(s).Build()
		var out bytes.Buffer
		ioStreams := cmdutil.IOStreams{In: os.Stdin, Out: &out, ErrOut: &out}

		err := InstallTraitDefinition(k8sClient, validTraitData, ioStreams)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), "Installing trait test-trait")

		var td v1beta1.TraitDefinition
		err = k8sClient.Get(context.Background(), client.ObjectKey{Name: "test-trait", Namespace: types.DefaultKubeVelaNS}, &td)
		assert.NoError(t, err)
		assert.Equal(t, "test-trait", td.Name)
	})

	t.Run("already exists", func(t *testing.T) {
		k8sClient := fake.NewClientBuilder().WithScheme(s).WithObjects(existingTrait).Build()
		var out bytes.Buffer
		ioStreams := cmdutil.IOStreams{In: os.Stdin, Out: &out, ErrOut: &out}

		err := InstallTraitDefinition(k8sClient, validTraitData, ioStreams)
		assert.NoError(t, err)
	})

	t.Run("client error", func(t *testing.T) {
		k8sClient := fake.NewClientBuilder().WithScheme(s).WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				return errors.New("client create error")
			},
		}).Build()
		var out bytes.Buffer
		ioStreams := cmdutil.IOStreams{In: os.Stdin, Out: &out, ErrOut: &out}

		err := InstallTraitDefinition(k8sClient, validTraitData, ioStreams)
		assert.Error(t, err)
		assert.Equal(t, "client create error", err.Error())
	})

	t.Run("invalid data", func(t *testing.T) {
		k8sClient := fake.NewClientBuilder().WithScheme(s).Build()
		var out bytes.Buffer
		ioStreams := cmdutil.IOStreams{In: os.Stdin, Out: &out, ErrOut: &out}
		invalidData := []byte(`{"invalid": "yaml"`)

		err := InstallTraitDefinition(k8sClient, invalidData, ioStreams)
		assert.Error(t, err)
	})
}

func TestAddSourceIntoDefinition(t *testing.T) {
	caseJson := []byte(`{"template":""}`)
	wantJson := []byte(`{"source":{"repoName":"foo"},"template":""}`)
	source := types.Source{RepoName: "foo"}
	testcase := runtime.RawExtension{Raw: caseJson}
	err := addSourceIntoExtension(&testcase, &source)
	if err != nil {
		t.Error("meet an error ", err)
		return
	}
	var result, want map[string]interface{}
	err = json.Unmarshal(testcase.Raw, &result)
	if err != nil {
		t.Error("marshaling object meet an error ", err)
		return
	}
	err = json.Unmarshal(wantJson, &want)
	if err != nil {
		t.Error("marshaling object meet an error ", err)
		return
	}
	if !reflect.DeepEqual(result, want) {
		t.Errorf("error result want %s, got %s", result, testcase)
	}
}

func TestCheckLabelExistence(t *testing.T) {
	cases := map[string]struct {
		labels  map[string]string
		label   string
		existed bool
	}{
		"label exists": {
			labels: map[string]string{
				"env": "prod",
			},
			label:   "env=prod",
			existed: true,
		},

		"label's key matches": {
			labels: map[string]string{
				"env": "prod",
			},
			label:   "env=dev",
			existed: false,
		},
		"label's key doesn't match": {
			labels: map[string]string{
				"env": "prod",
			},
			label:   "type=terraform",
			existed: false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			result := CheckLabelExistence(tc.labels, tc.label)
			assert.Equal(t, result, tc.existed)
		})
	}
}
