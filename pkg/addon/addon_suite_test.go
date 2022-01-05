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

package addon

import (
	"context"
	"fmt"
	"time"

	types2 "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

var _ = Describe("Addon test", func() {
	ctx := context.Background()
	var app v1beta1.Application

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, &app)).Should(BeNil())
	})

	It("continueOrRestartWorkflow func test", func() {
		app = v1beta1.Application{}
		Expect(yaml.Unmarshal([]byte(appYaml), &app)).Should(BeNil())
		app.SetNamespace(testns)
		Expect(k8sClient.Create(ctx, &app)).Should(BeNil())

		Eventually(func() error {
			checkApp := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, types2.NamespacedName{Namespace: app.Namespace, Name: app.Name}, checkApp); err != nil {
				return err
			}
			appPatch := client.MergeFrom(checkApp.DeepCopy())
			checkApp.Status.Workflow = &common.WorkflowStatus{Suspend: true}
			if err := k8sClient.Status().Patch(ctx, checkApp, appPatch); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())

		Eventually(func() error {
			checkApp := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, types2.NamespacedName{Namespace: app.Namespace, Name: app.Name}, checkApp); err != nil {
				return err
			}
			if !checkApp.Status.Workflow.Suspend {
				return fmt.Errorf("app haven't not suspend")
			}

			h := Installer{ctx: ctx, cli: k8sClient, addon: &InstallPackage{Meta: Meta{Name: "test-app"}}}
			if err := h.continueOrRestartWorkflow(); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())

		Eventually(func() error {
			checkApp := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, types2.NamespacedName{Namespace: app.Namespace, Name: app.Name}, checkApp); err != nil {
				return err
			}
			if checkApp.Status.Workflow.Suspend {
				return fmt.Errorf("app haven't not continue")
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
	})

	It("continueOrRestartWorkflow func test, test restart workflow", func() {
		app = v1beta1.Application{}
		Expect(yaml.Unmarshal([]byte(appYaml), &app)).Should(BeNil())
		app.SetNamespace(testns)
		Expect(k8sClient.Create(ctx, &app)).Should(BeNil())

		Eventually(func() error {
			checkApp := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, types2.NamespacedName{Namespace: app.Namespace, Name: app.Name}, checkApp); err != nil {
				return err
			}
			appPatch := client.MergeFrom(checkApp.DeepCopy())
			checkApp.Status.Workflow = &common.WorkflowStatus{Message: "someMessage", AppRevision: "test-revision"}
			checkApp.Status.Phase = common.ApplicationRunning
			if err := k8sClient.Status().Patch(ctx, checkApp, appPatch); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())

		Eventually(func() error {
			checkApp := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, types2.NamespacedName{Namespace: app.Namespace, Name: app.Name}, checkApp); err != nil {
				return err
			}
			if checkApp.Status.Phase != common.ApplicationRunning {
				return fmt.Errorf("app haven't not running")
			}

			h := Installer{ctx: ctx, cli: k8sClient, addon: &InstallPackage{Meta: Meta{Name: "test-app"}}}
			if err := h.continueOrRestartWorkflow(); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())

		Eventually(func() error {
			checkApp := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, types2.NamespacedName{Namespace: app.Namespace, Name: app.Name}, checkApp); err != nil {
				return err
			}
			if checkApp.Status.Workflow != nil {
				return fmt.Errorf("app workflow havenot been restart")
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
	})

	It(" FetchAddonRelatedApp func test", func() {
		app = v1beta1.Application{}
		Expect(yaml.Unmarshal([]byte(legacyAppYaml), &app)).Should(BeNil())
		app.SetNamespace(testns)
		Expect(k8sClient.Create(ctx, &app)).Should(BeNil())

		Eventually(func() error {
			app, err := FetchAddonRelatedApp(ctx, k8sClient, "legacy-addon")
			if err != nil {
				return err
			}
			if app.Name != "legacy-addon" {
				return fmt.Errorf("error addon app name")
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
	})

	It(" determineAddonAppName func test", func() {
		app = v1beta1.Application{}
		Expect(yaml.Unmarshal([]byte(legacyAppYaml), &app)).Should(BeNil())
		app.SetNamespace(testns)
		Expect(k8sClient.Create(ctx, &app)).Should(BeNil())

		Eventually(func() error {
			appName, err := determineAddonAppName(ctx, k8sClient, "legacy-addon")
			if err != nil {
				return err
			}
			if appName != "legacy-addon" {
				return fmt.Errorf("error addon app name")
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())

		notExsitAppName, err := determineAddonAppName(ctx, k8sClient, "not-exist")
		Expect(err).Should(BeNil())
		Expect(notExsitAppName).Should(BeEquivalentTo("addon-not-exist"))
	})
})

const (
	appYaml = `apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: addon-test-app
spec:
  components:
    - name: express-server
      type: webservice
      properties:
        image: crccheck/hello-world
        port: 8000
`
	legacyAppYaml = `apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: legacy-addon
spec:
  components:
    - name: express-server
      type: webservice
      properties:
        image: crccheck/hello-world
        port: 8000
`
)
