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

package workloads

import (
	"github.com/crossplane/crossplane-runtime/pkg/event"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("deployment controller", func() {
	var (
		c         DeploymentController
		ns        corev1.Namespace
		name      string
		namespace string
		//appRollout v1beta1.AppRollout
		deploy v1.Deployment
	)
	Context("VerifySpec", func() {
		BeforeEach(func() {
			namespace = "rollout-ns"
			name = "rollout1"
			appRollout := v1beta1.AppRollout{ObjectMeta: metav1.ObjectMeta{Name: name}}
			c = DeploymentController{
				client: k8sClient,
				rolloutSpec: &v1alpha1.RolloutPlan{
					RolloutBatches: []v1alpha1.RolloutBatch{{
						Replicas: intstr.FromInt(1),
					},
					},
				},
				sourceNamespacedName: types.NamespacedName{Namespace: namespace, Name: name},
				rolloutStatus:        &v1alpha1.RolloutStatus{RollingState: v1alpha1.RolloutSucceedState},
				parentController:     &appRollout,
				recorder: event.NewAPIRecorder(mgr.GetEventRecorderFor("AppRollout")).
					WithAnnotations("controller", "AppRollout"),
			}

			ns = corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			By("Create a namespace")
			Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
		})

		It("Could not fetch deployment workload", func() {
			consistent, err := c.VerifySpec(ctx)
			Expect(err).Should(BeNil())
			Expect(consistent).Should(BeFalse())
		})

		It("Could not fetch deployment workload", func() {
			By("Create a deployment")
			deploy = v1.Deployment{
				TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
				Spec: v1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"env": "staging"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"env": "staging"}},
						Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: name, Image: "nignx"}}},
					},
				},
			}
			Expect(k8sClient.Create(ctx, &deploy)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("verify")
			consistent, err := c.VerifySpec(ctx)
			Expect(err).Should(BeNil())
			Expect(consistent).Should(BeFalse())
		})
	})
})
