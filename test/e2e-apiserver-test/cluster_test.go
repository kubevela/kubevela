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

package e2e_apiserver

import (
	"io/ioutil"
	"net/http"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	v1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	util "github.com/oam-dev/kubevela/pkg/utils"
)

const (
	WorkerClusterName           = "cluster-worker"
	WorkerClusterKubeConfigPath = "/tmp/worker.kubeconfig"
)

var _ = Describe("Test cluster rest api", func() {

	var clusterName string

	BeforeEach(func() {
		clusterName = WorkerClusterName + "-" + util.RandomString(8)
		kubeconfigBytes, err := ioutil.ReadFile(WorkerClusterKubeConfigPath)
		Expect(err).Should(Succeed())
		resp, err := CreateRequest(http.MethodPost, "/clusters", v1.CreateClusterRequest{
			Name:       clusterName,
			KubeConfig: string(kubeconfigBytes),
		})
		Expect(err).Should(Succeed())
		Expect(resp.StatusCode).Should(Equal(200))
		Expect(resp.Body).ShouldNot(BeNil())
		Expect(resp.Body.Close()).Should(Succeed())
	})

	AfterEach(func() {
		resp, err := CreateRequest(http.MethodDelete, "/clusters/"+clusterName, nil)
		Expect(err).Should(Succeed())
		Expect(resp.StatusCode).Should(Equal(200))
		Expect(resp.Body).ShouldNot(BeNil())
		Expect(resp.Body.Close()).Should(Succeed())
	})

	It("Test get cluster", func() {
		resp, err := CreateRequest(http.MethodGet, "/clusters/"+clusterName, nil)
		clusterResp := &v1.DetailClusterResponse{}
		Expect(DecodeResponseBody(resp, err, clusterResp)).Should(Succeed())
		Expect(clusterResp.Status).Should(Equal("Healthy"))
	})

	It("Test get clusters", func() {
		resp, err := CreateRequest(http.MethodGet, "/clusters/?page=1&pageSize=5", nil)
		clusterResp := &v1.ListClusterResponse{}
		Expect(DecodeResponseBody(resp, err, clusterResp)).Should(Succeed())
		Expect(len(clusterResp.Clusters)).ShouldNot(Equal(0))
	})

	It("Test modify cluster", func() {
		kubeconfigBytes, err := ioutil.ReadFile(WorkerClusterKubeConfigPath)
		Expect(err).Should(Succeed())
		resp, err := CreateRequest(http.MethodPost, "/clusters/"+clusterName, v1.CreateClusterRequest{
			Name:        clusterName,
			KubeConfig:  string(kubeconfigBytes),
			Description: "Example description",
		})
		clusterResp := &v1.ClusterBase{}
		Expect(DecodeResponseBody(resp, err, clusterResp)).Should(Succeed())
		Expect(clusterResp.Description).ShouldNot(Equal(""))
	})
})
