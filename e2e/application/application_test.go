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
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/Netflix/go-expect"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/e2e"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var (
	envName                     = "env-application"
	workloadType                = "webservice"
	applicationName             = "app-basic"
	traitAlias                  = "scaler"
	appNameForInit              = "initmyapp"
	jsonAppFile                 = `{"name":"nginx-vela","services":{"nginx":{"type":"webservice","image":"nginx:1.9.4","ports":[{port: 80, expose: true}]}}}`
	testDeleteJsonAppFile       = `{"name":"test-vela-delete","services":{"nginx-test":{"type":"webservice","image":"nginx:1.9.4","ports":[{port: 80, expose: true}]}}}`
	appbasicJsonAppFile         = `{"name":"app-basic","services":{"app-basic":{"type":"webservice","image":"nginx:1.9.4","ports":[{port: 80, expose: true}]}}}`
	appbasicAddTraitJsonAppFile = `{"name":"app-basic","services":{"app-basic":{"type":"webservice","image":"nginx:1.9.4","ports":[{port: 80, expose: true}],"scaler":{"replicas":2}}}}`
	velaQL                      = "test-component-pod-view{appNs=default,appName=nginx-vela,name=nginx}"

	waitAppfileToSuccess = `{"name":"app-wait-success","services":{"app-basic1":{"type":"webservice","image":"nginx:1.9.4","ports":[{port: 80, expose: true}]}}}`
	waitAppfileToFail    = `{"name":"app-wait-fail","services":{"app-basic2":{"type":"webservice","image":"nginx:fail","ports":[{port: 80, expose: true}]}}}`
)

var _ = ginkgo.Describe("Test Vela Application", func() {
	e2e.JsonAppFileContext("json appfile apply", jsonAppFile)
	e2e.EnvSetContext("env set default", "default")
	e2e.DeleteEnvFunc("env delete", envName)
	e2e.EnvInitContext("env init env-application", envName)
	e2e.EnvSetContext("env set", envName)
	e2e.JsonAppFileContext("deploy app-basic", appbasicJsonAppFile)
	ApplicationExecContext("exec -- COMMAND", applicationName)
	ApplicationPortForwardContext("port-forward", applicationName)
	e2e.JsonAppFileContext("update app-basic, add scaler trait with replicas 2", appbasicAddTraitJsonAppFile)
	e2e.ComponentListContext("ls", applicationName, workloadType, traitAlias)
	ApplicationStatusContext("status", applicationName, workloadType)
	ApplicationStatusDeeplyContext("status", applicationName, workloadType, envName)
	e2e.WorkloadDeleteContext("delete", applicationName)

	ApplicationInitIntercativeCliContext("test vela init app", appNameForInit, workloadType)
	e2e.WorkloadDeleteContext("delete", appNameForInit)

	e2e.JsonAppFileContext("json appfile apply", testDeleteJsonAppFile)
	ApplicationDeleteWithWaitOptions("test delete with wait option", "test-vela-delete")

	e2e.JsonAppFileContext("json appfile apply", testDeleteJsonAppFile)
	ApplicationDeleteWithForceOptions("test delete with force option", "test-vela-delete")

	VelaQLPodListContext("ql", velaQL)

	e2e.JsonAppFileContextWithWait("json appfile apply with wait", waitAppfileToSuccess)
	e2e.JsonAppFileContextWithTimeout("json appfile apply with wait but timeout", waitAppfileToFail, "3s")
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
			app := &v1beta1.Application{}
			gomega.Eventually(func() bool {
				_ = k8sclient.Get(context2.Background(), client.ObjectKey{Name: applicationName, Namespace: "default"}, app)
				return app.Status.LatestRevision != nil
			}, 180*time.Second, 1*time.Second).Should(gomega.BeTrue())

			cli := fmt.Sprintf("vela status %s", applicationName)
			output, err := e2e.LongTimeExec(cli, 120*time.Second)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(strings.ToLower(output)).To(gomega.ContainSubstring("healthy"))
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
			cli := fmt.Sprintf("vela port-forward %s 8080:80 ", appName)
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
						q: "Which image would you like to use for your service ",
						a: "nginx:latest",
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
			gomega.Expect(output).To(gomega.ContainSubstring("Waiting app to be healthy"))
		})
	})
}

var ApplicationDeleteWithWaitOptions = func(context string, appName string) bool {
	return ginkgo.Context(context, func() {
		ginkgo.It("should print successful deletion information", func() {
			cli := fmt.Sprintf("vela delete %s --wait -y", appName)
			output, err := e2e.ExecAndTerminate(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("succeeded"))
		})
	})
}

