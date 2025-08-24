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
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Netflix/go-expect"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

// execCommandString executes a shell command string and returns its output and error
func execCommandString(cmdString string) (string, error) {
	// Simple command parsing - this doesn't handle complex quoting, but works for basic commands
	parts := strings.Fields(cmdString)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty command string")
	}

	// Create a new command
	cmd := exec.Command(parts[0], parts[1:]...)

	// Set PATH to include the current directory for finding the vela binary
	// This is needed for e2e tests where vela might be in the current working directory
	currentPath := os.Getenv("PATH")
	currentDir, err := os.Getwd()
	if err == nil {
		cmd.Env = append(os.Environ(), fmt.Sprintf("PATH=%s:%s", currentDir, currentPath))
	}

	return ExecCommand(cmd)
}

var (

	// EnvInitContext used for test Env
	EnvInitContext = func(context string, envName string) bool {
		return ginkgo.It(context+": should print environment initiation successful message", func() {
			cli := fmt.Sprintf("vela env init %s", envName)
			var answer = "default"
			if envName != "env-application" {
				answer = "vela-system"
			}
			output, err := InteractiveExec(cli, func(c *expect.Console) {
				data := []struct {
					q, a string
				}{
					{
						q: "Would you like to choose an existing namespaces as your env?",
						a: answer,
					},
				}
				for _, qa := range data {
					_, err := c.ExpectString(qa.q)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
					_, err = c.SendLine(qa.a)
					gomega.Expect(err).NotTo(gomega.HaveOccurred())
				}
				c.ExpectEOF()
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			expectedOutput := fmt.Sprintf("environment %s with namespace %s created", envName, answer)
			gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput))
		})
	}

	EnvInitWithNamespaceOptionContext = func(context string, envName string, namespace string) bool {
		return ginkgo.It(context+": should print environment initiation successful message", func() {
			cli := fmt.Sprintf("vela env init %s --namespace %s", envName, namespace)
			output, err := execCommandString(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			expectedOutput := fmt.Sprintf("environment %s with namespace %s created", envName, namespace)
			gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput))
		})
	}

	JsonAppFileContext = func(context, jsonAppFile string) bool {
		return ginkgo.It(context+": Start the application through the app file in JSON format.", func() {
			writeStatus := os.WriteFile("vela.json", []byte(jsonAppFile), 0644)
			gomega.Expect(writeStatus).NotTo(gomega.HaveOccurred())
			output, err := execCommandString("vela up -f vela.json")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).NotTo(gomega.ContainSubstring("Error:"))
		})
	}

	JsonAppFileContextWithWait = func(context, jsonAppFile string) bool {
		return ginkgo.It(context+": Start the application through the app file in JSON format.", func() {
			writeStatus := os.WriteFile("vela.json", []byte(jsonAppFile), 0644)
			gomega.Expect(writeStatus).NotTo(gomega.HaveOccurred())
			output, err := execCommandString("vela up -f vela.json --wait")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("Application Deployed Successfully!"))
		})
	}

	JsonAppFileContextWithTimeout = func(context, jsonAppFile, duration string) bool {
		return ginkgo.It(context+": Start the application through the app file in JSON format.", func() {
			writeStatus := os.WriteFile("vela.json", []byte(jsonAppFile), 0644)
			gomega.Expect(writeStatus).NotTo(gomega.HaveOccurred())
			output, err := execCommandString("vela up -f vela.json --wait --timeout=" + duration)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("Timeout waiting Application to be healthy!"))
		})
	}

	DeleteEnvFunc = func(context string, envName string) bool {
		return ginkgo.It(context+": should print env does not exist message", func() {
			cli := fmt.Sprintf("vela env delete %s", envName)
			output, err := execCommandString(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			expectedOutput := fmt.Sprintf("environment %s does not exist", envName)
			gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput))
		})
	}

	EnvShowContext = func(context string, envName string) bool {
		return ginkgo.It(context+": should show detailed environment message", func() {
			cli := fmt.Sprintf("vela env ls %s", envName)
			output, err := execCommandString(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("NAME"))
			gomega.Expect(output).To(gomega.ContainSubstring("NAMESPACE"))
			gomega.Expect(output).To(gomega.ContainSubstring(envName))
		})
	}

	EnvSetContext = func(context string, envName string) bool {
		return ginkgo.It(context+": should show environment set message", func() {
			cli := fmt.Sprintf("vela env sw %s", envName)
			output, err := execCommandString(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("switched to"))
		})
	}

	EnvDeleteContext = func(context string, envName string) bool {
		return ginkgo.It(context+": should delete an environment", func() {
			cli := fmt.Sprintf("vela env delete %s", envName)
			output, err := execCommandString(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			expectedOutput := fmt.Sprintf("%s deleted", envName)
			gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput))
		})
	}

	WorkloadDeleteContext = func(context string, applicationName string) bool {
		return ginkgo.It(context+": should print successful deletion information", func() {
			cli := fmt.Sprintf("vela delete %s -y", applicationName)
			output, err := execCommandString(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("succeeded"))
		})
	}

	WorkloadCapabilityListContext = func() bool {
		return ginkgo.It("list workload capabilities: should sync capabilities from cluster before listing workload capabilities", func() {
			output, err := Exec("vela components")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("webservice"))
		})
	}

	TraitCapabilityListContext = func() bool {
		return ginkgo.It("list traits capabilities: should sync capabilities from cluster before listing trait capabilities", func() {
			output, err := execCommandString("vela traits")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("scaler"))
		})
	}

	// ComponentListContext used for test vela svc ls
	ComponentListContext = func(context string, applicationName string, workloadType string, traitAlias string) bool {
		return ginkgo.It(context+": should list all applications", func() {
			output, err := Exec("vela ls")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("COMPONENT"))
			gomega.Expect(output).To(gomega.ContainSubstring(applicationName))
			gomega.Expect(output).To(gomega.ContainSubstring(workloadType))
			if traitAlias != "" {
				gomega.Expect(output).To(gomega.ContainSubstring(traitAlias))
			}
		})
	}

	ShowCapabilityReference = func(context string, capabilityName string) bool {
		return ginkgo.It(context+": should show capability reference", func() {
			cli := fmt.Sprintf("vela show %s", capabilityName)
			_, err := execCommandString(cli)
			gomega.Expect(err).Should(gomega.BeNil())
		})
	}

	ShowCapabilityReferenceMarkdown = func(context string, capabilityName string) bool {
		return ginkgo.It(context+": should show capability reference in markdown", func() {
			cli := fmt.Sprintf("vela show %s --format=markdown", capabilityName)
			_, err := execCommandString(cli)
			gomega.Expect(err).Should(gomega.BeNil())
		})
	}
)
