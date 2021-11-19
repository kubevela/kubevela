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
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	v1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	util "github.com/oam-dev/kubevela/pkg/utils"
)

const (
	WorkerClusterName           = "cluster-worker"
	WorkerClusterKubeConfigPath = "/tmp/worker.kubeconfig"
)

var _ = Describe("Test cluster rest api", func() {

	Context("Test basic cluster CURD", func() {

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

		It("Test list clusters", func() {
			resp, err := CreateRequest(http.MethodGet, "/clusters/?page=1&pageSize=5", nil)
			clusterResp := &v1.ListClusterResponse{}
			Expect(DecodeResponseBody(resp, err, clusterResp)).Should(Succeed())
			Expect(len(clusterResp.Clusters) >= 2).Should(BeTrue())
			Expect(clusterResp.Clusters[0].Name).Should(Equal(multicluster.ClusterLocalName))
			Expect(clusterResp.Clusters[1].Name).Should(Equal(clusterName))
			resp, err = CreateRequest(http.MethodGet, "/clusters/?page=1&pageSize=5&query="+WorkerClusterName, nil)
			clusterResp = &v1.ListClusterResponse{}
			Expect(DecodeResponseBody(resp, err, clusterResp)).Should(Succeed())
			Expect(len(clusterResp.Clusters) >= 1).Should(BeTrue())
			Expect(clusterResp.Clusters[0].Name).Should(Equal(clusterName))
		})

		It("Test modify cluster", func() {
			kubeconfigBytes, err := ioutil.ReadFile(WorkerClusterKubeConfigPath)
			Expect(err).Should(Succeed())
			resp, err := CreateRequest(http.MethodPut, "/clusters/"+clusterName, v1.CreateClusterRequest{
				Name:        clusterName,
				KubeConfig:  string(kubeconfigBytes),
				Description: "Example description",
			})
			clusterResp := &v1.ClusterBase{}
			Expect(DecodeResponseBody(resp, err, clusterResp)).Should(Succeed())
			Expect(clusterResp.Description).ShouldNot(Equal(""))
		})

		It("Test create ns in cluster", func() {
			testNamespace := fmt.Sprintf("test-%d", time.Now().Unix())
			resp, err := CreateRequest(http.MethodPost, "/clusters/"+clusterName+"/namespaces", v1.CreateClusterNamespaceRequest{Namespace: testNamespace})
			Expect(err).Should(Succeed())
			nsResp := &v1.CreateClusterNamespaceResponse{}
			Expect(DecodeResponseBody(resp, err, nsResp)).Should(Succeed())
			Expect(nsResp.Exists).Should(Equal(false))
			resp, err = CreateRequest(http.MethodPost, "/clusters/"+clusterName+"/namespaces", v1.CreateClusterNamespaceRequest{Namespace: testNamespace})
			Expect(err).Should(Succeed())
			nsResp = &v1.CreateClusterNamespaceResponse{}
			Expect(DecodeResponseBody(resp, err, nsResp)).Should(Succeed())
			Expect(nsResp.Exists).Should(Equal(true))
		})

	})

	PContext("Test cloud cluster rest api", func() {

		var clusterName string

		BeforeEach(func() {
			clusterName = WorkerClusterName + "-" + util.RandomString(8)
		})

		AfterEach(func() {
			resp, err := CreateRequest(http.MethodDelete, "/clusters/"+clusterName, nil)
			Expect(err).Should(Succeed())
			Expect(resp.StatusCode).Should(Equal(200))
			Expect(resp.Body).ShouldNot(BeNil())
			Expect(resp.Body.Close()).Should(Succeed())
		})

		It("Test list aliyun cloud cluster and connect", func() {
			AccessKeyID := os.Getenv("ALIYUN_ACCESS_KEY_ID")
			AccessKeySecret := os.Getenv("ALIYUN_ACCESS_KEY_SECRET")
			resp, err := CreateRequest(http.MethodPost, "/clusters/cloud-clusters/aliyun/?page=1&pageSize=5", v1.AccessKeyRequest{
				AccessKeyID:     AccessKeyID,
				AccessKeySecret: AccessKeySecret,
			})
			clusterResp := &v1.ListCloudClusterResponse{}
			Expect(DecodeResponseBody(resp, err, clusterResp)).Should(Succeed())
			Expect(len(clusterResp.Clusters)).ShouldNot(Equal(0))

			ClusterID := clusterResp.Clusters[0].ID
			resp, err = CreateRequest(http.MethodPost, "/clusters/cloud-clusters/aliyun/connect", v1.ConnectCloudClusterRequest{
				AccessKeyID:     AccessKeyID,
				AccessKeySecret: AccessKeySecret,
				ClusterID:       ClusterID,
				Name:            clusterName,
			})
			clusterBase := &v1.ClusterBase{}
			Expect(DecodeResponseBody(resp, err, clusterBase)).Should(Succeed())
			Expect(clusterBase.Status).Should(Equal("Healthy"))
		})

	})
})
