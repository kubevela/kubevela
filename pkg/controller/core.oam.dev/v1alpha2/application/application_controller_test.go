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
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"time"

	velatypes "github.com/oam-dev/kubevela/apis/types"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/ghodss/yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1beta12 "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
)

// TODO: Refactor the tests to not copy and paste duplicated code 10 times
var _ = Describe("Test Application Controller", func() {
	ctx := context.TODO()
	appwithConfig := &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-with-config",
			Namespace: "app-with-config",
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []v1beta1.ApplicationComponent{
				{
					Name:       "myweb1",
					Type:       "worker",
					Properties: runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","config":"myconfig"}`)},
				},
			},
		},
	}
	appwithNoTrait := &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "app-with-no-trait",
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []v1beta1.ApplicationComponent{
				{
					Name:       "myweb2",
					Type:       "worker",
					Properties: runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
				},
			},
		},
	}

	appFailParse := appwithNoTrait.DeepCopy()
	appFailParse.SetName("app-fail-to-parsed")
	appFailParse.Spec.Components[0].Type = "fakeWorker"

	appFailRender := appwithNoTrait.DeepCopy()
	appFailRender.SetName("app-fail-to-render")
	appFailRender.Spec.Components[0].Properties = runtime.RawExtension{
		Raw: []byte(`{"cmd1":["sleep","1000"],"image1":"busybox"}`),
	}

	appImportPkg := &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "app-import-pkg",
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []v1beta1.ApplicationComponent{
				{
					Name:       "myweb",
					Type:       "worker-import",
					Properties: runtime.RawExtension{Raw: []byte("{\"cmd\":[\"sleep\",\"1000\"],\"image\":\"busybox\"}")},
					Traits: []v1beta1.ApplicationTrait{
						{
							Type:       "ingress-import",
							Properties: runtime.RawExtension{Raw: []byte("{\"http\":{\"/\":80},\"domain\":\"abc.com\"}")},
						},
					},
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
					"workload.oam.dev/type":   "worker",
					"app.oam.dev/component":   compName,
					"app.oam.dev/name":        appName,
					"app.oam.dev/appRevision": appName + "-v1",
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
	appWithTrait.Spec.Components[0].Traits = []v1beta1.ApplicationTrait{
		{
			Type:       "scaler",
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
					"trait.oam.dev/type":      "scaler",
					"app.oam.dev/component":   compName,
					"app.oam.dev/name":        appName,
					"app.oam.dev/appRevision": appName + "-v1",
					"trait.oam.dev/resource":  "scaler",
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
	appWithTwoComp.Spec.Components = append(appWithTwoComp.Spec.Components, v1beta1.ApplicationComponent{
		Name:       "myweb6",
		Type:       "worker",
		Properties: runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox2","config":"myconfig"}`)},
		Scopes:     map[string]string{"healthscopes.core.oam.dev": "app-with-two-comp-default-health"},
	})

	cd := &v1beta1.ComponentDefinition{}
	cDDefJson, _ := yaml.YAMLToJSON([]byte(componentDefYaml))

	importWd := &v1beta1.WorkloadDefinition{}
	importWdJson, _ := yaml.YAMLToJSON([]byte(wDImportYaml))

	importTd := &v1alpha2.TraitDefinition{}

	webserverwd := &v1alpha2.ComponentDefinition{}
	webserverwdJson, _ := yaml.YAMLToJSON([]byte(webComponentDefYaml))

	td := &v1beta1.TraitDefinition{}
	tDDefJson, _ := yaml.YAMLToJSON([]byte(traitDefYaml))

	sd := &v1beta1.ScopeDefinition{}
	sdDefJson, _ := yaml.YAMLToJSON([]byte(scopeDefYaml))

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "kubevela-app-with-config-myweb1-myconfig", Namespace: appwithConfig.Namespace},
		Data:       map[string]string{"c1": "v1", "c2": "v2"},
	}

	BeforeEach(func() {
		Expect(json.Unmarshal(cDDefJson, cd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, cd.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		Expect(json.Unmarshal(importWdJson, importWd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, importWd.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		importTdJson, err := yaml.YAMLToJSON([]byte(tdImportedYaml))
		Expect(err).ShouldNot(HaveOccurred())
		Expect(json.Unmarshal(importTdJson, importTd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, importTd.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		Expect(json.Unmarshal(tDDefJson, td)).Should(BeNil())
		Expect(k8sClient.Create(ctx, td.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		Expect(json.Unmarshal(sdDefJson, sd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, sd.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		Expect(json.Unmarshal(webserverwdJson, webserverwd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, webserverwd.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		var deployDef v1alpha2.WorkloadDefinition
		Expect(yaml.Unmarshal([]byte(deploymentWorkloadDefinition), &deployDef)).Should(BeNil())
		Expect(k8sClient.Create(ctx, &deployDef)).Should(SatisfyAny(BeNil()))
	})

	AfterEach(func() {
		var tobeDeletedDeployDef v1alpha2.WorkloadDefinition
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "deployment", Namespace: "default"}, &tobeDeletedDeployDef)).Should(SatisfyAny(BeNil()))
		Expect(k8sClient.Delete(ctx, &tobeDeletedDeployDef)).Should(SatisfyAny(BeNil()))
		By("[TEST] Clean up resources after an integration test")
	})

	It("app step will set event", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-app-without-trait-event",
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

		events, err := recorder.GetEventsWithName(appwithNoTrait.Name)
		Expect(err).Should(BeNil())
		Expect(len(events)).ShouldNot(Equal(0))
		for _, event := range events {
			Expect(event.EventType).ShouldNot(Equal(corev1.EventTypeWarning))
			Expect(event.EventType).Should(Equal(corev1.EventTypeNormal))
		}

		// fail to parse application
		appFailParse.SetNamespace(ns.Name)
		appFailParseKey := client.ObjectKey{
			Name:      appFailParse.Name,
			Namespace: appFailParse.Namespace,
		}

		Expect(k8sClient.Create(ctx, appFailParse.DeepCopyObject())).Should(BeNil())
		reconcileOnce(reconciler, reconcile.Request{NamespacedName: appFailParseKey})

		parseEvents, err := recorder.GetEventsWithName(appFailParse.Name)
		Expect(err).Should(BeNil())
		Expect(len(parseEvents)).Should(Equal(1))
		for _, event := range parseEvents {
			Expect(event.EventType).Should(Equal(corev1.EventTypeWarning))
			Expect(event.Reason).Should(Equal(velatypes.ReasonFailedParse))
		}

		// fail to render application
		appFailRender.SetNamespace(ns.Name)
		appFailRenderKey := client.ObjectKey{
			Name:      appFailRender.Name,
			Namespace: appFailRender.Namespace,
		}
		Expect(k8sClient.Create(ctx, appFailRender.DeepCopyObject())).Should(BeNil())
		reconcileOnce(reconciler, reconcile.Request{NamespacedName: appFailRenderKey})

		renderEvents, err := recorder.GetEventsWithName(appFailRender.Name)
		Expect(err).Should(BeNil())
		Expect(len(renderEvents)).Should(Equal(2))

		var count int
		for _, event := range renderEvents {
			if event.EventType == corev1.EventTypeWarning {
				Expect(event.Reason).Should(Equal(velatypes.ReasonFailedRender))
				count++
			}
		}
		Expect(count).Should(Equal(1))

	})

	It("app can consume db secret generated by other application", func() {
		var (
			appName          = "webapp"
			ns               = "default"
			componentName    = "express-server-test"
			targetSecretName = "db-conn"
			secretData       = map[string][]byte{
				"endpoint": []byte("aaa"),
				"password": []byte("bbb"),
				"username": []byte("ccc"),
			}

			businessApplication = `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: webapp
  namespace: default
spec:
  components:
    - name: express-server-test
      type: deployment
      properties:
        image: "nignx:latest"
        ports: 80
        dbSecret: "db-conn"
`
			appKey = client.ObjectKey{
				Name:      appName,
				Namespace: ns,
			}
		)

		By("Check WorkloadDefinition")
		var wd v1alpha2.WorkloadDefinition
		err := k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "deployment"}, &wd)
		Expect(err).Should(BeNil())

		By("create secret")
		s := &corev1.Secret{
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetSecretName,
				Namespace: ns,
			},
			Data: secretData,
		}
		err = k8sClient.Create(ctx, s)
		Expect(err).Should(BeNil())

		By("apply business application which needs consume db")
		var app v1beta1.Application
		err = yaml.Unmarshal([]byte(businessApplication), &app)
		Expect(err).Should(BeNil())

		err = k8sClient.Create(ctx, &app)
		Expect(err).Should(BeNil())

		reconcileRetry(reconciler, reconcile.Request{NamespacedName: appKey})

		By("checking application")
		var a v1beta1.Application
		err = k8sClient.Get(ctx, appKey, &a)
		Expect(err).Should(BeNil())

		By("Check ApplicationContext Created")
		var appContext v1alpha2.ApplicationContext
		Expect(k8sClient.Get(ctx, appKey, &appContext)).Should(BeNil())

		By("Check Component Created with the expected workload spec")
		var component v1alpha2.Component
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: componentName}, &component)).Should(BeNil())

		var deploy v1.Deployment
		Expect(json.Unmarshal(component.Spec.Workload.Raw, &deploy)).Should(BeNil())
		containers := deploy.Spec.Template.Spec.Containers
		Expect(len(containers)).Should(Equal(1))
		envs := containers[0].Env
		Expect(len(envs)).Should(Equal(1))
		Expect(envs[0].Value).Should(Equal("ccc"))
	})

	It("app-without-trait will only create workload", func() {
		expDeployment := getExpDeployment("myweb2", appwithNoTrait.Name)
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-app-without-trait",
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
		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(Equal(common.ApplicationRunning))

		By("Check ApplicationContext Created")
		appContext := &v1alpha2.ApplicationContext{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: appwithNoTrait.Namespace,
			Name:      appwithNoTrait.Name,
		}, appContext)).Should(BeNil())
		// check that the new appContext has the correct annotation and labels
		Expect(appContext.GetAnnotations()[oam.AnnotationAppRollout]).Should(BeEmpty())
		Expect(appContext.GetLabels()[oam.LabelAppRevisionHash]).ShouldNot(BeEmpty())
		Expect(appContext.Spec.ApplicationRevisionName).ShouldNot(BeEmpty())

		By("Check Component Created with the expected workload spec")
		var component v1alpha2.Component
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: appwithNoTrait.Namespace,
			Name:      "myweb2",
		}, &component)).Should(BeNil())
		Expect(component.ObjectMeta.Labels).Should(BeEquivalentTo(map[string]string{oam.LabelAppName: "app-with-no-trait"}))
		Expect(component.ObjectMeta.OwnerReferences[0].Name).Should(BeEquivalentTo("app-with-no-trait"))
		Expect(component.ObjectMeta.OwnerReferences[0].Kind).Should(BeEquivalentTo("Application"))
		Expect(component.ObjectMeta.OwnerReferences[0].APIVersion).Should(BeEquivalentTo("core.oam.dev/v1beta1"))
		Expect(component.ObjectMeta.OwnerReferences[0].Controller).Should(BeEquivalentTo(pointer.BoolPtr(true)))
		Expect(component.Status.LatestRevision).ShouldNot(BeNil())

		// check the workload created should be the same as the raw data in the component
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
		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(Equal(common.ApplicationRunning))

		By("Check ApplicationContext Created")
		appContext := &v1alpha2.ApplicationContext{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      app.Name,
		}, appContext)).Should(BeNil())

		By("Check Component Created with the expected workload spec")
		component := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      "myweb1",
		}, component)).Should(BeNil())
		Expect(component.ObjectMeta.Labels).Should(BeEquivalentTo(map[string]string{oam.LabelAppName: "app-with-config"}))
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
		curApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.Phase).Should(Equal(common.ApplicationRunning))

		By("Check ApplicationContext and trait created as expected")
		appContext := &v1alpha2.ApplicationContext{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      app.Name,
		}, appContext)).Should(BeNil())
		appRevision := &v1beta1.ApplicationRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      curApp.Status.LatestRevision.Name,
		}, appRevision)).Should(BeNil())

		gotTrait := unstructured.Unstructured{}

		ac, err := util.RawExtension2AppConfig(appRevision.Spec.ApplicationConfiguration)
		Expect(err).Should(BeNil())
		Expect(json.Unmarshal(ac.Spec.Components[0].Traits[0].Trait.Raw,
			&gotTrait)).Should(BeNil())
		Expect(gotTrait).Should(BeEquivalentTo(expectScalerTrait("myweb3", app.Name)))

		By("Check component created as expected")
		component := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      "myweb3",
		}, component)).Should(BeNil())
		Expect(component.ObjectMeta.Labels).Should(BeEquivalentTo(map[string]string{oam.LabelAppName: "app-with-trait"}))
		Expect(component.ObjectMeta.OwnerReferences[0].Name).Should(BeEquivalentTo("app-with-trait"))
		Expect(component.ObjectMeta.OwnerReferences[0].Kind).Should(BeEquivalentTo("Application"))
		Expect(component.ObjectMeta.OwnerReferences[0].APIVersion).Should(BeEquivalentTo("core.oam.dev/v1beta1"))
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
		appWithComposedWorkload.Spec.Components[0].Type = "webserver"
		appWithComposedWorkload.SetName(appname)
		appWithComposedWorkload.Spec.Components[0].Traits = []v1beta1.ApplicationTrait{
			{
				Type:       "scaler",
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
		curApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.Phase).Should(Equal(common.ApplicationRunning))

		By("Check AppConfig and trait created as expected")
		appContext := &v1alpha2.ApplicationContext{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      app.Name,
		}, appContext)).Should(BeNil())
		appRevision := &v1beta1.ApplicationRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      curApp.Status.LatestRevision.Name,
		}, appRevision)).Should(BeNil())

		Expect(appContext.Spec.ApplicationRevisionName).Should(Equal(appRevision.Name))

		ac, err := util.RawExtension2AppConfig(appRevision.Spec.ApplicationConfiguration)
		Expect(err).Should(BeNil())
		Expect(len(ac.Spec.Components[0].Traits)).Should(BeEquivalentTo(2))
		Expect(ac.Spec.Components[0].ComponentName).Should(BeEmpty())
		Expect(ac.Spec.Components[0].RevisionName).ShouldNot(BeEmpty())
		// component create handler may create a v2 when it can't find v1
		Expect(ac.Spec.Components[0].RevisionName).Should(
			SatisfyAny(BeEquivalentTo(utils.ConstructRevisionName(compName, 1)),
				BeEquivalentTo(utils.ConstructRevisionName(compName, 2))))

		gotTrait := unstructured.Unstructured{}
		By("Check the first trait should be service")
		expectServiceTrait := unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"trait.oam.dev/type":      "AuxiliaryWorkload",
					"app.oam.dev/name":        "app-with-composedworkload-trait",
					"app.oam.dev/appRevision": "app-with-composedworkload-trait-v1",
					"app.oam.dev/component":   "myweb-composed-3",
					"trait.oam.dev/resource":  "service",
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
		ac, err = util.RawExtension2AppConfig(appRevision.Spec.ApplicationConfiguration)
		Expect(err).Should(BeNil())
		Expect(json.Unmarshal(ac.Spec.Components[0].Traits[0].Trait.Raw, &gotTrait)).Should(BeNil())
		fmt.Println(cmp.Diff(expectServiceTrait, gotTrait))
		Expect(assert.ObjectsAreEqual(expectServiceTrait, gotTrait)).Should(BeTrue())

		By("Check the second trait should be scaler")
		gotTrait = unstructured.Unstructured{}
		Expect(json.Unmarshal(ac.Spec.Components[0].Traits[1].Trait.Raw, &gotTrait)).Should(BeNil())
		Expect(gotTrait).Should(BeEquivalentTo(expectScalerTrait("myweb-composed-3", app.Name)))

		By("Check component created as expected")
		component := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      compName,
		}, component)).Should(BeNil())
		Expect(component.ObjectMeta.Labels).Should(BeEquivalentTo(map[string]string{oam.LabelAppName: appname}))
		Expect(component.ObjectMeta.OwnerReferences[0].Name).Should(BeEquivalentTo(appname))
		Expect(component.ObjectMeta.OwnerReferences[0].Kind).Should(BeEquivalentTo("Application"))
		Expect(component.ObjectMeta.OwnerReferences[0].APIVersion).Should(BeEquivalentTo("core.oam.dev/v1beta1"))
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
		curApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.Phase).Should(Equal(common.ApplicationRunning))

		By("Check App scope status")
		scopes := curApp.Status.Services[0].Scopes
		Expect(len(scopes)).Should(BeEquivalentTo(1))
		Expect(scopes[0].APIVersion).Should(BeEquivalentTo("core.oam.dev/v1alpha2"))
		Expect(scopes[0].Kind).Should(BeEquivalentTo("HealthScope"))
		Expect(scopes[0].Name).Should(BeEquivalentTo("appWithTraitAndScope-default-health"))

		By("Check AppConfig and trait created as expected")
		appContext := &v1alpha2.ApplicationContext{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      app.Name,
		}, appContext)).Should(BeNil())
		appRevision := &v1beta1.ApplicationRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      curApp.Status.LatestRevision.Name,
		}, appRevision)).Should(BeNil())
		Expect(appContext.Spec.ApplicationRevisionName).Should(Equal(appRevision.Name))
		Expect(appContext.GetAnnotations()[oam.AnnotationInplaceUpgrade]).Should(Equal("true"))

		gotTrait := unstructured.Unstructured{}
		ac, err := util.RawExtension2AppConfig(appRevision.Spec.ApplicationConfiguration)
		Expect(err).Should(BeNil())
		Expect(json.Unmarshal(ac.Spec.Components[0].Traits[0].Trait.Raw, &gotTrait)).Should(BeNil())
		Expect(gotTrait).Should(BeEquivalentTo(expectScalerTrait("myweb4", app.Name)))

		Expect(ac.Spec.Components[0].Scopes[0].ScopeReference).Should(BeEquivalentTo(v1alpha1.TypedReference{
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
		Expect(component.ObjectMeta.Labels).Should(BeEquivalentTo(map[string]string{oam.LabelAppName: "app-with-trait-and-scope"}))
		Expect(component.ObjectMeta.OwnerReferences[0].Name).Should(BeEquivalentTo("app-with-trait-and-scope"))
		Expect(component.ObjectMeta.OwnerReferences[0].Kind).Should(BeEquivalentTo("Application"))
		Expect(component.ObjectMeta.OwnerReferences[0].APIVersion).Should(BeEquivalentTo("core.oam.dev/v1beta1"))
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
		curApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.Phase).Should(Equal(common.ApplicationRunning))

		By("Check AppConfig and trait created as expected")
		appContext := &v1alpha2.ApplicationContext{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      app.Name,
		}, appContext)).Should(BeNil())
		appRevision := &v1beta1.ApplicationRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      curApp.Status.LatestRevision.Name,
		}, appRevision)).Should(BeNil())
		Expect(appContext.Spec.ApplicationRevisionName).Should(Equal(appRevision.Name))
		Expect(appContext.GetAnnotations()[oam.AnnotationInplaceUpgrade]).Should(Equal("true"))

		gotTrait := unstructured.Unstructured{}
		ac, err := util.RawExtension2AppConfig(appRevision.Spec.ApplicationConfiguration)
		Expect(err).Should(BeNil())
		Expect(json.Unmarshal(ac.Spec.Components[0].Traits[0].Trait.Raw, &gotTrait)).Should(BeNil())
		Expect(gotTrait).Should(BeEquivalentTo(expectScalerTrait("myweb5", app.Name)))

		Expect(ac.Spec.Components[0].Scopes[0].ScopeReference).Should(BeEquivalentTo(v1alpha1.TypedReference{
			APIVersion: "core.oam.dev/v1alpha2",
			Kind:       "HealthScope",
			Name:       "app-with-two-comp-default-health",
		}))
		Expect(ac.Spec.Components[1].Scopes[0].ScopeReference).Should(BeEquivalentTo(v1alpha1.TypedReference{
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
		Expect(component5.ObjectMeta.Labels).Should(BeEquivalentTo(map[string]string{oam.LabelAppName: app.Name}))
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
		Expect(component6.ObjectMeta.Labels).Should(BeEquivalentTo(map[string]string{oam.LabelAppName: app.Name}))
		gotD2 := &v1.Deployment{}
		Expect(json.Unmarshal(component6.Spec.Workload.Raw, gotD2)).Should(BeNil())
		fmt.Println(cmp.Diff(expDeployment6, gotD2))
		Expect(gotD2).Should(BeEquivalentTo(expDeployment6))

		By("Update Application with new revision, component5 with new spec, rename component6 it should create new component ")

		curApp.SetNamespace(app.Namespace)
		curApp.Spec.Components[0] = v1beta1.ApplicationComponent{
			Name:       "myweb5",
			Type:       "worker",
			Properties: runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox3"}`)},
			Scopes:     map[string]string{"healthscopes.core.oam.dev": "app-with-two-comp-default-health"},
		}
		curApp.Spec.Components[1] = v1beta1.ApplicationComponent{
			Name:       "myweb7",
			Type:       "worker",
			Properties: runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
			Scopes:     map[string]string{"healthscopes.core.oam.dev": "app-with-two-comp-default-health"},
		}
		Expect(k8sClient.Update(ctx, curApp)).Should(BeNil())
		reconcileRetry(reconciler, reconcile.Request{NamespacedName: appKey})

		By("Check App updated successfully")
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.Phase).Should(Equal(common.ApplicationRunning))

		By("check AC and Component updated")
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      app.Name,
		}, appContext)).Should(BeNil())
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      curApp.Status.LatestRevision.Name,
		}, appRevision)).Should(BeNil())
		Expect(appContext.Spec.ApplicationRevisionName).Should(Equal(appRevision.Name))
		Expect(appContext.GetAnnotations()[oam.AnnotationInplaceUpgrade]).Should(Equal("true"))

		Expect(json.Unmarshal(ac.Spec.Components[0].Traits[0].Trait.Raw, &gotTrait)).Should(BeNil())
		Expect(gotTrait).Should(BeEquivalentTo(expectScalerTrait("myweb5", app.Name)))

		Expect(ac.Spec.Components[0].Scopes[0].ScopeReference).Should(BeEquivalentTo(v1alpha1.TypedReference{
			APIVersion: "core.oam.dev/v1alpha2",
			Kind:       "HealthScope",
			Name:       "app-with-two-comp-default-health",
		}))
		Expect(ac.Spec.Components[1].Scopes[0].ScopeReference).Should(BeEquivalentTo(v1alpha1.TypedReference{
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
		expDeployment.Labels["app.oam.dev/appRevision"] = app.Name + "-v2"
		Expect(gotD).Should(BeEquivalentTo(expDeployment))

		expDeployment7 := getExpDeployment("myweb7", app.Name)
		component7 := &v1alpha2.Component{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      "myweb7",
		}, component7)).Should(BeNil())
		Expect(component7.ObjectMeta.Labels).Should(BeEquivalentTo(map[string]string{oam.LabelAppName: app.Name}))
		gotD3 := &v1.Deployment{}
		Expect(json.Unmarshal(component7.Spec.Workload.Raw, gotD3)).Should(BeNil())
		expDeployment7.Labels["app.oam.dev/appRevision"] = app.Name + "-v2"
		fmt.Println(cmp.Diff(gotD3, expDeployment7))
		Expect(gotD3).Should(BeEquivalentTo(expDeployment7))
		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("app-with-trait will create workload and trait with http task", func() {
		s := NewMock()
		defer s.Close()
		expTrait := expectScalerTrait(appWithTrait.Spec.Components[0].Name, appWithTrait.Name)
		expTrait.Object["spec"].(map[string]interface{})["token"] = "test-token"

		By("change trait definition with http task")
		ntd, otd := &v1beta1.TraitDefinition{}, &v1beta1.TraitDefinition{}
		tDDefJson, _ := yaml.YAMLToJSON([]byte(tdDefYamlWithHttp))
		Expect(json.Unmarshal(tDDefJson, ntd)).Should(BeNil())
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: ntd.Name, Namespace: ntd.Namespace}, otd)).Should(BeNil())
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
		curApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.Phase).Should(Equal(common.ApplicationRunning))

		By("Check AppConfig and trait created as expected")
		appContext := &v1alpha2.ApplicationContext{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      app.Name,
		}, appContext)).Should(BeNil())
		appRevision := &v1beta1.ApplicationRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      curApp.Status.LatestRevision.Name,
		}, appRevision)).Should(BeNil())
		Expect(appContext.Spec.ApplicationRevisionName).Should(Equal(appRevision.Name))
		gotTrait := unstructured.Unstructured{}

		ac, err := util.RawExtension2AppConfig(appRevision.Spec.ApplicationConfiguration)
		Expect(err).Should(BeNil())
		Expect(json.Unmarshal(ac.Spec.Components[0].Traits[0].Trait.Raw, &gotTrait)).Should(BeNil())
		Expect(gotTrait).Should(BeEquivalentTo(expTrait))

		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("app with health policy for workload", func() {
		By("change workload and trait definition with health policy")
		ncd, ocd := &v1beta1.ComponentDefinition{}, &v1beta1.ComponentDefinition{}
		cDDefJson, _ := yaml.YAMLToJSON([]byte(componentDefWithHealthYaml))
		Expect(json.Unmarshal(cDDefJson, ncd)).Should(BeNil())
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: ncd.Name, Namespace: ncd.Namespace}, ocd)).Should(BeNil())
		ncd.ResourceVersion = ocd.ResourceVersion
		Expect(k8sClient.Update(ctx, ncd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		ntd, otd := &v1beta1.TraitDefinition{}, &v1beta1.TraitDefinition{}
		tDDefJson, _ := yaml.YAMLToJSON([]byte(tDDefWithHealthYaml))
		Expect(json.Unmarshal(tDDefJson, ntd)).Should(BeNil())
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: ntd.Name, Namespace: ntd.Namespace}, otd)).Should(BeNil())
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
			oam.LabelAppName:         app.Name,
			"trait.oam.dev/type":     "scaler",
			"app.oam.dev/component":  "myweb-health",
			"trait.oam.dev/resource": "scaler",
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
			checkApp := &v1beta1.Application{}
			err = k8sClient.Get(ctx, appKey, checkApp)
			if err != nil {
				return err.Error()
			}
			if checkApp.Status.Phase != common.ApplicationRunning {
				fmt.Println(checkApp.Status.Conditions)
			}
			return string(checkApp.Status.Phase)
		}(), 5*time.Second, time.Second).Should(BeEquivalentTo(common.ApplicationRunning))

		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("app with rollout annotation", func() {
		By("create application with rolling out annotation")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-app-with-rollout",
			},
		}
		rolloutApp := appWithTraitAndScope.DeepCopy()
		rolloutApp.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		compName := rolloutApp.Spec.Components[0].Name
		// set the annotation
		rolloutApp.SetAnnotations(map[string]string{
			oam.AnnotationAppRollout: strconv.FormatBool(true),
			"keep":                   strconv.FormatBool(true),
		})
		Expect(k8sClient.Create(ctx, rolloutApp)).Should(BeNil())

		appKey := client.ObjectKey{
			Name:      rolloutApp.Name,
			Namespace: rolloutApp.Namespace,
		}
		reconcileRetry(reconciler, reconcile.Request{NamespacedName: appKey})

		By("Check AppRevision created as expected")
		Expect(k8sClient.Get(ctx, appKey, rolloutApp)).Should(Succeed())
		appRevision := &v1beta1.ApplicationRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: rolloutApp.Namespace,
			Name:      utils.ConstructRevisionName(rolloutApp.Name, 1),
		}, appRevision)).Should(BeNil())

		By("Check ApplicationContext not created")
		appContext := &v1alpha2.ApplicationContext{}
		// no appContext same name as app exist
		Expect(k8sClient.Get(ctx, appKey, appContext)).ShouldNot(Succeed())
		// no appContext same name as apprevision exist
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: rolloutApp.Namespace,
			Name:      utils.ConstructRevisionName(rolloutApp.Name, 1),
		}, appContext)).ShouldNot(Succeed())

		By("Check Component Created with the expected workload spec")
		var component v1alpha2.Component
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: rolloutApp.Namespace,
			Name:      compName,
		}, &component)).Should(BeNil())
		Expect(component.Status.LatestRevision).ShouldNot(BeNil())
		Expect(component.Status.LatestRevision.Revision).Should(BeEquivalentTo(1))
		// check that the appconfig has the correct annotation and labels
		ac, err := util.RawExtension2AppConfig(appRevision.Spec.ApplicationConfiguration)
		Expect(err).Should(BeNil())
		Expect(ac.GetAnnotations()[oam.AnnotationAppRollout]).Should(Equal(strconv.FormatBool(true)))
		Expect(ac.GetAnnotations()["keep"]).Should(Equal("true"))
		Expect(ac.Spec.Components[0].ComponentName).Should(BeEmpty())
		Expect(ac.Spec.Components[0].RevisionName).Should(Equal(component.Status.LatestRevision.Name))

		By("Reconcile again to make sure we are not creating more appConfigs")
		reconcileRetry(reconciler, reconcile.Request{NamespacedName: appKey})
		By("Verify that no new AppRevision created")
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: rolloutApp.Namespace,
			Name:      utils.ConstructRevisionName(rolloutApp.Name, 2),
		}, appRevision)).ShouldNot(Succeed())
		// no appContext same name as app exist
		Expect(k8sClient.Get(ctx, appKey, appContext)).ShouldNot(Succeed())
		// no appContext same name as apprevision exist
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: rolloutApp.Namespace,
			Name:      utils.ConstructRevisionName(rolloutApp.Name, 1),
		}, appContext)).ShouldNot(Succeed())

		By("Check no new Component created")
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: rolloutApp.Namespace,
			Name:      compName,
		}, &component)).Should(BeNil())
		Expect(component.Status.LatestRevision).ShouldNot(BeNil())
		Expect(component.Status.LatestRevision.Revision).ShouldNot(BeNil())
		Expect(component.Status.LatestRevision.Revision).Should(BeEquivalentTo(1))

		By("Remove rollout annotation should lead to new appContext created")
		Expect(k8sClient.Get(ctx, appKey, rolloutApp)).Should(Succeed())
		rolloutApp.SetAnnotations(map[string]string{
			"keep": "true",
		})
		Expect(k8sClient.Update(ctx, rolloutApp)).Should(BeNil())
		reconcileRetry(reconciler, reconcile.Request{NamespacedName: appKey})
		// app should create an appContext
		Expect(k8sClient.Get(ctx, appKey, appContext)).Should(Succeed())
		Expect(appContext.Spec.ApplicationRevisionName).Should(Equal(utils.ConstructRevisionName(rolloutApp.Name, 1)))
		By("Verify that no new AppRevision created")
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: rolloutApp.Namespace,
			Name:      utils.ConstructRevisionName(rolloutApp.Name, 2),
		}, appRevision)).ShouldNot(Succeed())
		// no appContext same name as apprevision exist
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: rolloutApp.Namespace,
			Name:      utils.ConstructRevisionName(rolloutApp.Name, 1),
		}, appContext)).ShouldNot(Succeed())
		By("Delete Application, clean the resource")
		Expect(k8sClient.Delete(ctx, rolloutApp)).Should(BeNil())
	})

	It("app with health policy and custom status for workload", func() {
		By("change workload and trait definition with health policy")
		ncd := &v1beta1.ComponentDefinition{}
		cDDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(cDDefJson, ncd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, ncd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		ntd := &v1beta1.TraitDefinition{}
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
		app.Spec.Components[0].Type = "nworker"
		app.Spec.Components[0].Properties = runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox3","lives":"3","enemies":"alain"}`)}
		app.Spec.Components[0].Traits[0].Type = "ingress"
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
					"trait.oam.dev/type":      "AuxiliaryWorkload",
					"app.oam.dev/component":   compName,
					"app.oam.dev/name":        app.Name,
					"trait.oam.dev/resource":  "gameconfig",
					"app.oam.dev/appRevision": app.Name + "-v1",
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
					"trait.oam.dev/type":      "ingress",
					"trait.oam.dev/resource":  "ingress",
					"app.oam.dev/component":   compName,
					"app.oam.dev/name":        app.Name,
					"app.oam.dev/appRevision": app.Name + "-v1",
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
					"trait.oam.dev/type":      "ingress",
					"trait.oam.dev/resource":  "service",
					"app.oam.dev/component":   compName,
					"app.oam.dev/name":        app.Name,
					"app.oam.dev/appRevision": app.Name + "-v1",
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
		checkApp := &v1beta1.Application{}
		Eventually(func() string {
			_, err := reconciler.Reconcile(reconcile.Request{NamespacedName: appKey})
			if err != nil {
				return err.Error()
			}
			err = k8sClient.Get(ctx, appKey, checkApp)
			if err != nil {
				return err.Error()
			}
			if checkApp.Status.Phase != common.ApplicationRunning {
				fmt.Println(checkApp.Status.Conditions)
			}
			return string(checkApp.Status.Phase)
		}(), 5*time.Second, time.Second).Should(BeEquivalentTo(common.ApplicationRunning))
		Expect(checkApp.Status.Services).Should(BeEquivalentTo([]common.ApplicationComponentStatus{
			{
				Name:               compName,
				WorkloadDefinition: ncd.Spec.Workload.Definition,
				Healthy:            true,
				Message:            "type: busybox,\t enemies:alien",
				Traits: []common.ApplicationTraitStatus{
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

	It("app with a component refer to an existing WorkloadDefinition", func() {
		appRefertoWd := appwithNoTrait.DeepCopy()
		appRefertoWd.Spec.Components[0] = v1beta1.ApplicationComponent{
			Name:       "mytask",
			Type:       "task",
			Properties: runtime.RawExtension{Raw: []byte(`{"image":"busybox", "cmd":["sleep","1000"]}`)},
		}
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-app-with-workload-task",
			},
		}
		appRefertoWd.SetName("test-app-with-workload-task")
		appRefertoWd.SetNamespace(ns.Name)

		taskWd := &v1beta1.WorkloadDefinition{}
		wDDefJson, _ := yaml.YAMLToJSON([]byte(workloadDefYaml))
		Expect(json.Unmarshal(wDDefJson, taskWd)).Should(BeNil())
		taskWd.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		Expect(k8sClient.Create(ctx, taskWd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		Expect(k8sClient.Create(ctx, appRefertoWd.DeepCopyObject())).Should(BeNil())

		appKey := client.ObjectKey{
			Name:      appRefertoWd.Name,
			Namespace: appRefertoWd.Namespace,
		}
		reconcileRetry(reconciler, reconcile.Request{NamespacedName: appKey})
		By("Check Application Created with the correct revision")
		curApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.Phase).Should(Equal(common.ApplicationRunning))
		Expect(curApp.Status.LatestRevision).ShouldNot(BeNil())
		Expect(curApp.Status.LatestRevision.Revision).Should(BeEquivalentTo(1))

		By("Check AppRevision created as expected")
		appRevision := &v1beta1.ApplicationRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: curApp.Namespace,
			Name:      curApp.Status.LatestRevision.Name,
		}, appRevision)).Should(BeNil())

		By("Check ApplicationContext created")
		appContext := &v1alpha2.ApplicationContext{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: curApp.Namespace,
			Name:      curApp.Name,
		}, appContext)).Should(BeNil())
	})

	It("app with two components and one component refer to an existing WorkloadDefinition", func() {
		appMix := appWithTwoComp.DeepCopy()
		appMix.Spec.Components[1] = v1beta1.ApplicationComponent{
			Name:       "mytask",
			Type:       "task",
			Properties: runtime.RawExtension{Raw: []byte(`{"image":"busybox", "cmd":["sleep","1000"]}`)},
		}
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-app-with-mix-components",
			},
		}
		appMix.SetName("test-app-with-mix-components")
		appMix.SetNamespace(ns.Name)

		taskWd := &v1beta1.WorkloadDefinition{}
		wDDefJson, _ := yaml.YAMLToJSON([]byte(workloadDefYaml))
		Expect(json.Unmarshal(wDDefJson, taskWd)).Should(BeNil())
		taskWd.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		Expect(k8sClient.Create(ctx, taskWd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		Expect(k8sClient.Create(ctx, appMix.DeepCopyObject())).Should(BeNil())

		appKey := client.ObjectKey{
			Name:      appMix.Name,
			Namespace: appMix.Namespace,
		}
		reconcileRetry(reconciler, reconcile.Request{NamespacedName: appKey})
		By("Check Application Created with the correct revision")
		curApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.Phase).Should(Equal(common.ApplicationRunning))
		Expect(curApp.Status.LatestRevision).ShouldNot(BeNil())
		Expect(curApp.Status.LatestRevision.Revision).Should(BeEquivalentTo(1))

		By("Check AppRevision created as expected")
		appRevision := &v1beta1.ApplicationRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: curApp.Namespace,
			Name:      curApp.Status.LatestRevision.Name,
		}, appRevision)).Should(BeNil())

		By("Check ApplicationContext created")
		appContext := &v1alpha2.ApplicationContext{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: curApp.Namespace,
			Name:      curApp.Name,
		}, appContext)).Should(BeNil())
	})

	It("app-import-pkg will create workload by imported kube package", func() {
		expDeployment := getExpDeployment("myweb", appImportPkg.Name)
		expDeployment.Labels["workload.oam.dev/type"] = "worker-import"
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-app-import-pkg",
			},
		}
		appImportPkg.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		Expect(k8sClient.Create(ctx, appImportPkg.DeepCopyObject())).Should(BeNil())

		appKey := client.ObjectKey{
			Name:      appImportPkg.Name,
			Namespace: appImportPkg.Namespace,
		}
		reconcileRetry(reconciler, reconcile.Request{NamespacedName: appKey})
		By("Check Application Created with the correct revision")
		curApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.Phase).Should(Equal(common.ApplicationRunning))
		Expect(curApp.Status.LatestRevision).ShouldNot(BeNil())
		Expect(curApp.Status.LatestRevision.Revision).Should(BeEquivalentTo(1))

		By("Check AppRevision created as expected")
		appRevision := &v1beta1.ApplicationRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: curApp.Namespace,
			Name:      curApp.Status.LatestRevision.Name,
		}, appRevision)).Should(BeNil())
		appConfig, err := util.RawExtension2AppConfig(appRevision.Spec.ApplicationConfiguration)
		Expect(err).ShouldNot(HaveOccurred())
		var gotSvc corev1.Service
		Expect(json.Unmarshal(appConfig.Spec.Components[0].Traits[0].Trait.Raw, &gotSvc)).ShouldNot(HaveOccurred())
		Expect(cmp.Diff(&gotSvc, &corev1.Service{
			TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name: "myweb",
				Labels: map[string]string{
					"app.oam.dev/component":   "myweb",
					"app.oam.dev/name":        "app-import-pkg",
					"trait.oam.dev/resource":  "service",
					"trait.oam.dev/type":      "ingress-import",
					"app.oam.dev/appRevision": "app-import-pkg-v1",
				}},
			Spec: corev1.ServiceSpec{
				Ports:    []corev1.ServicePort{{Port: 80, TargetPort: intstr.FromInt(80)}},
				Selector: map[string]string{"app.oam.dev/component": "myweb"},
			}})).Should(BeEquivalentTo(""))
		var gotIngress v1beta12.Ingress
		Expect(json.Unmarshal(appConfig.Spec.Components[0].Traits[1].Trait.Raw, &gotIngress)).ShouldNot(HaveOccurred())
		Expect(cmp.Diff(&gotIngress, &v1beta12.Ingress{
			TypeMeta: metav1.TypeMeta{Kind: "Ingress", APIVersion: "networking.k8s.io/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{
				Name: "myweb",
				Labels: map[string]string{
					"app.oam.dev/component":   "myweb",
					"app.oam.dev/name":        "app-import-pkg",
					"trait.oam.dev/resource":  "ingress",
					"trait.oam.dev/type":      "ingress-import",
					"app.oam.dev/appRevision": "app-import-pkg-v1",
				}},
			Spec: v1beta12.IngressSpec{Rules: []v1beta12.IngressRule{{Host: "abc.com",
				IngressRuleValue: v1beta12.IngressRuleValue{HTTP: &v1beta12.HTTPIngressRuleValue{Paths: []v1beta12.HTTPIngressPath{{
					Path:    "/",
					Backend: v1beta12.IngressBackend{ServiceName: "myweb", ServicePort: intstr.FromInt(80)}}}}}}},
			}})).Should(BeEquivalentTo(""))

		By("Check ApplicationContext created")
		appContext := &v1alpha2.ApplicationContext{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: curApp.Namespace,
			Name:      curApp.Name,
		}, appContext)).Should(BeNil())
		// check that the new appContext has the correct annotation and labels
		Expect(appContext.GetAnnotations()[oam.AnnotationAppRollout]).Should(BeEmpty())
		Expect(appContext.GetLabels()[oam.LabelAppRevisionHash]).ShouldNot(BeEmpty())

		By("Check Component Created with the expected workload spec")
		var component v1alpha2.Component
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: appImportPkg.Namespace,
			Name:      "myweb",
		}, &component)).Should(BeNil())
		Expect(component.ObjectMeta.Labels).Should(BeEquivalentTo(map[string]string{oam.LabelAppName: "app-import-pkg"}))
		Expect(component.ObjectMeta.OwnerReferences[0].Name).Should(BeEquivalentTo("app-import-pkg"))
		Expect(component.ObjectMeta.OwnerReferences[0].Kind).Should(BeEquivalentTo("Application"))
		Expect(component.ObjectMeta.OwnerReferences[0].APIVersion).Should(BeEquivalentTo("core.oam.dev/v1beta1"))
		Expect(component.ObjectMeta.OwnerReferences[0].Controller).Should(BeEquivalentTo(pointer.BoolPtr(true)))
		Expect(component.Status.LatestRevision).ShouldNot(BeNil())

		// check the workload created should be the same as the raw data in the component
		gotD := &v1.Deployment{}
		Expect(json.Unmarshal(component.Spec.Workload.Raw, gotD)).Should(BeNil())
		fmt.Println(cmp.Diff(expDeployment, gotD))
		Expect(assert.ObjectsAreEqual(expDeployment, gotD)).Should(BeEquivalentTo(true))
		By("Delete Application, clean the resource")
		Expect(k8sClient.Delete(ctx, appImportPkg)).Should(BeNil())
	})
	It("revision should exist in created workload render by context.appRevision", func() {

		expDeployment := getExpDeployment("myweb", "revision-app1")
		expDeployment.Labels["workload.oam.dev/type"] = "cd1"
		expDeployment.Spec.Template.Spec.Containers[0].Command = nil
		expDeployment.Spec.Template.Labels["app.oam.dev/revision"] = "revision-app1-v1"
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-app-revisionname",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())

		cd := &v1beta1.ComponentDefinition{}
		Expect(common2.ReadYamlToObject("testdata/revision/cd1.yaml", cd)).Should(BeNil())
		cd.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, cd.DeepCopyObject())).Should(BeNil())

		app := &v1beta1.Application{}
		Expect(common2.ReadYamlToObject("testdata/revision/app1.yaml", app)).Should(BeNil())
		app.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, app.DeepCopyObject())).Should(BeNil())

		appKey := client.ObjectKey{
			Name:      app.Name,
			Namespace: app.Namespace,
		}
		reconcileRetry(reconciler, reconcile.Request{NamespacedName: appKey})
		By("Check Application Created with the correct revision")
		curApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.Phase).Should(Equal(common.ApplicationRunning))
		Expect(curApp.Status.LatestRevision).ShouldNot(BeNil())
		Expect(curApp.Status.LatestRevision.Revision).Should(BeEquivalentTo(1))

		By("Check Component Created with the expected workload spec")
		var component v1alpha2.Component
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      "myweb",
		}, &component)).Should(BeNil())
		Expect(component.ObjectMeta.Labels).Should(BeEquivalentTo(map[string]string{oam.LabelAppName: app.Name}))
		Expect(component.Status.LatestRevision).ShouldNot(BeNil())

		// check the workload created should be the same as the raw data in the component
		gotD := &v1.Deployment{}
		Expect(json.Unmarshal(component.Spec.Workload.Raw, gotD)).Should(BeNil())
		fmt.Println(cmp.Diff(expDeployment, gotD))
		Expect(assert.ObjectsAreEqual(expDeployment, gotD)).Should(BeEquivalentTo(true))
		By("Delete Application, clean the resource")
		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})
})

