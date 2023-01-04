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
package common

import (
	"context"
	"testing"

	terraformapi "github.com/oam-dev/terraform-controller/api/v1beta2"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
)

func TestPrepareToForceDeleteTerraformComponents(t *testing.T) {
	ctx := context.Background()
	s := runtime.NewScheme()
	v1beta1.AddToScheme(s)
	corev1.AddToScheme(s)
	terraformapi.AddToScheme(s)
	app1 := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app1",
			Namespace: "default",
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{
					Name: "c1",
					Type: "d1",
				},
			},
		},
	}
	def1 := &v1beta1.ComponentDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ComponentDefinition",
			APIVersion: "core.oam.dev/v1beta2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "d1",
			Namespace: types.DefaultKubeVelaNS,
		},
		Spec: v1beta1.ComponentDefinitionSpec{
			Schematic: &common.Schematic{
				Terraform: &common.Terraform{
					Configuration: "abc",
				},
			},
		},
	}
	conf1 := &terraformapi.Configuration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "c1",
			Namespace: "default",
		},
	}

	userNamespace := "another-namespace"
	def2 := def1.DeepCopy()
	def2.SetNamespace(userNamespace)
	app2 := app1.DeepCopy()
	app2.SetNamespace(userNamespace)
	app2.SetName("app2")
	conf2 := conf1.DeepCopy()
	conf2.SetNamespace(userNamespace)

	k8sClient1 := fake.NewClientBuilder().WithScheme(s).WithObjects(app1, def1, conf1).Build()

	k8sClient2 := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()

	k8sClient3 := fake.NewClientBuilder().WithScheme(s).WithObjects(app1).Build()

	k8sClient4 := fake.NewClientBuilder().WithScheme(s).WithObjects(app1, def1).Build()

	k8sClient5 := fake.NewClientBuilder().WithScheme(s).WithObjects(app2, def2, conf2).Build()
	type args struct {
		k8sClient client.Client
		namespace string
		name      string
	}
	type want struct {
		errMsg string
	}

	testcases := map[string]struct {
		args args
		want want
	}{
		"valid": {
			args: args{
				k8sClient1,
				"default",
				"app1",
			},
			want: want{},
		},
		"application not found": {
			args: args{
				k8sClient1,
				"default",
				"app99",
			},
			want: want{
				errMsg: "already deleted or not exist",
			},
		},
		"failed to get application": {
			args: args{
				k8sClient2,
				"default",
				"app1",
			},
			want: want{
				errMsg: "delete application err",
			},
		},
		"definition is not available": {
			args: args{
				k8sClient3,
				"default",
				"app1",
			},
			want: want{
				errMsg: "componentdefinitions.core.oam.dev \"d1\" not found",
			},
		},
		"Configuration is not available": {
			args: args{
				k8sClient4,
				"default",
				"app1",
			},
			want: want{
				errMsg: "configurations.terraform.core.oam.dev \"c1\" not found",
			},
		},
		"can read definition from application namespace": {
			args: args{
				k8sClient5,
				userNamespace,
				"app2",
			},
			want: want{},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			err := PrepareToForceDeleteTerraformComponents(ctx, tc.args.k8sClient, tc.args.namespace, tc.args.name)
			if err != nil {
				assert.NotEmpty(t, tc.want.errMsg)
				assert.Contains(t, err.Error(), tc.want.errMsg)
			} else {
				assert.Empty(t, tc.want.errMsg)
			}
		})
	}

}
