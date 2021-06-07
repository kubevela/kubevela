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
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oamcomm "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("ComponentDefinition Normal tests", func() {
	ctx := context.Background()

	var namespace string
	var ns corev1.Namespace
	var app v1beta1.Application

	BeforeEach(func() {
		namespace = randomNamespaceName("app-dep-e2e-test")
		ns = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}

		Eventually(func() error {
			return k8sClient.Create(ctx, &ns)
		}, time.Second*3, time.Microsecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})

	AfterEach(func() {
		By("Clean up resources after a test")
		k8sClient.DeleteAllOf(ctx, &v1beta1.Application{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.AppDeployment{}, client.InNamespace(namespace))

		By(fmt.Sprintf("Delete the entire namespaceName %s", ns.Name))
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(Succeed())
	})

	applyApp := func(source string) {
		By("Apply an application")
		var newApp v1beta1.Application
		Expect(common.ReadYamlToObject("testdata/appdeployment/"+source, &newApp)).Should(BeNil())
		newApp.Namespace = namespace
		Expect(k8sClient.Create(ctx, &newApp)).Should(Succeed())

		By("Get Application latest status")
		Eventually(
			func() *oamcomm.Revision {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: newApp.Name}, &app)
				if app.Status.LatestRevision != nil {
					return app.Status.LatestRevision
				}
				return nil
			},
			time.Second*30, time.Millisecond*500).ShouldNot(BeNil())
	}

	Context("Test validating admission control for appDeployment", func() {
		It("Test appDeployment which only set appRevisions", func() {
			applyApp("application.yaml")

			var appDeployment v1beta1.AppDeployment
			Expect(common.ReadYamlToObject("testdata/appdeployment/appdeployment-1.yaml", &appDeployment)).Should(BeNil())
			appDeployment.Namespace = namespace
			Expect(k8sClient.Create(ctx, &appDeployment)).Should(Succeed())
			Eventually(
				func() *v1beta1.AppDeploymentPhase {
					k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appDeployment.Name}, &appDeployment)
					if appDeployment.Status.Phase != v1beta1.PhaseCompleted {
						return &appDeployment.Status.Phase
					}
					return nil
				},
				time.Second*30, time.Millisecond*500).ShouldNot(BeNil())
		})

		It("Test appDeployment when not applied application", func() {
			var appDeployment v1beta1.AppDeployment
			Expect(common.ReadYamlToObject("testdata/appdeployment/appdeployment-1.yaml", &appDeployment)).Should(BeNil())
			appDeployment.Namespace = namespace
			Expect(k8sClient.Create(ctx, &appDeployment)).Should(HaveOccurred())
		})
	})
})
