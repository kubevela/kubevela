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

func TestNamespaceList_Body(t *testing.T) {
	nsList := NamespaceList{}
	assert.Equal(t, len(nsList.ToTableBody()), 1)
	assert.Equal(t, nsList.ToTableBody()[0], []string{AllNamespace, "*", "*"})
}

var _ = Describe("test namespace", func() {
	ctx := context.Background()
	It("list namespace", func() {
		nsList, err := ListNamespaces(ctx, k8sClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(nsList.ToTableBody())).To(Equal(6))
		Expect(nsList.ToTableBody()[0]).To(Equal([]string{"all", "*", "*"}))
	})
})
