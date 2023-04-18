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

package operation

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	kruisev1alpha1 "github.com/openkruise/rollouts/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Kruise rollout test", func() {
	ctx := context.Background()
	BeforeEach(func() {
		Expect(k8sClient.Create(ctx, myRollout.DeepCopy())).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		Expect(k8sClient.Create(ctx, rt.DeepCopy())).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		Expect(k8sClient.Create(ctx, app.DeepCopy())).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
	})

	It("Suspend workflow", func() {
		checkApp := v1beta1.Application{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "opt-app"}, &checkApp)).Should(BeNil())
		checkApp.Status.Workflow = &common.WorkflowStatus{
			Suspend:   false,
			StartTime: metav1.Now(),
		}
		Expect(k8sClient.Status().Update(ctx, &checkApp)).Should(BeNil())
		operator := NewApplicationWorkflowOperator(k8sClient, nil, checkApp.DeepCopy())
		checkApp = v1beta1.Application{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "opt-app"}, &checkApp)).Should(BeNil())
		Expect(operator.Suspend(ctx)).Should(BeNil())
		checkApp = v1beta1.Application{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "opt-app"}, &checkApp)).Should(BeNil())
		Expect(checkApp.Status.Workflow.Suspend).Should(BeEquivalentTo(true))
	})

	It("Resume workflow", func() {
		checkApp := v1beta1.Application{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "opt-app"}, &checkApp)).Should(BeNil())
		operator := NewApplicationWorkflowOperator(k8sClient, nil, checkApp.DeepCopy())
		Expect(operator.Resume(ctx)).Should(BeNil())
		checkApp = v1beta1.Application{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "opt-app"}, &checkApp)).Should(BeNil())
		Expect(checkApp.Status.Workflow.Suspend).Should(BeEquivalentTo(false))
	})

	It("Terminate workflow", func() {
		checkApp := v1beta1.Application{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "opt-app"}, &checkApp)).Should(BeNil())
		checkApp.Status.Workflow = &common.WorkflowStatus{
			Steps: []workflowv1alpha1.WorkflowStepStatus{
				{
					StepStatus: workflowv1alpha1.StepStatus{
						Name:  "step1",
						Type:  "suspend",
						Phase: workflowv1alpha1.WorkflowStepPhaseSuspending,
					},
				},
			},
		}
		operator := NewApplicationWorkflowOperator(k8sClient, nil, checkApp.DeepCopy())
		Expect(operator.Terminate(ctx)).Should(BeNil())
		checkApp = v1beta1.Application{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "opt-app"}, &checkApp)).Should(BeNil())
		Expect(checkApp.Status.Workflow.Terminated).Should(BeEquivalentTo(true))
	})

	It("Restart workflow", func() {
		checkApp := v1beta1.Application{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "opt-app"}, &checkApp)).Should(BeNil())
		operator := NewApplicationWorkflowOperator(k8sClient, nil, checkApp.DeepCopy())
		Expect(operator.Restart(ctx)).Should(BeNil())
		checkApp = v1beta1.Application{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "opt-app"}, &checkApp)).Should(BeNil())
		Expect(checkApp.Status.Workflow).Should(BeNil())
	})

	It("Resume workflow from step", func() {
		checkApp := v1beta1.Application{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "opt-app"}, &checkApp)).Should(BeNil())
		checkApp.Status.Workflow = &common.WorkflowStatus{
			Steps: []workflowv1alpha1.WorkflowStepStatus{
				{
					StepStatus: workflowv1alpha1.StepStatus{
						Name:  "step1",
						Type:  "suspend",
						Phase: workflowv1alpha1.WorkflowStepPhaseSuspending,
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, &checkApp)).Should(BeNil())
		operator := NewApplicationWorkflowStepOperator(k8sClient, nil, checkApp.DeepCopy())
		Expect(operator.Resume(ctx, "step1")).Should(BeNil())
		checkApp = v1beta1.Application{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "opt-app"}, &checkApp)).Should(BeNil())
		Expect(checkApp.Status.Workflow.Suspend).Should(BeEquivalentTo(false))
	})

	It("Restart workflow from step", func() {
		checkApp := v1beta1.Application{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "opt-app"}, &checkApp)).Should(BeNil())
		checkApp.Status.Workflow = &common.WorkflowStatus{
			Steps: []workflowv1alpha1.WorkflowStepStatus{
				{
					StepStatus: workflowv1alpha1.StepStatus{
						Name:  "step1",
						Phase: workflowv1alpha1.WorkflowStepPhaseFailed,
					},
				},
			},
		}
		Expect(k8sClient.Status().Update(ctx, &checkApp)).Should(BeNil())
		operator := NewApplicationWorkflowStepOperator(k8sClient, nil, checkApp.DeepCopy())
		Expect(operator.Restart(ctx, "step1")).Should(BeNil())
		checkApp = v1beta1.Application{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "opt-app"}, &checkApp)).Should(BeNil())
		Expect(checkApp.Status.Workflow.Steps).Should(BeNil())
	})

	It("Rollback workflow", func() {
		Expect(k8sClient.Create(ctx, &appRev)).Should(BeNil())
		checkAppRev := v1beta1.ApplicationRevision{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "opt-app-v1"}, &checkAppRev)).Should(BeNil())
		checkAppRev.Status.Succeeded = true
		Expect(k8sClient.Status().Update(ctx, checkAppRev.DeepCopy())).Should(BeNil())

		checkApp := v1beta1.Application{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "opt-app"}, &checkApp)).Should(BeNil())
		checkApp.Status.Workflow = &common.WorkflowStatus{
			Finished: true,
		}
		Expect(k8sClient.Status().Update(ctx, &checkApp)).Should(BeNil())
		checkApp.Annotations = map[string]string{
			oam.AnnotationPublishVersion: "v2",
		}
		operator := NewApplicationWorkflowOperator(k8sClient, nil, checkApp.DeepCopy())
		Expect(operator.Rollback(ctx)).Should(BeNil())

		checkApp = v1beta1.Application{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "opt-app"}, &checkApp)).Should(BeNil())
		// must rollback to v1
		Expect(oam.GetPublishVersion(&checkApp)).Should(BeEquivalentTo("v1"))
		Expect(checkApp.Status.LatestRevision.Name).Should(BeEquivalentTo("opt-app-v1"))
		Expect(checkApp.Status.LatestRevision.Revision).Should(BeEquivalentTo(1))
	})
})

var app = v1beta1.Application{
	TypeMeta: metav1.TypeMeta{
		APIVersion: "core.oam.dev/v1beta1",
		Kind:       "Application",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:       "opt-app",
		Namespace:  "default",
		Generation: 1,
		Labels: map[string]string{
			oam.AnnotationPublishVersion: "v2",
		},
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
			"app.oam.dev/appRevision": "opt-app-v1",
			"app.oam.dev/name":        "opt-app",
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
		},
	},
}

var appRev = v1beta1.ApplicationRevision{
	TypeMeta: metav1.TypeMeta{
		APIVersion: "core.oam.dev/v1beta1",
		Kind:       "ApplicationRevision",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:      "opt-app-v1",
		Namespace: "default",
		Labels: map[string]string{
			"app.oam.dev/name": "opt-app",
		},
		Annotations: map[string]string{
			oam.AnnotationPublishVersion: "v1",
		},
	},
	Spec: v1beta1.ApplicationRevisionSpec{
		ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
			Application: v1beta1.Application{
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{},
				},
			},
		},
	},
}

var myRollout = kruisev1alpha1.Rollout{
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
