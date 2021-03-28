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

package e2e

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/e2e"
	"github.com/oam-dev/kubevela/references/apiserver/apis"
	"github.com/oam-dev/kubevela/references/apiserver/util"
	"github.com/oam-dev/kubevela/references/appfile/api"
)

var (
	envHelloMeta = apis.Environment{
		EnvName:   "env-e2e-hello",
		Namespace: "env-e2e-hello",
	}

	envWorldMeta = apis.Environment{
		EnvName:   "env-e2e-world",
		Namespace: "env-e2e-world",
	}

	envWorldMetaUpdate = apis.EnvironmentBody{
		Namespace: "env-e2e-world-modified",
	}

	workloadType    = "webservice"
	applicationName = "app-e2e-api-hello"
	svcName         = "svc-e2e-api-hello"

	applicationCreationBodyWithoutImageFlag = api.AppFile{
		Name: applicationName,
		Services: map[string]api.Service{
			svcName: map[string]interface{}{},
		},
	}

	applicationCreationBody = api.AppFile{
		Name: applicationName,
		Services: map[string]api.Service{
			svcName: map[string]interface{}{
				"type":  workloadType,
				"image": "wordpress:php7.4-apache",
				"port":  "80",
				"cpu":   "1",
			},
		},
	}
)

var notExistedEnvMeta = apis.Environment{
	EnvName:   "env-e2e-api-NOT-EXISTED-JUST-FOR-TEST",
	Namespace: "env-e2e-api-NOT-EXISTED-JUST-FOR-TEST",
}

var webserviceWorkloadType = "webservice"
var workerWorkloadType = "worker"
var taskWorkloadType = "task"

