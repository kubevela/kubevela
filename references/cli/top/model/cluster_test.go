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

	"github.com/bmizerany/assert"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestClusterList_ToTableBody(t *testing.T) {
	clusterList := &ClusterList{{"local", "", "", "", ""}}
	assert.Equal(t, len(clusterList.ToTableBody()), 2)
	assert.Equal(t, clusterList.ToTableBody()[1], []string{"local", "", "", "", ""})
}

var _ = Describe("test cluster list", func() {
	ctx := context.WithValue(context.Background(), &CtxKeyAppName, "first-vela-app")
	ctx = context.WithValue(ctx, &CtxKeyNamespace, "default")

	It("list clusters", func() {
		clusterList, err := ListClusters(ctx, k8sClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(clusterList.ToTableBody())).To(Equal(2))
		Expect(clusterList.ToTableBody()[1]).To(Equal([]string{"local", "", "Internal", "-", ""}))
	})
})
