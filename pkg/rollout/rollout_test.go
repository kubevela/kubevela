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

package rollout

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	kruisev1alpha1 "github.com/openkruise/rollouts/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Kruise rollout test", func() {
	ctx := context.Background()
	BeforeEach(func() {
		Expect(k8sClient.Create(ctx, rollout.DeepCopy())).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		Expect(k8sClient.Create(ctx, rt.DeepCopy())).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		Expect(k8sClient.Create(ctx, app.DeepCopy())).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		Expect(k8sClient.Create(ctx, rollingReleaseRollout.DeepCopy())).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
	})

	It("test get associated rollout func", func() {
		rollouts, err := getAssociatedRollouts(ctx, k8sClient, &app, false)
		Expect(err).Should(BeNil())
		// test will only fetch one rollout in result
		Expect(len(rollouts)).Should(BeEquivalentTo(1))
	})

	It("Suspend rollout", func() {
		r := kruisev1alpha1.Rollout{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "my-rollout"}, &r)).Should(BeNil())
		r.Status.Phase = kruisev1alpha1.RolloutPhaseProgressing
		Expect(k8sClient.Status().Update(ctx, &r)).Should(BeNil())
		Expect(SuspendRollout(ctx, k8sClient, &app, nil))
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "my-rollout"}, &r))
		Expect(r.Spec.Strategy.Paused).Should(BeEquivalentTo(true))
	})

	It("Resume rollout", func() {
		r := kruisev1alpha1.Rollout{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "my-rollout"}, &r)).Should(BeNil())
		Expect(r.Spec.Strategy.Paused).Should(BeEquivalentTo(true))
		Expect(ResumeRollout(ctx, k8sClient, &app, nil))
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "my-rollout"}, &r))
		Expect(r.Spec.Strategy.Paused).Should(BeEquivalentTo(false))
	})

	It("Rollback rollout", func() {
		r := kruisev1alpha1.Rollout{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "my-rollout"}, &r)).Should(BeNil())
		r.Spec.Strategy.Paused = true
		Expect(k8sClient.Update(ctx, &r)).Should(BeNil())
		Expect(RollbackRollout(ctx, k8sClient, &app, nil))
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "my-rollout"}, &r))
		Expect(r.Spec.Strategy.Paused).Should(BeEquivalentTo(false))
	})
})

var app = v1beta1.Application{
	TypeMeta: metav1.TypeMeta{
		APIVersion: "core.oam.dev/v1beta1",
		Kind:       "Application",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:       "rollout-app",
		Namespace:  "default",
		Generation: 1,
	},
	Spec: v1beta1.ApplicationSpec{
		Components: []common.ApplicationComponent{},
	},
}

var rt = v1beta1.ResourceTracker{
	TypeMeta: metav1.TypeMeta{
		APIVersion: "core.oam.dev/v1beta1",
		Kind:       "ResourceTracker",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name: "rollout-app",
		Labels: map[string]string{
			"app.oam.dev/appRevision": "rollout-app-v1",
			"app.oam.dev/name":        "rollout-app",
			"app.oam.dev/namespace":   "default",
		},
	},
	Spec: v1beta1.ResourceTrackerSpec{
		ApplicationGeneration: 1,
		Type:                  v1beta1.ResourceTrackerTypeVersioned,
		ManagedResources: []v1beta1.ManagedResource{
			{
				ClusterObjectReference: common.ClusterObjectReference{
					ObjectReference: v1.ObjectReference{
						APIVersion: "rollouts.kruise.io/v1alpha1",
						Kind:       "Rollout",
						Name:       "my-rollout",
						Namespace:  "default",
					},
				},
				OAMObjectReference: common.OAMObjectReference{
					Component: "my-rollout",
				},
			},
			{
				ClusterObjectReference: common.ClusterObjectReference{
					ObjectReference: v1.ObjectReference{
						APIVersion: "rollouts.kruise.io/v1alpha1",
						Kind:       "Rollout",
						Name:       "rolling-release-rollout",
						Namespace:  "default",
					},
				},
				OAMObjectReference: common.OAMObjectReference{
					Component: "my-rollout",
				},
			},
		},
	},
}

var rollout = kruisev1alpha1.Rollout{
	TypeMeta: metav1.TypeMeta{
		APIVersion: "rollouts.kruise.io/v1alpha1",
		Kind:       "Rollout",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:      "my-rollout",
		Namespace: "default",
	},
	Spec: kruisev1alpha1.RolloutSpec{
		ObjectRef: kruisev1alpha1.ObjectRef{
			WorkloadRef: &kruisev1alpha1.WorkloadRef{
				APIVersion: "appsv1",
				Kind:       "Deployment",
				Name:       "canary-demo",
			},
		},
		Strategy: kruisev1alpha1.RolloutStrategy{
			Canary: &kruisev1alpha1.CanaryStrategy{
				Steps: []kruisev1alpha1.CanaryStep{
					{
						Weight: 30,
					},
				},
			},
			Paused: false,
		},
	},
}

var rollingReleaseRollout = kruisev1alpha1.Rollout{
	TypeMeta: metav1.TypeMeta{
		APIVersion: "rollouts.kruise.io/v1alpha1",
		Kind:       "Rollout",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:      "rolling-release-rollout",
		Namespace: "default",
		Annotations: map[string]string{
			oam.AnnotationSkipResume: "true",
		},
	},
	Spec: kruisev1alpha1.RolloutSpec{
		ObjectRef: kruisev1alpha1.ObjectRef{
			WorkloadRef: &kruisev1alpha1.WorkloadRef{
				APIVersion: "appsv1",
				Kind:       "Deployment",
				Name:       "canary-demo",
			},
		},
		Strategy: kruisev1alpha1.RolloutStrategy{
			Canary: &kruisev1alpha1.CanaryStrategy{
				Steps: []kruisev1alpha1.CanaryStep{
					{
						Weight: 30,
					},
				},
			},
			Paused: false,
		},
	},
}
