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
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodList_ToTableBody(t *testing.T) {
	pod := Pod{
		Name:      "",
		Namespace: "",
		Cluster:   "",
		Ready:     "",
		Status:    "",
		CPU:       "",
		Mem:       "",
		CPUR:      "",
		CPUL:      "",
		MemR:      "",
		MemL:      "",
		IP:        "",
		NodeName:  "",
		Age:       "",
	}
	podList := &PodList{pod}
	assert.Equal(t, len(podList.ToTableBody()), 1)
}

var _ = Describe("test pod", func() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, &CtxKeyAppName, "first-vela-app")
	ctx = context.WithValue(ctx, &CtxKeyNamespace, "default")
	ctx = context.WithValue(ctx, &CtxKeyCluster, "local")
	ctx = context.WithValue(ctx, &CtxKeyClusterNamespace, "")
	ctx = context.WithValue(ctx, &CtxKeyComponentName, "deploy1")

	It("list pods", func() {
		podList, err := ListPods(ctx, cfg, k8sClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(podList.ToTableBody())).To(Equal(1))
	})

	It("load pod detail", func() {
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "pod",
				Namespace:         "ns",
				CreationTimestamp: metav1.Time{Time: time.Now()},
			},
			Spec: v1.PodSpec{
				NodeName: "node-1",
			},
			Status: v1.PodStatus{
				Phase:             "running",
				PodIP:             "10.1.1.1",
				ContainerStatuses: []v1.ContainerStatus{{Ready: true}},
			},
		}
		podInfo := LoadPodDetail(k8sClient, cfg, pod, "local")
		Expect(podInfo.Name).To(Equal("pod"))
		Expect(podInfo.Namespace).To(Equal("ns"))
		Expect(podInfo.Cluster).To(Equal("local"))
		Expect(podInfo.Ready).To(Equal("1/1"))
		Expect(podInfo.IP).To(Equal("10.1.1.1"))
		Expect(podInfo.NodeName).To(Equal("node-1"))
	})
})
