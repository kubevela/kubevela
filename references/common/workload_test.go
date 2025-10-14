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
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corecommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestInitApplication(t *testing.T) {
	c := common.Args{}
	c.SetClient(fake.NewClientBuilder().Build())

	t.Run("with appGroup", func(t *testing.T) {
		app, err := InitApplication("default", c, "my-workload", "my-app")
		assert.NoError(t, err)
		assert.NotNil(t, app)
		assert.Equal(t, "my-app", app.Name)
	})

	t.Run("without appGroup", func(t *testing.T) {
		app, err := InitApplication("default", c, "my-workload", "")
		assert.NoError(t, err)
		assert.NotNil(t, app)
		assert.Equal(t, "my-workload", app.Name)
	})
}

func TestBaseComplete(t *testing.T) {
	s := runtime.NewScheme()
	assert.NoError(t, v1beta1.AddToScheme(s))

	template := &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "worker",
			Namespace: oam.SystemDefinitionNamespace,
		},
		Spec: v1beta1.ComponentDefinitionSpec{
			Workload: corecommon.WorkloadTypeDescriptor{
				Type: types.AutoDetectWorkloadDefinition,
			},
			Schematic: &corecommon.Schematic{
				CUE: &corecommon.CUE{
					Template: `
parameter: {
	image: string
	port: *8080 | int
}
`,
				},
			},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(s).WithObjects(template).Build()
	c := common.Args{}
	c.SetClient(k8sClient)

	t.Run("success", func(t *testing.T) {
		flagSet := pflag.NewFlagSet("test", pflag.ContinueOnError)
		flagSet.String("image", "my-image", "")
		flagSet.Int64("port", 80, "")

		app, err := BaseComplete(oam.SystemDefinitionNamespace, c, "my-workload", "my-app", flagSet, "worker")
		assert.NoError(t, err)
		assert.NotNil(t, app)

		workload, ok := app.Services["my-workload"]
		assert.True(t, ok)
		assert.Equal(t, "my-image", workload["image"])
		assert.Equal(t, int64(80), workload["port"])
	})

	t.Run("missing required flag", func(t *testing.T) {
		flagSet := pflag.NewFlagSet("test", pflag.ContinueOnError)
		// not setting the required "image" flag

		_, err := BaseComplete(oam.SystemDefinitionNamespace, c, "my-workload", "my-app", flagSet, "worker")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), `required flag(s) "image" not set`)
	})
}
