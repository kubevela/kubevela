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
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
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
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test Application Controller", func() {
	ctx := context.Background()
	appwithConfig := &v1alpha2.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "core.oam.dev/v1alpha2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-with-config",
			Namespace: "app-with-config",
		},
		Spec: v1alpha2.ApplicationSpec{
			Components: []v1alpha2.ApplicationComponent{
				{
					Name:         "myweb1",
					WorkloadType: "worker",
					Settings:     runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","config":"myconfig"}`)},
				},
			},
		},
	}
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
					Name:         "myweb2",
					WorkloadType: "worker",
					Settings:     runtime.RawExtension{Raw: []byte("{\"cmd\":[\"sleep\",\"1000\"],\"image\":\"busybox\"}")},
				},
			},
		},
	}

	var getExpDeployment = func(compName, appName string) *v1.Deployment {
		return &v1.Deployment{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Deployment",
				APIVersion: "apps/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"workload.oam.dev/type": "worker",
					"app.oam.dev/component": compName,
					"app.oam.dev/name":      appName,
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
	}

	appWithTrait := appwithNoTrait.DeepCopy()
	appWithTrait.SetName("app-with-trait")
	appWithTrait.Spec.Components[0].Traits = []v1alpha2.ApplicationTrait{
		{
			Name:       "scaler",
			Properties: runtime.RawExtension{Raw: []byte(`{"replicas":2}`)},
		},
	}
	appWithTrait.Spec.Components[0].Name = "myweb3"
	expectScalerTrait := func(compName, appName string) unstructured.Unstructured {
		return unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "core.oam.dev/v1alpha2",
			"kind":       "ManualScalerTrait",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"trait.oam.dev/type":    "scaler",
					"app.oam.dev/component": compName,
					"app.oam.dev/name":      appName,
				},
			},
			"spec": map[string]interface{}{
				"replicaCount": int64(2),
			},
		}}
	}

	appWithTraitAndScope := appWithTrait.DeepCopy()
	appWithTraitAndScope.SetName("app-with-trait-and-scope")
	appWithTraitAndScope.Spec.Components[0].Scopes = map[string]string{"healthscopes.core.oam.dev": "appWithTraitAndScope-default-health"}
	appWithTraitAndScope.Spec.Components[0].Name = "myweb4"

	appWithTwoComp := appWithTraitAndScope.DeepCopy()
	appWithTwoComp.SetName("app-with-two-comp")
	appWithTwoComp.Spec.Components[0].Scopes = map[string]string{"healthscopes.core.oam.dev": "app-with-two-comp-default-health"}
	appWithTwoComp.Spec.Components[0].Name = "myweb5"
	appWithTwoComp.Spec.Components = append(appWithTwoComp.Spec.Components, v1alpha2.ApplicationComponent{
		Name:         "myweb6",
		WorkloadType: "worker",
		Settings:     runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox2","config":"myconfig"}`)},
		Scopes:       map[string]string{"healthscopes.core.oam.dev": "app-with-two-comp-default-health"},
	})

	wd := &v1alpha2.WorkloadDefinition{}
	wDDefJson, _ := yaml.YAMLToJSON([]byte(wDDefYaml))

	webserverwd := &v1alpha2.WorkloadDefinition{}
	webserverwdJson, _ := yaml.YAMLToJSON([]byte(webserverYaml))

	td := &v1alpha2.TraitDefinition{}
	tDDefJson, _ := yaml.YAMLToJSON([]byte(tDDefYaml))

	sd := &v1alpha2.ScopeDefinition{}
	sdDefJson, _ := yaml.YAMLToJSON([]byte(sDDefYaml))

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "kubevela-app-with-config-myweb1-myconfig", Namespace: appwithConfig.Namespace},
		Data:       map[string]string{"c1": "v1", "c2": "v2"},
	}

	BeforeEach(func() {

		Expect(json.Unmarshal(wDDefJson, wd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, wd.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		Expect(json.Unmarshal(tDDefJson, td)).Should(BeNil())
		Expect(k8sClient.Create(ctx, td.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		Expect(json.Unmarshal(sdDefJson, sd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, sd.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		Expect(json.Unmarshal(webserverwdJson, webserverwd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, webserverwd.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	})
	AfterEach(func() {
	})

	It("app-without-trait will only create workload", func() {
		expDeployment := getExpDeployment("myweb2", appwithNoTrait.Name)
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test",
			},
		}
		appwithNoTrait.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		Expect(k8sClient.Create(ctx, appwithNoTrait.DeepCopyObject())).Should(BeNil())

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
		var component v1alpha2.Component
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: appwithNoTrait.Namespace,
			Name:      "myweb2",
		}, &component)).Should(BeNil())
		Expect(component.ObjectMeta.Labels).Should(BeEquivalentTo(map[string]string{"application.oam.dev": "app-with-no-trait"}))
		Expect(component.ObjectMeta.OwnerReferences[0].Name).Should(BeEquivalentTo("app-with-no-trait"))
		Expect(component.ObjectMeta.OwnerReferences[0].Kind).Should(BeEquivalentTo("Application"))
		Expect(component.ObjectMeta.OwnerReferences[0].APIVersion).Should(BeEquivalentTo("core.oam.dev/v1alpha2"))
		Expect(component.ObjectMeta.OwnerReferences[0].Controller).Should(BeEquivalentTo(pointer.BoolPtr(true)))
		gotD := &v1.Deployment{}

		Expect(json.Unmarshal(component.Spec.Workload.Raw, gotD)).Should(BeNil())
		fmt.Println(cmp.Diff(expDeployment, gotD))
		Expect(assert.ObjectsAreEqual(expDeployment, gotD)).Should(BeEquivalentTo(true))
		By("Delete Application, clean the resource")
		Expect(k8sClient.Delete(ctx, appwithNoTrait)).Should(BeNil())
	})

	It("app-with-config will create workload with config data", func() {
		expConfigDeployment := getExpDeployment("myweb1", appwithConfig.Name)
		expConfigDeployment.SetAnnotations(map[string]string{"c1": "v1", "c2": "v2"})
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: appwithConfig.Namespace,
			},
		}
		appwithConfig.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		Expect(k8sClient.Create(ctx, cm.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		Expect(k8sClient.Create(ctx, appwithConfig.DeepCopyObject())).Should(BeNil())
		app := appwithConfig
		appKey := client.ObjectKey{
			Name:      app.Name,
			Namespace: app.Namespace,
		}
		reconcileRetry(reconciler, reconcile.Request{NamespacedName: appKey})
		By("Check Application Created")
		checkApp := &v1alpha2.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(Equal(v1alpha2.ApplicationRunning))

		By("Check ApplicationConfiguration Created")
		appConfig := &v1alpha2.ApplicationConfiguration{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      app.Name,
		}, appConfig)).Should(BeNil())

		By("Check Component Created with the expected workload spec")
		component := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      "myweb1",
		}, component)).Should(BeNil())
		Expect(component.ObjectMeta.Labels).Should(BeEquivalentTo(map[string]string{"application.oam.dev": "app-with-config"}))
		Expect(component.ObjectMeta.OwnerReferences[0].Name).Should(BeEquivalentTo("app-with-config"))
		gotD := &v1.Deployment{}
		Expect(json.Unmarshal(component.Spec.Workload.Raw, gotD)).Should(BeNil())

		Expect(gotD).Should(BeEquivalentTo(expConfigDeployment))
		By("Delete Application, clean the resource")
		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("app-with-trait will create workload and trait", func() {
		expDeployment := getExpDeployment("myweb3", appWithTrait.Name)
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-with-trait",
			},
		}
		appWithTrait.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		app := appWithTrait.DeepCopy()
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())

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
		Expect(gotTrait).Should(BeEquivalentTo(expectScalerTrait("myweb3", app.Name)))

		By("Check component created as expected")
		component := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      "myweb3",
		}, component)).Should(BeNil())
		Expect(component.ObjectMeta.Labels).Should(BeEquivalentTo(map[string]string{"application.oam.dev": "app-with-trait"}))
		Expect(component.ObjectMeta.OwnerReferences[0].Name).Should(BeEquivalentTo("app-with-trait"))
		Expect(component.ObjectMeta.OwnerReferences[0].Kind).Should(BeEquivalentTo("Application"))
		Expect(component.ObjectMeta.OwnerReferences[0].APIVersion).Should(BeEquivalentTo("core.oam.dev/v1alpha2"))
		Expect(component.ObjectMeta.OwnerReferences[0].Controller).Should(BeEquivalentTo(pointer.BoolPtr(true)))
		gotD := &v1.Deployment{}
		Expect(json.Unmarshal(component.Spec.Workload.Raw, gotD)).Should(BeNil())
		Expect(gotD).Should(BeEquivalentTo(expDeployment))

		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("app-with-composedworkload-trait will create workload and trait", func() {
		compName := "myweb-composed-3"
		var appname = "app-with-composedworkload-trait"
		expDeployment := getExpDeployment(compName, appname)
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-with-composedworkload-trait",
			},
		}

		appWithComposedWorkload := appwithNoTrait.DeepCopy()
		appWithComposedWorkload.Spec.Components[0].WorkloadType = "webserver"
		appWithComposedWorkload.SetName(appname)
		appWithComposedWorkload.Spec.Components[0].Traits = []v1alpha2.ApplicationTrait{
			{
				Name:       "scaler",
				Properties: runtime.RawExtension{Raw: []byte(`{"replicas":2}`)},
			},
		}
		appWithComposedWorkload.Spec.Components[0].Name = compName
		appWithComposedWorkload.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		app := appWithComposedWorkload.DeepCopy()
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())

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

		Expect(len(appConfig.Spec.Components[0].Traits)).Should(BeEquivalentTo(2))

		gotTrait := unstructured.Unstructured{}
		By("Check the first trait should be service")
		expectServiceTrait := unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"trait.oam.dev/type":     "AuxiliaryWorkload",
					"app.oam.dev/name":       "app-with-composedworkload-trait",
					"app.oam.dev/component":  "myweb-composed-3",
					"trait.oam.dev/resource": "service",
				},
			},
			"spec": map[string]interface{}{
				"ports": []interface{}{
					map[string]interface{}{"port": int64(80), "targetPort": int64(80)},
				},
				"selector": map[string]interface{}{
					"app.oam.dev/component": compName,
				},
			},
		}}
		Expect(json.Unmarshal(appConfig.Spec.Components[0].Traits[0].Trait.Raw, &gotTrait)).Should(BeNil())
		fmt.Println(cmp.Diff(expectServiceTrait, gotTrait))
		Expect(assert.ObjectsAreEqual(expectServiceTrait, gotTrait)).Should(BeTrue())

		By("Check the second trait should be scaler")
		gotTrait = unstructured.Unstructured{}
		Expect(json.Unmarshal(appConfig.Spec.Components[0].Traits[1].Trait.Raw, &gotTrait)).Should(BeNil())
		Expect(gotTrait).Should(BeEquivalentTo(expectScalerTrait("myweb-composed-3", app.Name)))

		By("Check component created as expected")
		component := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      compName,
		}, component)).Should(BeNil())
		Expect(component.ObjectMeta.Labels).Should(BeEquivalentTo(map[string]string{"application.oam.dev": appname}))
		Expect(component.ObjectMeta.OwnerReferences[0].Name).Should(BeEquivalentTo(appname))
		Expect(component.ObjectMeta.OwnerReferences[0].Kind).Should(BeEquivalentTo("Application"))
		Expect(component.ObjectMeta.OwnerReferences[0].APIVersion).Should(BeEquivalentTo("core.oam.dev/v1alpha2"))
		Expect(component.ObjectMeta.OwnerReferences[0].Controller).Should(BeEquivalentTo(pointer.BoolPtr(true)))
		gotD := &v1.Deployment{}
		expDeployment.ObjectMeta.Labels["workload.oam.dev/type"] = "webserver"
		expDeployment.Spec.Template.Spec.Containers[0].Ports = []corev1.ContainerPort{{ContainerPort: 80}}
		Expect(json.Unmarshal(component.Spec.Workload.Raw, gotD)).Should(BeNil())
		fmt.Println(cmp.Diff(expDeployment, gotD))
		Expect(gotD).Should(BeEquivalentTo(expDeployment))

		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("app-with-trait-and-scope will create workload, trait and scope", func() {
		expDeployment := getExpDeployment("myweb4", appWithTraitAndScope.Name)
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-with-trait-scope",
			},
		}
		appWithTraitAndScope.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		app := appWithTraitAndScope.DeepCopy()
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())

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
		Expect(gotTrait).Should(BeEquivalentTo(expectScalerTrait("myweb4", app.Name)))

		Expect(appConfig.Spec.Components[0].Scopes[0].ScopeReference).Should(BeEquivalentTo(v1alpha1.TypedReference{
			APIVersion: "core.oam.dev/v1alpha2",
			Kind:       "HealthScope",
			Name:       "appWithTraitAndScope-default-health",
		}))

		By("Check component created as expected")
		component := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      "myweb4",
		}, component)).Should(BeNil())
		Expect(component.ObjectMeta.Labels).Should(BeEquivalentTo(map[string]string{"application.oam.dev": "app-with-trait-and-scope"}))
		Expect(component.ObjectMeta.OwnerReferences[0].Name).Should(BeEquivalentTo("app-with-trait-and-scope"))
		Expect(component.ObjectMeta.OwnerReferences[0].Kind).Should(BeEquivalentTo("Application"))
		Expect(component.ObjectMeta.OwnerReferences[0].APIVersion).Should(BeEquivalentTo("core.oam.dev/v1alpha2"))
		Expect(component.ObjectMeta.OwnerReferences[0].Controller).Should(BeEquivalentTo(pointer.BoolPtr(true)))
		gotD := &v1.Deployment{}
		Expect(json.Unmarshal(component.Spec.Workload.Raw, gotD)).Should(BeNil())
		Expect(gotD).Should(BeEquivalentTo(expDeployment))

		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("app with two components and update", func() {
		expDeployment := getExpDeployment("myweb5", appWithTwoComp.Name)
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-two-comps",
			},
		}
		appWithTwoComp.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		app := appWithTwoComp.DeepCopy()
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())

		cm.SetNamespace(ns.Name)
		cm.SetName("kubevela-app-with-two-comp-myweb6-myconfig")
		Expect(k8sClient.Create(ctx, cm.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

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
		Expect(gotTrait).Should(BeEquivalentTo(expectScalerTrait("myweb5", app.Name)))

		Expect(appConfig.Spec.Components[0].Scopes[0].ScopeReference).Should(BeEquivalentTo(v1alpha1.TypedReference{
			APIVersion: "core.oam.dev/v1alpha2",
			Kind:       "HealthScope",
			Name:       "app-with-two-comp-default-health",
		}))
		Expect(appConfig.Spec.Components[1].Scopes[0].ScopeReference).Should(BeEquivalentTo(v1alpha1.TypedReference{
			APIVersion: "core.oam.dev/v1alpha2",
			Kind:       "HealthScope",
			Name:       "app-with-two-comp-default-health",
		}))

		By("Check component created as expected")
		component5 := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      "myweb5",
		}, component5)).Should(BeNil())
		Expect(component5.ObjectMeta.Labels).Should(BeEquivalentTo(map[string]string{"application.oam.dev": app.Name}))
		gotD := &v1.Deployment{}
		Expect(json.Unmarshal(component5.Spec.Workload.Raw, gotD)).Should(BeNil())
		Expect(gotD).Should(BeEquivalentTo(expDeployment))

		expDeployment6 := getExpDeployment("myweb6", app.Name)
		expDeployment6.SetAnnotations(map[string]string{"c1": "v1", "c2": "v2"})
		expDeployment6.Spec.Template.Spec.Containers[0].Image = "busybox2"
		component6 := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      "myweb6",
		}, component6)).Should(BeNil())
		Expect(component6.ObjectMeta.Labels).Should(BeEquivalentTo(map[string]string{"application.oam.dev": app.Name}))
		gotD2 := &v1.Deployment{}
		Expect(json.Unmarshal(component6.Spec.Workload.Raw, gotD2)).Should(BeNil())
		fmt.Println(cmp.Diff(expDeployment6, gotD2))
		Expect(gotD2).Should(BeEquivalentTo(expDeployment6))

		By("update component5 with new spec, rename component6 it should create new component ")

		checkApp.SetNamespace(app.Namespace)
		checkApp.Spec.Components[0] = v1alpha2.ApplicationComponent{
			Name:         "myweb5",
			WorkloadType: "worker",
			Settings:     runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox3"}`)},
			Scopes:       map[string]string{"healthscopes.core.oam.dev": "app-with-two-comp-default-health"},
		}
		checkApp.Spec.Components[1] = v1alpha2.ApplicationComponent{
			Name:         "myweb7",
			WorkloadType: "worker",
			Settings:     runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
			Scopes:       map[string]string{"healthscopes.core.oam.dev": "app-with-two-comp-default-health"},
		}
		Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
		reconcileRetry(reconciler, reconcile.Request{NamespacedName: appKey})

		By("Check App updated successfully")
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(Equal(v1alpha2.ApplicationRunning))

		By("check AC and Component updated")
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      app.Name,
		}, appConfig)).Should(BeNil())

		Expect(json.Unmarshal(appConfig.Spec.Components[0].Traits[0].Trait.Raw, &gotTrait)).Should(BeNil())
		Expect(gotTrait).Should(BeEquivalentTo(expectScalerTrait("myweb5", app.Name)))

		Expect(appConfig.Spec.Components[0].Scopes[0].ScopeReference).Should(BeEquivalentTo(v1alpha1.TypedReference{
			APIVersion: "core.oam.dev/v1alpha2",
			Kind:       "HealthScope",
			Name:       "app-with-two-comp-default-health",
		}))
		Expect(appConfig.Spec.Components[1].Scopes[0].ScopeReference).Should(BeEquivalentTo(v1alpha1.TypedReference{
			APIVersion: "core.oam.dev/v1alpha2",
			Kind:       "HealthScope",
			Name:       "app-with-two-comp-default-health",
		}))

		By("Check component created as expected")
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      "myweb5",
		}, component5)).Should(BeNil())
		Expect(json.Unmarshal(component5.Spec.Workload.Raw, gotD)).Should(BeNil())
		expDeployment.Spec.Template.Spec.Containers[0].Image = "busybox3"
		Expect(gotD).Should(BeEquivalentTo(expDeployment))

		expDeployment7 := getExpDeployment("myweb7", app.Name)
		component7 := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      "myweb7",
		}, component7)).Should(BeNil())
		Expect(component7.ObjectMeta.Labels).Should(BeEquivalentTo(map[string]string{"application.oam.dev": app.Name}))
		gotD3 := &v1.Deployment{}
		Expect(json.Unmarshal(component7.Spec.Workload.Raw, gotD3)).Should(BeNil())
		fmt.Println(cmp.Diff(gotD3, expDeployment7))
		Expect(gotD3).Should(BeEquivalentTo(expDeployment7))

		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      "myweb6",
		}, component6)).Should(&util.NotFoundMatcher{})

		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("app-with-trait will create workload and trait with http task", func() {
		s := NewMock()
		defer s.Close()
		expTrait := expectScalerTrait(appWithTrait.Spec.Components[0].Name, appWithTrait.Name)
		expTrait.Object["spec"].(map[string]interface{})["token"] = "test-token"

		By("change trait definition with http task")
		ntd, otd := &v1alpha2.TraitDefinition{}, &v1alpha2.TraitDefinition{}
		tDDefJson, _ := yaml.YAMLToJSON([]byte(tdDefYamlWithHttp))
		Expect(json.Unmarshal(tDDefJson, ntd)).Should(BeNil())
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "scaler"}, otd)).Should(BeNil())
		ntd.ResourceVersion = otd.ResourceVersion
		Expect(k8sClient.Update(ctx, ntd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-with-trait-http",
			},
		}
		appWithTrait.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		app := appWithTrait.DeepCopy()
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())

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
		Expect(gotTrait).Should(BeEquivalentTo(expTrait))

		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("app with health policy for workload", func() {
		By("change workload and trait definition with health policy")
		nwd, owd := &v1alpha2.WorkloadDefinition{}, &v1alpha2.WorkloadDefinition{}
		wDDefJson, _ := yaml.YAMLToJSON([]byte(wDDefWithHealthYaml))
		Expect(json.Unmarshal(wDDefJson, nwd)).Should(BeNil())
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "worker"}, owd)).Should(BeNil())
		nwd.ResourceVersion = owd.ResourceVersion
		Expect(k8sClient.Update(ctx, nwd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		ntd, otd := &v1alpha2.TraitDefinition{}, &v1alpha2.TraitDefinition{}
		tDDefJson, _ := yaml.YAMLToJSON([]byte(tDDefWithHealthYaml))
		Expect(json.Unmarshal(tDDefJson, ntd)).Should(BeNil())
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "scaler"}, otd)).Should(BeNil())
		ntd.ResourceVersion = otd.ResourceVersion
		Expect(k8sClient.Update(ctx, ntd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		compName := "myweb-health"
		expDeployment := getExpDeployment(compName, appWithTrait.Name)

		By("create the new namespace")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-with-health",
			},
		}
		appWithTrait.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())

		app := appWithTrait.DeepCopy()
		app.Spec.Components[0].Name = compName
		expDeployment.Name = app.Name
		expDeployment.Namespace = ns.Name
		expDeployment.Labels[oam.LabelAppName] = app.Name
		expDeployment.Labels[oam.LabelAppComponent] = compName
		expDeployment.Labels["app.oam.dev/resourceType"] = "WORKLOAD"
		Expect(k8sClient.Create(ctx, expDeployment)).Should(BeNil())
		expTrait := expectScalerTrait(compName, app.Name)
		expTrait.SetName(app.Name)
		expTrait.SetNamespace(app.Namespace)
		expTrait.SetLabels(map[string]string{
			oam.LabelAppName:        app.Name,
			"trait.oam.dev/type":    "scaler",
			"app.oam.dev/component": "myweb-health",
		})
		(expTrait.Object["spec"].(map[string]interface{}))["workloadRef"] = map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       app.Name,
		}
		Expect(k8sClient.Create(ctx, &expTrait)).Should(BeNil())

		By("enrich the status of deployment and scaler trait")
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())
		got := &v1.Deployment{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      app.Name,
		}, got)).Should(BeNil())
		expTrait.Object["status"] = v1alpha1.ConditionedStatus{
			Conditions: []v1alpha1.Condition{{
				Status:             corev1.ConditionTrue,
				LastTransitionTime: metav1.Now(),
			}},
		}
		Expect(k8sClient.Status().Update(ctx, &expTrait)).Should(BeNil())
		tGot := &unstructured.Unstructured{}
		tGot.SetAPIVersion("core.oam.dev/v1alpha2")
		tGot.SetKind("ManualScalerTrait")
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      app.Name,
		}, tGot)).Should(BeNil())

		By("apply appfile")
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		appKey := client.ObjectKey{
			Name:      app.Name,
			Namespace: app.Namespace,
		}
		reconcileRetry(reconciler, reconcile.Request{NamespacedName: appKey})

		By("Check App running successfully")

		Eventually(func() string {
			_, err := reconciler.Reconcile(reconcile.Request{NamespacedName: appKey})
			if err != nil {
				return err.Error()
			}
			checkApp := &v1alpha2.Application{}
			err = k8sClient.Get(ctx, appKey, checkApp)
			if err != nil {
				return err.Error()
			}
			if checkApp.Status.Phase != v1alpha2.ApplicationRunning {
				fmt.Println(checkApp.Status.Conditions)
			}
			return string(checkApp.Status.Phase)
		}(), 5*time.Second, time.Second).Should(BeEquivalentTo(v1alpha2.ApplicationRunning))

		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("app with rolling out annotation", func() {
		By("create application with rolling out annotation")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-test-with-rollout",
			},
		}
		appWithTrait.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		app := appWithTrait.DeepCopy()
		app.SetAnnotations(map[string]string{
			oam.AnnotationAppRollout: "true",
		})

		By("apply appfile")
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		appKey := client.ObjectKey{
			Name:      app.Name,
			Namespace: app.Namespace,
		}
		result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: appKey})
		Expect(result).To(BeIdenticalTo(ctrl.Result{RequeueAfter: RolloutReconcileWaitTime}))
		Expect(err).ToNot(HaveOccurred())
		By("Check App status is rollingout")
		checkApp := &v1alpha2.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(Equal(v1alpha2.ApplicationRollingOut))

		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("app with health policy and custom status for workload", func() {
		By("change workload and trait definition with health policy")
		nwd := &v1alpha2.WorkloadDefinition{}
		wDDefJson, _ := yaml.YAMLToJSON([]byte(wdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(wDDefJson, nwd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, nwd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		ntd := &v1alpha2.TraitDefinition{}
		tDDefJson, _ := yaml.YAMLToJSON([]byte(tDDefWithHealthStatusYaml))
		Expect(json.Unmarshal(tDDefJson, ntd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, ntd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		compName := "myweb-health-status"
		appWithTraitHealthStatus := appWithTrait.DeepCopy()
		appWithTraitHealthStatus.Name = "app-trait-health-status"
		expDeployment := getExpDeployment(compName, appWithTraitHealthStatus.Name)

		By("create the new namespace")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-with-health-status",
			},
		}
		appWithTraitHealthStatus.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())

		app := appWithTraitHealthStatus.DeepCopy()
		app.Spec.Components[0].Name = compName
		app.Spec.Components[0].WorkloadType = "nworker"
		app.Spec.Components[0].Settings = runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox3","lives":"3","enemies":"alain"}`)}
		app.Spec.Components[0].Traits[0].Name = "ingress"
		app.Spec.Components[0].Traits[0].Properties = runtime.RawExtension{Raw: []byte(`{"domain":"example.com","http":{"/":80}}`)}

		expDeployment.Name = app.Name
		expDeployment.Namespace = ns.Name
		expDeployment.Labels[oam.LabelAppName] = app.Name
		expDeployment.Labels[oam.LabelAppComponent] = compName
		expDeployment.Labels["app.oam.dev/resourceType"] = "WORKLOAD"
		Expect(k8sClient.Create(ctx, expDeployment)).Should(BeNil())

		expWorkloadTrait := unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"trait.oam.dev/type":     "AuxiliaryWorkload",
					"app.oam.dev/component":  compName,
					"app.oam.dev/name":       app.Name,
					"trait.oam.dev/resource": "gameconfig",
				},
			},
			"data": map[string]interface{}{
				"enemies": "alien",
				"lives":   "3",
			},
		}}
		expWorkloadTrait.SetName("myweb-health-statusgame-config")
		expWorkloadTrait.SetNamespace(app.Namespace)
		Expect(k8sClient.Create(ctx, &expWorkloadTrait)).Should(BeNil())

		expTrait := unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "networking.k8s.io/v1beta1",
			"kind":       "Ingress",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"trait.oam.dev/type":     "ingress",
					"trait.oam.dev/resource": "ingress",
					"app.oam.dev/component":  compName,
					"app.oam.dev/name":       app.Name,
				},
			},
			"spec": map[string]interface{}{
				"rules": []interface{}{
					map[string]interface{}{
						"host": "example.com",
					},
				},
			},
		}}
		expTrait.SetName(compName)
		expTrait.SetNamespace(app.Namespace)
		Expect(k8sClient.Create(ctx, &expTrait)).Should(BeNil())

		expTrait2 := unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"trait.oam.dev/type":     "ingress",
					"trait.oam.dev/resource": "service",
					"app.oam.dev/component":  compName,
					"app.oam.dev/name":       app.Name,
				},
			},
			"spec": map[string]interface{}{
				"clusterIP": "10.0.0.4",
				"ports": []interface{}{
					map[string]interface{}{
						"port": 80,
					},
				},
			},
		}}
		expTrait2.SetName(app.Name)
		expTrait2.SetNamespace(app.Namespace)
		Expect(k8sClient.Create(ctx, &expTrait2)).Should(BeNil())

		By("enrich the status of deployment and ingress trait")
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())
		got := &v1.Deployment{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      app.Name,
		}, got)).Should(BeNil())

		By("apply appfile")
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		appKey := client.ObjectKey{
			Name:      app.Name,
			Namespace: app.Namespace,
		}
		reconcileRetry(reconciler, reconcile.Request{NamespacedName: appKey})

		By("Check App running successfully")
		checkApp := &v1alpha2.Application{}
		Eventually(func() string {
			_, err := reconciler.Reconcile(reconcile.Request{NamespacedName: appKey})
			if err != nil {
				return err.Error()
			}
			err = k8sClient.Get(ctx, appKey, checkApp)
			if err != nil {
				return err.Error()
			}
			if checkApp.Status.Phase != v1alpha2.ApplicationRunning {
				fmt.Println(checkApp.Status.Conditions)
			}
			return string(checkApp.Status.Phase)
		}(), 5*time.Second, time.Second).Should(BeEquivalentTo(v1alpha2.ApplicationRunning))
		Expect(checkApp.Status.Services).Should(BeEquivalentTo([]v1alpha2.ApplicationComponentStatus{
			{
				Name:    compName,
				Healthy: true,
				Message: "type: busybox,\t enemies:alien",
				Traits: []v1alpha2.ApplicationTraitStatus{
					{
						Type:    "ingress",
						Healthy: true,
						Message: "type: ClusterIP,\t clusterIP:10.0.0.4,\t ports:80,\t domainexample.com",
					},
				},
			},
		}))
		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})
})

