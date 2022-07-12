///*
//Copyright 2022 The KubeVela Authors.
//
//Licensed under the Apache License, Version 2.0 (the "License");
//you may not use this file except in compliance with the License.
//You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
//Unless required by applicable law or agreed to in writing, software
//distributed under the License is distributed on an "AS IS" BASIS,
//WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//See the License for the specific language governing permissions and
//limitations under the License.
//*/
//
package service

//
//import (
//	"context"
//	"encoding/json"
//	"sort"
//	"strings"
//	"testing"
//	"time"
//
//	. "github.com/agiledragon/gomonkey/v2"
//	terraformtypes "github.com/oam-dev/terraform-controller/api/types"
//	terraformapi "github.com/oam-dev/terraform-controller/api/v1beta1"
//	"gotest.tools/assert"
//	corev1 "k8s.io/api/core/v1"
//	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
//	"k8s.io/apimachinery/pkg/runtime"
//	"k8s.io/client-go/rest"
//	"sigs.k8s.io/controller-runtime/pkg/client"
//	"sigs.k8s.io/controller-runtime/pkg/client/fake"
//
//	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
//	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
//	"github.com/oam-dev/kubevela/apis/types"
//	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
//	apis "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
//	"github.com/oam-dev/kubevela/pkg/definition"
//	"github.com/oam-dev/kubevela/pkg/multicluster"
//)
//
//func TestListConfigTypes(t *testing.T) {
//	s := runtime.NewScheme()
//	v1beta1.AddToScheme(s)
//	corev1.AddToScheme(s)
//	def1 := &v1beta1.ComponentDefinition{
//		TypeMeta: metav1.TypeMeta{
//			Kind:       "ComponentDefinition",
//			APIVersion: "core.oam.dev/v1beta1",
//		},
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      "def1",
//			Namespace: types.DefaultKubeVelaNS,
//			Labels: map[string]string{
//				definition.UserPrefix + "catalog.config.oam.dev": types.VelaCoreConfig,
//				definitionType: types.TerraformProvider,
//			},
//		},
//	}
//	def2 := &v1beta1.ComponentDefinition{
//		TypeMeta: metav1.TypeMeta{
//			Kind:       "ComponentDefinition",
//			APIVersion: "core.oam.dev/v1beta1",
//		},
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      "def2",
//			Namespace: types.DefaultKubeVelaNS,
//			Annotations: map[string]string{
//				definitionAlias: "Def2",
//			},
//			Labels: map[string]string{
//				definition.UserPrefix + "catalog.config.oam.dev": types.VelaCoreConfig,
//			},
//		},
//	}
//	k8sClient := fake.NewClientBuilder().WithScheme(s).WithObjects(def1, def2).Build()
//
//	patches := ApplyFunc(multicluster.GetMulticlusterKubernetesClient, func() (client.Client, *rest.Config, error) {
//		return k8sClient, nil, nil
//	})
//	defer patches.Reset()
//
//	h := &configServiceImpl{
//		KubeClient: k8sClient,
//	}
//
//	type args struct {
//		h ConfigService
//	}
//
//	type want struct {
//		configTypes []*apis.ConfigType
//		errMsg      string
//	}
//
//	ctx := context.Background()
//
//	testcases := []struct {
//		name string
//		args args
//		want want
//	}{
//		{
//			name: "success",
//			args: args{
//				h: h,
//			},
//			want: want{
//				configTypes: []*apis.ConfigType{
//					{
//						Name:        "def2",
//						Alias:       "Def2",
//						Definitions: []string{"def2"},
//					},
//					{
//						Alias: "Terraform Cloud Provider",
//						Name:  types.TerraformProvider,
//						Definitions: []string{
//							"def1",
//						},
//					},
//				},
//			},
//		},
//	}
//
//	for _, tc := range testcases {
//		t.Run(tc.name, func(t *testing.T) {
//			got, err := tc.args.h.ListConfigTypes(ctx, "")
//			if tc.want.errMsg != "" || err != nil {
//				assert.ErrorContains(t, err, tc.want.errMsg)
//			}
//			assert.DeepEqual(t, got, tc.want.configTypes)
//		})
//	}
//}
//
//func TestGetConfigType(t *testing.T) {
//	s := runtime.NewScheme()
//	v1beta1.AddToScheme(s)
//	corev1.AddToScheme(s)
//	def2 := &v1beta1.ComponentDefinition{
//		TypeMeta: metav1.TypeMeta{
//			Kind:       "ComponentDefinition",
//			APIVersion: "core.oam.dev/v1beta1",
//		},
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      "def2",
//			Namespace: types.DefaultKubeVelaNS,
//			Annotations: map[string]string{
//				definitionAlias: "Def2",
//			},
//			Labels: map[string]string{
//				definition.UserPrefix + "catalog.config.oam.dev": types.VelaCoreConfig,
//			},
//		},
//	}
//	k8sClient := fake.NewClientBuilder().WithScheme(s).WithObjects(def2).Build()
//
//	patches := ApplyFunc(multicluster.GetMulticlusterKubernetesClient, func() (client.Client, *rest.Config, error) {
//		return k8sClient, nil, nil
//	})
//	defer patches.Reset()
//
//	h := &configServiceImpl{KubeClient: k8sClient}
//
//	type args struct {
//		h    ConfigService
//		name string
//	}
//
//	type want struct {
//		configType *apis.ConfigType
//		errMsg     string
//	}
//
//	ctx := context.Background()
//
//	testcases := []struct {
//		name string
//		args args
//		want want
//	}{
//		{
//			name: "success",
//			args: args{
//				h:    h,
//				name: "def2",
//			},
//			want: want{
//				configType: &apis.ConfigType{
//					Alias: "Def2",
//					Name:  "def2",
//				},
//			},
//		},
//		{
//			name: "error",
//			args: args{
//				h:    h,
//				name: "def99",
//			},
//			want: want{
//				errMsg: "failed to get config type",
//			},
//		},
//	}
//
//	for _, tc := range testcases {
//		t.Run(tc.name, func(t *testing.T) {
//			got, err := tc.args.h.GetConfigType(ctx, tc.args.name)
//			if tc.want.errMsg != "" || err != nil {
//				assert.ErrorContains(t, err, tc.want.errMsg)
//			}
//			assert.DeepEqual(t, got, tc.want.configType)
//		})
//	}
//}
//
//func TestCreateConfig(t *testing.T) {
//	s := runtime.NewScheme()
//	v1beta1.AddToScheme(s)
//	corev1.AddToScheme(s)
//
//	k8sClient := fake.NewClientBuilder().WithScheme(s).Build()
//
//	h := &configServiceImpl{KubeClient: k8sClient}
//
//	type args struct {
//		h   ConfigService
//		req apis.CreateConfigRequest
//	}
//
//	type want struct {
//		errMsg string
//	}
//
//	ctx := context.Background()
//
//	properties, err := json.Marshal(map[string]interface{}{
//		"name": "default",
//	})
//	assert.NilError(t, err)
//
//	testcases := []struct {
//		name string
//		args args
//		want want
//	}{
//		{
//			name: "delete config when it's not ready",
//			args: args{
//				h: h,
//				req: apis.CreateConfigRequest{
//					Name:          "a",
//					ComponentType: "b",
//					Project:       "c",
//				},
//			},
//		},
//		{
//			name: "create terraform-alibaba config",
//			args: args{
//				h: h,
//				req: apis.CreateConfigRequest{
//					Name:          "n1",
//					ComponentType: "terraform-alibaba",
//					Project:       "p1",
//					Properties:    string(properties),
//				},
//			},
//		},
//	}
//
//	for _, tc := range testcases {
//		t.Run(tc.name, func(t *testing.T) {
//			err := tc.args.h.CreateConfig(ctx, tc.args.req)
//			if tc.want.errMsg != "" || err != nil {
//				assert.ErrorContains(t, err, tc.want.errMsg)
//			}
//		})
//	}
//}
//
//func TestGetConfigs(t *testing.T) {
//	s := runtime.NewScheme()
//	v1beta1.AddToScheme(s)
//	corev1.AddToScheme(s)
//	terraformapi.AddToScheme(s)
//	createdTime, _ := time.Parse(time.UnixDate, "Wed Apr 7 11:06:39 PST 2022")
//
//	provider1 := &terraformapi.Provider{
//		ObjectMeta: metav1.ObjectMeta{
//			Name:              "provider1",
//			Namespace:         "default",
//			CreationTimestamp: metav1.NewTime(createdTime),
//		},
//		Status: terraformapi.ProviderStatus{
//			State: terraformtypes.ProviderIsReady,
//		},
//	}
//
//	provider2 := &terraformapi.Provider{
//		ObjectMeta: metav1.ObjectMeta{
//			Name:              "provider2",
//			Namespace:         "default",
//			CreationTimestamp: metav1.NewTime(createdTime),
//		},
//		Status: terraformapi.ProviderStatus{
//			State: terraformtypes.ProviderIsNotReady,
//		},
//	}
//
//	provider3 := &terraformapi.Provider{
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      "provider3",
//			Namespace: "default",
//		},
//	}
//
//	app1 := &v1beta1.Application{
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      "provider3",
//			Namespace: types.DefaultKubeVelaNS,
//			Labels: map[string]string{
//				types.LabelConfigType: "terraform-alibaba",
//			},
//			CreationTimestamp: metav1.NewTime(createdTime),
//		},
//		Status: common.AppStatus{
//			Phase: common.ApplicationRendering,
//		},
//	}
//
//	k8sClient := fake.NewClientBuilder().WithScheme(s).WithObjects(provider1, provider2, provider3, app1).Build()
//
//	h := &configServiceImpl{KubeClient: k8sClient}
//
//	type args struct {
//		configType string
//		h          ConfigService
//	}
//
//	type want struct {
//		configs []*apis.Config
//		errMsg  string
//	}
//
//	ctx := context.Background()
//
//	testcases := []struct {
//		name string
//		args args
//		want want
//	}{
//		{
//			name: "success",
//			args: args{
//				configType: types.TerraformProvider,
//				h:          h,
//			},
//			want: want{
//				configs: []*apis.Config{
//					{
//						Name:        "provider1",
//						CreatedTime: &createdTime,
//						Status:      "Ready",
//					},
//					{
//						Name:        "provider2",
//						CreatedTime: &createdTime,
//						Status:      "Not ready",
//					},
//					{
//						Name:              "provider3",
//						CreatedTime:       &createdTime,
//						Status:            "Not ready",
//						ConfigType:        "terraform-alibaba",
//						ApplicationStatus: common.ApplicationRendering,
//					},
//				},
//			},
//		},
//	}
//
//	for _, tc := range testcases {
//		t.Run(tc.name, func(t *testing.T) {
//			got, err := tc.args.h.GetConfigs(ctx, tc.args.configType)
//			if tc.want.errMsg != "" || err != nil {
//				assert.ErrorContains(t, err, tc.want.errMsg)
//			}
//			assert.DeepEqual(t, got, tc.want.configs)
//		})
//	}
//}
//
//func TestMergeTargets(t *testing.T) {
//	currentTargets := []ApplicationDeployTarget{
//		{
//			Namespace: "n1",
//			Clusters:  []string{"c1", "c2"},
//		}, {
//			Namespace: "n2",
//			Clusters:  []string{"c3"},
//		},
//	}
//	targets := []*model.ClusterTarget{
//		{
//			Namespace:   "n3",
//			ClusterName: "c4",
//		}, {
//			Namespace:   "n1",
//			ClusterName: "c5",
//		},
//		{
//			Namespace:   "n2",
//			ClusterName: "c3",
//		},
//	}
//
//	expected := []ApplicationDeployTarget{
//		{
//			Namespace: "n1",
//			Clusters:  []string{"c1", "c2", "c5"},
//		}, {
//			Namespace: "n2",
//			Clusters:  []string{"c3"},
//		}, {
//			Namespace: "n3",
//			Clusters:  []string{"c4"},
//		},
//	}
//
//	got := mergeTargets(currentTargets, targets)
//
//	for i, g := range got {
//		clusters := g.Clusters
//		sort.SliceStable(clusters, func(i, j int) bool {
//			return clusters[i] < clusters[j]
//		})
//		got[i].Clusters = clusters
//	}
//	assert.DeepEqual(t, expected, got)
//}
//
//func TestConvert(t *testing.T) {
//	targets := []*model.ClusterTarget{
//		{
//			Namespace:   "n3",
//			ClusterName: "c4",
//		}, {
//			Namespace:   "n1",
//			ClusterName: "c5",
//		},
//		{
//			Namespace:   "n2",
//			ClusterName: "c3",
//		},
//		{
//			Namespace:   "n3",
//			ClusterName: "c5",
//		},
//	}
//
//	expected := []ApplicationDeployTarget{
//		{
//			Namespace: "n3",
//			Clusters:  []string{"c4", "c5"},
//		},
//		{
//			Namespace: "n1",
//			Clusters:  []string{"c5"},
//		}, {
//			Namespace: "n2",
//			Clusters:  []string{"c3"},
//		},
//	}
//
//	got := convertClusterTargets(targets)
//
//	for i, g := range got {
//		clusters := g.Clusters
//		sort.SliceStable(clusters, func(i, j int) bool {
//			return clusters[i] < clusters[j]
//		})
//		got[i].Clusters = clusters
//	}
//	assert.DeepEqual(t, expected, got)
//}
//
//func TestDestroySyncConfigsApp(t *testing.T) {
//	s := runtime.NewScheme()
//	v1beta1.AddToScheme(s)
//	corev1.AddToScheme(s)
//	app1 := &v1beta1.Application{
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      "config-sync-p1",
//			Namespace: types.DefaultKubeVelaNS,
//		},
//	}
//	k8sClient1 := fake.NewClientBuilder().WithScheme(s).WithObjects(app1).Build()
//
//	k8sClient2 := fake.NewClientBuilder().Build()
//
//	type args struct {
//		project   string
//		k8sClient client.Client
//	}
//
//	type want struct {
//		errMsg string
//	}
//
//	ctx := context.Background()
//
//	testcases := map[string]struct {
//		args args
//		want want
//	}{
//		"found": {
//			args: args{
//				project:   "p1",
//				k8sClient: k8sClient1,
//			},
//		},
//		"not found": {
//			args: args{
//				project:   "p1",
//				k8sClient: k8sClient2,
//			},
//			want: want{
//				errMsg: "no kind is registered for the type v1beta1.Application",
//			},
//		},
//	}
//	for name, tc := range testcases {
//		t.Run(name, func(t *testing.T) {
//			err := destroySyncConfigsApp(ctx, tc.args.k8sClient, tc.args.project)
//			if err != nil || tc.want.errMsg != "" {
//				if !strings.Contains(err.Error(), tc.want.errMsg) {
//					assert.ErrorContains(t, err, tc.want.errMsg)
//				}
//			}
//		})
//	}
//}
//
//func TestSyncConfigs(t *testing.T) {
//	s := runtime.NewScheme()
//	v1beta1.AddToScheme(s)
//	corev1.AddToScheme(s)
//	secret1 := &corev1.Secret{
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      "s1",
//			Namespace: types.DefaultKubeVelaNS,
//			Labels: map[string]string{
//				types.LabelConfigCatalog:            types.VelaCoreConfig,
//				types.LabelConfigProject:            "p1",
//				types.LabelConfigSyncToMultiCluster: "true",
//			},
//		},
//	}
//
//	policies := []ApplicationDeployTarget{{
//		Namespace: "n9",
//		Clusters:  []string{"c19"},
//	}}
//	properties, _ := json.Marshal(policies)
//	app1 := &v1beta1.Application{
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      "config-sync-p2",
//			Namespace: types.DefaultKubeVelaNS,
//		},
//		Spec: v1beta1.ApplicationSpec{
//			Policies: []v1beta1.AppPolicy{{
//				Name:       "c19",
//				Type:       "topology",
//				Properties: &runtime.RawExtension{Raw: properties},
//			}},
//		},
//	}
//
//	k8sClient := fake.NewClientBuilder().WithScheme(s).WithObjects(secret1, app1).Build()
//
//	type args struct {
//		project string
//		targets []*model.ClusterTarget
//	}
//
//	type want struct {
//		errMsg string
//	}
//
//	ctx := context.Background()
//
//	testcases := []struct {
//		name string
//		args args
//		want want
//	}{
//		{
//			name: "create",
//			args: args{
//				project: "p1",
//				targets: []*model.ClusterTarget{{
//					ClusterName: "c1",
//					Namespace:   "n1",
//				}},
//			},
//		},
//		{
//			name: "update",
//			args: args{
//				project: "p2",
//				targets: []*model.ClusterTarget{{
//					ClusterName: "c1",
//					Namespace:   "n1",
//				}},
//			},
//		},
//		{
//			name: "skip config sync",
//			args: args{
//				project: "p3",
//				targets: []*model.ClusterTarget{{
//					ClusterName: "c1",
//					Namespace:   "n1",
//				}},
//			},
//		},
//	}
//
//	for _, tc := range testcases {
//		t.Run(tc.name, func(t *testing.T) {
//			err := SyncConfigs(ctx, k8sClient, tc.args.project, tc.args.targets)
//			if tc.want.errMsg != "" || err != nil {
//				assert.ErrorContains(t, err, tc.want.errMsg)
//			}
//		})
//	}
//}
//
//func TestDeleteConfig(t *testing.T) {
//	s := runtime.NewScheme()
//	v1beta1.AddToScheme(s)
//	corev1.AddToScheme(s)
//	terraformapi.AddToScheme(s)
//	provider1 := &terraformapi.Provider{
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      "p1",
//			Namespace: "default",
//		},
//	}
//
//	provider2 := &terraformapi.Provider{
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      "p2",
//			Namespace: "default",
//		},
//	}
//
//	provider3 := &terraformapi.Provider{
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      "p3",
//			Namespace: "default",
//		},
//	}
//
//	app1 := &v1beta1.Application{
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      "config-terraform-provider-p1",
//			Namespace: types.DefaultKubeVelaNS,
//			Labels: map[string]string{
//				types.LabelConfigType: "terraform-alibaba",
//			},
//		},
//	}
//
//	app2 := &v1beta1.Application{
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      "p2",
//			Namespace: types.DefaultKubeVelaNS,
//			Labels: map[string]string{
//				types.LabelConfigType: "terraform-alibaba",
//			},
//		},
//	}
//
//	normalApp := &v1beta1.Application{
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      "a9",
//			Namespace: types.DefaultKubeVelaNS,
//		},
//	}
//
//	k8sClient := fake.NewClientBuilder().WithScheme(s).WithObjects(provider1, provider2, provider3, app1, app2, normalApp).Build()
//
//	h := &configServiceImpl{KubeClient: k8sClient}
//
//	type args struct {
//		configType string
//		name       string
//		h          ConfigService
//	}
//
//	type want struct {
//		errMsg string
//	}
//
//	ctx := context.Background()
//
//	testcases := []struct {
//		name string
//		args args
//		want want
//	}{
//		{
//			name: "delete a legacy terraform provider",
//			args: args{
//				configType: "terraform-alibaba",
//				name:       "p1",
//				h:          h,
//			},
//			want: want{},
//		},
//		{
//			name: "delete a terraform provider",
//			args: args{
//				configType: "terraform-alibaba",
//				name:       "p2",
//				h:          h,
//			},
//			want: want{},
//		},
//		{
//			name: "delete a terraform provider, but its application not found",
//			args: args{
//				configType: "terraform-alibaba",
//				name:       "p3",
//				h:          h,
//			},
//			want: want{
//				errMsg: "could not be disabled because it was created by enabling a Terraform provider or was manually created",
//			},
//		},
//		{
//			name: "delete a normal config, but failed",
//			args: args{
//				configType: "config-image-registry",
//				name:       "a10",
//				h:          h,
//			},
//			want: want{
//				errMsg: "not found",
//			},
//		},
//	}
//
//	for _, tc := range testcases {
//		t.Run(tc.name, func(t *testing.T) {
//			err := tc.args.h.DeleteConfig(ctx, tc.args.configType, tc.args.name)
//			if tc.want.errMsg != "" || err != nil {
//				assert.ErrorContains(t, err, tc.want.errMsg)
//			}
//		})
//	}
//}
