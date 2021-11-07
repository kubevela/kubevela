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
	context2 "context"
	"fmt"
	"time"

	"github.com/Netflix/go-expect"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/e2e"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var (
	envName                     = "env-application"
	workloadType                = "webservice"
	applicationName             = "app-basic"
	traitAlias                  = "scaler"
	appNameForInit              = "initmyapp"
	jsonAppFile                 = `{"name":"nginx-vela","services":{"nginx":{"type":"webservice","image":"nginx:1.9.4","port":80}}}`
	appbasicJsonAppFile         = `{"name":"app-basic","services":{"app-basic":{"type":"webservice","image":"nginx:1.9.4","port":80}}}`
	appbasicAddTraitJsonAppFile = `{"name":"app-basic","services":{"app-basic":{"type":"webservice","image":"nginx:1.9.4","port":80,"scaler":{"replicas":2}}}}`
)

var _ = ginkgo.Describe("Test Vela Application", func() {
	e2e.JsonAppFileContext("json appfile apply", jsonAppFile)
	e2e.EnvSetContext("env set", "default")
	e2e.DeleteEnvFunc("env delete", envName)
	e2e.EnvInitContext("env init", envName)
	e2e.EnvSetContext("env set", envName)
	e2e.JsonAppFileContext("deploy app-basic", appbasicJsonAppFile)
	ApplicationExecContext("exec -- COMMAND", applicationName)
	e2e.JsonAppFileContext("update app-basic, add scaler trait with replicas 2", appbasicAddTraitJsonAppFile)
	e2e.ComponentListContext("ls", applicationName, workloadType, traitAlias)
	ApplicationStatusContext("status", applicationName, workloadType)
	ApplicationStatusDeeplyContext("status", applicationName, workloadType, envName)
	// ApplicationPortForwardContext("port-forward", applicationName)
	e2e.WorkloadDeleteContext("delete", applicationName)

	ApplicationInitIntercativeCliContext("test vela init app", appNameForInit, workloadType)
	e2e.WorkloadDeleteContext("delete", appNameForInit)
})

var ApplicationStatusContext = func(context string, applicationName string, workloadType string) bool {
	return ginkgo.Context(context, func() {
		ginkgo.It("should get status for the application", func() {
			cli := fmt.Sprintf("vela status %s", applicationName)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring(applicationName))
			// TODO(roywang) add more assertion to check health status
		})
	})
}

var ApplicationStatusDeeplyContext = func(context string, applicationName, workloadType, envName string) bool {
	return ginkgo.Context(context, func() {
		ginkgo.It("should get status of the service", func() {
			ginkgo.By("init new k8s client")
			k8sclient, err := common.NewK8sClient()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("check Application reconciled ready")
			app := &v1alpha2.Application{}
			gomega.Eventually(func() bool {
				_ = k8sclient.Get(context2.Background(), client.ObjectKey{Name: applicationName, Namespace: "default"}, app)
				return app.Status.LatestRevision != nil
			}, 180*time.Second, 1*time.Second).Should(gomega.BeTrue())

			cli := fmt.Sprintf("vela status %s", applicationName)
			output, err := e2e.LongTimeExec(cli, 120*time.Second)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("Application Deployed Successfully"))
			// TODO(zzxwill) need to check workloadType after app status is refined
		})
	})
}

var ApplicationExecContext = func(context string, appName string) bool {
	return ginkgo.Context(context, func() {
		ginkgo.It("should get output of exec /bin/ls", func() {
			gomega.Eventually(func() string {
				cli := fmt.Sprintf("vela exec %s -- /bin/ls ", appName)
				output, err := e2e.Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return output
			}, 90*time.Second, 5*time.Second).Should(gomega.ContainSubstring("bin"))
		})
	})
}

var ApplicationPortForwardContext = func(context string, appName string) bool {
	return ginkgo.Context(context, func() {
		ginkgo.It("should get output of port-forward successfully", func() {
			cli := fmt.Sprintf("vela port-forward %s 80:80 ", appName)
			output, err := e2e.ExecAndTerminate(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("Forward successfully"))
		})
	})
}

var ApplicationInitIntercativeCliContext = func(context string, appName string, workloadType string) bool {
	return ginkgo.Context(context, func() {
		ginkgo.It("should init app through interactive questions", func() {
			cli := "vela init"
			output, err := e2e.InteractiveExec(cli, func(c *expect.Console) {
				data := []struct {
					q, a string
				}{
					{
						q: "What would you like to name your application (required): ",
						a: appName,
					},
					{
						q: "webservice",
						a: workloadType,
					},
					{
						q: "What would you like to name this webservice (required): ",
						a: "mysvc",
					},
					{
						q: "If addRevisionLabel is true, the appRevision label will be added to the underlying pods (optional, default is false):",
						a: "N",
					},
					{
						q: "Which image would you like to use for your service ",
						a: "nginx:latest",
					},
					{
						q: "Which port do you want customer traffic sent to ",
						a: "",
					},
					{
						q: "Specify image pull policy for your service ",
						a: "Always",
					},
					{
						q: "Number of CPU units for the service, like `0.5` (0.5 CPU core), `1` (1 CPU core) (optional):",
						a: "0.5",
					},
					{
						q: "Specifies the attributes of the memory resource required for the container. (optional):",
						a: "200M",
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
			gomega.Expect(output).To(gomega.ContainSubstring("Checking Status"))
		})
	})
}
