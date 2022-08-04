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

package cli

import (
	"context"
	"os"
	"testing"

	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/pkg/definition"

	terraformapi "github.com/oam-dev/terraform-controller/api/v1beta1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

func TestListProviders(t *testing.T) {
	ctx := context.Background()
	type args struct {
		k8sClient client.Client
	}
	type want struct {
		errMsg string
	}
	s := runtime.NewScheme()
	v1beta1.AddToScheme(s)
	corev1.AddToScheme(s)
	terraformapi.AddToScheme(s)
	p1 := &terraformapi.Provider{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Provider",
			APIVersion: "terraform.core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p1",
			Namespace: "default",
			Labels: map[string]string{
				"config.oam.dev/type": types.TerraformProvider,
			},
		},
	}
	p2 := &terraformapi.Provider{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Provider",
			APIVersion: "terraform.core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p2",
			Namespace: "default",
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(s).WithObjects(p1, p2).Build()

	ioStream := util.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}

	testcases := map[string]struct {
		args args
		want want
	}{
		"success": {
			args: args{
				k8sClient: k8sClient,
			},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			err := listProviders(ctx, tc.args.k8sClient, ioStream)
			if err != nil || tc.want.errMsg != "" {
				assert.Contains(t, err.Error(), tc.want.errMsg)
			}
		})
	}
}

func TestGetProviderTypes(t *testing.T) {
	s := runtime.NewScheme()
	v1beta1.AddToScheme(s)
	corev1.AddToScheme(s)
	terraformapi.AddToScheme(s)

	p1 := &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p1",
			Namespace: types.DefaultKubeVelaNS,
			Labels:    map[string]string{definition.DefinitionType: types.TerraformProvider},
		},
	}
	p2 := &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p2",
			Namespace: types.DefaultKubeVelaNS,
			Labels:    map[string]string{definition.UserPrefix + definition.DefinitionType: types.TerraformProvider},
		},
	}
	testCases := map[string]struct {
		objects  []client.Object
		gotTypes map[string]bool
		wantErr  error
	}{
		"success": {
			objects: []client.Object{p1},
			gotTypes: map[string]bool{
				"p1": true,
			},
		},
		"success with legacy provider": {
			objects: []client.Object{p2},
			gotTypes: map[string]bool{
				"p2": true,
			},
		},
		"not found": {
			objects: []client.Object{},
			wantErr: errors.New("no Terraform Cloud Provider ComponentDefinition found"),
		},
	}

	ctx := context.Background()
	for _, tc := range testCases {
		k8sClient = fake.NewClientBuilder().WithScheme(s).WithObjects(tc.objects...).Build()
		providerTypes, err := getTerraformProviderTypes(ctx, k8sClient)
		if tc.wantErr != nil {
			assert.Contains(t, err.Error(), tc.wantErr.Error())
		} else {
			assert.NoError(t, err)
			assert.Equal(t, len(tc.gotTypes), len(providerTypes))
			for _, gotType := range providerTypes {
				_, found := tc.gotTypes[gotType.Name]
				assert.True(t, found)
			}
		}
	}
}
