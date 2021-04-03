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
	"fmt"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	kruise "github.com/openkruise/kruise-api/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("cloneset controller", func() {
	var (
		c              CloneSetController
		ns             corev1.Namespace
		name           string
		namespace      string
		cloneSet       kruise.CloneSet
		namespacedName client.ObjectKey
	)
	Context("VerifySpec", func() {
		BeforeEach(func() {
			namespace = "rollout-ns"
			name = "rollout1"
			appRollout := v1beta1.AppRollout{ObjectMeta: metav1.ObjectMeta{Name: name}}
			namespacedName = client.ObjectKey{Name: name, Namespace: namespace}
			c = CloneSetController{
				client: k8sClient,
				rolloutSpec: &v1alpha1.RolloutPlan{
					RolloutBatches: []v1alpha1.RolloutBatch{{
						Replicas: intstr.FromInt(1),
					},
					},
				},
				rolloutStatus:    &v1alpha1.RolloutStatus{RollingState: v1alpha1.RolloutSucceedState},
				parentController: &appRollout,
				recorder: event.NewAPIRecorder(mgr.GetEventRecorderFor("AppRollout")).
					WithAnnotations("controller", "AppRollout"),
				workloadNamespacedName: namespacedName,
			}

			ns = corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			By("Create a namespace")
			Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
		})

		It("Could not fetch CloneSet workload", func() {
			consistent, err := c.VerifySpec(ctx)
			Expect(err).Should(BeNil())
			Expect(consistent).Should(BeFalse())
		})

		It("there is no difference between the source and target", func() {
			By("Create a CloneSet")
			cloneSet = kruise.CloneSet{
				TypeMeta:   metav1.TypeMeta{APIVersion: "apps.kruise.io/v1alpha1", Kind: "CloneSet"},
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
				Spec: kruise.CloneSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"env": "staging"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"env": "staging"}},
						Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: name, Image: "nginx"}}},
					},
				},
			}
			Expect(k8sClient.Create(ctx, &cloneSet)).Should(Succeed())

			By("verify")
			consistent, err := c.VerifySpec(ctx)
			Expect(err).Should(Equal(fmt.Errorf("there is no difference between the source and target, hash = ")))
			Expect(consistent).Should(BeFalse())

			By("clean up")
			var w kruise.CloneSet
			k8sClient.Get(ctx, namespacedName, &w)
			k8sClient.Delete(ctx, &w)
		})

		It("the cloneset is in the middle of updating", func() {
			By("Create a CloneSet")
			cloneSet = kruise.CloneSet{
				TypeMeta:   metav1.TypeMeta{APIVersion: "apps.kruise.io/v1alpha1", Kind: "CloneSet"},
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
				Spec: kruise.CloneSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"env": "staging"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"env": "staging"}},
						Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: name, Image: "nginx"}}},
					},
				},
			}
			Expect(k8sClient.Create(ctx, &cloneSet)).Should(Succeed())

			By("setting more field for CloneSetController")
			c.rolloutStatus.LastAppliedPodTemplateIdentifier = "abc"

			By("verify")
			consistent, err := c.VerifySpec(ctx)
			Expect(err).Should(Equal(fmt.Errorf("the cloneset rollout1 is in the middle of updating, need to be paused first")))
			Expect(consistent).Should(BeFalse())

			By("clean up")
			var w kruise.CloneSet
			k8sClient.Get(ctx, namespacedName, &w)
			k8sClient.Delete(ctx, &w)
		})

		It("xxx", func() {
			By("Create a CloneSet")
			cloneSet = kruise.CloneSet{
				TypeMeta:   metav1.TypeMeta{APIVersion: "apps.kruise.io/v1alpha1", Kind: "CloneSet"},
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
				Spec: kruise.CloneSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"env": "staging"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"env": "staging"}},
						Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: name, Image: "nginx"}}},
					},
					UpdateStrategy: kruise.CloneSetUpdateStrategy{
						Paused: true,
					},
				},
			}
			Expect(k8sClient.Create(ctx, &cloneSet)).Should(Succeed())

			By("setting more field for CloneSetController")
			c.rolloutStatus.LastAppliedPodTemplateIdentifier = "abc"

			By("verify")
			consistent, err := c.VerifySpec(ctx)
			Expect(err).Should(BeNil())
			Expect(consistent).Should(BeTrue())

			By("clean up")
			var w kruise.CloneSet
			k8sClient.Get(ctx, namespacedName, &w)
			k8sClient.Delete(ctx, &w)
		})
	})
})
