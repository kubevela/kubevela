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
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver"
	"github.com/oam-dev/kubevela/pkg/apiserver/config"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/service"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
)

var k8sClient client.Client
var token string

const (
	baseDomain   = "http://127.0.0.1:8000"
	baseURL      = "http://127.0.0.1:8000/api/v1"
	testNSprefix = "api-e2e-test-"
)

func TestE2eApiserverTest(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2eApiserverTest Suite")
}

// Suite test in e2e-apiserver-test relies on the pre-setup kubernetes environment
var _ = BeforeSuite(func() {

	ctx := context.Background()

	cfg := config.Config{
		BindAddr: "127.0.0.1:8000",
		Datastore: datastore.Config{
			Type:     "kubeapi",
			Database: "kubevela",
		},
		AddonCacheTime: 10 * time.Minute,
		KubeQPS:        100,
		KubeBurst:      300,
	}
	cfg.LeaderConfig.ID = uuid.New().String()
	cfg.LeaderConfig.LockName = "apiserver-lock"
	cfg.LeaderConfig.Duration = time.Second * 10

	server := apiserver.New(cfg)
	Expect(server).ShouldNot(BeNil())
	go func() {
		err := server.Run(ctx, make(chan error))
		Expect(err).ShouldNot(HaveOccurred())
	}()
	By("wait for api server to start")
	Eventually(
		func() error {
			password := os.Getenv("VELA_UX_PASSWORD")
			if password == "" {
				password = service.InitAdminPassword
			}
			var req = apisv1.LoginRequest{
				Username: "admin",
				Password: password,
			}
			bodyByte, err := json.Marshal(req)
			Expect(err).Should(BeNil())
			resp, err := http.Post("http://127.0.0.1:8000/api/v1/auth/login", "application/json", bytes.NewBuffer(bodyByte))
			if err != nil {
				return err
			}
			if resp.StatusCode == 200 {
				loginResp := &apisv1.LoginResponse{}
				err = json.NewDecoder(resp.Body).Decode(loginResp)
				Expect(err).Should(BeNil())
				token = "Bearer " + loginResp.AccessToken
				var req = apisv1.CreateProjectRequest{
					Name:        appProject,
					Description: "test project",
				}
				_ = post("/projects", req)
				return nil
			}
			code := &bcode.Bcode{}
			err = json.NewDecoder(resp.Body).Decode(code)
			Expect(err).Should(BeNil())
			return fmt.Errorf("rest service not ready code:%d message:%s", resp.StatusCode, code.Message)
		}, time.Second*10, time.Millisecond*200).Should(BeNil())
	var err error
	k8sClient, err = clients.GetKubeClient()
	Expect(err).ShouldNot(HaveOccurred())
	By("api server started")
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	var nsList v1.NamespaceList
	if k8sClient != nil {
		err := k8sClient.List(context.TODO(), &nsList)
		Expect(err).ToNot(HaveOccurred())
		for _, ns := range nsList.Items {
			if strings.HasPrefix(ns.Name, testNSprefix) {
				_ = k8sClient.Delete(context.TODO(), &ns)
			}
		}
	}
})

func post(path string, body interface{}) *http.Response {
	b, err := json.Marshal(body)
	Expect(err).Should(BeNil())
	client := &http.Client{}
	if !strings.HasPrefix(path, "/v1") {
		path = baseURL + path
	} else {
		path = baseDomain + path
	}
	req, err := http.NewRequest(http.MethodPost, path, bytes.NewBuffer(b))
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
	if !strings.HasPrefix(path, "/v1") {
		path = baseURL + path
	} else {
		path = baseDomain + path
	}
	req, err := http.NewRequest(http.MethodPut, path, bytes.NewBuffer(b))
	Expect(err).Should(BeNil())
	req.Header.Add("Authorization", token)
	req.Header.Set("Content-Type", "application/json")

	response, err := client.Do(req)
	Expect(err).Should(BeNil())
	return response
}

func get(path string) *http.Response {
	client := &http.Client{}
	if !strings.HasPrefix(path, "/v1") {
		path = baseURL + path
	} else {
		path = baseDomain + path
	}
	req, err := http.NewRequest(http.MethodGet, path, nil)
	Expect(err).Should(BeNil())
	req.Header.Add("Authorization", token)

	response, err := client.Do(req)
	Expect(err).Should(BeNil())
	return response
}

func delete(path string) *http.Response {
	client := &http.Client{}
	if !strings.HasPrefix(path, "/v1") {
		path = baseURL + path
	} else {
		path = baseDomain + path
	}
	req, err := http.NewRequest(http.MethodDelete, path, nil)
	Expect(err).Should(BeNil())
	req.Header.Add("Authorization", token)
	response, err := client.Do(req)
	Expect(err).Should(BeNil())
	return response
}

func decodeResponseBody(resp *http.Response, dst interface{}) error {
	if resp.Body == nil {
		return fmt.Errorf("response body is nil")
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if dst != nil {
		err = json.Unmarshal(body, dst)
		if err != nil {
			return err
		}
		return nil
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("response code is not 200: %d body: %s", resp.StatusCode, string(body))
	}
	return nil
}
