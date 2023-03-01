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

package application

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	wfTypes "github.com/kubevela/workflow/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/testutil"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test Workflow", func() {
	ctx := context.Background()
	namespace := "test-ns"

	appWithWorkflow := &oamcore.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: namespace,
		},
		Spec: oamcore.ApplicationSpec{
			Components: []common.ApplicationComponent{{
				Name:       "test-component",
				Type:       "worker",
				Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
			}},
			Workflow: &oamcore.Workflow{
				Steps: []workflowv1alpha1.WorkflowStep{{
					WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
						Name:       "test-wf1",
						Type:       "foowf",
						Properties: &runtime.RawExtension{Raw: []byte(`{"namespace":"test-ns"}`)},
					},
				}},
			},
		},
	}
	appWithWorkflowAndPolicy := appWithWorkflow.DeepCopy()
	appWithWorkflowAndPolicy.Name = "test-wf-policy"
	appWithWorkflowAndPolicy.Spec.Policies = []oamcore.AppPolicy{{
		Name:       "test-policy-and-wf",
		Type:       "foopolicy",
		Properties: &runtime.RawExtension{Raw: []byte(`{"key":"test"}`)},
	}}

	appWithPolicy := &oamcore.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app-only-with-policy",
			Namespace: namespace,
		},
		Spec: oamcore.ApplicationSpec{
			Components: []common.ApplicationComponent{{
				Name:       "test-component-with-policy",
				Type:       "worker",
				Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
			}},
			Policies: []oamcore.AppPolicy{{
				Name:       "test-policy",
				Type:       "foopolicy",
				Properties: &runtime.RawExtension{Raw: []byte(`{"key":"test"}`)},
			}},
		},
	}

	testDefinitions := []string{componentDefYaml, policyDefYaml, wfStepDefYaml}

	BeforeEach(func() {
		setupFooCRD(ctx)
		setupNamespace(ctx, namespace)
		setupTestDefinitions(ctx, testDefinitions, namespace)
		By("[TEST] Set up definitions before integration test")
	})

	AfterEach(func() {
		Expect(k8sClient.DeleteAllOf(ctx, &appsv1.ControllerRevision{}, client.InNamespace(namespace))).Should(Succeed())
	})

	It("should create workload in application when policy is specified", func() {
		Expect(k8sClient.Create(ctx, appWithPolicy)).Should(BeNil())

		// first try to add finalizer
		tryReconcile(reconciler, appWithPolicy.Name, appWithPolicy.Namespace)
		tryReconcile(reconciler, appWithPolicy.Name, appWithPolicy.Namespace)

		deploy := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      appWithPolicy.Spec.Components[0].Name,
			Namespace: appWithPolicy.Namespace,
		}, deploy)).Should(BeNil())

		policyObj := &unstructured.Unstructured{}
		policyObj.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "example.com",
			Kind:    "Foo",
			Version: "v1",
		})

		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      "test-policy",
			Namespace: appWithPolicy.Namespace,
		}, policyObj)).Should(BeNil())
	})

	It("should create workload in policy", func() {
		appWithPolicyAndWorkflow := appWithWorkflow.DeepCopy()
		appWithPolicyAndWorkflow.Spec.Policies = []oamcore.AppPolicy{{
			Name:       "test-foo-policy",
			Type:       "foopolicy",
			Properties: &runtime.RawExtension{Raw: []byte(`{"key":"test"}`)},
		}}

		Expect(k8sClient.Create(ctx, appWithPolicyAndWorkflow)).Should(BeNil())

		// first try to add finalizer
		tryReconcile(reconciler, appWithPolicyAndWorkflow.Name, appWithPolicyAndWorkflow.Namespace)
		tryReconcile(reconciler, appWithPolicyAndWorkflow.Name, appWithPolicyAndWorkflow.Namespace)

		appRev := &oamcore.ApplicationRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      appWithWorkflow.Name + "-v1",
			Namespace: namespace,
		}, appRev)).Should(BeNil())

		policyObj := &unstructured.Unstructured{}
		policyObj.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "example.com",
			Kind:    "Foo",
			Version: "v1",
		})

		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      "test-foo-policy",
			Namespace: appWithPolicyAndWorkflow.Namespace,
		}, policyObj)).Should(BeNil())

		// check resource created
		stepObj := &unstructured.Unstructured{}
		stepObj.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "example.com",
			Kind:    "Foo",
			Version: "v1",
		})

		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      "test-foo",
			Namespace: appWithWorkflow.Namespace,
		}, stepObj)).Should(BeNil())

		// check workflow status is waiting
		appObj := &oamcore.Application{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      appWithWorkflow.Name,
			Namespace: appWithWorkflow.Namespace,
		}, appObj)).Should(BeNil())

		Expect(appObj.Status.Workflow.Steps[0].Name).Should(Equal("test-wf1"))
		Expect(appObj.Status.Workflow.Steps[0].Type).Should(Equal("foowf"))
		Expect(appObj.Status.Workflow.Steps[0].Phase).Should(Equal(workflowv1alpha1.WorkflowStepPhaseRunning))
		Expect(appObj.Status.Workflow.Steps[0].Reason).Should(Equal("Wait"))

		// update spec to trigger spec
		triggerWorkflowStepToSucceed(stepObj)
		Expect(k8sClient.Update(ctx, stepObj)).Should(BeNil())

		tryReconcile(reconciler, appWithWorkflow.Name, appWithWorkflow.Namespace)
		tryReconcile(reconciler, appWithWorkflow.Name, appWithWorkflow.Namespace)

		// check workflow status is succeeded
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      appWithWorkflow.Name,
			Namespace: appWithWorkflow.Namespace,
		}, appObj)).Should(BeNil())

		Expect(appObj.Status.Workflow.Steps[0].Phase).Should(Equal(workflowv1alpha1.WorkflowStepPhaseSucceeded))
		Expect(appObj.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))
	})

	It("test workflow suspend", func() {
		suspendApp := appWithWorkflow.DeepCopy()
		suspendApp.Name = "test-app-suspend"
		suspendApp.Spec.Workflow.Steps = []workflowv1alpha1.WorkflowStep{{
			WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
				Name:       "suspend",
				Type:       "suspend",
				Properties: &runtime.RawExtension{Raw: []byte(`{}`)},
			},
		}}
		Expect(k8sClient.Create(ctx, suspendApp)).Should(BeNil())

		// first try to add finalizer
		tryReconcile(reconciler, suspendApp.Name, suspendApp.Namespace)
		tryReconcile(reconciler, suspendApp.Name, suspendApp.Namespace)

		appObj := &oamcore.Application{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      suspendApp.Name,
			Namespace: suspendApp.Namespace,
		}, appObj)).Should(BeNil())

		Expect(appObj.Status.Workflow.Suspend).Should(BeTrue())
		Expect(appObj.Status.Phase).Should(BeEquivalentTo(common.ApplicationWorkflowSuspending))
		Expect(appObj.Status.Workflow.Steps[0].Phase).Should(BeEquivalentTo(workflowv1alpha1.WorkflowStepPhaseSuspending))
		Expect(appObj.Status.Workflow.Steps[0].ID).ShouldNot(BeEquivalentTo(""))
		// resume
		appObj.Status.Workflow.Suspend = false
		appObj.Status.Workflow.Steps[0].Phase = workflowv1alpha1.WorkflowStepPhaseRunning
		Expect(k8sClient.Status().Patch(ctx, appObj, client.Merge)).Should(BeNil())
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      suspendApp.Name,
			Namespace: suspendApp.Namespace,
		}, appObj)).Should(BeNil())
		Expect(appObj.Status.Workflow.Suspend).Should(BeFalse())

		tryReconcile(reconciler, suspendApp.Name, suspendApp.Namespace)

		appObj = &oamcore.Application{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      suspendApp.Name,
			Namespace: suspendApp.Namespace,
		}, appObj)).Should(BeNil())

		Expect(appObj.Status.Workflow.Suspend).Should(BeFalse())
		Expect(appObj.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))
	})

	It("test workflow terminate a suspend workflow", func() {
		suspendApp := appWithWorkflow.DeepCopy()
		suspendApp.Name = "test-terminate-suspend-app"
		suspendApp.Spec.Workflow.Steps = []workflowv1alpha1.WorkflowStep{
			{
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "suspend",
					Type:       "suspend",
					Properties: &runtime.RawExtension{Raw: []byte(`{}`)},
				},
			},
			{
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "suspend-1",
					Type:       "suspend",
					Properties: &runtime.RawExtension{Raw: []byte(`{}`)},
				},
			}}
		Expect(k8sClient.Create(ctx, suspendApp)).Should(BeNil())

		// first try to add finalizer
		tryReconcile(reconciler, suspendApp.Name, suspendApp.Namespace)
		tryReconcile(reconciler, suspendApp.Name, suspendApp.Namespace)
		tryReconcile(reconciler, suspendApp.Name, suspendApp.Namespace)

		appObj := &oamcore.Application{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      suspendApp.Name,
			Namespace: suspendApp.Namespace,
		}, appObj)).Should(BeNil())

		Expect(appObj.Status.Workflow.Suspend).Should(BeTrue())
		Expect(appObj.Status.Phase).Should(BeEquivalentTo(common.ApplicationWorkflowSuspending))

		// terminate
		appObj.Status.Workflow.Terminated = true
		appObj.Status.Workflow.Suspend = false
		appObj.Status.Workflow.Steps[0].Phase = workflowv1alpha1.WorkflowStepPhaseFailed
		appObj.Status.Workflow.Steps[0].Reason = wfTypes.StatusReasonTerminate
		Expect(k8sClient.Status().Patch(ctx, appObj, client.Merge)).Should(BeNil())

		tryReconcile(reconciler, suspendApp.Name, suspendApp.Namespace)
		tryReconcile(reconciler, suspendApp.Name, suspendApp.Namespace)

		appObj = &oamcore.Application{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      suspendApp.Name,
			Namespace: suspendApp.Namespace,
		}, appObj)).Should(BeNil())

		Expect(appObj.Status.Workflow.Suspend).Should(BeFalse())
		Expect(appObj.Status.Workflow.Terminated).Should(BeTrue())
		Expect(appObj.Status.Phase).Should(BeEquivalentTo(common.ApplicationWorkflowTerminated))
	})

	It("test application with input/output and workflow", func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-inout-workflow",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())

		healthComponentDef := &oamcore.ComponentDefinition{}
		hCDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(hCDefJson, healthComponentDef)).Should(BeNil())
		healthComponentDef.Name = "worker-with-health"
		healthComponentDef.Namespace = ns.Name
		Expect(k8sClient.Create(ctx, healthComponentDef)).Should(BeNil())
		appwithInputOutput := &oamcore.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-inout-workflow",
				Namespace: "app-with-inout-workflow",
			},
			Spec: oamcore.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
						Inputs: workflowv1alpha1.StepInputs{
							{
								From:         "message",
								ParameterKey: "properties.enemies",
							},
							{
								From:         "message",
								ParameterKey: "properties.lives",
							},
						},
					},
					{
						Name:       "myweb2",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
						Outputs: workflowv1alpha1.StepOutputs{
							{Name: "message", ValueFrom: "output.status.conditions[0].message+\",\"+outputs.gameconfig.data.lives"},
						},
					},
				},
				Workflow: &oamcore.Workflow{
					Steps: []workflowv1alpha1.WorkflowStep{{
						WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
							Name:       "test-web2",
							Type:       "apply-component",
							Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb2"}`)},
						},
					}, {
						WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
							Name:       "test-web1",
							Type:       "apply-component",
							Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb1"}`)},
						},
					}},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), appwithInputOutput)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: appwithInputOutput.Name}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

		expDeployment := &appsv1.Deployment{}
		web1Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb1"}
		web2Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb2"}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(util.NotFoundMatcher{})

		checkApp := &oamcore.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(BeNil())

		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		expDeployment.Status.Conditions = []appsv1.DeploymentCondition{{
			Message: "hello",
		}}
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		expDeployment = &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(BeNil())
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		checkApp = &oamcore.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Workflow.Mode).Should(BeEquivalentTo(fmt.Sprintf("%s-%s", workflowv1alpha1.WorkflowModeStep, workflowv1alpha1.WorkflowModeDAG)))
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))

		checkCM := &corev1.ConfigMap{}
		cmKey := types.NamespacedName{
			Name:      "myweb1game-config",
			Namespace: ns.Name,
		}
		Expect(k8sClient.Get(ctx, cmKey, checkCM)).Should(BeNil())
		Expect(checkCM.Data["enemies"]).Should(BeEquivalentTo("hello,i am lives"))
		Expect(checkCM.Data["lives"]).Should(BeEquivalentTo("hello,i am lives"))
	})

	It("test application with depends on and workflow", func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-dependson-workflow",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())

		healthComponentDef := &oamcore.ComponentDefinition{}
		hCDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(hCDefJson, healthComponentDef)).Should(BeNil())
		healthComponentDef.Name = "worker-with-health"
		healthComponentDef.Namespace = ns.Name
		Expect(k8sClient.Create(ctx, healthComponentDef)).Should(BeNil())
		appwithDependsOn := &oamcore.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-dependson-workflow",
				Namespace: ns.Name,
			},
			Spec: oamcore.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
						DependsOn:  []string{"myweb2"},
					},
					{
						Name:       "myweb2",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
				},
				Workflow: &oamcore.Workflow{
					Steps: []workflowv1alpha1.WorkflowStep{{
						WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
							Name:       "test-web2",
							Type:       "apply-component",
							Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb2"}`)},
						},
					}, {
						WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
							Name:       "test-web1",
							Type:       "apply-component",
							Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb1"}`)},
						},
					}},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), appwithDependsOn)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: appwithDependsOn.Name}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

		expDeployment := &appsv1.Deployment{}
		web1Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb1"}
		web2Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb2"}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(util.NotFoundMatcher{})

		checkApp := &oamcore.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(BeNil())

		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		expDeployment.Status.Conditions = []appsv1.DeploymentCondition{{
			Message: "hello",
		}}
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		expDeployment = &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(BeNil())
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		checkApp = &oamcore.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Workflow.Mode).Should(BeEquivalentTo(fmt.Sprintf("%s-%s", workflowv1alpha1.WorkflowModeStep, workflowv1alpha1.WorkflowModeDAG)))
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))
	})

	It("test application with depends on and input output and workflow", func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-inout-dependson-workflow",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())

		healthComponentDef := &oamcore.ComponentDefinition{}
		hCDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(hCDefJson, healthComponentDef)).Should(BeNil())
		healthComponentDef.Name = "worker-with-health"
		healthComponentDef.Namespace = ns.Name
		Expect(k8sClient.Create(ctx, healthComponentDef)).Should(BeNil())
		appwithInOutDependsOn := &oamcore.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-inout-dependson-workflow",
				Namespace: ns.Name,
			},
			Spec: oamcore.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
						DependsOn:  []string{"myweb2"},
						Inputs: workflowv1alpha1.StepInputs{
							{
								From:         "message",
								ParameterKey: "properties.enemies",
							},
							{
								From:         "message",
								ParameterKey: "properties.lives",
							},
						},
					},
					{
						Name:       "myweb2",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
						Outputs: workflowv1alpha1.StepOutputs{
							{Name: "message", ValueFrom: "output.status.conditions[0].message+\",\"+outputs.gameconfig.data.lives"},
						},
					},
				},
				Workflow: &oamcore.Workflow{
					Steps: []workflowv1alpha1.WorkflowStep{{
						WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
							Name:       "test-web2",
							Type:       "apply-component",
							Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb2"}`)},
						},
					}, {
						WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
							Name:       "test-web1",
							Type:       "apply-component",
							Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb1"}`)},
						},
					}},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), appwithInOutDependsOn)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: appwithInOutDependsOn.Name}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

		expDeployment := &appsv1.Deployment{}
		web1Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb1"}
		web2Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb2"}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(util.NotFoundMatcher{})

		checkApp := &oamcore.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(BeNil())

		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		expDeployment.Status.Conditions = []appsv1.DeploymentCondition{{
			Message: "hello",
		}}
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		expDeployment = &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(BeNil())
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		checkApp = &oamcore.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Workflow.Mode).Should(BeEquivalentTo(fmt.Sprintf("%s-%s", workflowv1alpha1.WorkflowModeStep, workflowv1alpha1.WorkflowModeDAG)))
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))
	})

	It("add workflow to an existing app ", func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "existing-app-add-workflow",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())

		webComponentDef := &oamcore.ComponentDefinition{}
		webDefJson, _ := yaml.YAMLToJSON([]byte(webComponentDefYaml))
		Expect(json.Unmarshal(webDefJson, webComponentDef)).Should(BeNil())
		webComponentDef.Namespace = ns.Name
		Expect(k8sClient.Create(ctx, webComponentDef)).Should(BeNil())
		app := &oamcore.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "existing-app-add-workflow",
				Namespace: ns.Name,
			},
			Spec: oamcore.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "webserver",
						Properties: &runtime.RawExtension{Raw: []byte(`{"image":"busybox"}`)},
					},
					{
						Name:       "myweb2",
						Type:       "webserver",
						Properties: &runtime.RawExtension{Raw: []byte(`{"image":"busybox"}`)},
					},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})
		updateApp := &oamcore.Application{}
		Expect(k8sClient.Get(ctx, appKey, updateApp)).Should(BeNil())
		Expect(updateApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))
		updateApp.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(`{}`)}
		updateApp.Spec.Workflow = &oamcore.Workflow{
			Steps: []workflowv1alpha1.WorkflowStep{{
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "test-web2",
					Type:       "apply-component",
					Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb2"}`)},
					Outputs: workflowv1alpha1.StepOutputs{
						{Name: "image", ValueFrom: "output.spec.template.spec.containers[0].image"},
					},
				},
			}, {
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "test-web1",
					Type:       "apply-component",
					Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb1"}`)},
					Inputs: workflowv1alpha1.StepInputs{
						{
							From:         "image",
							ParameterKey: "image",
						},
					},
				},
			}},
		}
		Expect(k8sClient.Update(context.Background(), updateApp)).Should(BeNil())
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})
		checkApp := &oamcore.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))
	})
})

