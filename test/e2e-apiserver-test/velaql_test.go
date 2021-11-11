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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	apiv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

type PodStatus struct {
	Name       string      `json:"name"`
	Containers []string    `json:"containers"`
	Events     interface{} `json:"events"`
}
type Status struct {
	PodList []PodStatus `json:"podList"`
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

		req := apiv1.ApplicationRequest{
			Components: app.Spec.Components,
		}
		bodyByte, err := json.Marshal(req)
		Expect(err).Should(BeNil())
		res, err := http.Post(
			fmt.Sprintf("http://127.0.0.1:8000/v1/namespaces/%s/applications/%s", namespace, appName),
			"application/json",
			bytes.NewBuffer(bodyByte),
		)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(res.StatusCode).Should(Equal(200))

		oldApp := new(v1beta1.Application)
		Eventually(func() error {
			if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: appName, Namespace: namespace}, oldApp); err != nil {
				return err
			}
			if oldApp.Status.Phase != common2.ApplicationRunning {
				return errors.New("application is not ready")
			}
			return nil
		}, 3*time.Second, 300*time.Microsecond).Should(BeNil())

		queryRes, err := http.Get(
			fmt.Sprintf("http://127.0.0.1:8000/api/v1/query?velaql=%s{name=%s,namespace=%s}.%s", "read-view", appName, namespace, "output.value.spec"),
		)
		Expect(err).Should(BeNil())
		Expect(queryRes.StatusCode).Should(Equal(200))

		defer queryRes.Body.Close()
		var appSpec v1beta1.ApplicationSpec
		err = json.NewDecoder(queryRes.Body).Decode(&appSpec)
		Expect(err).ShouldNot(HaveOccurred())

		var existApp v1beta1.Application
		Expect(k8sClient.Get(context.Background(), client.ObjectKey{Name: appName, Namespace: namespace}, &existApp)).Should(BeNil())

		Expect(len(appSpec.Components)).Should(Equal(len(existApp.Spec.Components)))
	})

	It("Test query application status with wrong velaQL", func() {
		queryRes, err := http.Get(
			fmt.Sprintf("http://127.0.0.1:8000/api/v1/query?velaql=%s{err=,name=%s,namespace=%s}.%s", "read-object", appName, namespace, "output.value.spec"),
		)
		Expect(err).Should(BeNil())
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
			if oldApp.Status.Phase != common2.ApplicationRunning {
				return errors.New("application is not ready")
			}
			return nil
		}, 3*time.Second, 300*time.Microsecond).Should(BeNil())

		queryRes, err := http.Get(
			fmt.Sprintf("http://127.0.0.1:8000/api/v1/query?velaql=%s{name=%s,namespace=%s,componentName=%s}.%s", "test-component-pod-view", appName, namespace, component1Name, "status"),
		)
		Expect(err).Should(BeNil())
		Expect(queryRes.StatusCode).Should(Equal(200))

		defer queryRes.Body.Close()
		status := new(Status)
		err = json.NewDecoder(queryRes.Body).Decode(status)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(len(status.PodList)).Should(Equal(1))
		Expect(status.PodList[0].Containers[0]).Should(Equal(component1Name))

		Eventually(func() error {
			queryRes1, err := http.Get(
				fmt.Sprintf("http://127.0.0.1:8000/api/v1/query?velaql=%s{name=%s,namespace=%s,componentName=%s}.%s", "test-component-pod-view", appName, namespace, component2Name, "status"),
			)
			if err != nil {
				return err
			}
			if queryRes1.StatusCode != 200 {
				return errors.Errorf("status code is %d", queryRes1.StatusCode)
			}
			defer queryRes1.Body.Close()
			status1 := new(Status)
			err = json.NewDecoder(queryRes1.Body).Decode(status1)
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
		}, 3*time.Second, 300*time.Microsecond).Should(BeNil())

		newApp := new(v1beta1.Application)
		Eventually(func() error {
			if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(oldApp), newApp); err != nil {
				return err
			}
			if newApp.Status.Phase != common2.ApplicationRunning {
				return errors.New("application is not ready")
			}
			return nil
		}, 3*time.Second, 300*time.Microsecond).Should(BeNil())

		newWorkload := new(batchv1beta1.CronJob)
		Eventually(func() error {
			return k8sClient.Get(context.Background(), client.ObjectKey{Name: component2Name, Namespace: namespace}, newWorkload)
		}, 10*time.Second, 300*time.Microsecond).Should(BeNil())

		Eventually(func() error {
			queryRes, err := http.Get(
				fmt.Sprintf("http://127.0.0.1:8000/api/v1/query?velaql=%s{name=%s,namespace=%s,componentName=%s}.%s", "test-component-pod-view", appName, namespace, component2Name, "status"),
			)
			if err != nil {
				return err
			}
			if queryRes.StatusCode != 200 {
				return errors.Errorf("status code is %d", queryRes.StatusCode)
			}
			defer queryRes.Body.Close()
			status := new(Status)
			err = json.NewDecoder(queryRes.Body).Decode(status)
			if err != nil {
				return err
			}
			if len(status.PodList) == 0 {
				return errors.New("pod list is 0")
			}
			return nil
		}, 2*time.Minute, 3*time.Microsecond).Should(BeNil())
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
