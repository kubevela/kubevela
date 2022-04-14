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
	"encoding/json"
	"sort"
	"strings"
	"testing"

	. "github.com/agiledragon/gomonkey/v2"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
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
				definitionType: types.TerraformProvider,
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
						Name:  types.TerraformProvider,
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

func TestCreateConfig(t *testing.T) {
	s := runtime.NewScheme()
	v1beta1.AddToScheme(s)
	corev1.AddToScheme(s)

	k8sClient := fake.NewClientBuilder().WithScheme(s).Build()

	h := &configUseCaseImpl{kubeClient: k8sClient}

	type args struct {
		h   ConfigHandler
		req apis.CreateConfigRequest
	}

	type want struct {
		errMsg string
	}

	ctx := context.Background()

	testcases := []struct {
		name string
		args args
		want want
	}{
		{
			name: "delete config when it's not ready",
			args: args{
				h: h,
				req: apis.CreateConfigRequest{
					Name:          "a",
					ComponentType: "b",
					Project:       "c",
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.args.h.CreateConfig(ctx, tc.args.req)
			if tc.want.errMsg != "" || err != nil {
				assert.ErrorContains(t, err, tc.want.errMsg)
			}
		})
	}
}

func TestGetConfigs(t *testing.T) {
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
				definitionType: types.TerraformProvider,
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

	h := &configUseCaseImpl{kubeClient: k8sClient}

	type args struct {
		configType string
		h          ConfigHandler
	}

	type want struct {
		configs []*apis.Config
		errMsg  string
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
				configType: types.TerraformProvider,
				h:          h,
			},
			want: want{
				configs: nil,
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.args.h.GetConfigs(ctx, tc.args.configType)
			if tc.want.errMsg != "" || err != nil {
				assert.ErrorContains(t, err, tc.want.errMsg)
			}
			assert.DeepEqual(t, got, tc.want.configs)
		})
	}
}

func TestMergeTargets(t *testing.T) {
	currentTargets := []ApplicationDeployTarget{
		{
			Namespace: "n1",
			Clusters:  []string{"c1", "c2"},
		}, {
			Namespace: "n2",
			Clusters:  []string{"c3"},
		},
	}
	targets := []*model.ClusterTarget{
		{
			Namespace:   "n3",
			ClusterName: "c4",
		}, {
			Namespace:   "n1",
			ClusterName: "c5",
		},
		{
			Namespace:   "n2",
			ClusterName: "c3",
		},
	}

	expected := []ApplicationDeployTarget{
		{
			Namespace: "n1",
			Clusters:  []string{"c1", "c2", "c5"},
		}, {
			Namespace: "n2",
			Clusters:  []string{"c3"},
		}, {
			Namespace: "n3",
			Clusters:  []string{"c4"},
		},
	}

	got := mergeTargets(currentTargets, targets)

	for i, g := range got {
		clusters := g.Clusters
		sort.SliceStable(clusters, func(i, j int) bool {
			return clusters[i] < clusters[j]
		})
		got[i].Clusters = clusters
	}
	assert.DeepEqual(t, expected, got)
}

func TestConvert(t *testing.T) {
	targets := []*model.ClusterTarget{
		{
			Namespace:   "n3",
			ClusterName: "c4",
		}, {
			Namespace:   "n1",
			ClusterName: "c5",
		},
		{
			Namespace:   "n2",
			ClusterName: "c3",
		},
		{
			Namespace:   "n3",
			ClusterName: "c5",
		},
	}

	expected := []ApplicationDeployTarget{
		{
			Namespace: "n3",
			Clusters:  []string{"c4", "c5"},
		},
		{
			Namespace: "n1",
			Clusters:  []string{"c5"},
		}, {
			Namespace: "n2",
			Clusters:  []string{"c3"},
		},
	}

	got := convertClusterTargets(targets)

	for i, g := range got {
		clusters := g.Clusters
		sort.SliceStable(clusters, func(i, j int) bool {
			return clusters[i] < clusters[j]
		})
		got[i].Clusters = clusters
	}
	assert.DeepEqual(t, expected, got)
}

func TestDestroySyncConfigsApp(t *testing.T) {
	s := runtime.NewScheme()
	v1beta1.AddToScheme(s)
	corev1.AddToScheme(s)
	app1 := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "config-sync-p1",
			Namespace: types.DefaultKubeVelaNS,
		},
	}
	k8sClient1 := fake.NewClientBuilder().WithScheme(s).WithObjects(app1).Build()

	k8sClient2 := fake.NewClientBuilder().Build()

	type args struct {
		project   string
		k8sClient client.Client
	}

	type want struct {
		errMsg string
	}

	ctx := context.Background()

	testcases := map[string]struct {
		args args
		want want
	}{
		"found": {
			args: args{
				project:   "p1",
				k8sClient: k8sClient1,
			},
		},
		"not found": {
			args: args{
				project:   "p1",
				k8sClient: k8sClient2,
			},
			want: want{
				errMsg: "no kind is registered for the type v1beta1.Application",
			},
		},
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			err := destroySyncConfigsApp(ctx, tc.args.k8sClient, tc.args.project)
			if err != nil || tc.want.errMsg != "" {
				if !strings.Contains(err.Error(), tc.want.errMsg) {
					assert.ErrorContains(t, err, tc.want.errMsg)
				}
			}
		})
	}
}

func TestSyncConfigs(t *testing.T) {
	s := runtime.NewScheme()
	v1beta1.AddToScheme(s)
	corev1.AddToScheme(s)
	secret1 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "s1",
			Namespace: types.DefaultKubeVelaNS,
			Labels: map[string]string{
				types.LabelConfigCatalog:            velaCoreConfig,
				types.LabelConfigProject:            "p1",
				types.LabelConfigSyncToMultiCluster: "true",
			},
		},
	}

	policies := []ApplicationDeployTarget{{
		Namespace: "n9",
		Clusters:  []string{"c19"},
	}}
	properties, _ := json.Marshal(policies)
	app1 := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "config-sync-p2",
			Namespace: types.DefaultKubeVelaNS,
		},
		Spec: v1beta1.ApplicationSpec{
			Policies: []v1beta1.AppPolicy{{
				Name:       "c19",
				Type:       "topology",
				Properties: &runtime.RawExtension{Raw: properties},
			}},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(s).WithObjects(secret1, app1).Build()

	type args struct {
		project string
		targets []*model.ClusterTarget
	}

	type want struct {
		errMsg string
	}

	ctx := context.Background()

	testcases := []struct {
		name string
		args args
		want want
	}{
		{
			name: "create",
			args: args{
				project: "p1",
				targets: []*model.ClusterTarget{{
					ClusterName: "c1",
					Namespace:   "n1",
				}},
			},
		},
		{
			name: "update",
			args: args{
				project: "p2",
				targets: []*model.ClusterTarget{{
					ClusterName: "c1",
					Namespace:   "n1",
				}},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			err := SyncConfigs(ctx, k8sClient, tc.args.project, tc.args.targets)
			if tc.want.errMsg != "" || err != nil {
				assert.ErrorContains(t, err, tc.want.errMsg)
			}
		})
	}
}