func reconcileRetry(r reconcile.Reconciler, req reconcile.Request) {
	Eventually(func() error {
		_, err := r.Reconcile(req)
		if err != nil {
			fmt.Println("reconcile err: ", err)
		}
		return err
	}, 3*time.Second, time.Second).Should(BeNil())
}

const (
	sDDefYaml = `apiVersion: core.oam.dev/v1alpha2
kind: ScopeDefinition
metadata:
  name: healthscopes.core.oam.dev
  namespace: default
spec:
  workloadRefsPath: spec.workloadRefs
  allowComponentOverlap: true
  definitionRef:
    name: healthscopes.core.oam.dev`

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
          metadata: {
              annotations: {
                  if context["config"] != _|_ {
                      for _, v in context.config {
                          "\(v.name)" : v.value
                      }
                  }
              }
          }
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
      }
`

	webserverYaml = `apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: webserver
  annotations:
    definition.oam.dev/description: "webserver was composed by deployment and service"
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

      					if parameter["env"] != _|_ {
      						env: parameter.env
      					}

      					if context["config"] != _|_ {
      						env: context.config
      					}

      					ports: [{
      						containerPort: parameter.port
      					}]

      					if parameter["cpu"] != _|_ {
      						resources: {
      							limits:
      								cpu: parameter.cpu
      							requests:
      								cpu: parameter.cpu
      						}
      					}
      				}]
      		}
      		}
      	}
      }
      // workload can have extra object composition by using 'outputs' keyword
      outputs: service: {
      	apiVersion: "v1"
      	kind:       "Service"
      	spec: {
      		selector: {
      			"app.oam.dev/component": context.name
      		}
      		ports: [
      			{
      				port:       parameter.port
      				targetPort: parameter.port
      			},
      		]
      	}
      }
      parameter: {
      	image: string
      	cmd?: [...string]
      	port: *80 | int
      	env?: [...{
      		name:   string
      		value?: string
      		valueFrom?: {
      			secretKeyRef: {
      				name: string
      				key:  string
      			}
      		}
      	}]
      	cpu?: string
      }