var _ = ginkgo.Describe("API", func() {
	//API Env
	e2e.APIEnvInitContext("post /envs/", envHelloMeta)

	ginkgo.Context("get /envs/:envName", func() {
		ginkgo.It("should get an environment", func() {
			resp, err := http.Get(util.URL("/envs/" + envHelloMeta.EnvName))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			defer resp.Body.Close()
			result, err := ioutil.ReadAll(resp.Body)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			var r apis.Response
			err = json.Unmarshal(result, &r)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(http.StatusOK).To(gomega.Equal(r.Code))
			//TODO(zzxwill) Need to compare r.Data with envMeta
		})
	})

	e2e.APIEnvInitContext("post /envs/", envWorldMeta)

	ginkgo.Context("set /envs/:envName", func() {
		ginkgo.It("should set an environment as the currently using one", func() {
			req, err := http.NewRequest("PATCH", util.URL("/envs/"+envHelloMeta.EnvName), nil)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			resp, err := http.DefaultClient.Do(req)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			defer resp.Body.Close()
			result, err := ioutil.ReadAll(resp.Body)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			var r apis.Response
			err = json.Unmarshal(result, &r)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(http.StatusOK).To(gomega.Equal(r.Code))
			content := fmt.Sprintf("Set environment succeed, current environment is " + envHelloMeta.EnvName)
			gomega.Expect(r.Data.(string)).To(gomega.ContainSubstring(content))
		})
	})

	ginkgo.Context("get /envs/", func() {
		ginkgo.It("should get an env", func() {
			resp, err := http.Get(util.URL("/envs/"))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			defer resp.Body.Close()
			result, err := ioutil.ReadAll(resp.Body)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			var r apis.Response
			err = json.Unmarshal(result, &r)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(http.StatusOK).To(gomega.Equal(r.Code))
			//TODO(zzxwill) Need to compare r.Data with envMeta
		})
	})

	ginkgo.Context("put /envs/:envName", func() {
		ginkgo.It("should update an env", func() {
			data, _ := json.Marshal(&envWorldMetaUpdate)
			req, err := http.NewRequest("PUT", util.URL("/envs/"+envWorldMeta.EnvName), strings.NewReader(string(data)))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			resp, err := http.DefaultClient.Do(req)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			defer resp.Body.Close()
			result, err := ioutil.ReadAll(resp.Body)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			var r apis.Response
			err = json.Unmarshal(result, &r)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(http.StatusOK).To(gomega.Equal(r.Code))
			gomega.Expect(r.Data.(string)).To(gomega.ContainSubstring("Update env succeed"))
		})
	})

	ginkgo.Context("delete /envs/:envName", func() {
		ginkgo.It("should delete an env", func() {
			req, err := http.NewRequest("DELETE", util.URL("/envs/"+envWorldMeta.EnvName), nil)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			resp, err := http.DefaultClient.Do(req)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			defer resp.Body.Close()
			result, err := ioutil.ReadAll(resp.Body)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			var r apis.Response
			err = json.Unmarshal(result, &r)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(http.StatusOK).To(gomega.Equal(r.Code))
			gomega.Expect(r.Data.(string)).To(gomega.ContainSubstring(envWorldMeta.EnvName + " deleted"))
		})
	})

	// API Application
	ginkgo.Context("get /envs/:envName/apps/", func() {
		ginkgo.It("should report error for not existed env", func() {
			envName := notExistedEnvMeta.EnvName
			url := fmt.Sprintf("/envs/%s/apps/", envName)
			resp, err := http.Get(util.URL(url))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			defer resp.Body.Close()
			result, err := ioutil.ReadAll(resp.Body)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			var r apis.Response
			err = json.Unmarshal(result, &r)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(http.StatusInternalServerError).To(gomega.Equal(r.Code))
			expectedContent := fmt.Sprintf("env %s not exist", envName)
			gomega.Expect(r.Data.(string)).To(gomega.ContainSubstring(expectedContent))
		})
	})

	ginkgo.Context("Workloads", func() {
		ginkgo.It("create an application", func() {
			data, err := json.Marshal(&applicationCreationBody)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			url := fmt.Sprintf("/envs/%s/apps/", envHelloMeta.EnvName)
			resp, err := http.Post(util.URL(url), "application/json", strings.NewReader(string(data)))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			defer resp.Body.Close()
			result, err := ioutil.ReadAll(resp.Body)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			var r apis.Response
			err = json.Unmarshal(result, &r)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(http.StatusOK).Should(gomega.Equal(r.Code), string(result))
			output := fmt.Sprintf("application %s is successfully created", applicationName)
			gomega.Expect(r.Data.(string)).To(gomega.ContainSubstring(output))
		})

		ginkgo.It("create an application without compulsory flag of a service", func() {
			data, err := json.Marshal(&applicationCreationBodyWithoutImageFlag)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			url := fmt.Sprintf("/envs/%s/apps/", envHelloMeta.EnvName)
			resp, err := http.Post(util.URL(url), "application/json", strings.NewReader(string(data)))
			// TODO(zzxwill) revise the check process if we need to work on https://github.com/oam-dev/kubevela/discussions/933
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			defer resp.Body.Close()
			result, err := ioutil.ReadAll(resp.Body)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			var r apis.Response
			err = json.Unmarshal(result, &r)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(http.StatusOK).Should(gomega.Equal(r.Code), string(result))
			output := fmt.Sprintf("application %s is successfully created", applicationName)
			gomega.Expect(r.Data.(string)).To(gomega.ContainSubstring(output))
		})

		ginkgo.It("should list all ComponentDefinitions", func() {
			resp, err := http.Get(util.URL("/components/"))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			defer resp.Body.Close()
			result, err := ioutil.ReadAll(resp.Body)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			var r apis.Response
			err = json.Unmarshal(result, &r)
			gomega.Expect(http.StatusOK).To(gomega.Equal(r.Code))
			var data = r.Data.([]interface{})
			for _, i := range data {
				var workloadDefinition = i.(map[string]interface{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect([]string{webserviceWorkloadType, workerWorkloadType, taskWorkloadType}).To(gomega.Or(gomega.ContainElement(workloadDefinition["name"])))
			}
		})

		ginkgo.It("should delete an application", func() {
			req, err := http.NewRequest("DELETE", util.URL("/envs/"+envHelloMeta.EnvName+"/apps/"+applicationName), nil)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			resp, err := http.DefaultClient.Do(req)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			defer resp.Body.Close()
			result, err := ioutil.ReadAll(resp.Body)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			var r apis.Response
			err = json.Unmarshal(result, &r)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(http.StatusOK).To(gomega.Equal(r.Code))
			gomega.Expect(r.Data.(string)).To(gomega.ContainSubstring("deleted from env"))
		})
	})
})
