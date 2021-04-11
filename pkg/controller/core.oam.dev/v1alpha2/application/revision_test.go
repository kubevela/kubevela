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
	"math/rand"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("test generate revision ", func() {
	var appRevision1, appRevision2 v1beta1.ApplicationRevision
	var app v1beta1.Application
	cd := v1beta1.ComponentDefinition{}
	webCompDef := v1beta1.ComponentDefinition{}
	wd := v1beta1.WorkloadDefinition{}
	td := v1beta1.TraitDefinition{}
	sd := v1beta1.ScopeDefinition{}
	var handler appHandler
	var ac *v1alpha2.ApplicationConfiguration
	var comps []*v1alpha2.Component
	var namespaceName string
	var ns corev1.Namespace
	ctx := context.Background()

	BeforeEach(func() {
		namespaceName = "apply-test-" + strconv.FormatInt(rand.Int63n(10000000), 16)
		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespaceName,
			},
		}

		componentDefJson, _ := yaml.YAMLToJSON([]byte(componentDefYaml))
		Expect(json.Unmarshal(componentDefJson, &cd)).Should(BeNil())
		cd.ResourceVersion = ""
		Expect(k8sClient.Create(ctx, &cd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		traitDefJson, _ := yaml.YAMLToJSON([]byte(traitDefYaml))
		Expect(json.Unmarshal(traitDefJson, &td)).Should(BeNil())
		td.ResourceVersion = ""
		Expect(k8sClient.Create(ctx, &td)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		scopeDefJson, _ := yaml.YAMLToJSON([]byte(scopeDefYaml))
		Expect(json.Unmarshal(scopeDefJson, &sd)).Should(BeNil())
		sd.ResourceVersion = ""
		Expect(k8sClient.Create(ctx, &sd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		webserverCDJson, _ := yaml.YAMLToJSON([]byte(webComponentDefYaml))
		Expect(json.Unmarshal(webserverCDJson, &webCompDef)).Should(BeNil())
		webCompDef.ResourceVersion = ""
		Expect(k8sClient.Create(ctx, &webCompDef)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		workloadDefJson, _ := yaml.YAMLToJSON([]byte(workloadDefYaml))
		Expect(json.Unmarshal(workloadDefJson, &wd)).Should(BeNil())
		wd.ResourceVersion = ""
		Expect(k8sClient.Create(ctx, &wd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		By("Create the Namespace for test")
		Expect(k8sClient.Create(ctx, &ns)).Should(Succeed())

		app = v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "revision-apply-test",
				Namespace: namespaceName,
				UID:       "f97e2615-3822-4c62-a3bd-fb880e0bcec5",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []v1beta1.ApplicationComponent{
					{
						Type:   cd.Name,
						Name:   "express-server",
						Scopes: map[string]string{"healthscopes.core.oam.dev": "myapp-default-health"},
						Properties: runtime.RawExtension{
							Raw: []byte(`{"image": "oamdev/testapp:v1", "cmd": ["node", "server.js"]}`),
						},
						Traits: []v1beta1.ApplicationTrait{
							{
								Type: td.Name,
								Properties: runtime.RawExtension{
									Raw: []byte(`{"replicas": 5}`),
								},
							},
						},
					},
				},
			},
		}
		// create the application
		Expect(k8sClient.Create(ctx, &app)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		appRevision1 = v1beta1.ApplicationRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name: "appRevision1",
			},
			Spec: v1beta1.ApplicationRevisionSpec{
				ComponentDefinitions: make(map[string]v1beta1.ComponentDefinition),
				WorkloadDefinitions:  make(map[string]v1beta1.WorkloadDefinition),
				TraitDefinitions:     make(map[string]v1beta1.TraitDefinition),
				ScopeDefinitions:     make(map[string]v1beta1.ScopeDefinition),
			},
		}
		appRevision1.Spec.Application = app
		appRevision1.Spec.ComponentDefinitions[cd.Name] = cd
		appRevision1.Spec.WorkloadDefinitions[wd.Name] = wd
		appRevision1.Spec.TraitDefinitions[td.Name] = td
		appRevision1.Spec.ScopeDefinitions[sd.Name] = sd

		appRevision2 = *appRevision1.DeepCopy()
		appRevision2.Name = "appRevision2"

		handler = appHandler{
			r:      reconciler,
			app:    &app,
			logger: reconciler.Log.WithValues("apply", "unit-test"),
		}

	})

	AfterEach(func() {
		By("[TEST] Clean up resources after an integration test")
		Expect(k8sClient.Delete(context.TODO(), &ns)).Should(Succeed())
	})

	verifyEqual := func() {
		appHash1, err := ComputeAppRevisionHash(&appRevision1)
		Expect(err).Should(Succeed())
		appHash2, err := ComputeAppRevisionHash(&appRevision2)
		Expect(err).Should(Succeed())
		Expect(appHash1).Should(Equal(appHash2))
		// and compare
		Expect(DeepEqualRevision(&appRevision1, &appRevision2)).Should(BeTrue())
	}

	verifyNotEqual := func() {
		appHash1, err := ComputeAppRevisionHash(&appRevision1)
		Expect(err).Should(Succeed())
		appHash2, err := ComputeAppRevisionHash(&appRevision2)
		Expect(err).Should(Succeed())
		Expect(appHash1).ShouldNot(Equal(appHash2))
		Expect(DeepEqualRevision(&appRevision1, &appRevision2)).ShouldNot(BeTrue())
	}

	It("Test same app revisions should produce same hash and equal", func() {
		verifyEqual()
	})

	It("Test app revisions with same spec should produce same hash and equal regardless of other fields", func() {
		// add an annotation to workload Definition
		wd.SetAnnotations(map[string]string{oam.AnnotationAppRollout: "true"})
		appRevision2.Spec.WorkloadDefinitions[wd.Name] = wd
		// add status to td
		td.SetConditions(v1alpha1.NewPositiveCondition("Test"))
		appRevision2.Spec.TraitDefinitions[td.Name] = td
		// change the cd meta
		cd.ClusterName = "testCluster"
		appRevision2.Spec.ComponentDefinitions[cd.Name] = cd

		verifyEqual()
	})

	It("Test app revisions with different trait spec should produce different hash and not equal", func() {
		// change td spec
		td.Spec.AppliesToWorkloads = append(td.Spec.AppliesToWorkloads, "allWorkload")
		appRevision2.Spec.TraitDefinitions[td.Name] = td

		verifyNotEqual()
	})

	It("Test app revisions with different application spec should produce different hash and not equal", func() {
		// change application setting
		appRevision2.Spec.Application.Spec.Components[0].Properties.Raw =
			[]byte(`{"image": "oamdev/testapp:v2", "cmd": ["node", "server.js"]}`)

		verifyNotEqual()
	})

	It("Test app revisions with different application spec should produce different hash and not equal", func() {
		// add a component definition
		appRevision1.Spec.ComponentDefinitions[webCompDef.Name] = webCompDef

		verifyNotEqual()
	})

	It("Test apply success for none rollout case", func() {
		By("Apply the application")
		appParser := appfile.NewApplicationParser(reconciler.Client, reconciler.dm, reconciler.pd)
		ctx = util.SetNamespaceInCtx(ctx, app.Namespace)
		annoKey1 := "testKey1"
		app.SetAnnotations(map[string]string{annoKey1: "true"})
		generatedAppfile, err := appParser.GenerateAppFile(ctx, &app)
		Expect(err).Should(Succeed())
		ac, comps, err = generatedAppfile.GenerateApplicationConfiguration()
		Expect(err).Should(Succeed())
		handler.appfile = generatedAppfile
		Expect(ac.Namespace).Should(Equal(app.Namespace))
		appRev, err := handler.GenerateAppRevision(ctx)
		Expect(err).Should(Succeed())
		Expect(handler.apply(context.Background(), appRev, ac, comps)).Should(Succeed())

		curApp := &v1beta1.Application{}
		Eventually(
			func() error {
				return handler.r.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: app.Name},
					curApp)
			},
			time.Second*10, time.Millisecond*500).Should(BeNil())
		Expect(curApp.Status.LatestRevision.Revision).Should(BeEquivalentTo(1))
		By("Verify the created appRevision is exactly what it is")
		curAppRevision := &v1beta1.ApplicationRevision{}
		Eventually(
			func() error {
				return handler.r.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: curApp.Status.LatestRevision.Name},
					curAppRevision)
			},
			time.Second*5, time.Millisecond*500).Should(BeNil())
		appHash1, err := ComputeAppRevisionHash(curAppRevision)
		Expect(err).Should(Succeed())
		Expect(curAppRevision.GetLabels()[oam.LabelAppRevisionHash]).Should(Equal(appHash1))
		Expect(appHash1).Should(Equal(curApp.Status.LatestRevision.RevisionHash))
		Expect(metav1.GetControllerOf(curAppRevision)).Should(BeNil())
		Expect(len(curAppRevision.GetOwnerReferences())).Should(BeEquivalentTo(1))
		Expect(curAppRevision.GetOwnerReferences()[0].Kind).Should(Equal(v1alpha2.ApplicationKind))

		By("Verify that an application context is created to point to the correct appRevision")
		curAC := &v1alpha2.ApplicationContext{}
		Expect(handler.r.Get(ctx,
			types.NamespacedName{Namespace: ns.Name, Name: app.Name},
			curAC)).NotTo(HaveOccurred())
		Expect(curAC.GetLabels()[oam.LabelAppRevisionHash]).Should(Equal(appHash1))
		Expect(curAC.Spec.ApplicationRevisionName).Should(Equal(curApp.Status.LatestRevision.Name))
		Expect(curAC.GetAnnotations()[annoKey1]).ShouldNot(BeEmpty())
		Expect(curAC.GetAnnotations()[oam.AnnotationInplaceUpgrade]).Should(Equal("true"))

		Expect(metav1.GetControllerOf(curAC)).ShouldNot(BeNil())
		Expect(metav1.GetControllerOf(curAC).Kind).Should(Equal(v1alpha2.ApplicationKind))
		By("Apply the application again without any spec change")
		// there can be annotation change and appContext should have the exact label/annotation as app
		annoKey2 := "testKey2"
		app.SetAnnotations(map[string]string{annoKey2: "true"})
		lastRevision := curApp.Status.LatestRevision.Name
		appRev, err = handler.GenerateAppRevision(ctx)
		Expect(err).Should(Succeed())
		Expect(handler.apply(context.Background(), appRev, ac, comps)).Should(Succeed())
		Eventually(
			func() error {
				return handler.r.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: app.Name},
					curApp)
			},
			time.Second*10, time.Millisecond*500).Should(BeNil())
		// no new revision should be created
		Expect(curApp.Status.LatestRevision.Name).Should(Equal(lastRevision))
		Expect(curApp.Status.LatestRevision.RevisionHash).Should(Equal(appHash1))
		By("Verify the appRevision is not changed")
		// reset appRev
		curAppRevision = &v1beta1.ApplicationRevision{}
		Eventually(
			func() error {
				return handler.r.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: lastRevision},
					curAppRevision)
			},
			time.Second*5, time.Millisecond*500).Should(BeNil())
		Expect(err).Should(Succeed())
		Expect(curAppRevision.GetLabels()[oam.LabelAppRevisionHash]).Should(Equal(appHash1))
		By("Verify that an application context is created to point to the same appRevision")
		curAC = &v1alpha2.ApplicationContext{}
		Expect(handler.r.Get(ctx,
			types.NamespacedName{Namespace: ns.Name, Name: app.Name},
			curAC)).NotTo(HaveOccurred())
		Expect(curAC.GetLabels()[oam.LabelAppRevisionHash]).Should(Equal(appHash1))
		Expect(curAC.Spec.ApplicationRevisionName).Should(Equal(lastRevision))
		Expect(curAC.GetAnnotations()[annoKey1]).Should(BeEmpty())
		Expect(curAC.GetAnnotations()[annoKey2]).ShouldNot(BeEmpty())

		By("Change the application and apply again")
		// bump the image tag
		app.ResourceVersion = curApp.ResourceVersion
		app.Spec.Components[0].Properties = runtime.RawExtension{
			Raw: []byte(`{"image": "oamdev/testapp:v2", "cmd": ["node", "server.js"]}`),
		}
		// persist the app
		Expect(k8sClient.Update(ctx, &app)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		generatedAppfile, err = appParser.GenerateAppFile(ctx, &app)
		Expect(err).Should(Succeed())
		ac, comps, err = generatedAppfile.GenerateApplicationConfiguration()
		Expect(err).Should(Succeed())
		handler.appfile = generatedAppfile
		handler.app = &app
		appRev, err = handler.GenerateAppRevision(ctx)
		Expect(err).Should(Succeed())
		Expect(handler.apply(context.Background(), appRev, ac, comps)).Should(Succeed())
		Eventually(
			func() error {
				return handler.r.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: app.Name},
					curApp)
			},
			time.Second*10, time.Millisecond*500).Should(BeNil())
		// new revision should be created
		Expect(curApp.Status.LatestRevision.Name).ShouldNot(Equal(lastRevision))
		Expect(curApp.Status.LatestRevision.Revision).Should(BeEquivalentTo(2))
		Expect(curApp.Status.LatestRevision.RevisionHash).ShouldNot(Equal(appHash1))
		By("Verify the appRevision is changed")
		// reset appRev
		curAppRevision = &v1beta1.ApplicationRevision{}
		Eventually(
			func() error {
				return handler.r.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: curApp.Status.LatestRevision.Name},
					curAppRevision)
			},
			time.Second*5, time.Millisecond*500).Should(BeNil())
		appHash2, err := ComputeAppRevisionHash(curAppRevision)
		Expect(err).Should(Succeed())
		Expect(appHash1).ShouldNot(Equal(appHash2))
		Expect(curAppRevision.GetLabels()[oam.LabelAppRevisionHash]).Should(Equal(appHash2))
		Expect(curApp.Status.LatestRevision.RevisionHash).Should(Equal(appHash2))
		By("Verify that an application context is created to point to the right appRevision")
		curAC = &v1alpha2.ApplicationContext{}
		Expect(handler.r.Get(ctx,
			types.NamespacedName{Namespace: ns.Name, Name: app.Name},
			curAC)).NotTo(HaveOccurred())
		Expect(curAC.GetLabels()[oam.LabelAppRevisionHash]).Should(Equal(appHash2))
		Expect(curAC.Spec.ApplicationRevisionName).Should(Equal(curApp.Status.LatestRevision.Name))
		Expect(curAC.GetAnnotations()[annoKey2]).ShouldNot(BeEmpty())
		Expect(metav1.GetControllerOf(curAC).Kind).Should(Equal(v1alpha2.ApplicationKind))
	})

	It("Test App with rollout template", func() {
		By("Apply the application")
		appParser := appfile.NewApplicationParser(reconciler.Client, reconciler.dm, reconciler.pd)
		ctx = util.SetNamespaceInCtx(ctx, app.Namespace)
		// mark the app as rollout
		app.SetAnnotations(map[string]string{oam.AnnotationAppRollout: strconv.FormatBool(true)})
		generatedAppfile, err := appParser.GenerateAppFile(ctx, &app)
		Expect(err).Should(Succeed())
		ac, comps, err = generatedAppfile.GenerateApplicationConfiguration()
		Expect(err).Should(Succeed())
		handler.appfile = generatedAppfile
		Expect(ac.Namespace).Should(Equal(app.Namespace))
		appRev, err := handler.GenerateAppRevision(ctx)
		Expect(err).Should(Succeed())
		Expect(handler.apply(context.Background(), appRev, ac, comps)).Should(Succeed())
		curApp := &v1beta1.Application{}
		Eventually(
			func() error {
				return handler.r.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: app.Name},
					curApp)
			},
			time.Second*10, time.Millisecond*500).Should(BeNil())
		Expect(curApp.Status.LatestRevision.Revision).Should(BeEquivalentTo(1))
		By("Verify the created appRevision is exactly what it is")
		curAppRevision := &v1beta1.ApplicationRevision{}
		Eventually(
			func() error {
				return handler.r.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: curApp.Status.LatestRevision.Name},
					curAppRevision)
			},
			time.Second*5, time.Millisecond*500).Should(BeNil())
		appHash1, err := ComputeAppRevisionHash(curAppRevision)
		Expect(err).Should(Succeed())
		Expect(curAppRevision.GetLabels()[oam.LabelAppRevisionHash]).Should(Equal(appHash1))
		Expect(appHash1).Should(Equal(curApp.Status.LatestRevision.RevisionHash))
		Expect(metav1.GetControllerOf(curAppRevision)).Should(BeNil())
		Expect(len(curAppRevision.GetOwnerReferences())).Should(BeEquivalentTo(1))
		Expect(curAppRevision.GetOwnerReferences()[0].Kind).Should(Equal(v1alpha2.ApplicationKind))

		By("Verify that no application context is created")
		curACs := &v1alpha2.ApplicationContextList{}
		opts := []client.ListOption{
			client.InNamespace(namespaceName),
		}
		Eventually(
			func() error {
				return handler.r.List(ctx, curACs, opts...)
			}, time.Second*5, time.Microsecond*500).Should(Succeed())
		Expect(len(curACs.Items)).Should(BeEquivalentTo(0))

		By("Apply the application again without any spec change but remove the rollout annotation")
		// there can be annotation change and appContext should have the exact label/annotation as app
		annoKey2 := "testKey2"
		app.SetAnnotations(map[string]string{annoKey2: "true"})
		lastRevision := curApp.Status.LatestRevision.Name
		appRev, err = handler.GenerateAppRevision(ctx)
		Expect(err).Should(Succeed())
		Expect(handler.apply(context.Background(), appRev, ac, comps)).Should(Succeed())
		Eventually(
			func() error {
				return handler.r.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: app.Name},
					curApp)
			},
			time.Second*10, time.Millisecond*500).Should(BeNil())
		// no new revision should be created
		Expect(curApp.Status.LatestRevision.Name).Should(Equal(lastRevision))
		Expect(curApp.Status.LatestRevision.RevisionHash).Should(Equal(appHash1))
		By("Verify the appRevision is not changed")
		// reset appRev
		curAppRevision = &v1beta1.ApplicationRevision{}
		Eventually(
			func() error {
				return handler.r.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: lastRevision},
					curAppRevision)
			},
			time.Second*5, time.Millisecond*500).Should(BeNil())
		Expect(err).Should(Succeed())
		Expect(curAppRevision.GetLabels()[oam.LabelAppRevisionHash]).Should(Equal(appHash1))
		Expect(curAppRevision.GetAnnotations()[annoKey2]).ShouldNot(BeEmpty())
		By("Verify that an application context is created to point to the same appRevision")
		curAC := &v1alpha2.ApplicationContext{}
		Expect(handler.r.Get(ctx,
			types.NamespacedName{Namespace: ns.Name, Name: app.Name},
			curAC)).NotTo(HaveOccurred())
		Expect(curAC.GetLabels()[oam.LabelAppRevisionHash]).Should(Equal(appHash1))
		Expect(curAC.Spec.ApplicationRevisionName).Should(Equal(lastRevision))
		Expect(curAC.GetAnnotations()[annoKey2]).ShouldNot(BeEmpty())

		By("Change the application and apply again with rollout")
		// bump the image tag
		app.SetAnnotations(map[string]string{oam.AnnotationAppRollout: strconv.FormatBool(true)})
		app.ResourceVersion = curApp.ResourceVersion
		app.Spec.Components[0].Properties = runtime.RawExtension{
			Raw: []byte(`{"image": "oamdev/testapp:v2", "cmd": ["node", "server.js"]}`),
		}
		// persist the app
		Expect(k8sClient.Update(ctx, &app)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		generatedAppfile, err = appParser.GenerateAppFile(ctx, &app)
		Expect(err).Should(Succeed())
		ac, comps, err = generatedAppfile.GenerateApplicationConfiguration()
		Expect(err).Should(Succeed())
		handler.appfile = generatedAppfile
		handler.app = &app
		appRev, err = handler.GenerateAppRevision(ctx)
		Expect(err).Should(Succeed())
		Expect(handler.apply(context.Background(), appRev, ac, comps)).Should(Succeed())
		Eventually(
			func() error {
				return handler.r.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: app.Name},
					curApp)
			},
			time.Second*10, time.Millisecond*500).Should(BeNil())
		// new revision should be created
		Expect(curApp.Status.LatestRevision.Name).ShouldNot(Equal(lastRevision))
		Expect(curApp.Status.LatestRevision.Revision).Should(BeEquivalentTo(2))
		Expect(curApp.Status.LatestRevision.RevisionHash).ShouldNot(Equal(appHash1))
		By("Verify the appRevision is changed")
		// reset appRev
		curAppRevision = &v1beta1.ApplicationRevision{}
		Eventually(
			func() error {
				return handler.r.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: curApp.Status.LatestRevision.Name},
					curAppRevision)
			},
			time.Second*5, time.Millisecond*500).Should(BeNil())
		appHash2, err := ComputeAppRevisionHash(curAppRevision)
		Expect(err).Should(Succeed())
		Expect(appHash1).ShouldNot(Equal(appHash2))
		Expect(curAppRevision.GetLabels()[oam.LabelAppRevisionHash]).Should(Equal(appHash2))
		Expect(curApp.Status.LatestRevision.RevisionHash).Should(Equal(appHash2))
		Expect(curAppRevision.GetAnnotations()[annoKey2]).Should(BeEmpty())
		Expect(curAppRevision.GetAnnotations()[oam.AnnotationAppRollout]).ShouldNot(BeEmpty())
		By("Verify that no more application context is created")
		Eventually(
			func() error {
				return handler.r.List(ctx, curACs, opts...)
			}, time.Second*5, time.Microsecond*500).Should(Succeed())
		Expect(len(curACs.Items)).Should(BeEquivalentTo(1))
	})

	It("Test apply passes all label and annotation from app to appRevision", func() {
		By("Apply the application")
		appParser := appfile.NewApplicationParser(reconciler.Client, reconciler.dm, reconciler.pd)
		ctx = util.SetNamespaceInCtx(ctx, app.Namespace)
		labelKey1 := "labelKey1"
		app.SetLabels(map[string]string{labelKey1: "true"})
		annoKey1 := "annoKey1"
		app.SetAnnotations(map[string]string{annoKey1: "true"})
		generatedAppfile, err := appParser.GenerateAppFile(ctx, &app)
		Expect(err).Should(Succeed())
		ac, comps, err = generatedAppfile.GenerateApplicationConfiguration()
		Expect(err).Should(Succeed())
		handler.appfile = generatedAppfile
		Expect(ac.Namespace).Should(Equal(app.Namespace))
		appRev, err := handler.GenerateAppRevision(ctx)
		Expect(err).Should(Succeed())
		Expect(handler.apply(context.Background(), appRev, ac, comps)).Should(Succeed())

		curApp := &v1beta1.Application{}
		Eventually(
			func() error {
				return handler.r.Get(ctx, types.NamespacedName{Namespace: ns.Name, Name: app.Name}, curApp)
			}, time.Second*10, time.Millisecond*500).Should(BeNil())
		Expect(curApp.Status.LatestRevision.Revision).Should(BeEquivalentTo(1))
		By("Verify the created appRevision is exactly what it is")
		curAppRevision := &v1beta1.ApplicationRevision{}
		Eventually(
			func() error {
				return handler.r.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: curApp.Status.LatestRevision.Name},
					curAppRevision)
			},
			time.Second*5, time.Millisecond*500).Should(BeNil())
		appHash1, err := ComputeAppRevisionHash(curAppRevision)
		Expect(err).Should(Succeed())
		Expect(curAppRevision.GetLabels()[oam.LabelAppRevisionHash]).Should(Equal(appHash1))
		Expect(appHash1).Should(Equal(curApp.Status.LatestRevision.RevisionHash))
		Expect(curAppRevision.GetLabels()[labelKey1]).Should(Equal("true"))
		Expect(curAppRevision.GetAnnotations()[annoKey1]).Should(Equal("true"))
		// there can be annotation change and appContext should have the exact label/annotation as app
		annoKey2 := "testKey2"
		app.SetAnnotations(map[string]string{annoKey2: "true"})
		labelKey2 := "labelKey2"
		app.SetLabels(map[string]string{labelKey2: "true"})
		lastRevision := curApp.Status.LatestRevision.Name
		appRev, err = handler.GenerateAppRevision(ctx)
		Expect(err).Should(Succeed())
		Expect(handler.apply(context.Background(), appRev, ac, comps)).Should(Succeed())
		Eventually(
			func() error {
				return handler.r.Get(ctx, types.NamespacedName{Namespace: ns.Name, Name: app.Name}, curApp)
			}, time.Second*10, time.Millisecond*500).Should(BeNil())
		// no new revision should be created
		Expect(curApp.Status.LatestRevision.Name).Should(Equal(lastRevision))
		Expect(curApp.Status.LatestRevision.RevisionHash).Should(Equal(appHash1))
		By("Verify the appRevision is not changed")
		// reset appRev
		curAppRevision = &v1beta1.ApplicationRevision{}
		Eventually(
			func() error {
				return handler.r.Get(ctx, types.NamespacedName{Namespace: ns.Name, Name: lastRevision}, curAppRevision)
			}, time.Second*5, time.Millisecond*500).Should(BeNil())
		Expect(err).Should(Succeed())
		Expect(curAppRevision.GetLabels()[oam.LabelAppRevisionHash]).Should(Equal(appHash1))
		Expect(curAppRevision.GetLabels()[labelKey1]).Should(BeEmpty())
		Expect(curAppRevision.GetLabels()[labelKey2]).Should(Equal("true"))
		Expect(curAppRevision.GetAnnotations()[annoKey1]).Should(BeEmpty())
		Expect(curAppRevision.GetAnnotations()[annoKey2]).Should(Equal("true"))
	})
})
