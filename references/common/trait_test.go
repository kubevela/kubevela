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

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestListRawWorkloadDefinitions(t *testing.T) {
	s := runtime.NewScheme()
	v1beta1.AddToScheme(s)

	sysWorkload := v1beta1.WorkloadDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sys-worker",
			Namespace: oam.SystemDefinitionNamespace,
		},
	}
	userWorkload := v1beta1.WorkloadDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "user-worker",
			Namespace: "my-ns",
		},
	}

	t.Run("list from both user and system namespaces", func(t *testing.T) {
		k8sClient := fake.NewClientBuilder().WithScheme(s).WithObjects(&sysWorkload, &userWorkload).Build()
		c := common.Args{}
		c.SetClient(k8sClient)

		defs, err := ListRawWorkloadDefinitions("my-ns", c)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(defs))
	})

	t.Run("list from only system namespace", func(t *testing.T) {
		k8sClient := fake.NewClientBuilder().WithScheme(s).WithObjects(&sysWorkload).Build()
		c := common.Args{}
		c.SetClient(k8sClient)

		defs, err := ListRawWorkloadDefinitions("my-ns", c)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(defs))
		assert.Equal(t, "sys-worker", defs[0].Name)
	})

	t.Run("list from only user namespace", func(t *testing.T) {
		k8sClient := fake.NewClientBuilder().WithScheme(s).WithObjects(&userWorkload).Build()
		c := common.Args{}
		c.SetClient(k8sClient)

		defs, err := ListRawWorkloadDefinitions("my-ns", c)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(defs))
		assert.Equal(t, "user-worker", defs[0].Name)
	})

	t.Run("no definitions found", func(t *testing.T) {
		k8sClient := fake.NewClientBuilder().WithScheme(s).Build()
		c := common.Args{}
		c.SetClient(k8sClient)

		defs, err := ListRawWorkloadDefinitions("my-ns", c)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(defs))
	})
}