`
	wDDefWithHealthYaml = `
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
    healthPolicy: |
      isHealth: context.output.status.readyReplicas == context.output.status.replicas 
    template: |
      output: {
          apiVersion: "apps/v1"
          kind:       "Deployment"
          metadata: {
              annotations: {
                  if context["config"] != _|_ {
                      for _, v in context.config {
                          "\(v.name)" : v.value
                      }
                  }
              }
          }
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
      }
`
	wdDefWithHealthStatusYaml = `apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: nworker
  annotations:
    definition.oam.dev/description: "Describes long-running, scalable, containerized services that running at backend. They do NOT have network endpoint to receive external network traffic."
spec:
  definitionRef:
    name: deployments.apps
  status:
    healthPolicy: |
      isHealth: (context.output.status.readyReplicas > 0) && (context.output.status.readyReplicas == context.output.status.replicas)
    customStatus: |-
      message: "type: " + context.output.spec.template.spec.containers[0].image + ",\t enemies:" + context.outputs.gameconfig.data.enemies
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
    					envFrom: [{
    						configMapRef: name: context.name + "game-config"
    					}]
    					if parameter["cmd"] != _|_ {
    						command: parameter.cmd
    					}
    				}]
    			}
    		}
    	}
    }

    outputs: gameconfig: {
    	apiVersion: "v1"
    	kind:       "ConfigMap"
    	metadata: {
    		name: context.name + "game-config"
    	}
    	data: {
    		enemies: parameter.enemies
    		lives:   parameter.lives
    	}
    }

    parameter: {
    	// +usage=Which image would you like to use for your service
    	// +short=i
    	image: string
    	// +usage=Commands to run in the container
    	cmd?: [...string]
    	lives:   string
    	enemies: string
    }
