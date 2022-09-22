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

func TestListManagedResource(t *testing.T) {
	list := ManagedResourceList{{"", "", "", "", "", "", ""}}
	assert.Equal(t, list.ToTableBody(), [][]string{{"", "", "", "", "", "", ""}})
}

func TestManagedResourceList_FilterCluster(t *testing.T) {
	list := ManagedResourceList{
		{"", "", "", "", "1", "", ""},
		{"", "", "", "", "2", "", ""},
	}
	list.FilterCluster("1")
	assert.Equal(t, len(list), 1)
}

func TestManagedResourceList_FilterClusterNamespace(t *testing.T) {
	list := ManagedResourceList{
		{"", "1", "", "", "1", "", ""},
		{"", "2", "", "", "2", "", ""},
	}
	list.FilterClusterNamespace("2")
	assert.Equal(t, len(list), 1)
}

var _ = Describe("test managed resource", func() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, &CtxKeyAppName, "first-vela-app")
	ctx = context.WithValue(ctx, &CtxKeyNamespace, "default")
	ctx = context.WithValue(ctx, &CtxKeyCluster, "local")

	It("list managed resource", func() {
		list, err := ListManagedResource(ctx, k8sClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(list.ToTableBody())).To(Equal(4))
	})
})
