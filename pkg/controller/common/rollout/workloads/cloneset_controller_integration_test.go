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
	"context"
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

	Context("TestNewCloneSetController", func() {
		It("init a CloneSet Controller", func() {
			recorder := event.NewAPIRecorder(mgr.GetEventRecorderFor("AppRollout")).
				WithAnnotations("controller", "AppRollout")
			parentController := &v1beta1.AppRollout{ObjectMeta: metav1.ObjectMeta{Name: name}}
			rolloutSpec := &v1alpha1.RolloutPlan{
				RolloutBatches: []v1alpha1.RolloutBatch{{
					Replicas: intstr.FromInt(1),
				},
				},
			}
			rolloutStatus := &v1alpha1.RolloutStatus{RollingState: v1alpha1.RolloutSucceedState}
			workloadNamespacedName := client.ObjectKey{Name: name, Namespace: namespace}
			got := NewCloneSetController(k8sClient, recorder, parentController, rolloutSpec, rolloutStatus, workloadNamespacedName)
			c := &CloneSetController{
				client:                 k8sClient,
				recorder:               recorder,
				parentController:       parentController,
				rolloutSpec:            rolloutSpec,
				rolloutStatus:          rolloutStatus,
				workloadNamespacedName: workloadNamespacedName,
			}
			Expect(got).Should(Equal(c))
		})
	})

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

		It("could not fetch CloneSet workload", func() {
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

		It("spec is valid", func() {
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

	Context("TestInitialize", func() {
		BeforeEach(func() {
			ctx = context.TODO()
			namespace = "rollout-ns"
			name = "rollout1"
			namespacedName = client.ObjectKey{Name: name, Namespace: namespace}
			ns = corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}

			By("Create a namespace")
			Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
		})

		It("successfully initialized CloneSet", func() {
			By("Create a CloneSet")
			appRollout := v1beta1.AppRollout{ObjectMeta: metav1.ObjectMeta{Name: name, UID: "567890"}}
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
			cloneSet = kruise.CloneSet{
				TypeMeta: metav1.TypeMeta{APIVersion: "apps.kruise.io/v1alpha1", Kind: "CloneSet"},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      name,
				},
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

			By("initialize")
			initialized, err := c.Initialize(ctx)
			Expect(initialized).Should(BeTrue())
			Expect(err).Should(BeNil())

			By("clean up")
			var w kruise.CloneSet
			k8sClient.Get(ctx, namespacedName, &w)
			k8sClient.Delete(ctx, &w)
		})

		It("failed to get totalReplicas due to no CloneSet", func() {
			appRollout := v1beta1.AppRollout{ObjectMeta: metav1.ObjectMeta{Name: name}}
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

			By("initializing")
			initialized, err := c.Initialize(ctx)
			Expect(initialized).Should(BeFalse())
			Expect(err).Should(BeNil())
		})

		It("workload CloneSet is controlled by upper resource", func() {
			By("Create a CloneSet")
			var controllered = true
			appRollout := v1beta1.AppRollout{ObjectMeta: metav1.ObjectMeta{Name: name}}
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
			cloneSet = kruise.CloneSet{
				TypeMeta: metav1.TypeMeta{APIVersion: "apps.kruise.io/v1alpha1", Kind: "CloneSet"},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      name,
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: v1beta1.SchemeGroupVersion.String(),
						Kind:       v1beta1.AppRolloutKind,
						Name:       "def",
						UID:        "123456",
						Controller: &controllered,
					}},
				},
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

			By("initializing")
			initialized, err := c.Initialize(ctx)
			Expect(initialized).Should(BeTrue())
			Expect(err).Should(BeNil())

			By("clean up")
			var w kruise.CloneSet
			k8sClient.Get(ctx, namespacedName, &w)
			k8sClient.Delete(ctx, &w)
		})

		It("failed to patch the owner of CloneSet", func() {
			By("Create a CloneSet")
			appRollout := v1beta1.AppRollout{ObjectMeta: metav1.ObjectMeta{Name: name}}
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
			cloneSet = kruise.CloneSet{
				TypeMeta: metav1.TypeMeta{APIVersion: "apps.kruise.io/v1alpha1", Kind: "CloneSet"},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      name,
				},
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

			By("initialize")
			initialized, err := c.Initialize(ctx)
			Expect(initialized).Should(BeFalse())
			Expect(err).Should(BeNil())

			By("clean up")
			var w kruise.CloneSet
			k8sClient.Get(ctx, namespacedName, &w)
			k8sClient.Delete(ctx, &w)
		})

	})

	Context("TestRolloutOneBatchPods", func() {
		BeforeEach(func() {
			ctx = context.TODO()
			namespace = "rollout-ns"
			name = "rollout1"
			namespacedName = client.ObjectKey{Name: name, Namespace: namespace}
			ns = corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}

			By("Create a namespace")
			Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
		})

		It("successfully rollout, current batch number is not equal to the expected one", func() {
			By("Create a CloneSet")
			appRollout := v1beta1.AppRollout{ObjectMeta: metav1.ObjectMeta{Name: name}}
			c = CloneSetController{
				client: k8sClient,
				rolloutSpec: &v1alpha1.RolloutPlan{
					RolloutBatches: []v1alpha1.RolloutBatch{{
						Replicas: intstr.FromInt(1),
					},
					},
				},
				rolloutStatus: &v1alpha1.RolloutStatus{
					RollingState: v1alpha1.RolloutSucceedState,
					CurrentBatch: int32(2),
				},
				parentController: &appRollout,
				recorder: event.NewAPIRecorder(mgr.GetEventRecorderFor("AppRollout")).
					WithAnnotations("controller", "AppRollout"),
				workloadNamespacedName: namespacedName,
			}
			cloneSet = kruise.CloneSet{
				TypeMeta: metav1.TypeMeta{APIVersion: "apps.kruise.io/v1alpha1", Kind: "CloneSet"},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      name,
				},
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

			By("rollout")
			done, err := c.RolloutOneBatchPods(ctx)
			Expect(done).Should(BeTrue())
			Expect(err).Should(BeNil())

			By("clean up")
			var w kruise.CloneSet
			k8sClient.Get(ctx, namespacedName, &w)
			k8sClient.Delete(ctx, &w)
		})

		It("successfully rollout, current batch number is equal to the expected one", func() {
			By("Create a CloneSet")
			appRollout := v1beta1.AppRollout{ObjectMeta: metav1.ObjectMeta{Name: name}}
			c = CloneSetController{
				client: k8sClient,
				rolloutSpec: &v1alpha1.RolloutPlan{
					RolloutBatches: []v1alpha1.RolloutBatch{
						{
							Replicas: intstr.FromInt(1),
						},

						{
							Replicas: intstr.FromInt(2),
						},
						{
							Replicas: intstr.FromInt(3),
						},
					},
				},
				rolloutStatus: &v1alpha1.RolloutStatus{
					RollingState: v1alpha1.RolloutSucceedState,
					CurrentBatch: int32(2),
				},
				parentController: &appRollout,
				recorder: event.NewAPIRecorder(mgr.GetEventRecorderFor("AppRollout")).
					WithAnnotations("controller", "AppRollout"),
				workloadNamespacedName: namespacedName,
			}
			cloneSet = kruise.CloneSet{
				TypeMeta: metav1.TypeMeta{APIVersion: "apps.kruise.io/v1alpha1", Kind: "CloneSet"},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      name,
				},
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

			By("checking")
			done, err := c.RolloutOneBatchPods(ctx)
			Expect(done).Should(BeTrue())
			Expect(err).Should(BeNil())

			By("clean up")
			var w kruise.CloneSet
			k8sClient.Get(ctx, namespacedName, &w)
			k8sClient.Delete(ctx, &w)
		})
	})

	Context("TestCheckOneBatchPods", func() {
		BeforeEach(func() {
			ctx = context.TODO()
			namespace = "rollout-ns"
			name = "rollout1"
			namespacedName = client.ObjectKey{Name: name, Namespace: namespace}
			ns = corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}

			By("Create a namespace")
			Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
		})

		It("failed to check batch Pod when current batch number is less than expected one", func() {
			By("Create a CloneSet")
			appRollout := v1beta1.AppRollout{ObjectMeta: metav1.ObjectMeta{Name: name}}
			c = CloneSetController{
				client: k8sClient,
				rolloutSpec: &v1alpha1.RolloutPlan{
					RolloutBatches: []v1alpha1.RolloutBatch{{
						Replicas: intstr.FromInt(1),
					},
					},
				},
				rolloutStatus: &v1alpha1.RolloutStatus{
					RollingState: v1alpha1.RolloutSucceedState,
					CurrentBatch: int32(0),
				},
				parentController: &appRollout,
				recorder: event.NewAPIRecorder(mgr.GetEventRecorderFor("AppRollout")).
					WithAnnotations("controller", "AppRollout"),
				workloadNamespacedName: namespacedName,
			}
			cloneSet = kruise.CloneSet{
				TypeMeta: metav1.TypeMeta{APIVersion: "apps.kruise.io/v1alpha1", Kind: "CloneSet"},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      name,
				},
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

			By("checking")
			done, err := c.CheckOneBatchPods(ctx)
			Expect(done).Should(BeFalse())
			Expect(err).Should(BeNil())

			By("clean up")
			var w kruise.CloneSet
			k8sClient.Get(ctx, namespacedName, &w)
			k8sClient.Delete(ctx, &w)
		})

		It("failed to check batch Pod when current batch number exceeds the expected ones", func() {
			By("Create a CloneSet")
			appRollout := v1beta1.AppRollout{ObjectMeta: metav1.ObjectMeta{Name: name}}
			c = CloneSetController{
				client: k8sClient,
				rolloutSpec: &v1alpha1.RolloutPlan{
					RolloutBatches: []v1alpha1.RolloutBatch{{
						Replicas: intstr.FromInt(1),
					},
					},
				},
				rolloutStatus: &v1alpha1.RolloutStatus{
					RollingState: v1alpha1.RolloutSucceedState,
					CurrentBatch: int32(2),
				},
				parentController: &appRollout,
				recorder: event.NewAPIRecorder(mgr.GetEventRecorderFor("AppRollout")).
					WithAnnotations("controller", "AppRollout"),
				workloadNamespacedName: namespacedName,
			}
			cloneSet = kruise.CloneSet{
				TypeMeta: metav1.TypeMeta{APIVersion: "apps.kruise.io/v1alpha1", Kind: "CloneSet"},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      name,
				},
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

			By("checking")
			done, err := c.CheckOneBatchPods(ctx)
			Expect(done).Should(BeFalse())
			Expect(err).Should(BeNil())

			By("clean up")
			var w kruise.CloneSet
			k8sClient.Get(ctx, namespacedName, &w)
			k8sClient.Delete(ctx, &w)
		})

		It("the pods are all available according to the rollout plan", func() {
			By("create a CloneSet")
			appRollout := v1beta1.AppRollout{ObjectMeta: metav1.ObjectMeta{Name: name}}
			cloneSet = kruise.CloneSet{
				TypeMeta: metav1.TypeMeta{APIVersion: "apps.kruise.io/v1alpha1", Kind: "CloneSet"},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      name,
				},
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

			By("add CloneSetController properties")
			cloneSet.Status.UpdatedReadyReplicas = 2
			c = CloneSetController{
				client: k8sClient,
				rolloutSpec: &v1alpha1.RolloutPlan{
					RolloutBatches: []v1alpha1.RolloutBatch{
						{
							Replicas: intstr.FromInt(1),
						},
						{
							Replicas: intstr.FromInt(1),
						},
						{
							Replicas: intstr.FromInt(2),
						},
					},
				},
				rolloutStatus: &v1alpha1.RolloutStatus{
					RollingState: v1alpha1.RolloutSucceedState,
					CurrentBatch: int32(2),
				},
				parentController: &appRollout,
				recorder: event.NewAPIRecorder(mgr.GetEventRecorderFor("AppRollout")).
					WithAnnotations("controller", "AppRollout"),
				workloadNamespacedName: namespacedName,
				cloneSet:               &cloneSet,
			}

			By("checking")
			done, err := c.CheckOneBatchPods(ctx)
			Expect(done).Should(BeTrue())
			Expect(err).Should(BeNil())

			By("clean up")
			var w kruise.CloneSet
			k8sClient.Get(ctx, namespacedName, &w)
			k8sClient.Delete(ctx, &w)
		})
	})

	Context("TestFinalizeOneBatch", func() {
		It("finalize one batch", func() {
			c = CloneSetController{}
			finalized, err := c.FinalizeOneBatch(context.TODO())
			Expect(finalized).Should(BeTrue())
			Expect(err).Should(BeNil())
		})
	})

	Context("TestFinalize", func() {
		BeforeEach(func() {
			ctx = context.TODO()
			namespace = "rollout-ns"
			name = "rollout1"
			namespacedName = client.ObjectKey{Name: name, Namespace: namespace}
			ns = corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}

			By("Create a namespace")
			Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
		})

		It("failed to fetch CloneSet", func() {
			By("finalizing")
			appRollout := v1beta1.AppRollout{ObjectMeta: metav1.ObjectMeta{Name: name, UID: "123456"}}
			c = CloneSetController{
				client: k8sClient,
				rolloutSpec: &v1alpha1.RolloutPlan{
					RolloutBatches: []v1alpha1.RolloutBatch{{
						Replicas: intstr.FromInt(1),
					},
					},
				},
				rolloutStatus: &v1alpha1.RolloutStatus{
					RollingState: v1alpha1.RolloutSucceedState,
					CurrentBatch: int32(0),
				},
				parentController: &appRollout,
				recorder: event.NewAPIRecorder(mgr.GetEventRecorderFor("AppRollout")).
					WithAnnotations("controller", "AppRollout"),
				workloadNamespacedName: namespacedName,
			}
			finalized := c.Finalize(ctx, true)
			Expect(finalized).Should(BeFalse())
		})

		It("successfully to finalize CloneSet", func() {
			By("Create a CloneSet")
			appRollout := v1beta1.AppRollout{ObjectMeta: metav1.ObjectMeta{Name: name, UID: "123456"}}
			c = CloneSetController{
				client: k8sClient,
				rolloutSpec: &v1alpha1.RolloutPlan{
					RolloutBatches: []v1alpha1.RolloutBatch{{
						Replicas: intstr.FromInt(1),
					},
					},
				},
				rolloutStatus: &v1alpha1.RolloutStatus{
					RollingState: v1alpha1.RolloutSucceedState,
					CurrentBatch: int32(0),
				},
				parentController: &appRollout,
				recorder: event.NewAPIRecorder(mgr.GetEventRecorderFor("AppRollout")).
					WithAnnotations("controller", "AppRollout"),
				workloadNamespacedName: namespacedName,
			}
			cloneSet = kruise.CloneSet{
				TypeMeta: metav1.TypeMeta{APIVersion: "apps.kruise.io/v1alpha1", Kind: "CloneSet"},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      name,
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: v1beta1.SchemeGroupVersion.String(),
						Kind:       "Kind1",
						Name:       "def",
						UID:        "123456",
					}},
				},
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

			By("finalizing")
			finalized := c.Finalize(ctx, true)
			Expect(finalized).Should(BeTrue())

			By("clean up")
			var w kruise.CloneSet
			k8sClient.Get(ctx, namespacedName, &w)
			k8sClient.Delete(ctx, &w)
		})
	})
})