`
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
	tdDefYamlWithHttp = `
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
          token: processing.output.token
      	}
      }
      parameter: {
      	//+short=r
        replicas: *1 | int
        serviceURL: *"http://127.0.0.1:8090/api/v1/token?val=test-token" | string
      }
      processing: {
        output: {
          token ?: string
        }
        http: {
          method: *"GET" | string
          url: parameter.serviceURL
          request: {
              body ?: bytes
              header: {}
              trailer: {}
          }
        }
      }
`
	tDDefWithHealthYaml = `
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
    healthPolicy: |
      isHealth: context.output.status.conditions[0].status == "True"
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

	tDDefWithHealthStatusYaml = `apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name: ingress
spec:
  status:
    customStatus: |-
      message: "type: "+ context.outputs.service.spec.type +",\t clusterIP:"+ context.outputs.service.spec.clusterIP+",\t ports:"+ "\(context.outputs.service.spec.ports[0].port)"+",\t domain"+context.outputs.ingress.spec.rules[0].host
    healthPolicy: |
      isHealth: len(context.outputs.service.spec.clusterIP) > 0
  template: |
    parameter: {
    	domain: string
    	http: [string]: int
    }
    // trait template can have multiple outputs in one trait
    outputs: service: {
    	apiVersion: "v1"
    	kind:       "Service"
    	spec: {
    		selector:
    			app: context.name
    		ports: [
    			for k, v in parameter.http {
    				port:       v
    				targetPort: v
    			},
    		]
    	}
    }
    outputs: ingress: {
    	apiVersion: "networking.k8s.io/v1beta1"
    	kind:       "Ingress"
    	metadata:
    		name: context.name
    	spec: {
    		rules: [{
    			host: parameter.domain
    			http: {
    				paths: [
    					for k, v in parameter.http {
    						path: k
    						backend: {
    							serviceName: context.name
    							servicePort: v
    						}
    					},
    				]
    			}
    		}]
    	}
    }
`
)

func NewMock() *httptest.Server {
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			fmt.Printf("Expected 'GET' request, got '%s'", r.Method)
		}
		if r.URL.EscapedPath() != "/api/v1/token" {
			fmt.Printf("Expected request to '/person', got '%s'", r.URL.EscapedPath())
		}
		r.ParseForm()
		token := r.Form.Get("val")
		tokenBytes, _ := json.Marshal(map[string]interface{}{"token": token})

		w.WriteHeader(http.StatusOK)
		w.Write(tokenBytes)
	}))
	l, _ := net.Listen("tcp", "127.0.0.1:8090")
	ts.Listener.Close()
	ts.Listener = l
	ts.Start()
	return ts
}
