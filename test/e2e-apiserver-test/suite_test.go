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

package e2e_apiserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	arest "github.com/oam-dev/kubevela/pkg/apiserver/rest"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	e2e_apiserver "github.com/oam-dev/kubevela/test/e2e-apiserver-test"
)

var k8sClient client.Client
var token string

func TestE2eApiserverTest(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2eApiserverTest Suite")
}

// Suite test in e2e-apiserver-test relies on the pre-setup kubernetes environment
var _ = BeforeSuite(func() {
	By("new kube client")
	var err error
	k8sClient, err = clients.GetKubeClient()
	Expect(err).Should(BeNil())
	Expect(k8sClient).ToNot(BeNil())
	By("new kube client success")

	ctx := context.Background()

	cfg := arest.Config{
		BindAddr: "127.0.0.1:8000",
		Datastore: datastore.Config{
			Type:     "kubeapi",
			Database: "kubevela",
		},
		AddonCacheTime: 10 * time.Minute,
	}
	cfg.LeaderConfig.ID = uuid.New().String()
	cfg.LeaderConfig.LockName = "apiserver-lock"
	cfg.LeaderConfig.Duration = time.Second * 10

	server, err := arest.New(cfg)
	Expect(err).ShouldNot(HaveOccurred())
	Expect(server).ShouldNot(BeNil())
	go func() {
		err = server.Run(ctx)
		Expect(err).ShouldNot(HaveOccurred())
	}()
	By("wait for api server to start")
	Eventually(
		func() error {
			secret := &v1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "admin", Namespace: "vela-system"}, secret)
			if err != nil {
				return err
			}
			var req = apisv1.LoginRequest{
				Username: "admin",
				Password: string(secret.Data["admin"]),
			}
			bodyByte, err := json.Marshal(req)
			Expect(err).Should(BeNil())
			resp, err := http.Post("http://127.0.0.1:8000/api/v1/auth/login", "application/json", bytes.NewBuffer(bodyByte))
			if err != nil {
				return err
			}
			loginResp := &apisv1.LoginResponse{}
			err = json.NewDecoder(resp.Body).Decode(loginResp)
			Expect(err).Should(BeNil())
			token = "Bearer " + loginResp.AccessToken
			if resp.StatusCode == http.StatusOK {
				var req = apisv1.CreateProjectRequest{
					Name:        appProject,
					Description: "test project",
				}
				_ = post("/projects", req)
				return nil
			}
			return errors.New("rest service not ready")
		}, time.Second*5, time.Millisecond*200).Should(BeNil())
	By("api server started")
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	var nsList v1.NamespaceList
	err := k8sClient.List(context.TODO(), &nsList)
	Expect(err).ToNot(HaveOccurred())
	for _, ns := range nsList.Items {
		if strings.HasPrefix(ns.Name, e2e_apiserver.TestNSprefix) {
			_ = k8sClient.Delete(context.TODO(), &ns)
		}
	}
})

const (
	baseURL      = "http://127.0.0.1:8000/api/v1"
	testNSprefix = "api-e2e-test-"
)

func post(path string, body interface{}) *http.Response {
	b, err := json.Marshal(body)
	Expect(err).Should(BeNil())
	client := &http.Client{}
	req, err := http.NewRequest(http.MethodPost, baseURL+path, bytes.NewBuffer(b))
	Expect(err).Should(BeNil())
	req.Header.Add("Authorization", token)
	req.Header.Add("Content-Type", "application/json")

	response, err := client.Do(req)
	Expect(err).Should(BeNil())
	return response
}

func put(path string, body interface{}) *http.Response {
	b, err := json.Marshal(body)
	Expect(err).Should(BeNil())
	client := &http.Client{}
	req, err := http.NewRequest(http.MethodPut, baseURL+path, bytes.NewBuffer(b))
	Expect(err).Should(BeNil())
	req.Header.Add("Authorization", token)
	req.Header.Set("Content-Type", "application/json")

	response, err := client.Do(req)
	Expect(err).Should(BeNil())
	return response
}

func get(path string) *http.Response {
	client := &http.Client{}
	req, err := http.NewRequest(http.MethodGet, baseURL+path, nil)
	Expect(err).Should(BeNil())
	req.Header.Add("Authorization", token)

	response, err := client.Do(req)
	Expect(err).Should(BeNil())
	return response
}

func delete(path string) *http.Response {
	client := &http.Client{}
	req, err := http.NewRequest(http.MethodDelete, baseURL+path, nil)
	Expect(err).Should(BeNil())
	req.Header.Add("Authorization", token)

	response, err := client.Do(req)
	Expect(err).Should(BeNil())
	defer response.Body.Close()
	return response
}

func decodeResponseBody(resp *http.Response, dst interface{}) error {
	if resp.StatusCode != 200 {
		return fmt.Errorf("response code is not 200: %d", resp.StatusCode)
	}
	if resp.Body == nil {
		return fmt.Errorf("response body is nil")
	}
	if dst != nil {
		err := json.NewDecoder(resp.Body).Decode(dst)
		Expect(err).Should(BeNil())
		return resp.Body.Close()
	}
	return resp.Body.Close()
}