var ApplicationDeleteWithForceOptions = func(context string, appName string) bool {
	return ginkgo.Context(context, func() {
		ginkgo.It("should print successful deletion information", func() {
			args := common.Args{
				Schema: common.Scheme,
			}
			ctx := context2.Background()

			k8sClient, err := args.GetClient()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			app := new(v1beta1.Application)
			gomega.Eventually(func() error {
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: appName, Namespace: "default"}, app); err != nil {
					return err
				}
				meta.AddFinalizer(app, "test")
				return k8sClient.Update(ctx, app)
			}, time.Second*3, time.Millisecond*300).Should(gomega.BeNil())

			cli := fmt.Sprintf("vela delete %s --force -y", appName)
			output, err := e2e.LongTimeExec(cli, 3*time.Minute)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("timed out"))

			app = new(v1beta1.Application)
			gomega.Eventually(func(g gomega.Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: appName, Namespace: "default"}, app)).Should(gomega.Succeed())
				meta.RemoveFinalizer(app, "test")
				g.Expect(k8sClient.Update(ctx, app)).Should(gomega.Succeed())
			}, time.Second*5, time.Millisecond*300).Should(gomega.Succeed())

			cli = fmt.Sprintf("vela delete %s --force -y", appName)
			output, err = e2e.ExecAndTerminate(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("deleted"))
		})
	})
}

type PodList struct {
	PodList []Pod `form:"podList" json:"podList"`
}

type Pod struct {
	Status   Status   `form:"status" json:"status"`
	Cluster  string   `form:"cluster" json:"cluster"`
	Metadata Metadata `form:"metadata" json:"metadata"`
	Workload Workload `form:"workload" json:"workload"`
}

type Status struct {
	Phase    string `form:"phase" json:"phase"`
	NodeName string `form:"nodeName" json:"nodeName"`
}

type Metadata struct {
	Namespace string `form:"namespace" json:"namespace"`
}

type Workload struct {
	ApiVersion string `form:"apiVersion" json:"apiVersion"`
	Kind       string `form:"kind" json:"kind"`
}

var VelaQLPodListContext = func(context string, velaQL string) bool {
	return ginkgo.Context(context, func() {
		ginkgo.It("should get successful result for executing vela ql", func() {
			args := common.Args{
				Schema: common.Scheme,
			}
			ctx := context2.Background()

			k8sClient, err := args.GetClient()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			componentView := new(corev1.ConfigMap)
			gomega.Eventually(func(g gomega.Gomega) {
				g.Expect(common.ReadYamlToObject("./component-pod-view.yaml", componentView)).Should(gomega.BeNil())
				g.Expect(k8sClient.Create(ctx, componentView)).Should(gomega.SatisfyAny(gomega.Succeed(), util.AlreadyExistMatcher{}))
			}, time.Second*3, time.Millisecond*300).Should(gomega.Succeed())

			cli := fmt.Sprintf("vela ql %s", velaQL)
			output, err := e2e.Exec(cli)

			// remove warning like: W0406 14:07:49.832144 2443978 tree.go:958] ignore list resources: EndpointSlice as no matches for kind "EndpointSlice" in version "discovery.k8s.io/v1beta1"
			re := regexp.MustCompile(`W\d{4}.*`)
			output = re.ReplaceAllString(output, "")

			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			var list PodList
			err = json.Unmarshal([]byte(output), &list)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, v := range list.PodList {
				if v.Cluster != "" {
					gomega.Expect(v.Cluster).To(gomega.ContainSubstring("local"))
				}
				if v.Status.Phase != "" {
					gomega.Expect(v.Status.Phase).To(gomega.ContainSubstring("Running"))
				}
				if v.Status.NodeName != "" {
					gomega.Expect(v.Status.NodeName).To(gomega.ContainSubstring("k3d-k3s-default-server-0"))
				}
				if v.Metadata.Namespace != "" {
					gomega.Expect(v.Metadata.Namespace).To(gomega.ContainSubstring("default"))
				}
				if v.Workload.ApiVersion != "" {
					gomega.Expect(v.Workload.ApiVersion).To(gomega.ContainSubstring("apps/v1"))
				}
				if v.Workload.Kind != "" {
					gomega.Expect(v.Workload.Kind).To(gomega.ContainSubstring("ReplicaSet"))
				}
			}
		})
	})
}
