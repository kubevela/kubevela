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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghodss/yaml"
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

	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/workflow"
)

var _ = Describe("Test Workflow", func() {
	ctx := context.Background()
	namespace := "test-workflow"

	appWithWorkflow := &oamcore.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: namespace,
		},
		Spec: oamcore.ApplicationSpec{
			Components: []oamcore.ApplicationComponent{{
				Name:       "test-component",
				Type:       "worker",
				Properties: runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
			}},
			Workflow: &oamcore.Workflow{
				Steps: []oamcore.WorkflowStep{{
					Name:       "test-wf1",
					Type:       "foowf",
					Properties: runtime.RawExtension{Raw: []byte(`{"key":"test"}`)},
				}, {
					Name:       "test-wf2",
					Type:       "foowf",
					Properties: runtime.RawExtension{Raw: []byte(`{"key":"test"}`)},
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

		Expect(cm.Data["resources"]).Should(Equal(compressJSON(appWithWorkflowAndPolicyResources)))
	})

	It("should execute workflow steps one by one", func() {
		Expect(k8sClient.Create(ctx, appWithWorkflow)).Should(BeNil())

		// first try to add finalizer
		tryReconcile(reconciler, appWithWorkflowAndPolicy.Name, appWithWorkflowAndPolicy.Namespace)
		tryReconcile(reconciler, appWithWorkflow.Name, appWithWorkflow.Namespace)

		// check step 1 created, step 2 not
		step1obj := &unstructured.Unstructured{}
		step1obj.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "example.com",
			Kind:    "Foo",
			Version: "v1",
		})

		step2obj := &unstructured.Unstructured{}
		step2obj.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "example.com",
			Kind:    "Foo",
			Version: "v1",
		})

		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      "test-wf1",
			Namespace: appWithWorkflow.Namespace,
		}, step1obj)).Should(BeNil())

		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      "test-wf2",
			Namespace: appWithWorkflow.Namespace,
		}, step2obj)).Should(&util.NotFoundMatcher{})

		// mark step 1 succeeded, reconcile
		markWorkflowSucceeded(step1obj)
		Expect(k8sClient.Update(ctx, step1obj)).Should(BeNil())

		tryReconcile(reconciler, appWithWorkflow.Name, appWithWorkflow.Namespace)
		// check step 2 created
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      "test-wf2",
			Namespace: appWithWorkflow.Namespace,
		}, step2obj)).Should(BeNil())
	})
})

func markWorkflowSucceeded(obj *unstructured.Unstructured) {
	succeededMessage, _ := json.Marshal(&workflow.SucceededMessage{ObservedGeneration: 2})

	m := map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{
				"type":    workflow.CondTypeWorkflowFinish,
				"reason":  workflow.CondReasonSucceeded,
				"message": string(succeededMessage),
				"status":  workflow.CondStatusTrue,
			},
		},
	}

	unstructured.SetNestedMap(obj.Object, m, "status")
}

func compressJSON(d string) string {
	m := &json.RawMessage{}
	r := bytes.NewBuffer([]byte(d))
	dec := json.NewDecoder(r)
	w := &bytes.Buffer{}

	for {
		err := dec.Decode(m)
		if err != nil {
			break
		}
		b, _ := json.Marshal(m)
		w.Write(b)
	}
	return w.String()
}

func tryReconcile(r *Reconciler, name, ns string) {
	appKey := client.ObjectKey{
		Name:      name,
		Namespace: ns,
	}

	Eventually(func() error {
		_, err := r.Reconcile(reconcile.Request{NamespacedName: appKey})
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

	appWithWorkflowAndPolicyResources = `{
  "apiVersion": "apps/v1",
  "kind": "Deployment",
  "metadata": {
    "annotations": {},
    "labels": {
      "app.oam.dev/appRevision": "test-wf-policy-v1",
      "app.oam.dev/component": "test-component",
      "app.oam.dev/name": "test-wf-policy",
      "workload.oam.dev/type": "worker"
    },
    "name": "test-component",
    "namespace": "test-workflow"
  },
  "spec": {
    "selector": {
      "matchLabels": {
        "app.oam.dev/component": "test-component"
      }
    },
    "template": {
      "metadata": {
        "labels": {
          "app.oam.dev/component": "test-component"
        }
      },
      "spec": {
        "containers": [
          {
            "command": [
              "sleep",
              "1000"
            ],
            "image": "busybox",
            "name": "test-component"
          }
        ]
      }
    }
  }
}
{
  "apiVersion": "example.com/v1",
  "kind": "Foo",
  "metadata": {
    "labels": {
      "app.oam.dev/appRevision": "test-wf-policy-v1",
      "app.oam.dev/component": "test-policy",
      "app.oam.dev/name": "test-wf-policy",
      "workload.oam.dev/type": "foopolicy"
    }
  },
  "spec": {
    "key": "test"
  }
}`
)
