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

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/velaql/providers/query"
	querytypes "github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
)

var _ = Describe("test resource", func() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, &CtxKeyAppName, "first-vela-app")
	ctx = context.WithValue(ctx, &CtxKeyNamespace, "default")
	ctx = context.WithValue(ctx, &CtxKeyCluster, "")
	ctx = context.WithValue(ctx, &CtxKeyClusterNamespace, "")
	ctx = context.WithValue(ctx, &CtxKeyComponentName, "webservice-test")

	It("collect resource", func() {
		opt := query.Option{
			Name:      "first-vela-app",
			Namespace: "default",
			Filter: query.FilterOption{
				Cluster:          "",
				ClusterNamespace: "",
				Components:       []string{"deploy1"},
				APIVersion:       "v1",
				Kind:             "Pod",
			},
			WithTree: true,
		}
		podList, err := collectResource(ctx, k8sClient, opt)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(podList)).To(Equal(1))
	})

	It("convert object to yaml", func() {
		gvr := &GVR{
			GV: "core.oam.dev/v1beta1",
			R: Resource{
				Kind:      "Application",
				Name:      "first-vela-app",
				Namespace: "default",
			},
		}
		obj, err := GetResourceObject(k8sClient, gvr)
		Expect(err).NotTo(HaveOccurred())
		yaml, err := ToYaml(obj)
		Expect(err).NotTo(HaveOccurred())
		Expect(yaml).NotTo(ContainSubstring("can't load"))
		Expect(yaml).To(ContainSubstring("apiVersion: core.oam.dev/v1beta1"))
	})

})

func TestSonLeafResource(t *testing.T) {
	res := querytypes.AppliedResource{}
	node := &querytypes.ResourceTreeNode{
		LeafNodes: []*querytypes.ResourceTreeNode{
			{
				Object: unstructured.Unstructured{},
			},
		},
	}
	objs := sonLeafResource(res, node, "", "")
	assert.Equal(t, len(objs), 2)
}
