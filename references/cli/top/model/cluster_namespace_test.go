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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("test cluster namespace", func() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, &CtxKeyAppName, "first-vela-app")
	ctx = context.WithValue(ctx, &CtxKeyNamespace, "default")
	It("list cluster namespace", func() {
		cnsList, err := ListClusterNamespaces(ctx, k8sClient)
		Expect(err).NotTo(HaveOccurred())
		tableBody := cnsList.ToTableBody()
		Expect(len(tableBody)).To(Equal(2))
		Expect(len(tableBody[1])).To(Equal(3))
		Expect(tableBody[1][0]).To(Equal("default"))
		Expect(tableBody[1][1]).To(Equal("Active"))
	})
	It("load cluster namespace detail info", func() {
		ns, err := LoadNamespaceDetail(ctx, k8sClient, "default")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(ns.Status.Phase)).To(Equal("Active"))
	})
})
