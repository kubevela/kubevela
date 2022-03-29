/*
Copyright 2022 The KubeVela Authors.

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

package usecase

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	"testing"

	. "github.com/agiledragon/gomonkey/v2"
	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/definition"
	"github.com/oam-dev/kubevela/pkg/multicluster"
)

func TestListConfigTypes(t *testing.T) {
	s := runtime.NewScheme()
	v1beta1.AddToScheme(s)
	corev1.AddToScheme(s)
	def1 := &v1beta1.ComponentDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ComponentDefinition",
			APIVersion: "core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "def1",
			Namespace: types.DefaultKubeVelaNS,
			Labels: map[string]string{
				definition.UserPrefix + "catalog.config.oam.dev": velaCoreConfig,
				definitionType: terraformProviderType,
			},
		},
	}
	def2 := &v1beta1.ComponentDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ComponentDefinition",
			APIVersion: "core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "def2",
			Namespace: types.DefaultKubeVelaNS,
			Annotations: map[string]string{
				definitionAlias: "Def2",
			},
			Labels: map[string]string{
				definition.UserPrefix + "catalog.config.oam.dev": velaCoreConfig,
			},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(s).WithObjects(def1, def2).Build()

	patches := ApplyFunc(multicluster.GetMulticlusterKubernetesClient, func() (client.Client, *rest.Config, error) {
		return k8sClient, nil, nil
	})
	defer patches.Reset()

	h := NewConfigUseCase(nil)

	type args struct {
		h ConfigHandler
	}

	type want struct {
		configTypes []*apis.ConfigType
		errMsg      string
	}

	ctx := context.Background()

	testcases := []struct {
		name string
		args args
		want want
	}{
		{
			name: "success",
			args: args{
				h: h,
			},
			want: want{
				configTypes: []*apis.ConfigType{
					{
						Name:        "def2",
						Alias:       "Def2",
						Definitions: []string{"def2"},
					},
					{
						Alias: "Terraform Cloud Provider",
						Name:  terraformProviderType,
						Definitions: []string{
							"def1",
						},
					},
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.args.h.ListConfigTypes(ctx, "")
			if tc.want.errMsg != "" || err != nil {
				assert.ErrorContains(t, err, tc.want.errMsg)
			}
			assert.DeepEqual(t, got, tc.want.configTypes)
		})
	}
}

func TestGetConfigType(t *testing.T) {
	s := runtime.NewScheme()
	v1beta1.AddToScheme(s)
	corev1.AddToScheme(s)
	def2 := &v1beta1.ComponentDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ComponentDefinition",
			APIVersion: "core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "def2",
			Namespace: types.DefaultKubeVelaNS,
			Annotations: map[string]string{
				definitionAlias: "Def2",
			},
			Labels: map[string]string{
				definition.UserPrefix + "catalog.config.oam.dev": velaCoreConfig,
			},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(s).WithObjects(def2).Build()

	patches := ApplyFunc(multicluster.GetMulticlusterKubernetesClient, func() (client.Client, *rest.Config, error) {
		return k8sClient, nil, nil
	})
	defer patches.Reset()

	h := NewConfigUseCase(nil)

	type args struct {
		h    ConfigHandler
		name string
	}

	type want struct {
		configType *apis.ConfigType
		errMsg     string
	}

	ctx := context.Background()

	testcases := []struct {
		name string
		args args
		want want
	}{
		{
			name: "success",
			args: args{
				h:    h,
				name: "def2",
			},
			want: want{
				configType: &apis.ConfigType{
					Alias: "Def2",
					Name:  "def2",
				},
			},
		},
		{
			name: "error",
			args: args{
				h:    h,
				name: "def99",
			},
			want: want{
				errMsg: "failed to get config type",
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.args.h.GetConfigType(ctx, tc.args.name)
			if tc.want.errMsg != "" || err != nil {
				assert.ErrorContains(t, err, tc.want.errMsg)
			}
			assert.DeepEqual(t, got, tc.want.configType)
		})
	}
}
