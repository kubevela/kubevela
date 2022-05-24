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

package service

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	apiv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("Test oam application service function", func() {
	var oamAppService *oamApplicationServiceImpl
	var ctx context.Context
	var baseApp v1beta1.Application
	var ns corev1.Namespace
	var namespace string

	BeforeEach(func() {
		ctx = context.Background()
		namespace = randomNamespaceName("test-oam-app")
		ns = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		oamAppService = &oamApplicationServiceImpl{
			KubeClient: k8sClient,
		}
		Expect(common.ReadYamlToObject("./testdata/example-app.yaml", &baseApp)).Should(BeNil())

		Eventually(func() error {
			return k8sClient.Create(ctx, &ns)
		}, time.Second*3, time.Microsecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		baseApp.SetNamespace(namespace)
		Eventually(func() error {
			return k8sClient.Create(ctx, &baseApp)
		}, time.Second*3, time.Microsecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})

	AfterEach(func() {
		By("Clean up resources after a test")
		k8sClient.DeleteAllOf(ctx, &v1beta1.Application{}, client.InNamespace(namespace))
		baseApp = v1beta1.Application{}
		By(fmt.Sprintf("Delete the entire namespaceName %s", ns.Name))
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(Succeed())
	})

	It("Test CreateOrUpdateOAMApplication function", func() {
		By("test create application")
		appName := "test-new-app"
		appNs := randomNamespaceName("test-new-app")
		req := apiv1.ApplicationRequest{
			Components: baseApp.Spec.Components,
			Policies:   baseApp.Spec.Policies,
			Workflow:   baseApp.Spec.Workflow,
		}
		Expect(oamAppService.CreateOrUpdateOAMApplication(ctx, req, appName, appNs)).Should(BeNil())

		app := new(v1beta1.Application)
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: appNs, Name: appName}, app)).Should(BeNil())
		Expect(app.Spec.Components).Should(Equal(req.Components))
		Expect(app.Spec.Policies).Should(Equal(req.Policies))
		Expect(app.Spec.Workflow).Should(Equal(req.Workflow))

		By("test update application")
		updateReq := apiv1.ApplicationRequest{
			Components: baseApp.Spec.Components[1:],
		}
		Expect(oamAppService.CreateOrUpdateOAMApplication(ctx, updateReq, appName, appNs)).Should(BeNil())

		updatedApp := new(v1beta1.Application)
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: appNs, Name: appName}, updatedApp)).Should(BeNil())
		Expect(updatedApp.Spec.Components).Should(Equal(updateReq.Components))
		Expect(updatedApp.Spec.Policies).Should(BeNil())
		Expect(updatedApp.Spec.Workflow).Should(BeNil())
	})

	It("Test GetOAMApplication function", func() {
		By("test get an existed application")
		resp, err := oamAppService.GetOAMApplication(ctx, baseApp.Name, namespace)
		Expect(err).Should(BeNil())

		Expect(resp.Spec.Components).Should(Equal(baseApp.Spec.Components))
		Expect(resp.Spec.Policies).Should(Equal(baseApp.Spec.Policies))
		Expect(resp.Spec.Workflow).Should(Equal(baseApp.Spec.Workflow))
	})

	It("Test DeleteOAMApplication function", func() {
		By("test delete application")
		app := new(v1beta1.Application)
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: baseApp.Name}, app)).Should(BeNil())

		Expect(oamAppService.DeleteOAMApplication(ctx, baseApp.Name, namespace)).Should(BeNil())
		err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: baseApp.Name}, app)
		Expect(kerrors.IsNotFound(err)).Should(BeTrue())
	})
})
