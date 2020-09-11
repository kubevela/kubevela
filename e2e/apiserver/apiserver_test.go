package e2e

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/oam-dev/kubevela/e2e"

	"github.com/oam-dev/kubevela/pkg/server/util"

	"github.com/oam-dev/kubevela/pkg/server/apis"

	"github.com/onsi/gomega"

	"github.com/onsi/ginkgo"
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

	workloadType = "containerized"
	workloadName = "app-e2e-api-hello"

	workloadRunBodyWithoutImageFlag = apis.WorkloadRunBody{
		EnvName:      envHelloMeta.EnvName,
		WorkloadName: workloadName,
		WorkloadType: workloadType,
		Flags:        []apis.CommonFlag{{Name: "port", Value: "80"}},
	}
	workloadRunBody = apis.WorkloadRunBody{
		EnvName:      envHelloMeta.EnvName,
		WorkloadName: workloadName,
		WorkloadType: workloadType,
		Flags:        []apis.CommonFlag{{Name: "image", Value: "nginx:1.9.4"}, {Name: "port", Value: "80"}},
	}
)

var notExistedEnvMeta = apis.Environment{
	EnvName:   "env-e2e-api-NOT-EXISTED-JUST-FOR-TEST",
	Namespace: "env-e2e-api-NOT-EXISTED-JUST-FOR-TEST",
}

var containerizedWorkloadType = "containerized"
var deploymentWorkloadType = "deployment"

var _ = ginkgo.Describe("API", func() {
	//API Env
	e2e.APIEnvInitContext("post /envs/", envHelloMeta)

	ginkgo.Context("get /envs/:envName", func() {
		ginkgo.It("should get an env", func() {
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

	ginkgo.Context("switch /envs/:envName", func() {
		ginkgo.It("should switch an env", func() {
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
			content := fmt.Sprintf("Switch env succeed, current env is " + envHelloMeta.EnvName)
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
		ginkgo.It("run workload", func() {
			data, err := json.Marshal(&workloadRunBody)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			resp, err := http.Post(util.URL("/workloads/"), "application/json", strings.NewReader(string(data)))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			defer resp.Body.Close()
			result, err := ioutil.ReadAll(resp.Body)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			var r apis.Response
			err = json.Unmarshal(result, &r)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(http.StatusOK).Should(gomega.Equal(r.Code), string(result))
			output := fmt.Sprintf("Creating App %s\nSUCCEED", workloadName)
			gomega.Expect(r.Data.(string)).To(gomega.ContainSubstring(output))
		})

		ginkgo.It("run workload without compulsory flag", func() {
			data, err := json.Marshal(&workloadRunBodyWithoutImageFlag)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			resp, err := http.Post(util.URL("/workloads/"), "application/json", strings.NewReader(string(data)))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			defer resp.Body.Close()
			result, err := ioutil.ReadAll(resp.Body)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			var r apis.Response
			err = json.Unmarshal(result, &r)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(http.StatusInternalServerError).Should(gomega.Equal(r.Code))
			output := "required flag(s) \"image\" not set"
			gomega.Expect(r.Data.(string)).To(gomega.ContainSubstring(output))
		})

		ginkgo.It("should list all WorkloadDefinitions", func() {
			resp, err := http.Get(util.URL("/workloads/"))
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
				gomega.Expect([]string{containerizedWorkloadType, deploymentWorkloadType}).To(gomega.Or(gomega.ContainElement(workloadDefinition["name"])))
			}
		})

		ginkgo.It("should delete an application", func() {
			req, err := http.NewRequest("DELETE", util.URL("/envs/"+envHelloMeta.EnvName+"/apps/"+workloadRunBody.WorkloadName), nil)
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
			gomega.Expect(r.Data.(string)).To(gomega.ContainSubstring("delete apps succeed"))
		})
	})
})
