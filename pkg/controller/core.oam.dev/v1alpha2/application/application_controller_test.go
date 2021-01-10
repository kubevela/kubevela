/*
Copyright 2020 The KubeVela Authors.

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
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test Application Controller", func() {
	ctx := context.Background()

	appwithNoTrait := &v1alpha2.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "core.oam.dev/v1alpha2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "app-with-no-trait",
		},
		Spec: v1alpha2.ApplicationSpec{
			Components: []v1alpha2.ApplicationComponent{
				{
					Name:         "myweb",
					WorkloadType: "worker",
					Settings:     runtime.RawExtension{Raw: []byte("{\"cmd\":[\"sleep\",\"1000\"],\"image\":\"busybox\"}")},
				},
			},
		},
	}
	compName := "myweb"
	expDeployment := v1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"workload.oam.dev/type": "worker",
			},
		},
		Spec: v1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
				"app.oam.dev/component": compName,
			}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
					"app.oam.dev/component": compName,
				}},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Image:   "busybox",
					Name:    compName,
					Command: []string{"sleep", "1000"},
				},
				}}},
		},
	}

	appWithTrait := appwithNoTrait.DeepCopy()
	appWithTrait.SetName("app-with-trait")
	appWithTrait.Spec.Components[0].Traits = []v1alpha2.ApplicationTrait{
		{
			Name:       "scaler",
			Properties: runtime.RawExtension{Raw: []byte(`{"replicas":2}`)},
		},
	}
	expectScalerTrait := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "core.oam.dev/v1alpha2",
		"kind":       "ManualScalerTrait",
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{
				"trait.oam.dev/type": "scaler",
			},
		},
		"spec": map[string]interface{}{
			"replicaCount": int64(2),
		},
	}}
	appWithTraitAndScope := appWithTrait.DeepCopy()
	appWithTraitAndScope.SetName("app-with-trait-and-scope")
	appWithTraitAndScope.Spec.Components[0].Scopes = &runtime.RawExtension{Raw: []byte(`{"scopes.core.oam.dev":"appWithTraitAndScope-default-health"}`)}

	wd := &v1alpha2.WorkloadDefinition{}
	wDDefJson, _ := yaml.YAMLToJSON([]byte(wDDefYaml))

	td := &v1alpha2.TraitDefinition{}
	tDDefJson, _ := yaml.YAMLToJSON([]byte(tDDefYaml))

	BeforeEach(func() {

		Expect(json.Unmarshal(wDDefJson, wd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, wd.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		Expect(json.Unmarshal(tDDefJson, td)).Should(BeNil())
		Expect(k8sClient.Create(ctx, td.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	})
	AfterEach(func() {
	})

	It("app-without-trait will only create workload", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test",
			},
		}
		appwithNoTrait.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		Expect(k8sClient.Create(ctx, appwithNoTrait.DeepCopyObject())).Should(BeNil())
		reconciler := &Reconciler{
			Client: k8sClient,
			Log:    ctrl.Log.WithName("Application"),
			Scheme: testScheme,
		}
		appKey := client.ObjectKey{
			Name:      appwithNoTrait.Name,
			Namespace: appwithNoTrait.Namespace,
		}
		reconcileRetry(reconciler, reconcile.Request{NamespacedName: appKey})
		By("Check Application Created")
		checkApp := &v1alpha2.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(Equal(v1alpha2.ApplicationRunning))

		By("Check ApplicationConfiguration Created")
		appConfig := &v1alpha2.ApplicationConfiguration{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: appwithNoTrait.Namespace,
			Name:      appwithNoTrait.Name,
		}, appConfig)).Should(BeNil())

		By("Check Component Created with the expected workload spec")
		component := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: appwithNoTrait.Namespace,
			Name:      "myweb",
		}, component)).Should(BeNil())
		Expect(component.ObjectMeta.Labels).Should(BeEquivalentTo(map[string]string{"application.oam.dev": "app-with-no-trait"}))
		Expect(component.ObjectMeta.OwnerReferences[0].Name).Should(BeEquivalentTo("app-with-no-trait"))
		Expect(component.ObjectMeta.OwnerReferences[0].Kind).Should(BeEquivalentTo("Application"))
		Expect(component.ObjectMeta.OwnerReferences[0].APIVersion).Should(BeEquivalentTo("core.oam.dev/v1alpha2"))
		Expect(component.ObjectMeta.OwnerReferences[0].Controller).Should(BeEquivalentTo(pointer.BoolPtr(true)))
		gotD := v1.Deployment{}
		Expect(json.Unmarshal(component.Spec.Workload.Raw, &gotD)).Should(BeNil())
		Expect(gotD).Should(BeEquivalentTo(expDeployment))

		By("Delete Application, clean the resource")
		Expect(k8sClient.Delete(ctx, appwithNoTrait)).Should(BeNil())
	})

	It("app-with-trait will create workload and trait", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-with-trait",
			},
		}
		appWithTrait.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		app := appWithTrait.DeepCopy()
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		reconciler := &Reconciler{
			Client: k8sClient,
			Log:    ctrl.Log.WithName("Application"),
			Scheme: testScheme,
		}
		appKey := client.ObjectKey{
			Name:      app.Name,
			Namespace: app.Namespace,
		}
		reconcileRetry(reconciler, reconcile.Request{NamespacedName: appKey})

		By("Check App running successfully")
		checkApp := &v1alpha2.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(Equal(v1alpha2.ApplicationRunning))

		By("Check AppConfig and trait created as expected")
		appConfig := &v1alpha2.ApplicationConfiguration{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      app.Name,
		}, appConfig)).Should(BeNil())

		gotTrait := unstructured.Unstructured{}
		Expect(json.Unmarshal(appConfig.Spec.Components[0].Traits[0].Trait.Raw, &gotTrait)).Should(BeNil())
		Expect(gotTrait).Should(BeEquivalentTo(expectScalerTrait))

		By("Check component created as expected")
		component := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      "myweb",
		}, component)).Should(BeNil())
		Expect(component.ObjectMeta.Labels).Should(BeEquivalentTo(map[string]string{"application.oam.dev": "app-with-trait"}))
		Expect(component.ObjectMeta.OwnerReferences[0].Name).Should(BeEquivalentTo("app-with-trait"))
		Expect(component.ObjectMeta.OwnerReferences[0].Kind).Should(BeEquivalentTo("Application"))
		Expect(component.ObjectMeta.OwnerReferences[0].APIVersion).Should(BeEquivalentTo("core.oam.dev/v1alpha2"))
		Expect(component.ObjectMeta.OwnerReferences[0].Controller).Should(BeEquivalentTo(pointer.BoolPtr(true)))
		gotD := v1.Deployment{}
		Expect(json.Unmarshal(component.Spec.Workload.Raw, &gotD)).Should(BeNil())
		Expect(gotD).Should(BeEquivalentTo(expDeployment))

		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

})

func reconcileRetry(r reconcile.Reconciler, req reconcile.Request) {
	Eventually(func() error {
		_, err := r.Reconcile(req)
		return err
	}, 3*time.Second, time.Second).Should(BeNil())
}

const (
	wDDefYaml = `
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: worker
  annotations:
    definition.oam.dev/description: "Long-running scalable backend worker without network endpoint"
spec:
  definitionRef:
    name: deployments.apps
  extension:
    template: |
      output: {
      	apiVersion: "apps/v1"
      	kind:       "Deployment"
      	spec: {
      		selector: matchLabels: {
      			"app.oam.dev/component": context.name
      		}

      		template: {
      			metadata: labels: {
      				"app.oam.dev/component": context.name
      			}

      			spec: {
      				containers: [{
      					name:  context.name
      					image: parameter.image

      					if parameter["cmd"] != _|_ {
      						command: parameter.cmd
      					}
      				}]
      			}
      		}

      		selector:
      			matchLabels:
      				"app.oam.dev/component": context.name
      	}
      }

      parameter: {
      	// +usage=Which image would you like to use for your service
      	// +short=i
      	image: string

      	cmd?: [...string]
      }`
	tDDefYaml = `
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "Manually scale the app"
  name: scaler
spec:
  appliesToWorkloads:
    - webservice
    - worker
  definitionRef:
    name: manualscalertraits.core.oam.dev
  workloadRefPath: spec.workloadRef
  extension:
    template: |-
      output: {
      	apiVersion: "core.oam.dev/v1alpha2"
      	kind:       "ManualScalerTrait"
      	spec: {
      		replicaCount: parameter.replicas
      	}
      }
      parameter: {
      	//+short=r
      	replicas: *1 | int
      }

`
)
