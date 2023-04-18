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

package e2e_multicluster_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/oam/util"

	"sigs.k8s.io/yaml"
)

var _ = PDescribe("Test MultiCluster Rollout", func() {
	Context("Test Runtime Cluster Rollout", func() {
		var namespace string
		var hubCtx context.Context
		var workerCtx context.Context
		var rollout v1alpha1.Rollout
		var componentName string
		var targetDeploy appsv1.Deployment
		var sourceDeploy appsv1.Deployment

		BeforeEach(func() {
			hubCtx, workerCtx, namespace = initializeContextAndNamespace()
			componentName = "hello-world-server"
		})

		AfterEach(func() {
			cleanUpNamespace(hubCtx, workerCtx, namespace)
			ns := v1.Namespace{}
			Eventually(func() error { return k8sClient.Get(hubCtx, types.NamespacedName{Name: namespace}, &ns) }, 300*time.Second).Should(util.NotFoundMatcher{})
		})

		verifySucceed := func(componentRevision string) {
			By("check rollout status have succeed")
			Eventually(func() error {
				rolloutKey := types.NamespacedName{Namespace: namespace, Name: componentName}
				if err := k8sClient.Get(workerCtx, rolloutKey, &rollout); err != nil {
					return err
				}
				if rollout.Spec.TargetRevisionName != componentRevision {
					return fmt.Errorf("rollout have not point to right targetRevision")
				}
				if rollout.Status.RollingState != v1alpha1.RolloutSucceedState {
					return fmt.Errorf("error rollout status state %s", rollout.Status.RollingState)
				}
				compRevName := rollout.Spec.TargetRevisionName
				deployKey := types.NamespacedName{Namespace: namespace, Name: compRevName}
				if err := k8sClient.Get(workerCtx, deployKey, &targetDeploy); err != nil {
					return err
				}
				if *targetDeploy.Spec.Replicas != *rollout.Spec.RolloutPlan.TargetSize {
					return fmt.Errorf("targetDeploy replicas missMatch %d, %d", targetDeploy.Spec.Replicas, rollout.Spec.RolloutPlan.TargetSize)
				}
				if targetDeploy.Status.UpdatedReplicas != *targetDeploy.Spec.Replicas {
					return fmt.Errorf("update not finish")
				}
				if rollout.Status.LastSourceRevision == "" {
					return nil
				}
				deployKey = types.NamespacedName{Namespace: namespace, Name: rollout.Status.LastSourceRevision}
				if err := k8sClient.Get(workerCtx, deployKey, &sourceDeploy); err == nil || !apierrors.IsNotFound(err) {
					return fmt.Errorf("source deploy still exist")
				}
				return nil
			}, time.Second*60).Should(BeNil())
		}

		It("Test Rollout whole feature in runtime cluster ", func() {
			app := &v1beta1.Application{}
			appYaml, err := os.ReadFile("./testdata/app/app-rollout-envbinding.yaml")
			Expect(err).Should(Succeed())
			Expect(yaml.Unmarshal([]byte(appYaml), app)).Should(Succeed())
			app.SetNamespace(namespace)
			err = k8sClient.Create(hubCtx, app)
			Expect(err).Should(Succeed())
			verifySucceed(componentName + "-v1")

			By("update application to v2")
			checkApp := &v1beta1.Application{}
			Eventually(func() error {
				if err := k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: app.Name}, checkApp); err != nil {
					return err
				}
				checkApp.Spec.Components[0].Properties.Raw = []byte(`{"image": "stefanprodan/podinfo:5.0.2"}`)
				if err := k8sClient.Update(hubCtx, checkApp); err != nil {
					return err
				}
				return nil
			}, 30*time.Second).Should(BeNil())
			verifySucceed(componentName + "-v2")

			By("revert to v1, should guarantee compRev v1 still exist")
			appYaml, err = os.ReadFile("./testdata/app/revert-app-envbinding.yaml")
			Expect(err).Should(Succeed())

			Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: app.Name}, checkApp)).Should(BeNil())
			revertApp := &v1beta1.Application{}
			Expect(yaml.Unmarshal([]byte(appYaml), revertApp)).Should(Succeed())
			revertApp.SetNamespace(namespace)
			revertApp.SetResourceVersion(checkApp.ResourceVersion)

			Eventually(func() error {
				if err := k8sClient.Update(hubCtx, revertApp); err != nil {
					return err
				}
				return nil
			}, 30*time.Second).Should(BeNil())
			verifySucceed(componentName + "-v1")
		})

		// HealthScopeController will not work properly with authentication module now
		PIt("Test Rollout with health check policy, guarantee health scope controller work ", func() {
			app := &v1beta1.Application{}
			appYaml, err := os.ReadFile("./testdata/app/multi-cluster-health-policy.yaml")
			Expect(err).Should(Succeed())
			Expect(yaml.Unmarshal([]byte(appYaml), app)).Should(Succeed())
			app.SetNamespace(namespace)
			err = k8sClient.Create(hubCtx, app)
			Expect(err).Should(Succeed())
			verifySucceed(componentName + "-v1")
			Eventually(func() error {
				checkApp := v1beta1.Application{}
				if err := k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: app.Name}, &checkApp); err != nil {
					return err
				}
				if len(checkApp.Status.Services) == 0 {
					return fmt.Errorf("app status service haven't write back")
				}
				compStatus := checkApp.Status.Services[0]
				if compStatus.Env != "staging" {
					return fmt.Errorf("comp status env miss-match")
				}
				if !compStatus.Healthy {
					return fmt.Errorf("comp status not healthy")
				}
				if !strings.Contains(compStatus.Message, "Ready:2/2") {
					return fmt.Errorf("comp status workload check don't work")
				}
				return nil
			}, 30*time.Second).Should(BeNil())
			By("update application to v2")
			checkApp := &v1beta1.Application{}
			Eventually(func() error {
				if err := k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: app.Name}, checkApp); err != nil {
					return err
				}
				checkApp.Spec.Components[0].Properties.Raw = []byte(`{"image": "stefanprodan/podinfo:5.0.2"}`)
				if err := k8sClient.Update(hubCtx, checkApp); err != nil {
					return err
				}
				return nil
			}, 30*time.Second).Should(BeNil())
			verifySucceed(componentName + "-v2")
			Eventually(func() error {
				// Note: KubeVela will only check the workload of the target revision
				checkApp := v1beta1.Application{}
				if err := k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: app.Name}, &checkApp); err != nil {
					return err
				}
				if len(checkApp.Status.Services) == 0 {
					return fmt.Errorf("app status service haven't write back")
				}
				compStatus := checkApp.Status.Services[0]
				if compStatus.Env != "staging" {
					return fmt.Errorf("comp status env miss-match")
				}
				if !compStatus.Healthy {
					return fmt.Errorf("comp status not healthy")
				}
				if !strings.Contains(compStatus.Message, "Ready:2/2") {
					return fmt.Errorf("comp status workload check don't work")
				}
				return nil
			}, 60*time.Second).Should(BeNil())
		})
	})
})
