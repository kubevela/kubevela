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

package controllers_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	utilcommon "github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("HealthScope", func() {
	ctx := context.Background()
	var namespace string
	var ns corev1.Namespace
	BeforeEach(func() {
		namespace = randomNamespaceName("health-scope-test")
		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(Succeed())

		// create health scope definition
		sd := v1alpha2.ScopeDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "healthscope.core.oam.dev",
				Namespace: "vela-system",
			},
			Spec: v1alpha2.ScopeDefinitionSpec{
				AllowComponentOverlap: true,
				WorkloadRefsPath:      "spec.workloadRefs",
				Reference: common.DefinitionReference{
					Name: "healthscope.core.oam.dev",
				},
			},
		}
		logf.Log.Info("Creating health scope definition")
		Expect(k8sClient.Create(ctx, &sd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})
	AfterEach(func() {
		logf.Log.Info("Clean up resources")
		Expect(k8sClient.DeleteAllOf(ctx, &v1alpha2.HealthScope{}, client.InNamespace(namespace))).Should(BeNil())
		// delete the namespace with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(BeNil())
	})

	It("Test an application with health policy", func() {
		By("Apply a healthy application")
		var newApp v1beta1.Application
		var healthyAppName, unhealthyAppName string
		Expect(utilcommon.ReadYamlToObject("testdata/app/app_healthscope.yaml", &newApp)).Should(BeNil())
		newApp.Namespace = namespace
		Eventually(func() error {
			return k8sClient.Create(ctx, newApp.DeepCopy())
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		healthyAppName = newApp.Name
		By("Get Application latest status")
		Eventually(
			func() *common.Revision {
				var app v1beta1.Application
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: healthyAppName}, &app)
				if app.Status.LatestRevision != nil {
					return app.Status.LatestRevision
				}
				return nil
			},
			time.Second*30, time.Millisecond*500).ShouldNot(BeNil())

		By("Apply an unhealthy application")
		newApp = v1beta1.Application{}
		Expect(utilcommon.ReadYamlToObject("testdata/app/app_healthscope_unhealthy.yaml", &newApp)).Should(BeNil())
		newApp.Namespace = namespace
		Eventually(func() error {
			return k8sClient.Create(ctx, newApp.DeepCopy())
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		unhealthyAppName = newApp.Name
		By("Get Application latest status")
		Eventually(
			func() *common.Revision {
				var app v1beta1.Application
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: unhealthyAppName}, &app)
				if app.Status.LatestRevision != nil {
					return app.Status.LatestRevision
				}
				return nil
			},
			time.Second*30, time.Millisecond*500).ShouldNot(BeNil())

		By("Verify the healthy health scope")
		healthScopeObject := client.ObjectKey{
			Name:      "app-healthscope",
			Namespace: namespace,
		}

		healthScope := &v1alpha2.HealthScope{}
		Expect(k8sClient.Get(ctx, healthScopeObject, healthScope)).Should(Succeed())

		Eventually(
			func() v1alpha2.ScopeHealthCondition {
				*healthScope = v1alpha2.HealthScope{}
				k8sClient.Get(ctx, healthScopeObject, healthScope)
				return healthScope.Status.ScopeHealthCondition
			},
			time.Second*60, time.Millisecond*500).Should(Equal(v1alpha2.ScopeHealthCondition{
			HealthStatus:     v1alpha2.StatusHealthy,
			Total:            int64(2),
			HealthyWorkloads: int64(2),
		}))

		By("Verify the healthy application status")
		Eventually(func() error {
			healthyApp := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: healthyAppName}, healthyApp); err != nil {
				return err
			}
			appCompStatuses := healthyApp.Status.Services
			if len(appCompStatuses) != 2 {
				return fmt.Errorf("expect 2 comp statuses, but got %d", len(appCompStatuses))
			}
			compSts1 := appCompStatuses[0]
			if !compSts1.Healthy || !strings.Contains(compSts1.Message, "Ready:1/1") {
				return fmt.Errorf("expect healthy comp, but %v is unhealthy, msg: %q", compSts1.Name, compSts1.Message)
			}
			if len(compSts1.Traits) != 1 {
				return fmt.Errorf("expect 2 traits statuses, but got %d", len(compSts1.Traits))
			}
			Expect(compSts1.Traits[0].Message).Should(ContainSubstring("No loadBalancer found"))

			return nil
		}, time.Second*30, time.Millisecond*500).Should(Succeed())

		By("Verify the unhealthy health scope")
		healthScopeObject = client.ObjectKey{
			Name:      "app-healthscope-unhealthy",
			Namespace: namespace,
		}

		healthScope = &v1alpha2.HealthScope{}
		Expect(k8sClient.Get(ctx, healthScopeObject, healthScope)).Should(Succeed())

		Eventually(
			func() v1alpha2.ScopeHealthCondition {
				*healthScope = v1alpha2.HealthScope{}
				k8sClient.Get(ctx, healthScopeObject, healthScope)
				return healthScope.Status.ScopeHealthCondition
			},
			time.Second*60, time.Millisecond*500).Should(Equal(v1alpha2.ScopeHealthCondition{
			HealthStatus:       v1alpha2.StatusUnhealthy,
			Total:              int64(2),
			UnhealthyWorkloads: int64(1),
			HealthyWorkloads:   int64(1),
		}))

		By("Verify the unhealthy application status")
		Eventually(func() error {
			unhealthyApp := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: healthyAppName}, unhealthyApp); err != nil {
				return err
			}
			appCompStatuses := unhealthyApp.Status.Services
			if len(appCompStatuses) != 2 {
				return fmt.Errorf("expect 2 comp statuses, but got %d", len(appCompStatuses))
			}
			for _, cSts := range appCompStatuses {
				if cSts.Name == "my-server-unhealthy" {
					unhealthyCompSts := cSts
					if unhealthyCompSts.Healthy || !strings.Contains(unhealthyCompSts.Message, "Ready:0/1") {
						return fmt.Errorf("expect unhealthy comp, but %s is unhealthy, msg: %q", unhealthyCompSts.Name, unhealthyCompSts.Message)
					}
				}
			}
			return nil
		}, time.Second*30, time.Millisecond*500).Should(Succeed())
	})
})