func reconcileRetry(r reconcile.Reconciler, req reconcile.Request) {
	Eventually(func() error {
		result, err := r.Reconcile(req)
		if err != nil {
			By(fmt.Sprintf("reconcile err: %+v ", err))
		} else if result.Requeue || result.RequeueAfter > 0 {
			// retry if we need to requeue
			By("reconcile timeout as it still needs to requeue")
			return fmt.Errorf("reconcile timeout as it still needs to requeue")
		}
		return err
	}, 30*time.Second, time.Second).Should(BeNil())
}

func reconcileOnce(r reconcile.Reconciler, req reconcile.Request) {
	r.Reconcile(req)
}

const (
	scopeDefYaml = `apiVersion: core.oam.dev/v1beta1
kind: ScopeDefinition
metadata:
  name: healthscopes.core.oam.dev
  namespace: vela-system
spec:
  workloadRefsPath: spec.workloadRefs
  allowComponentOverlap: true
  definitionRef:
    name: healthscopes.core.oam.dev`

	componentDefYaml = `
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: worker
  namespace: vela-system
  annotations:
    definition.oam.dev/description: "Long-running scalable backend worker without network endpoint"
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
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

	wDImportYaml = `
apiVersion: core.oam.dev/v1beta1
kind: WorkloadDefinition
metadata:
  name: worker-import
  namespace: vela-system
  annotations:
    definition.oam.dev/description: "Long-running scalable backend worker without network endpoint"
spec:
  definitionRef:
    name: deployments.apps
  extension:
    template: |
      import (
          "k8s.io/apps/v1"
          appsv1 "kube/apps/v1"
      )
      output: v1.#Deployment & appsv1.#Deployment & {
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

	tdImportedYaml = `apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name: ingress-import
  namespace: vela-system
spec:
  appliesToWorkloads:
    - "*"
  schematic:
    cue:
      template: |
        import (
        	kubev1 "k8s.io/core/v1"
        	network "k8s.io/networking/v1beta1"
        )

        parameter: {
        	domain: string
        	http: [string]: int
        }

        outputs: {
        service: kubev1.#Service
        ingress: network.#Ingress
        }

        // trait template can have multiple outputs in one trait
        outputs: service: {
        	metadata:
        		name: context.name
        	spec: {
        		selector:
        			"app.oam.dev/component": context.name
        		ports: [
        			for k, v in parameter.http {
        				port:       v
        				targetPort: v
        			},
        		]
        	}
        }

        outputs: ingress: {
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
        }`

	webComponentDefYaml = `apiVersion: core.oam.dev/v1alpha2
kind: ComponentDefinition
metadata:
  name: webserver
  namespace: vela-system
  annotations:
    definition.oam.dev/description: "webserver was composed by deployment and service"
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
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
	componentDefWithHealthYaml = `
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: worker
  namespace: vela-system
  annotations:
    definition.oam.dev/description: "Long-running scalable backend worker without network endpoint"
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
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
	cdDefWithHealthStatusYaml = `apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: nworker
  namespace: vela-system
  annotations:
    definition.oam.dev/description: "Describes long-running, scalable, containerized services that running at backend. They do NOT have network endpoint to receive external network traffic."
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  status:
    healthPolicy: |
      isHealth: (context.output.status.readyReplicas > 0) && (context.output.status.readyReplicas == context.output.status.replicas)
    customStatus: |-
      message: "type: " + context.output.spec.template.spec.containers[0].image + ",\t enemies:" + context.outputs.gameconfig.data.enemies
  schematic:
    cue:
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
	workloadDefYaml = `
apiVersion: core.oam.dev/v1beta1
kind: WorkloadDefinition
metadata:
  name: task
  namespace: vela-system
  annotations:
    definition.oam.dev/description: "Describes jobs that run code or a script to completion."
spec:
  definitionRef:
    name: jobs.batch
  schematic:
    cue:
      template: |
        output: {
        	apiVersion: "batch/v1"
        	kind:       "Job"
        	spec: {
        		parallelism: parameter.count
        		completions: parameter.count
        		template: spec: {
        			restartPolicy: parameter.restart
        			containers: [{
        				name:  context.name
        				image: parameter.image
        
        				if parameter["cmd"] != _|_ {
        					command: parameter.cmd
        				}
        			}]
        		}
        	}
        }
        parameter: {
        	// +usage=specify number of tasks to run in parallel
        	// +short=c
        	count: *1 | int
        
        	// +usage=Which image would you like to use for your service
        	// +short=i
        	image: string
        
        	// +usage=Define the job restart policy, the value can only be Never or OnFailure. By default, it's Never.
        	restart: *"Never" | string
        
        	// +usage=Commands to run in the container
        	cmd?: [...string]
        }
`
	traitDefYaml = `
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "Manually scale the app"
  name: scaler
  namespace: vela-system
spec:
  appliesToWorkloads:
    - webservice
    - worker
  definitionRef:
    name: manualscalertraits.core.oam.dev
  workloadRefPath: spec.workloadRef
  extension:
    template: |-
      outputs: scaler: {
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
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "Manually scale the app"
  name: scaler
  namespace: vela-system
spec:
  appliesToWorkloads:
    - webservice
    - worker
  definitionRef:
    name: manualscalertraits.core.oam.dev
  workloadRefPath: spec.workloadRef
  extension:
    template: |-
      outputs: scaler: {
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
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "Manually scale the app"
  name: scaler
  namespace: vela-system
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
      outputs: scaler: {
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

	tDDefWithHealthStatusYaml = `apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: ingress
  namespace: vela-system
spec:
  status:
    customStatus: |-
      message: "type: "+ context.outputs.service.spec.type +",\t clusterIP:"+ context.outputs.service.spec.clusterIP+",\t ports:"+ "\(context.outputs.service.spec.ports[0].port)"+",\t domain"+context.outputs.ingress.spec.rules[0].host
    healthPolicy: |
      isHealth: len(context.outputs.service.spec.clusterIP) > 0
  schematic:
    cue:
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

	deploymentWorkloadDefinition = `
apiVersion: core.oam.dev/beta1
kind: WorkloadDefinition
metadata:
  name: deployment
  namespace: default
  annotations:
    definition.oam.dev/description: A Deployment provides declarative updates for Pods and ReplicaSets
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  extension:
    template: |
      output: {
      	apiVersion: "apps/v1"
      	kind:       "Deployment"
        metadata: name: "business-deploy"
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
      					name:  "business-deploy"
      					image: parameter.image

      					if parameter["cmd"] != _|_ {
      						command: parameter.cmd
      					}

      					if parameter["dbSecret"] != _|_ {
      						env: [
      							{
      								name: "username"
      								value: dbConn.username
      							},
      						]
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

      parameter: {
      	// +usage=Which image would you like to use for your service
      	// +short=i
      	image: string

      	// +usage=Commands to run in the container
      	cmd?: [...string]

      	// +usage=Which port do you want customer traffic sent to
      	// +short=p
      	port: *80 | int

      	// +usage=Referred db secret
      	// +insertSecretTo=dbConn
      	dbSecret?: string

      	// +usage=Number of CPU units for the service
      	cpu?: string
      }

      dbConn: {
      	username: string
      	endpoint: string
      	port:     string
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
