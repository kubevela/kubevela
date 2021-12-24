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
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	arest "github.com/oam-dev/kubevela/pkg/apiserver/rest"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	e2e_apiserver "github.com/oam-dev/kubevela/test/e2e-apiserver-test"
)

var k8sClient client.Client

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
			res, err := http.Get("http://127.0.0.1:8000/api/v1/projects")
			if err != nil {
				return err
			}
			if res.StatusCode == http.StatusOK {
				var req = apisv1.CreateProjectRequest{
					Name:        appProject,
					Description: "test project",
				}
				bodyByte, err := json.Marshal(req)
				Expect(err).ShouldNot(HaveOccurred())
				_, err = http.Post("http://127.0.0.1:8000/api/v1/projects", "application/json", bytes.NewBuffer(bodyByte))
				Expect(err).ShouldNot(HaveOccurred())
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