func triggerWorkflowStepToSucceed(obj *unstructured.Unstructured) {
	unstructured.SetNestedField(obj.Object, "ready", "spec", "key")
}

func tryReconcile(r *Reconciler, name, ns string) {
	appKey := client.ObjectKey{
		Name:      name,
		Namespace: ns,
	}

	Eventually(func() error {
		_, err := r.Reconcile(context.TODO(), reconcile.Request{NamespacedName: appKey})
		if err != nil {
			By(fmt.Sprintf("reconcile err: %+v ", err))
		}
		return err
	}, 10*time.Second, time.Second).Should(BeNil())
}

func setupNamespace(ctx context.Context, namespace string) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}
	Expect(k8sClient.Create(ctx, ns)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
}

func setupFooCRD(ctx context.Context) {
	trueVar := true
	foocrd := &crdv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo.example.com",
		},
		Spec: crdv1.CustomResourceDefinitionSpec{
			Group: "example.com",
			Names: crdv1.CustomResourceDefinitionNames{
				Kind:     "Foo",
				ListKind: "FooList",
				Plural:   "foo",
				Singular: "foo",
			},
			Versions: []crdv1.CustomResourceDefinitionVersion{{
				Name:    "v1",
				Served:  true,
				Storage: true,
				Schema: &crdv1.CustomResourceValidation{
					OpenAPIV3Schema: &crdv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]crdv1.JSONSchemaProps{
							"spec": {
								Type: "object",
								Properties: map[string]crdv1.JSONSchemaProps{
									"key": {Type: "string"},
								},
							},
						},
						XPreserveUnknownFields: &trueVar,
					},
				},
			},
			},
			Scope: crdv1.NamespaceScoped,
		},
	}
	Expect(k8sClient.Create(ctx, foocrd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
}

func setupTestDefinitions(ctx context.Context, defs []string, ns string) {
	for _, def := range defs {
		defJson, err := yaml.YAMLToJSON([]byte(def))
		Expect(err).Should(BeNil())
		u := &unstructured.Unstructured{}
		Expect(json.Unmarshal(defJson, u)).Should(BeNil())
		u.SetNamespace("vela-system")
		Expect(k8sClient.Create(ctx, u)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	}
}

const (
	policyDefYaml = `apiVersion: core.oam.dev/v1beta1
kind: PolicyDefinition
metadata:
  name: foopolicy
spec:
  schematic:
    cue:
      template: |
        output: {
          apiVersion: "example.com/v1"
          kind:       "Foo"
          spec: {
            key: parameter.key
          }
        }
        parameter: {
          key: string
        }
`
	wfStepDefYaml = `apiVersion: core.oam.dev/v1beta1
kind: WorkflowStepDefinition
metadata:
  name: foowf
spec:
  schematic:
    cue:
      template: |
        import ("vela/op")
        
        parameter: {
          namespace: string
        }
        
        // apply workload to kubernetes cluster
        apply: op.#Apply & {
          value: {
            apiVersion: "example.com/v1"
            kind: "Foo"
            metadata: {
              name: "test-foo"
              namespace: parameter.namespace
            }
          }
        }
        // wait until workload.status equal "Running"
        wait: op.#ConditionalWait & {
          continue: apply.value.spec.key != ""
        }
`
)
