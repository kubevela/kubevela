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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
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
				Properties: runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
			}},
			Workflow: &oamcore.Workflow{
				Steps: []oamcore.WorkflowStep{{
					Name:       "test-wf1",
					Type:       "foowf",
					Properties: runtime.RawExtension{Raw: []byte(`{"namespace":"test-ns"}`)},
				}},
			},
		},
	}
	appWithWorkflowAndPolicy := appWithWorkflow.DeepCopy()
	appWithWorkflowAndPolicy.Name = "test-wf-policy"
	appWithWorkflowAndPolicy.Spec.Policies = []oamcore.AppPolicy{{
		Name:       "test-policy",
		Type:       "foopolicy",
		Properties: runtime.RawExtension{Raw: []byte(`{"key":"test"}`)},
	}}

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

	It("should create ConfigMap with final resources for app with workflow", func() {
		Expect(k8sClient.Create(ctx, appWithWorkflowAndPolicy)).Should(BeNil())

		// first try to add finalizer
		tryReconcile(reconciler, appWithWorkflowAndPolicy.Name, appWithWorkflowAndPolicy.Namespace)
		tryReconcile(reconciler, appWithWorkflowAndPolicy.Name, appWithWorkflowAndPolicy.Namespace)

		appRev := &oamcore.ApplicationRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      appWithWorkflowAndPolicy.Name + "-v1",
			Namespace: namespace,
		}, appRev)).Should(BeNil())

		Expect(appRev.Spec.ResourcesConfigMap.Name).ShouldNot(BeEmpty())

		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      appRev.Name,
			Namespace: namespace,
		}, cm)).Should(BeNil())

		Expect(cm.Data[ConfigMapKeyComponents]).Should(Equal(testConfigMapComponentValue))
	})

	It("should create workload in policy before workflow start", func() {
		appWithPolicy := appWithWorkflow.DeepCopy()
		appWithPolicy.SetName("test-app-with-policy")
		appWithPolicy.Spec.Policies = []oamcore.AppPolicy{{
			Name:       "test-foo-policy",
			Type:       "foopolicy",
			Properties: runtime.RawExtension{Raw: []byte(`{"key":"test"}`)},
		}}

		Expect(k8sClient.Create(ctx, appWithPolicy)).Should(BeNil())

		// first try to add finalizer
		tryReconcile(reconciler, appWithPolicy.Name, appWithPolicy.Namespace)
		tryReconcile(reconciler, appWithPolicy.Name, appWithPolicy.Namespace)

		policyObj := &unstructured.Unstructured{}
		policyObj.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "example.com",
			Kind:    "Foo",
			Version: "v1",
		})

		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      "test-foo-policy",
			Namespace: appWithPolicy.Namespace,
		}, policyObj)).Should(BeNil())
	})

	It("should execute workflow step to apply and wait", func() {
		Expect(k8sClient.Create(ctx, appWithWorkflow.DeepCopy())).Should(BeNil())

		// first try to add finalizer
		tryReconcile(reconciler, appWithWorkflow.Name, appWithWorkflow.Namespace)
		tryReconcile(reconciler, appWithWorkflow.Name, appWithWorkflow.Namespace)

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
		Expect(appObj.Status.Workflow.Steps[0].Phase).Should(Equal(common.WorkflowStepPhaseRunning))
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

		Expect(appObj.Status.Workflow.Steps[0].Phase).Should(Equal(common.WorkflowStepPhaseSucceeded))
		Expect(appObj.Status.Workflow.Terminated).Should(BeTrue())
	})

	It("test workflow suspend", func() {
		suspendApp := appWithWorkflow.DeepCopy()
		suspendApp.Name = "test-app-suspend"
		suspendApp.Spec.Workflow.Steps = []oamcore.WorkflowStep{{
			Name:       "suspend",
			Type:       "suspend",
			Properties: runtime.RawExtension{Raw: []byte(`{}`)},
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

		// resume
		appObj.Status.Workflow.Suspend = false
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
		Expect(appObj.Status.Workflow.StepIndex).Should(BeEquivalentTo(1))
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
		u.SetNamespace(ns)
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

	testConfigMapComponentValue = `{"test-component":"{\"Scopes\":[],\"StandardWorkload\":\"{\\\"apiVersion\\\":\\\"apps/v1\\\",\\\"kind\\\":\\\"Deployment\\\",\\\"metadata\\\":{\\\"annotations\\\":{},\\\"labels\\\":{\\\"app.oam.dev/appRevision\\\":\\\"test-wf-policy-v1\\\",\\\"app.oam.dev/component\\\":\\\"test-component\\\",\\\"app.oam.dev/name\\\":\\\"test-wf-policy\\\",\\\"workload.oam.dev/type\\\":\\\"worker\\\"}},\\\"spec\\\":{\\\"selector\\\":{\\\"matchLabels\\\":{\\\"app.oam.dev/component\\\":\\\"test-component\\\"}},\\\"template\\\":{\\\"metadata\\\":{\\\"labels\\\":{\\\"app.oam.dev/component\\\":\\\"test-component\\\"}},\\\"spec\\\":{\\\"containers\\\":[{\\\"command\\\":[\\\"sleep\\\",\\\"1000\\\"],\\\"image\\\":\\\"busybox\\\",\\\"name\\\":\\\"test-component\\\"}]}}}}\",\"Traits\":[]}"}`
)
