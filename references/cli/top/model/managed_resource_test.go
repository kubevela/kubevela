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

package model

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
)

func TestK8SObjectList_Header(t *testing.T) {
	list := ManagedResourceList{title: []string{"name", "namespace", "kind", "APIVersion", "cluster", "status"}}
	assert.Equal(t, len(list.Header()), 6)
	assert.Equal(t, list.Header(), []string{"name", "namespace", "kind", "APIVersion", "cluster", "status"})
}

func TestK8SObjectList_Body(t *testing.T) {
	list := ManagedResourceList{data: []ManagedResource{{"", "", "", "", "", ""}}}
	assert.Equal(t, len(list.Body()), 1)
	assert.Equal(t, list.Body(), [][]string{{"", "", "", "", "", ""}})
}

var _ = Describe("test k8s object", func() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, &CtxKeyAppName, "first-vela-app")
	ctx = context.WithValue(ctx, &CtxKeyNamespace, "default")
	ctx = context.WithValue(ctx, &CtxKeyCluster, "local")

	It("list k8s object", func() {
		list, err := ListManagedResource(ctx, k8sClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(list.Header())).To(Equal(6))
		Expect(len(list.Body())).To(Equal(2))
	})
})
