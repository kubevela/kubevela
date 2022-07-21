/*
 Copyright 2021. The KubeVela Authors.

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
	"context"
	"encoding/json"
	"fmt"
	"time"

	types2 "github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	apiv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

type PodStatus struct {
	Name       string      `json:"name"`
	Containers []string    `json:"containers"`
	Events     interface{} `json:"events"`
}
type Status struct {
	PodList []PodStatus `json:"podList,omitempty"`
	Error   string      `json:"error,omitempty"`
}

var _ = Describe("Test velaQL rest api", func() {
	namespace := "test-velaql"
	appName := "example-app"
	component1Name := "ql-webservice"
	component2Name := "ql-worker"
	var app v1beta1.Application
	var readView corev1.ConfigMap

	It("Test query application status via view", func() {
		Expect(common.ReadYamlToObject("./testdata/read-view.yaml", &readView)).Should(BeNil())
		Expect(k8sClient.Create(context.Background(), &readView)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		Expect(common.ReadYamlToObject("./testdata/example-app.yaml", &app)).Should(BeNil())
		app.Spec.Components[0].Name = component1Name
		app.Spec.Components[1].Name = component2Name
		app.Name = appName

		req := apiv1.ApplicationRequest{
			Components: app.Spec.Components,
		}
		res := post(fmt.Sprintf("/v1/namespaces/%s/applications/%s", namespace, appName), req)
		Expect(res).ShouldNot(BeNil())
		Expect(res.StatusCode).Should(Equal(200))

		oldApp := new(v1beta1.Application)
		Eventually(func() error {
			if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: appName, Namespace: namespace}, oldApp); err != nil {
				return err
			}
			if len(oldApp.Status.AppliedResources) != 2 {
				return errors.Errorf("expect the applied resources number is %d, but get %d", 2, len(oldApp.Status.AppliedResources))
			}
			return nil
		}, 3*time.Second, 300*time.Microsecond).Should(BeNil())

		queryRes := get(fmt.Sprintf("/query?velaql=%s{name=%s,namespace=%s}.%s", "read-view", appName, namespace, "output.value.spec"))
		var appSpec v1beta1.ApplicationSpec
		Expect(decodeResponseBody(queryRes, &appSpec)).Should(Succeed())

		var existApp v1beta1.Application
		Expect(k8sClient.Get(context.Background(), client.ObjectKey{Name: appName, Namespace: namespace}, &existApp)).Should(BeNil())

		Expect(len(appSpec.Components)).Should(Equal(len(existApp.Spec.Components)))
	})

	It("Test query application status with wrong velaQL", func() {
		queryRes := get(fmt.Sprintf("/query?velaql=%s{err=,name=%s,namespace=%s}.%s", "read-object", appName, namespace, "output.value.spec"))
		Expect(queryRes.StatusCode).Should(Equal(400))
	})

	It("Test query application component view", func() {
		componentView := new(corev1.ConfigMap)
		Expect(common.ReadYamlToObject("./testdata/component-pod-view.yaml", componentView)).Should(BeNil())
		Expect(k8sClient.Create(context.Background(), componentView)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		oldApp := new(v1beta1.Application)
		Eventually(func() error {
			if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: appName, Namespace: namespace}, oldApp); err != nil {
				return err
			}
			if len(oldApp.Status.AppliedResources) != 2 {
				return errors.Errorf("expect the applied resources number is %d, but get %d", 2, len(oldApp.Status.AppliedResources))
			}
			return nil
		}, 3*time.Second, 300*time.Microsecond).Should(BeNil())

		queryRes := get(fmt.Sprintf("/query?velaql=%s{appName=%s,appNs=%s,name=%s}.%s", "test-component-pod-view", appName, namespace, component1Name, "status"))
		status := new(Status)
		Expect(decodeResponseBody(queryRes, status)).Should(Succeed())
		Expect(len(status.PodList)).Should(Equal(1))
		Expect(status.PodList[0].Containers[0]).Should(Equal(component1Name))

		Eventually(func() error {
			queryRes1 := get(fmt.Sprintf("/query?velaql=%s{appName=%s,appNs=%s,name=%s}.%s", "test-component-pod-view", appName, namespace, component2Name, "status"))
			if queryRes1.StatusCode != 200 {
				return errors.Errorf("status code is %d", queryRes1.StatusCode)
			}
			defer queryRes1.Body.Close()
			status1 := new(Status)
			err := json.NewDecoder(queryRes1.Body).Decode(status1)
			if err != nil {
				return err
			}
			if len(status1.PodList) != 1 {
				return errors.New("pod number is zero")
			}
			if status1.PodList[0].Containers[0] != component2Name {
				return errors.New("container name is not correct")
			}
			return nil
		}, 10*time.Second, 300*time.Microsecond).Should(BeNil())
	})

	It("Test collect pod from cronJob", func() {
		cronJob := new(v1beta1.ComponentDefinition)
		Expect(yaml.Unmarshal([]byte(cronJobComponentDefinition), cronJob)).Should(BeNil())
		Expect(k8sClient.Create(context.Background(), cronJob)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		oldApp := new(v1beta1.Application)
		Eventually(func() error {
			if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: appName, Namespace: namespace}, oldApp); err != nil {
				return err
			}
			oldApp.Spec.Components[1].Type = "cronjob"
			oldApp.Spec.Components[1].Properties = util.Object2RawExtension(map[string]interface{}{
				"image": "busybox",
				"cmd":   []string{"sleep", "1"},
			})
			if err := k8sClient.Update(context.Background(), oldApp); err != nil {
				return err
			}
			return nil
		}, 10*time.Second, 300*time.Microsecond).Should(BeNil())

		newApp := new(v1beta1.Application)
		Eventually(func() error {
			if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(oldApp), newApp); err != nil {
				return err
			}
			appliedCronJob := false
			for _, resource := range newApp.Status.AppliedResources {
				if resource.ObjectReference.Kind == "CronJob" {
					appliedCronJob = true
					break
				}
			}
			if !appliedCronJob {
				return errors.New("fail to apply cronjob")
			}
			return nil
		}, 10*time.Second, 300*time.Microsecond).Should(BeNil())

		newWorkload := new(batchv1beta1.CronJob)
		Eventually(func() error {
			return k8sClient.Get(context.Background(), client.ObjectKey{Name: component2Name, Namespace: namespace}, newWorkload)
		}, 10*time.Second, 300*time.Microsecond).Should(BeNil())

		Eventually(func() error {
			queryRes := get(fmt.Sprintf("/query?velaql=%s{appName=%s,appNs=%s}.%s", "test-component-pod-view", appName, namespace, "status"))
			if queryRes.StatusCode != 200 {
				return errors.Errorf("status code is %d", queryRes.StatusCode)
			}
			defer queryRes.Body.Close()
			status := new(Status)
			err := json.NewDecoder(queryRes.Body).Decode(status)
			if err != nil {
				return err
			}
			if len(status.PodList) == 0 {
				return errors.New("pod list is 0")
			}
			return nil
		}, 2*time.Minute, 3*time.Microsecond).Should(BeNil())
	})

	PIt("Test collect pod from helmRelease", func() {
		appWithHelm := new(v1beta1.Application)
		Expect(yaml.Unmarshal([]byte(podInfoApp), appWithHelm)).Should(BeNil())
		req := apiv1.ApplicationRequest{
			Components: appWithHelm.Spec.Components,
		}
		Eventually(func(g Gomega) {
			res := post(fmt.Sprintf("/v1/namespaces/%s/applications/%s", namespace, appWithHelm.Name), req)
			g.Expect(res).ShouldNot(BeNil())
			g.Expect(res.StatusCode).Should(Equal(200))
		}, 1*time.Minute).Should(Succeed())

		newApp := new(v1beta1.Application)
		Eventually(func() error {
			if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: appWithHelm.Name, Namespace: namespace}, newApp); err != nil {
				return err
			}
			if newApp.Status.Phase != common2.ApplicationRunning {
				return errors.New("application is not ready")
			}
			return nil
		}, 2*time.Minute, 1*time.Second).Should(BeNil())

		Eventually(func() error {
			queryRes := get(fmt.Sprintf("/query?velaql=%s{appName=%s,appNs=%s,name=%s}.%s", "test-component-pod-view", appWithHelm.Name, namespace, "podinfo", "status"))
			if queryRes.StatusCode != 200 {
				return errors.Errorf("status code is %d", queryRes.StatusCode)
			}
			defer queryRes.Body.Close()

			status := new(Status)
			err := json.NewDecoder(queryRes.Body).Decode(status)
			if err != nil {
				return err
			}
			if status.Error != "" {
				return errors.Errorf("error %v", status.Error)
			}
			if len(status.PodList) == 0 {
				return errors.New("pod list is 0")
			}
			return nil
		}, 2*time.Minute, 300*time.Microsecond).Should(BeNil())
	})

	It("Test collect legacy resources from application", func() {
		appWithGC := new(v1beta1.Application)
		Expect(yaml.Unmarshal([]byte(appWithGCPolicy), appWithGC)).Should(BeNil())
		req := apiv1.ApplicationRequest{
			Components: appWithGC.Spec.Components,
			Policies:   appWithGC.Spec.Policies,
		}
		res := post(fmt.Sprintf("/v1/namespaces/%s/applications/%s", namespace, appWithGC.Name), req)
		Expect(res).ShouldNot(BeNil())
		Expect(res.StatusCode).Should(Equal(200))

		newApp := new(v1beta1.Application)
		Eventually(func() error {
			if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: appWithGC.Name, Namespace: namespace}, newApp); err != nil {
				return err
			}
			if newApp.Status.Phase != common2.ApplicationRunning {
				return errors.New("application is not ready")
			}
			return nil
		}, 2*time.Minute, 1*time.Second).Should(BeNil())

		Eventually(func() error {
			queryRes := get(fmt.Sprintf("/query?velaql=%s{appName=%s,appNs=%s,name=%s}.%s", "test-component-pod-view", appWithGC.Name, namespace, "express-server", "status"))
			if queryRes.StatusCode != 200 {
				return errors.Errorf("status code is %d", queryRes.StatusCode)
			}
			defer queryRes.Body.Close()

			status := new(Status)
			err := json.NewDecoder(queryRes.Body).Decode(status)
			if err != nil {
				return err
			}
			if status.Error != "" {
				return errors.Errorf("error %v", status.Error)
			}
			if len(status.PodList) != 1 {
				return errors.New("pod is not ready")
			}
			return nil
		}, 2*time.Minute, 300*time.Microsecond).Should(BeNil())

		appWithGC.Spec.Components[0].Name = "new-express-server"
		updateReq := apiv1.ApplicationRequest{
			Components: appWithGC.Spec.Components,
			Policies:   appWithGC.Spec.Policies,
		}
		res = post(fmt.Sprintf("/v1/namespaces/%s/applications/%s", namespace, appWithGC.Name), updateReq)
		Expect(res).ShouldNot(BeNil())
		Expect(res.StatusCode).Should(Equal(200))

		Eventually(func() error {
			if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: appWithGC.Name, Namespace: namespace}, newApp); err != nil {
				return err
			}
			if newApp.Status.Phase != common2.ApplicationRunning {
				return errors.New("application is not ready")
			}
			return nil
		}, 2*time.Minute, 1*time.Second).Should(BeNil())

		Eventually(func() error {
			queryRes := get(fmt.Sprintf("/query?velaql=%s{appName=%s,appNs=%s,name=%s}.%s", "test-component-pod-view", appWithGC.Name, namespace, "express-server", "status"))
			if queryRes.StatusCode != 200 {
				return errors.Errorf("status code is %d", queryRes.StatusCode)
			}
			defer queryRes.Body.Close()

			status := new(Status)
			err := json.NewDecoder(queryRes.Body).Decode(status)
			if err != nil {
				return err
			}
			if status.Error != "" {
				return errors.Errorf("error %v", status.Error)
			}
			if len(status.PodList) != 1 {
				return errors.New("pod is not ready")
			}
			return nil
		}, 2*time.Minute, 300*time.Microsecond).Should(BeNil())

		Eventually(func() error {
			queryRes := get(fmt.Sprintf("/query?velaql=%s{appName=%s,appNs=%s,name=%s}.%s", "test-component-pod-view", appWithGC.Name, namespace, "new-express-server", "status"))
			if queryRes.StatusCode != 200 {
				return errors.Errorf("status code is %d", queryRes.StatusCode)
			}
			defer queryRes.Body.Close()

			status := new(Status)
			err := json.NewDecoder(queryRes.Body).Decode(status)
			if err != nil {
				return err
			}
			if status.Error != "" {
				return errors.Errorf("error %v", status.Error)
			}
			if len(status.PodList) != 1 {
				return errors.New("pod is not ready")
			}
			return nil
		}, 2*time.Minute, 300*time.Microsecond).Should(BeNil())
	})

	It("Test query logs in pod", func() {
		collectLogsQuery := new(corev1.ConfigMap)
		Expect(common.ReadYamlToObject("./testdata/collect-logs.yaml", collectLogsQuery)).Should(BeNil())
		Expect(k8sClient.Create(context.Background(), collectLogsQuery)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		podName := "hello-world-example-pod"
		containerName := "main"
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: "default"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name:    containerName,
					Image:   "busybox",
					Command: []string{"/bin/sh", "-c"},
					Args:    []string{"echo hello-world && sleep 3600"},
				}},
			}}
		Expect(k8sClient.Create(context.Background(), pod)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		defer k8sClient.Delete(context.Background(), pod)
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: podName, Namespace: "default"}, pod)).Should(Succeed())
			g.Expect(pod.Status.Phase).Should(Equal(corev1.PodRunning))
		}, 30*time.Second).Should(Succeed())
		queryRes := get(fmt.Sprintf("/query?velaql=%s{cluster=%s,namespace=%s,pod=%s,container=%s}.%s", "test-collect-logs", "local", "default", podName, containerName, "status"))
		status := &struct {
			Logs string `json:"logs"`
			Err  string `json:"err,omitempty"`
		}{}
		Expect(decodeResponseBody(queryRes, status)).Should(Succeed())
	})

	It("test appliedResource and application tree velaql", func() {
		ctx := context.Background()
		app := v1beta1.Application{}
		Expect(yaml.Unmarshal([]byte(testApp), &app)).Should(BeNil())
		Expect(k8sClient.Create(ctx, &app)).Should(BeNil())
		Eventually(func() error {
			queryRes := get(fmt.Sprintf("/query?velaql=%s{appNs=%s,appName=%s}.%s", "service-applied-resources-view", "default", "app-test-velaql", "status"))
			status := &struct {
				Resources []types2.AppliedResource `json:"resources"`
			}{}
			if err := decodeResponseBody(queryRes, status); err != nil {
				return err
			}
			if len(status.Resources) != 1 {
				return fmt.Errorf("applied resource velaql error, expect to be 1 but %d", len(status.Resources))
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())

		// test app resource tree velaql
		Eventually(func() error {
			queryRes := get(fmt.Sprintf("/query?velaql=%s{appNs=%s,appName=%s}.%s", "application-resource-tree-view", "default", "app-test-velaql", "status"))
			status := &struct {
				Resources []types2.AppliedResource `json:"resources"`
			}{}
			if err := decodeResponseBody(queryRes, status); err != nil {
				return err
			}
			if status.Resources[0].ResourceTree.Kind != "Deployment" &&
				status.Resources[0].ResourceTree.APIVersion != "apps/v1" {
				return fmt.Errorf("tree root error")
			}
			if len(status.Resources[0].ResourceTree.LeafNodes) != 1 {
				return fmt.Errorf("length application tree error")
			}
			if status.Resources[0].ResourceTree.LeafNodes[0].Kind != "ReplicaSet" &&
				status.Resources[0].ResourceTree.LeafNodes[0].APIVersion != "apps/v1" {
				return fmt.Errorf("replciaset not ready")
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())

		Expect(k8sClient.Delete(ctx, &app)).Should(BeNil())
	})
})

var cronJobComponentDefinition = `
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  annotations: {}
  name: cronjob
  namespace: vela-system
spec:
  schematic:
    cue:
      template: |
        output: {
                apiVersion: "batch/v1beta1"
                kind:       "CronJob"
                metadata: name: context.name
                spec: {
                        schedule: "*/1 * * * *"
                        jobTemplate: spec: template: spec: {
                                containers: [{
                                        name:            context.name
                                        image:           parameter.image
                                        imagePullPolicy: "IfNotPresent"
                                        command:         parameter.cmd
                                }]
                                restartPolicy: "OnFailure"
                        }
                }
        }
        parameter: {
                image: string
                cmd: [...string]
        }
  workload:
    type: autodetects.core.oam.dev
`

var podInfoApp = `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: podinfo
spec:
  components:
    - name: podinfo
      type: helm
      properties:
        chart: podinfo
        url: https://stefanprodan.github.io/podinfo
        repoType: helm
        version: 5.1.2
`

var appWithGCPolicy = `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: app-with-gc-policy
spec:
  components:
    - name: express-server
      type: webservice
      properties:
        image: crccheck/hello-world
        port: 8000
  policies:
    - name: keep-legacy-resource
      type: garbage-collect
      properties:
        keepLegacyResource: true
`

var testApp = `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: app-test-velaql
  namespace: default
spec:
  components:
    - name: app-test-velaql
      type: webservice
      properties:
        image: crccheck/hello-world
        port: 8000
`
