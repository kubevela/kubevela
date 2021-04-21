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

	"github.com/oam-dev/kubevela/references/apiserver/apis"
	"github.com/oam-dev/kubevela/references/apiserver/util"
)

var (

	// EnvInitContext used for test Env
	EnvInitContext = func(context string, envName string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should print environment initiation successful message", func() {
				cli := fmt.Sprintf("vela env init %s", envName)
				output, err := Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				expectedOutput := fmt.Sprintf("environment %s created,", envName)
				gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput))
			})
		})
	}

	EnvInitWithNamespaceOptionContext = func(context string, envName string, namespace string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should print environment initiation successful message", func() {
				cli := fmt.Sprintf("vela env init %s --namespace %s", envName, namespace)
				output, err := Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				expectedOutput := fmt.Sprintf("environment %s created,", envName)
				gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput))
			})
		})
	}

	JsonAppFileContext = func(context, jsonAppFile string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("Start the application through the app file in JSON format.", func() {
				writeStatus := ioutil.WriteFile("vela.json", []byte(jsonAppFile), 0644)
				gomega.Expect(writeStatus).NotTo(gomega.HaveOccurred())
				output, err := Exec("vela up -f vela.json")
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(output).NotTo(gomega.ContainSubstring("Error:"))
			})
		})
	}

	DeleteEnvFunc = func(context string, envName string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should print env does not exist message", func() {
				cli := fmt.Sprintf("vela env delete %s", envName)
				_, err := Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
			})
		})
	}

	EnvShowContext = func(context string, envName string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should show detailed environment message", func() {
				cli := fmt.Sprintf("vela env ls %s", envName)
				output, err := Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(output).To(gomega.ContainSubstring("NAME"))
				gomega.Expect(output).To(gomega.ContainSubstring("NAMESPACE"))
				gomega.Expect(output).To(gomega.ContainSubstring(envName))
			})
		})
	}

	EnvSetContext = func(context string, envName string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should show environment set message", func() {
				cli := fmt.Sprintf("vela env sw %s", envName)
				output, err := Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				expectedOutput := fmt.Sprintf("Set environment succeed, current environment is %s", envName)
				gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput))
			})
		})
	}

	EnvDeleteContext = func(context string, envName string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should delete an environment", func() {
				cli := fmt.Sprintf("vela env delete %s", envName)
				output, err := Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				expectedOutput := fmt.Sprintf("%s deleted", envName)
				gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput))
			})
		})
	}
	EnvDeleteCurrentUsingContext = func(context string, envName string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should delete all envs", func() {
				cli := fmt.Sprintf("vela env delete %s", envName)
				output, err := Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				expectedOutput := fmt.Sprintf("Error: you can't delete current using environment %s", envName)
				gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput))
			})
		})
	}

	WorkloadDeleteContext = func(context string, applicationName string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should print successful deletion information", func() {
				cli := fmt.Sprintf("vela delete %s", applicationName)
				output, err := Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(output).To(gomega.ContainSubstring("deleted from env"))
			})
		})
	}

	WorkloadCapabilityListContext = func() bool {
		return ginkgo.Context("list workload capabilities", func() {
			ginkgo.It("should sync capabilities from cluster before listing workload capabilities", func() {
				output, err := Exec("vela workloads")
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(output).To(gomega.ContainSubstring("webservice"))
			})
		})
	}

	TraitCapabilityListContext = func() bool {
		return ginkgo.Context("list traits capabilities", func() {
			ginkgo.It("should sync capabilities from cluster before listing trait capabilities", func() {
				output, err := Exec("vela traits")
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(output).To(gomega.ContainSubstring("scaler"))
			})
		})
	}

	// ComponentListContext used for test vela svc ls
	ComponentListContext = func(context string, applicationName string, workloadType string, traitAlias string) bool {
		return ginkgo.Context("ls", func() {
			ginkgo.It("should list all applications", func() {
				output, err := Exec("vela ls")
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(output).To(gomega.ContainSubstring("COMPONENT"))
				gomega.Expect(output).To(gomega.ContainSubstring(applicationName))
				gomega.Expect(output).To(gomega.ContainSubstring(workloadType))
				if traitAlias != "" {
					gomega.Expect(output).To(gomega.ContainSubstring(traitAlias))
				}
			})
		})
	}

	// APIEnvInitContext used for test api env
	APIEnvInitContext = func(context string, envMeta apis.Environment) bool {
		return ginkgo.Context("Post /envs/", func() {
			ginkgo.It("should create an env", func() {
				data, err := json.Marshal(&envMeta)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				resp, err := http.Post(util.URL("/envs/"), "application/json", strings.NewReader(string(data)))
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				defer resp.Body.Close()
				result, err := ioutil.ReadAll(resp.Body)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				var r apis.Response
				err = json.Unmarshal(result, &r)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(r.Code).Should(gomega.Equal(http.StatusOK))
				gomega.Expect(r.Data.(string)).To(gomega.ContainSubstring("created"))
			})
		})
	}

	ShowCapabilityReference = func(context string, capabilityName string) bool {
		return ginkgo.Context(context, func() {
			ginkgo.It("should show capability reference", func() {
				cli := fmt.Sprintf("vela show %s", capabilityName)
				_, err := Exec(cli)
				gomega.Expect(err).Should(gomega.BeNil())
			})
		})
	}
)
