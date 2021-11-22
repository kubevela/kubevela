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

package usecase

import (
	"context"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
)

var _ = Describe("Test namespace usecase functions", func() {
	var (
		namespaceUsecase *namespaceUsecaseImpl
	)
	BeforeEach(func() {
		namespaceUsecase = &namespaceUsecaseImpl{kubeClient: k8sClient}
	})
	It("Test CreateNamespace function", func() {
		req := apisv1.CreateNamespaceRequest{
			Name:        "test-namespace",
			Description: "this is a namespace description 王二",
		}
		base, err := namespaceUsecase.CreateNamespace(context.TODO(), req)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(base.Description, req.Description)).Should(BeEmpty())
	})

	It("Test ListNamespace function", func() {
		_, err := namespaceUsecase.ListNamespaces(context.TODO())
		Expect(err).Should(BeNil())
	})
})
