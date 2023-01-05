/*
Copyright 2023 The KubeVela Authors.

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
	"errors"
	"fmt"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
)

var _ = Describe("Test the rest api that rollback the application", func() {
	var appName = "test-rollback"
	var envName = "rollback"
	var revision = ""
	var waitAppReady = func(v string) apisv1.ApplicationStatusResponse {
		var sr apisv1.ApplicationStatusResponse
		Eventually(func() error {
			res := get("/applications/" + appName + "/envs/" + envName + "/status")
			if err := decodeResponseBody(res, &sr); err != nil {
				return err
			}
			if sr.Status == nil {
				return errors.New("the application is not running")
			}
			if sr.Status.Phase != common.ApplicationRunning {
				return fmt.Errorf("the application status is %s, not running", sr.Status.Phase)
			}
			return nil
		}).WithTimeout(time.Minute * 1).WithPolling(3 * time.Second).Should(BeNil())
		Eventually(func() error {
			res := get("/applications/" + appName + "/revisions/" + v)
			var version apisv1.DetailRevisionResponse
			if err := decodeResponseBody(res, &version); err != nil {
				return err
			}
			if version.Status == model.RevisionStatusComplete {
				return nil
			}
			return fmt.Errorf("the application revision status is %s, not complete", version.Status)
		}).WithTimeout(time.Minute * 1).WithPolling(3 * time.Second).Should(BeNil())
		return sr
	}

	var deployApp = func() string {
		var req = apisv1.ApplicationDeployRequest{
			Note:         "test apply",
			TriggerType:  "web",
			WorkflowName: "workflow-" + envName,
			Force:        false,
		}
		res := post("/applications/"+appName+"/deploy", req)
		var response apisv1.ApplicationDeployResponse
		Expect(decodeResponseBody(res, &response)).Should(Succeed())
		Expect(cmp.Diff(response.Status, model.RevisionStatusRunning)).Should(BeEmpty())
		return response.ApplicationRevisionBase.Version
	}
	It("Prepare the environment", func() {
		ct := apisv1.CreateTargetRequest{
			Name:    "rollback",
			Project: appProject,
			Cluster: &apisv1.ClusterTarget{
				ClusterName: "local",
				Namespace:   "rollback",
			},
		}
		res := post("/targets", ct)
		var targetBase apisv1.TargetBase
		Expect(decodeResponseBody(res, &targetBase)).Should(Succeed())

		ce := apisv1.CreateEnvRequest{
			Name:      envName,
			Project:   appProject,
			Namespace: "rollback",
			Targets:   []string{targetBase.Name},
		}

		env := post("/envs", ce)
		var envRes apisv1.Env
		Expect(decodeResponseBody(env, &envRes)).Should(Succeed())
	})
	It("Prepare the test application", func() {
		var req = apisv1.CreateApplicationRequest{
			Name:        appName,
			Project:     appProject,
			Description: "this is an application to testing the rollback",
			Icon:        "",
			Labels:      map[string]string{"test": "true"},
			EnvBinding:  []*apisv1.EnvBinding{{Name: envName}},
			Component: &apisv1.CreateComponentRequest{
				Name:          "webservice",
				ComponentType: "webservice",
				Properties:    "{\"image\":\"nginx\"}",
			},
		}
		res := post("/applications", req)
		var appBase apisv1.ApplicationBase
		Expect(decodeResponseBody(res, &appBase)).Should(Succeed())
	})
	It("Test rollback by the cluster revision", func() {
		// first deploy
		firstRevision := deployApp()
		status := waitAppReady(firstRevision)
		Expect(len(status.Status.Services)).Should(Equal(1))
		var createComponent = apisv1.CreateComponentRequest{
			Name:          "component2",
			ComponentType: "webservice",
			Properties:    `{"image": "nginx","ports": [{"port": 80, "expose": true}]}`,
		}
		res := post("/applications/"+appName+"/components", createComponent)
		var cb apisv1.ComponentBase
		Expect(decodeResponseBody(res, &cb)).Should(Succeed())
		Expect(cmp.Diff(cb.Name, "component2")).Should(BeEmpty())
		// second deploy
		revision = deployApp()
		status = waitAppReady(revision)
		Expect(len(status.Status.Services)).Should(Equal(2))

		// rollback to first revision
		res = post("/applications/"+appName+"/revisions/"+firstRevision+"/rollback", nil)
		var rollback apisv1.ApplicationRollbackResponse
		Expect(decodeResponseBody(res, &rollback)).Should(Succeed())
		Expect(rollback.WorkflowRecord.Name).ShouldNot(BeEmpty())
		status = waitAppReady(firstRevision)
		Expect(len(status.Status.Services)).Should(Equal(1))
	})
	It("Test rollback by the local revision", func() {
		res := post("/applications/"+appName+"/envs/"+envName+"/recycle", nil)
		Expect(decodeResponseBody(res, nil)).Should(Succeed())

		Eventually(func() error {
			var sr apisv1.ApplicationStatusResponse
			res := get("/applications/" + appName + "/envs/" + envName + "/status")
			if err := decodeResponseBody(res, &sr); err != nil {
				return err
			}
			if sr.Status == nil {
				return nil
			}
			return fmt.Errorf("the application status is %s", sr.Status.Phase)
		}).WithTimeout(time.Minute * 1).WithPolling(3 * time.Second).Should(BeNil())

		Expect(revision).ShouldNot(BeEmpty())

		res = post("/applications/"+appName+"/revisions/"+revision+"/rollback", nil)
		var rollback apisv1.ApplicationRollbackResponse
		Expect(decodeResponseBody(res, &rollback)).Should(Succeed())
		Expect(rollback.WorkflowRecord.Name).ShouldNot(BeEmpty())
		status := waitAppReady(revision)
		Expect(len(status.Status.Services)).Should(Equal(2))
	})
})
