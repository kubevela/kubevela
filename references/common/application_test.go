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
	"bytes"
	"context"
	"os"
	"testing"

	terraformapi "github.com/oam-dev/terraform-controller/api/v1beta2"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	sigs "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/references/appfile/api"

	utilcommon "github.com/oam-dev/kubevela/pkg/utils/common"
	querytypes "github.com/oam-dev/kubevela/pkg/utils/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

func TestExportFromAppFile(t *testing.T) {
	s := runtime.NewScheme()
	assert.NoError(t, v1beta1.AddToScheme(s))
	clt := fake.NewClientBuilder().WithScheme(s).Build()
	var out bytes.Buffer
	ioStream := cmdutil.IOStreams{In: os.Stdin, Out: &out, ErrOut: &out}

	o := &AppfileOptions{
		Kubecli:   clt,
		IO:        ioStream,
		Namespace: "default",
	}

	appFile := &api.AppFile{
		Name: "test-app-export-from",
	}

	c := utilcommon.Args{}
	c.SetClient(clt)

	result, data, err := o.ExportFromAppFile(appFile, "default", true, c)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, data)
	assert.Contains(t, string(data), "name: test-app-export-from")
	assert.Equal(t, "test-app-export-from", result.application.Name)
}

func TestApplyApp(t *testing.T) {
	s := runtime.NewScheme()
	v1beta1.AddToScheme(s)

	t.Run("create app if not exist", func(t *testing.T) {
		cltCreate := fake.NewClientBuilder().WithScheme(s).Build()
		var outCreate bytes.Buffer
		ioStreamCreate := cmdutil.IOStreams{In: os.Stdin, Out: &outCreate, ErrOut: &outCreate}
		oCreate := &AppfileOptions{
			Kubecli:   cltCreate,
			IO:        ioStreamCreate,
			Namespace: "default",
		}
		appCreate := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-apply",
				Namespace: "default",
			},
		}
		err := oCreate.ApplyApp(appCreate, nil)
		assert.NoError(t, err)
		assert.Contains(t, outCreate.String(), "App has not been deployed, creating a new deployment...")
		assert.Contains(t, outCreate.String(), "vela port-forward test-app-apply")
	})

	t.Run("update app if exists", func(t *testing.T) {
		existingApp := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-apply",
				Namespace: "default",
			},
		}
		cltUpdate := fake.NewClientBuilder().WithScheme(s).WithObjects(existingApp).Build()
		var outUpdate bytes.Buffer
		ioStreamUpdate := cmdutil.IOStreams{In: os.Stdin, Out: &outUpdate, ErrOut: &outUpdate}
		oUpdate := &AppfileOptions{
			Kubecli:   cltUpdate,
			IO:        ioStreamUpdate,
			Namespace: "default",
		}
		appUpdate := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app-apply",
				Namespace: "default",
			},
		}
		err := oUpdate.ApplyApp(appUpdate, nil)
		assert.NoError(t, err)
		assert.Contains(t, outUpdate.String(), "App exists, updating existing deployment...")
		assert.Contains(t, outUpdate.String(), "vela port-forward test-app-apply")
	})
}

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
		k8sClient sigs.Client
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

func TestIsAppfile(t *testing.T) {
	testCases := []struct {
		name     string
		data     []byte
		expected bool
	}{
		{
			name:     "valid appfile json",
			data:     []byte(`{"name": "test"}`),
			expected: true,
		},
		{
			name:     "invalid json",
			data:     []byte(`{"name": "test"`),
			expected: false,
		},
		{
			name: "application yaml",
			data: []byte(`
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: test-app
`),
			expected: false,
		},
		{
			name: "appfile yaml",
			data: []byte(`
name: test-app
`),
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := IsAppfile(tc.data)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestInfo(t *testing.T) {
	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "test-ns",
		},
	}
	info := Info(app)
	assert.Contains(t, info, "vela port-forward test-app -n test-ns")
	assert.Contains(t, info, "vela exec test-app -n test-ns")
	assert.Contains(t, info, "vela logs test-app -n test-ns")
	assert.Contains(t, info, "vela status test-app -n test-ns")
	assert.Contains(t, info, "vela status test-app -n test-ns --endpoint")
}

func TestSonLeafResource(t *testing.T) {
	node := &querytypes.ResourceTreeNode{
		LeafNodes: []*querytypes.ResourceTreeNode{
			{
				Object: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"kind":       "Deployment",
						"apiVersion": "apps/v1",
					},
				},
				Kind:       "Deployment",
				APIVersion: "apps/v1",
			},
		},
	}
	resources := sonLeafResource(node, "Deployment", "apps/v1")
	assert.Equal(t, 1, len(resources))
	assert.Equal(t, "Deployment", resources[0].GetKind())
}

func TestLoadAppFile(t *testing.T) {
	content := "name: test-app"
	tmpFile, err := os.CreateTemp("", "appfile-*.yaml")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(content)
	assert.NoError(t, err)
	assert.NoError(t, tmpFile.Close())

	appFile, err := LoadAppFile(tmpFile.Name())
	assert.NoError(t, err)
	assert.NotNil(t, appFile)
	assert.Equal(t, "test-app", appFile.Name)
}

func TestApplyApplication(t *testing.T) {
	s := runtime.NewScheme()
	v1beta1.AddToScheme(s)
	clt := fake.NewClientBuilder().WithScheme(s).Build()
	app := v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "default",
		},
	}
	var out bytes.Buffer
	ioStream := cmdutil.IOStreams{In: os.Stdin, Out: &out, ErrOut: &out}
	err := ApplyApplication(app, ioStream, clt)
	assert.NoError(t, err)
	assert.Contains(t, out.String(), "Applying an application in vela K8s object format...")
}
