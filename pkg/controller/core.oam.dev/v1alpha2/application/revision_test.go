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
	"reflect"
	"strconv"
	"testing"
	"time"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/require"

	"github.com/google/go-cmp/cmp"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	oamtypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("test generate revision ", func() {
	var appRevision1, appRevision2 v1beta1.ApplicationRevision
	var app v1beta1.Application
	cd := v1beta1.ComponentDefinition{}
	webCompDef := v1beta1.ComponentDefinition{}
	wd := v1beta1.WorkloadDefinition{}
	sd := v1beta1.ScopeDefinition{}
	rolloutTd := v1beta1.TraitDefinition{}
	var handler *AppHandler
	var comps []*oamtypes.ComponentManifest
	var namespaceName string
	var ns corev1.Namespace
	ctx := context.Background()

	BeforeEach(func() {
		namespaceName = randomNamespaceName("apply-app-test")
		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespaceName,
			},
		}

		componentDefJson, _ := yaml.YAMLToJSON([]byte(componentDefYaml))
		Expect(json.Unmarshal(componentDefJson, &cd)).Should(BeNil())
		cd.ResourceVersion = ""
		Expect(k8sClient.Create(ctx, &cd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

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

		rolloutDefJson, _ := yaml.YAMLToJSON([]byte(rolloutTraitDefinition))
		Expect(json.Unmarshal(rolloutDefJson, &rolloutTd)).Should(BeNil())
		rolloutTd.ResourceVersion = ""
		Expect(k8sClient.Create(ctx, &rolloutTd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

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
				Components: []common.ApplicationComponent{
					{
						Type:   cd.Name,
						Name:   "express-server",
						Scopes: map[string]string{"healthscopes.core.oam.dev": "myapp-default-health"},
						Properties: &runtime.RawExtension{
							Raw: []byte(`{"image": "oamdev/testapp:v1", "cmd": ["node", "server.js"]}`),
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
				ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
					ComponentDefinitions: make(map[string]*v1beta1.ComponentDefinition),
					WorkloadDefinitions:  make(map[string]v1beta1.WorkloadDefinition),
					TraitDefinitions:     make(map[string]*v1beta1.TraitDefinition),
					ScopeDefinitions:     make(map[string]v1beta1.ScopeDefinition),
				},
			},
		}
		appRevision1.Spec.Application = app
		appRevision1.Spec.ComponentDefinitions[cd.Name] = cd.DeepCopy()
		appRevision1.Spec.WorkloadDefinitions[wd.Name] = wd
		appRevision1.Spec.TraitDefinitions[rolloutTd.Name] = rolloutTd.DeepCopy()
		appRevision1.Spec.ScopeDefinitions[sd.Name] = sd

		appRevision2 = *appRevision1.DeepCopy()
		appRevision2.Name = "appRevision2"

		_handler, err := NewAppHandler(ctx, reconciler, &app, nil)
		Expect(err).Should(Succeed())
		handler = _handler
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
		appRevision2.Spec.ComponentDefinitions[cd.Name] = cd.DeepCopy()

		verifyEqual()
	})

	It("Test app revisions with different application spec should produce different hash and not equal", func() {
		// change application setting
		appRevision2.Spec.Application.Spec.Components[0].Properties.Raw =
			[]byte(`{"image": "oamdev/testapp:v2", "cmd": ["node", "server.js"]}`)

		verifyNotEqual()
	})

	It("Test app revisions with different application spec should produce different hash and not equal", func() {
		// add a component definition
		appRevision1.Spec.ComponentDefinitions[webCompDef.Name] = webCompDef.DeepCopy()

		verifyNotEqual()
	})

	It("Test application revision compare", func() {
		By("Apply the application")
		appParser := appfile.NewApplicationParser(reconciler.Client, reconciler.dm, reconciler.pd)
		ctx = util.SetNamespaceInCtx(ctx, app.Namespace)
		generatedAppfile, err := appParser.GenerateAppFile(ctx, &app)
		Expect(err).Should(Succeed())
		comps, err = generatedAppfile.GenerateComponentManifests()
		Expect(err).Should(Succeed())
		Expect(handler.PrepareCurrentAppRevision(ctx, generatedAppfile)).Should(Succeed())
		Expect(handler.FinalizeAndApplyAppRevision(ctx)).Should(Succeed())
		prevHash := generatedAppfile.AppRevisionHash
		handler.app.Status.LatestRevision = &common.Revision{Name: generatedAppfile.AppRevisionName, Revision: 1, RevisionHash: generatedAppfile.AppRevisionHash}
		generatedAppfile.Workloads[0].FullTemplate.ComponentDefinition = nil
		generatedAppfile.RelatedComponentDefinitions = map[string]*v1beta1.ComponentDefinition{}
		Expect(handler.PrepareCurrentAppRevision(ctx, generatedAppfile)).Should(Succeed())
		nonChangeHash := generatedAppfile.AppRevisionHash
		handler.app.Annotations = map[string]string{oam.AnnotationAutoUpdate: "true"}
		Expect(handler.PrepareCurrentAppRevision(ctx, generatedAppfile)).Should(Succeed())
		changedHash := generatedAppfile.AppRevisionHash
		Expect(nonChangeHash).Should(Equal(prevHash))
		Expect(changedHash).ShouldNot(Equal(prevHash))
	})

	It("Test apply success for none rollout case", func() {
		By("Apply the application")
		appParser := appfile.NewApplicationParser(reconciler.Client, reconciler.dm, reconciler.pd)
		ctx = util.SetNamespaceInCtx(ctx, app.Namespace)
		annoKey1 := "testKey1"
		app.SetAnnotations(map[string]string{annoKey1: "true"})
		generatedAppfile, err := appParser.GenerateAppFile(ctx, &app)
		Expect(err).Should(Succeed())
		comps, err = generatedAppfile.GenerateComponentManifests()
		Expect(err).Should(Succeed())
		Expect(handler.PrepareCurrentAppRevision(ctx, generatedAppfile)).Should(Succeed())
		Expect(handler.HandleComponentsRevision(ctx, comps)).Should(Succeed())
		Expect(handler.FinalizeAndApplyAppRevision(ctx)).Should(Succeed())
		Expect(handler.ProduceArtifacts(context.Background(), comps, nil)).Should(Succeed())
		Expect(handler.UpdateAppLatestRevisionStatus(ctx)).Should(Succeed())

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
		ctrlOwner := metav1.GetControllerOf(curAppRevision)
		Expect(ctrlOwner).ShouldNot(BeNil())
		Expect(ctrlOwner.Kind).Should(Equal(v1beta1.ApplicationKind))
		Expect(len(curAppRevision.GetOwnerReferences())).Should(BeEquivalentTo(1))
		Expect(curAppRevision.GetOwnerReferences()[0].Kind).Should(Equal(v1alpha2.ApplicationKind))

		By("Verify component revision")
		expectCompRevName := "express-server-v1"
		Expect(comps[0].RevisionName).Should(Equal(expectCompRevName))
		gotCR := &appsv1.ControllerRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: expectCompRevName, Namespace: namespaceName}, gotCR)).Should(Succeed())
		Expect(gotCR.Revision).Should(Equal(int64(1)))
		gotComp, err := util.RawExtension2Component(gotCR.Data)
		Expect(err).Should(BeNil())
		expectWorkload := comps[0].StandardWorkload.DeepCopy()
		util.RemoveLabels(expectWorkload, []string{oam.LabelAppRevision, oam.LabelAppRevisionHash, oam.LabelAppComponentRevision})
		var gotWL = unstructured.Unstructured{}
		err = json.Unmarshal(gotComp.Spec.Workload.Raw, &gotWL)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(&gotWL, expectWorkload)).Should(BeEmpty())

		By("Apply the application again without any spec change")
		annoKey2 := "testKey2"
		app.SetAnnotations(map[string]string{annoKey2: "true"})
		lastRevision := curApp.Status.LatestRevision.Name
		comps, err = generatedAppfile.GenerateComponentManifests()
		Expect(err).Should(Succeed())
		Expect(handler.PrepareCurrentAppRevision(ctx, generatedAppfile)).Should(Succeed())
		Expect(handler.HandleComponentsRevision(ctx, comps)).Should(Succeed())
		Expect(handler.FinalizeAndApplyAppRevision(ctx)).Should(Succeed())
		Expect(handler.ProduceArtifacts(context.Background(), comps, nil)).Should(Succeed())
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
		gotComp, err = util.RawExtension2Component(gotCR.Data)
		Expect(err).Should(BeNil())
		expectWorkload = comps[0].StandardWorkload.DeepCopy()
		util.RemoveLabels(expectWorkload, []string{oam.LabelAppRevision, oam.LabelAppRevisionHash, oam.LabelAppComponentRevision})
		Expect(cmp.Diff(gotComp.Spec.Workload, *util.Object2RawExtension(expectWorkload))).Should(BeEmpty())

		By("Verify component revision is not changed")
		expectCompRevName = "express-server-v1"
		Expect(comps[0].RevisionName).Should(Equal(expectCompRevName))
		gotCR = &appsv1.ControllerRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: expectCompRevName, Namespace: namespaceName}, gotCR)).Should(Succeed())
		Expect(gotCR.Revision).Should(Equal(int64(1)))

		By("Change the application and apply again")
		// bump the image tag
		app.ResourceVersion = curApp.ResourceVersion
		app.Spec.Components[0].Properties = &runtime.RawExtension{
			Raw: []byte(`{"image": "oamdev/testapp:v2", "cmd": ["node", "server.js"]}`),
		}
		// persist the app
		Expect(k8sClient.Update(ctx, &app)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		generatedAppfile, err = appParser.GenerateAppFile(ctx, &app)
		Expect(err).Should(Succeed())
		comps, err = generatedAppfile.GenerateComponentManifests()
		Expect(err).Should(Succeed())
		handler.app = &app
		Expect(handler.PrepareCurrentAppRevision(ctx, generatedAppfile)).Should(Succeed())
		Expect(handler.HandleComponentsRevision(ctx, comps)).Should(Succeed())
		Expect(handler.FinalizeAndApplyAppRevision(ctx)).Should(Succeed())
		Expect(handler.ProduceArtifacts(context.Background(), comps, nil)).Should(Succeed())
		Expect(handler.UpdateAppLatestRevisionStatus(ctx)).Should(Succeed())
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

		By("Verify component revision is changed")
		expectCompRevName = "express-server-v2"
		Expect(comps[0].RevisionName).Should(Equal(expectCompRevName))
		gotCR = &appsv1.ControllerRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: expectCompRevName, Namespace: namespaceName}, gotCR)).Should(Succeed())
		Expect(gotCR.Revision).Should(Equal(int64(2)))
		gotComp, err = util.RawExtension2Component(gotCR.Data)
		Expect(err).Should(BeNil())
		expectWorkload = comps[0].StandardWorkload.DeepCopy()
		util.RemoveLabels(expectWorkload, []string{oam.LabelAppRevision, oam.LabelAppRevisionHash, oam.LabelAppComponentRevision})
		Expect(cmp.Diff(gotComp.Spec.Workload, *util.Object2RawExtension(expectWorkload))).Should(BeEmpty())

		By("Change the application same as v1 and apply again")
		// bump the image tag
		app.ResourceVersion = curApp.ResourceVersion
		app.Spec.Components[0].Properties = &runtime.RawExtension{
			Raw: []byte(`{"image": "oamdev/testapp:v1", "cmd": ["node", "server.js"]}`),
		}
		// persist the app
		Expect(k8sClient.Update(ctx, &app)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		generatedAppfile, err = appParser.GenerateAppFile(ctx, &app)
		Expect(err).Should(Succeed())
		comps, err = generatedAppfile.GenerateComponentManifests()
		Expect(err).Should(Succeed())
		handler.app = &app
		Expect(handler.PrepareCurrentAppRevision(ctx, generatedAppfile)).Should(Succeed())
		Expect(handler.HandleComponentsRevision(ctx, comps)).Should(Succeed())
		Expect(handler.FinalizeAndApplyAppRevision(ctx)).Should(Succeed())
		Expect(handler.ProduceArtifacts(context.Background(), comps, nil)).Should(Succeed())
		Expect(handler.UpdateAppLatestRevisionStatus(ctx)).Should(Succeed())
		Eventually(
			func() error {
				return handler.r.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: app.Name},
					curApp)
			},
			time.Second*10, time.Millisecond*500).Should(BeNil())
		// new revision should be different with lastRevision
		Expect(curApp.Status.LatestRevision.Name).Should(Equal(lastRevision))
		Expect(curApp.Status.LatestRevision.Revision).Should(BeEquivalentTo(1))
		Expect(curApp.Status.LatestRevision.RevisionHash).ShouldNot(Equal(appHash2))
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
		appHash3, err := ComputeAppRevisionHash(curAppRevision)
		Expect(err).Should(Succeed())
		Expect(appHash2).ShouldNot(Equal(appHash3))
		Expect(curAppRevision.GetLabels()[oam.LabelAppRevisionHash]).Should(Equal(appHash3))
		Expect(curApp.Status.LatestRevision.RevisionHash).Should(Equal(appHash3))

		By("Verify no new component revision (v3) is created")
		gotCR = &appsv1.ControllerRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "express-server-v3", Namespace: namespaceName}, gotCR)).Should(util.NotFoundMatcher{})

		By("Verify component revision is set back to v1")
		expectCompRevName = "express-server-v1"
		Expect(comps[0].RevisionName).Should(Equal(expectCompRevName))
		gotCR = &appsv1.ControllerRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: expectCompRevName, Namespace: namespaceName}, gotCR)).Should(Succeed())
		Expect(gotCR.Revision).Should(Equal(int64(1)))
		gotComp, err = util.RawExtension2Component(gotCR.Data)
		Expect(err).Should(BeNil())
		expectWorkload = comps[0].StandardWorkload.DeepCopy()
		util.RemoveLabels(expectWorkload, []string{oam.LabelAppRevision, oam.LabelAppRevisionHash, oam.LabelAppComponentRevision})
		expectWorkload.SetAnnotations(map[string]string{"testKey1": "true"})
		Expect(cmp.Diff(gotComp.Spec.Workload, *util.Object2RawExtension(expectWorkload))).Should(BeEmpty())
	})

	It("Test App with rollout template", func() {
		By("Apply the application")
		appParser := appfile.NewApplicationParser(reconciler.Client, reconciler.dm, reconciler.pd)
		ctx = util.SetNamespaceInCtx(ctx, app.Namespace)
		// mark the app as rollout
		app.SetAnnotations(map[string]string{oam.AnnotationAppRollout: strconv.FormatBool(true)})
		generatedAppfile, err := appParser.GenerateAppFile(ctx, &app)
		Expect(err).Should(Succeed())
		comps, err = generatedAppfile.GenerateComponentManifests()
		Expect(err).Should(Succeed())
		Expect(handler.PrepareCurrentAppRevision(ctx, generatedAppfile)).Should(Succeed())
		Expect(handler.FinalizeAndApplyAppRevision(ctx)).Should(Succeed())
		Expect(handler.ProduceArtifacts(context.Background(), comps, nil)).Should(Succeed())
		Expect(handler.UpdateAppLatestRevisionStatus(ctx)).Should(Succeed())
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
		ctrlOwner := metav1.GetControllerOf(curAppRevision)
		Expect(ctrlOwner).ShouldNot(BeNil())
		Expect(ctrlOwner.Kind).Should(Equal(v1beta1.ApplicationKind))
		Expect(len(curAppRevision.GetOwnerReferences())).Should(BeEquivalentTo(1))
		Expect(curAppRevision.GetOwnerReferences()[0].Kind).Should(Equal(v1alpha2.ApplicationKind))

		By("Apply the application again without any spec change but remove the rollout annotation")
		annoKey2 := "testKey2"
		app.SetAnnotations(map[string]string{annoKey2: "true"})
		lastRevision := curApp.Status.LatestRevision.Name
		Expect(handler.PrepareCurrentAppRevision(ctx, generatedAppfile)).Should(Succeed())
		Expect(handler.FinalizeAndApplyAppRevision(ctx)).Should(Succeed())
		Expect(handler.ProduceArtifacts(context.Background(), comps, nil)).Should(Succeed())
		Expect(handler.UpdateAppLatestRevisionStatus(ctx)).Should(Succeed())
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

		By("Change the application and apply again with rollout")
		// bump the image tag
		app.SetAnnotations(map[string]string{oam.AnnotationAppRollout: strconv.FormatBool(true)})
		app.ResourceVersion = curApp.ResourceVersion
		app.Spec.Components[0].Properties = &runtime.RawExtension{
			Raw: []byte(`{"image": "oamdev/testapp:v2", "cmd": ["node", "server.js"]}`),
		}
		// persist the app
		Expect(k8sClient.Update(ctx, &app)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		generatedAppfile, err = appParser.GenerateAppFile(ctx, &app)
		Expect(err).Should(Succeed())
		comps, err = generatedAppfile.GenerateComponentManifests()
		Expect(err).Should(Succeed())
		handler.app = &app
		Expect(handler.PrepareCurrentAppRevision(ctx, generatedAppfile)).Should(Succeed())
		Expect(handler.FinalizeAndApplyAppRevision(ctx)).Should(Succeed())
		Expect(handler.ProduceArtifacts(context.Background(), comps, nil)).Should(Succeed())
		Expect(handler.UpdateAppLatestRevisionStatus(ctx)).Should(Succeed())
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
		comps, err = generatedAppfile.GenerateComponentManifests()
		Expect(err).Should(Succeed())
		Expect(handler.PrepareCurrentAppRevision(ctx, generatedAppfile)).Should(Succeed())
		Expect(handler.FinalizeAndApplyAppRevision(ctx)).Should(Succeed())
		Expect(handler.ProduceArtifacts(context.Background(), comps, nil)).Should(Succeed())
		Expect(handler.UpdateAppLatestRevisionStatus(ctx)).Should(Succeed())

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
		annoKey2 := "testKey2"
		app.SetAnnotations(map[string]string{annoKey2: "true"})
		labelKey2 := "labelKey2"
		app.SetLabels(map[string]string{labelKey2: "true"})
		lastRevision := curApp.Status.LatestRevision.Name
		Expect(handler.PrepareCurrentAppRevision(ctx, generatedAppfile)).Should(Succeed())
		Expect(handler.FinalizeAndApplyAppRevision(ctx)).Should(Succeed())
		Expect(handler.ProduceArtifacts(context.Background(), comps, nil)).Should(Succeed())
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

	It("Test specified component revision name", func() {
		By("Specify component revision name but revision does not exist")
		externalRevisionName1 := "specified-revision-v1"
		app.Spec.Components[0].ExternalRevision = externalRevisionName1
		Expect(k8sClient.Update(ctx, &app)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		appParser := appfile.NewApplicationParser(reconciler.Client, reconciler.dm, reconciler.pd)
		ctx = util.SetNamespaceInCtx(ctx, app.Namespace)
		generatedAppfile, err := appParser.GenerateAppFile(ctx, &app)
		Expect(err).Should(Succeed())
		comps, err = generatedAppfile.GenerateComponentManifests()
		Expect(err).Should(Succeed())
		Expect(handler.PrepareCurrentAppRevision(ctx, generatedAppfile)).Should(Succeed())
		Expect(handler.HandleComponentsRevision(ctx, comps)).Should(Succeed())
		Expect(handler.FinalizeAndApplyAppRevision(ctx)).Should(Succeed())
		Expect(handler.ProduceArtifacts(context.Background(), comps, nil)).Should(Succeed())
		Expect(handler.UpdateAppLatestRevisionStatus(ctx)).Should(Succeed())

		curApp := &v1beta1.Application{}
		Eventually(
			func() error {
				return handler.r.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: app.Name},
					curApp)
			},
			time.Second*10, time.Millisecond*500).Should(BeNil())
		Expect(curApp.Status.LatestRevision.Revision).Should(BeEquivalentTo(1))
		Expect(comps[0].RevisionName).Should(Equal(externalRevisionName1))
		gotCR := &appsv1.ControllerRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: externalRevisionName1, Namespace: namespaceName}, gotCR)).Should(Succeed())
		Expect(gotCR.Revision).Should(Equal(int64(1)))
		gotComp, err := util.RawExtension2Component(gotCR.Data)
		Expect(err).Should(BeNil())
		expectWorkload := comps[0].StandardWorkload.DeepCopy()
		util.RemoveLabels(expectWorkload, []string{oam.LabelAppRevision, oam.LabelAppRevisionHash, oam.LabelAppComponentRevision})
		var gotWL = unstructured.Unstructured{}
		err = json.Unmarshal(gotComp.Spec.Workload.Raw, &gotWL)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(&gotWL, expectWorkload)).Should(BeEmpty())

		By("Specify component revision name and revision already exist")
		externalRevisionName2 := "specified-revision-v2"
		newCR := gotCR.DeepCopy()
		newCR.Name = externalRevisionName2
		newCR.ResourceVersion = ""
		Expect(k8sClient.Create(ctx, newCR)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		app.Spec.Components[0].ExternalRevision = externalRevisionName2
		Expect(k8sClient.Update(ctx, &app)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		generatedAppfile, err = appParser.GenerateAppFile(ctx, &app)
		Expect(err).Should(Succeed())
		comps, err = generatedAppfile.GenerateComponentManifests()
		Expect(err).Should(Succeed())
		Expect(handler.PrepareCurrentAppRevision(ctx, generatedAppfile)).Should(Succeed())
		Expect(handler.HandleComponentsRevision(ctx, comps)).Should(Succeed())
		Expect(handler.FinalizeAndApplyAppRevision(ctx)).Should(Succeed())
		Expect(handler.ProduceArtifacts(context.Background(), comps, nil)).Should(Succeed())
		Expect(handler.UpdateAppLatestRevisionStatus(ctx)).Should(Succeed())

		Expect(comps[0].RevisionName).Should(Equal(externalRevisionName2))
		Expect(comps[0].RevisionHash).Should(Equal(gotCR.Labels[oam.LabelComponentRevisionHash]))
	})
})

var _ = Describe("Test ReplaceComponentRevisionContext func", func() {
	It("Test replace", func() {
		rollout := v1alpha1.Rollout{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1alpha1",
				Kind:       "Rollout",
			},
			Spec: v1alpha1.RolloutSpec{
				TargetRevisionName: process.ComponentRevisionPlaceHolder,
			},
		}
		u, err := util.Object2Unstructured(rollout)
		Expect(err).Should(BeNil())
		err = replaceComponentRevisionContext(u, "comp-rev1")
		Expect(err).Should(BeNil())
		jsRes, err := u.MarshalJSON()
		Expect(err).Should(BeNil())
		err = json.Unmarshal(jsRes, &rollout)
		Expect(err).Should(BeNil())
		Expect(rollout.Spec.TargetRevisionName).Should(BeEquivalentTo("comp-rev1"))
	})

	It("Test replace return error", func() {
		rollout := v1alpha1.Rollout{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1alpha1",
				Kind:       "Rollout",
			},
			Spec: v1alpha1.RolloutSpec{
				TargetRevisionName: process.ComponentRevisionPlaceHolder,
			},
		}
		u, err := util.Object2Unstructured(rollout)
		Expect(err).Should(BeNil())
		By("test replace with a bad revision")
		err = replaceComponentRevisionContext(u, "comp-rev1-\\}")
		Expect(err).ShouldNot(BeNil())
	})
})

var _ = Describe("Test PrepareCurrentAppRevision", func() {
	var app v1beta1.Application
	var apprev v1beta1.ApplicationRevision
	ctx := context.Background()
	var handler *AppHandler

	BeforeEach(func() {
		// prepare ComponentDefinition
		var compd v1beta1.ComponentDefinition
		Expect(yaml.Unmarshal([]byte(componentDefYaml), &compd)).To(Succeed())
		Expect(k8sClient.Create(ctx, &compd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		// prepare WorkflowStepDefinition
		wsdYaml := `
apiVersion: core.oam.dev/v1beta1
kind: WorkflowStepDefinition
metadata:
  annotations:
    definition.oam.dev/description: Apply application for your workflow steps
  labels:
    custom.definition.oam.dev/ui-hidden: "true"
  name: apply-application
  namespace: vela-system
spec:
  schematic:
    cue:
      template: |
        import (
        	"vela/op"
        )

        // apply application
        output: op.#ApplyApplication & {}
`
		var wsd v1beta1.WorkflowStepDefinition
		Expect(yaml.Unmarshal([]byte(wsdYaml), &wsd)).To(Succeed())
		Expect(k8sClient.Create(ctx, &wsd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		// prepare application and application revision
		appYaml := `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: backport-1-2-test-demo
  namespace: default
spec:
  components:
  - name: backport-1-2-test-demo
    properties:
      image: nginx
    type: worker
  workflow:
    steps:
    - name: apply
      type: apply-application
status:
  latestRevision:
    name: backport-1-2-test-demo-v1
    revision: 1
    revisionHash: 38ddf4e721073703
`
		Expect(yaml.Unmarshal([]byte(appYaml), &app)).To(Succeed())
		Expect(k8sClient.Create(ctx, &app)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		// prepare application revision
		apprevYaml := `
apiVersion: core.oam.dev/v1beta1
kind: ApplicationRevision
metadata:
  name: backport-1-2-test-demo-v1
  namespace: default
  ownerReferences:
  - apiVersion: core.oam.dev/v1beta1
    controller: true
    kind: Application
    name: backport-1-2-test-demo
    uid: b69fab34-7058-412b-994d-1465a9421f06
spec:
  application:
    apiVersion: core.oam.dev/v1beta1
    kind: Application
    metadata:
      name: backport-1-2-test-demo
      namespace: default
    spec:
      components:
      - name: backport-1-2-test-demo
        properties:
          image: nginx
        type: worker
    status: {}
  componentDefinitions:
    webservice:
      apiVersion: core.oam.dev/v1beta1
      kind: ComponentDefinition
      metadata:
        annotations:
          definition.oam.dev/description: Describes long-running, scalable, containerized
            services that have a stable network endpoint to receive external network
            traffic from customers.
          meta.helm.sh/release-name: kubevela
          meta.helm.sh/release-namespace: vela-system
        labels:
          app.kubernetes.io/managed-by: Helm
        name: webservice
        namespace: vela-system
      spec:
        schematic:
          cue:
            template: "import (\n\t\"strconv\"\n)\n\nmountsArray: {\n\tpvc: *[\n\t\tfor
              v in parameter.volumeMounts.pvc {\n\t\t\t{\n\t\t\t\tmountPath: v.mountPath\n\t\t\t\tname:
              \     v.name\n\t\t\t}\n\t\t},\n\t] | []\n\n\tconfigMap: *[\n\t\t\tfor
              v in parameter.volumeMounts.configMap {\n\t\t\t{\n\t\t\t\tmountPath:
              v.mountPath\n\t\t\t\tname:      v.name\n\t\t\t}\n\t\t},\n\t] | []\n\n\tsecret:
              *[\n\t\tfor v in parameter.volumeMounts.secret {\n\t\t\t{\n\t\t\t\tmountPath:
              v.mountPath\n\t\t\t\tname:      v.name\n\t\t\t}\n\t\t},\n\t] | []\n\n\temptyDir:
              *[\n\t\t\tfor v in parameter.volumeMounts.emptyDir {\n\t\t\t{\n\t\t\t\tmountPath:
              v.mountPath\n\t\t\t\tname:      v.name\n\t\t\t}\n\t\t},\n\t] | []\n\n\thostPath:
              *[\n\t\t\tfor v in parameter.volumeMounts.hostPath {\n\t\t\t{\n\t\t\t\tmountPath:
              v.mountPath\n\t\t\t\tname:      v.name\n\t\t\t}\n\t\t},\n\t] | []\n}\nvolumesArray:
              {\n\tpvc: *[\n\t\tfor v in parameter.volumeMounts.pvc {\n\t\t\t{\n\t\t\t\tname:
              v.name\n\t\t\t\tpersistentVolumeClaim: claimName: v.claimName\n\t\t\t}\n\t\t},\n\t]
              | []\n\n\tconfigMap: *[\n\t\t\tfor v in parameter.volumeMounts.configMap
              {\n\t\t\t{\n\t\t\t\tname: v.name\n\t\t\t\tconfigMap: {\n\t\t\t\t\tdefaultMode:
              v.defaultMode\n\t\t\t\t\tname:        v.cmName\n\t\t\t\t\tif v.items
              != _|_ {\n\t\t\t\t\t\titems: v.items\n\t\t\t\t\t}\n\t\t\t\t}\n\t\t\t}\n\t\t},\n\t]
              | []\n\n\tsecret: *[\n\t\tfor v in parameter.volumeMounts.secret {\n\t\t\t{\n\t\t\t\tname:
              v.name\n\t\t\t\tsecret: {\n\t\t\t\t\tdefaultMode: v.defaultMode\n\t\t\t\t\tsecretName:
              \ v.secretName\n\t\t\t\t\tif v.items != _|_ {\n\t\t\t\t\t\titems: v.items\n\t\t\t\t\t}\n\t\t\t\t}\n\t\t\t}\n\t\t},\n\t]
              | []\n\n\temptyDir: *[\n\t\t\tfor v in parameter.volumeMounts.emptyDir
              {\n\t\t\t{\n\t\t\t\tname: v.name\n\t\t\t\temptyDir: medium: v.medium\n\t\t\t}\n\t\t},\n\t]
              | []\n\n\thostPath: *[\n\t\t\tfor v in parameter.volumeMounts.hostPath
              {\n\t\t\t{\n\t\t\t\tname: v.name\n\t\t\t\thostPath: path: v.path\n\t\t\t}\n\t\t},\n\t]
              | []\n}\noutput: {\n\tapiVersion: \"apps/v1\"\n\tkind:       \"Deployment\"\n\tspec:
              {\n\t\tselector: matchLabels: \"app.oam.dev/component\": context.name\n\n\t\ttemplate:
              {\n\t\t\tmetadata: {\n\t\t\t\tlabels: {\n\t\t\t\t\tif parameter.labels
              != _|_ {\n\t\t\t\t\t\tparameter.labels\n\t\t\t\t\t}\n\t\t\t\t\tif parameter.addRevisionLabel
              {\n\t\t\t\t\t\t\"app.oam.dev/revision\": context.revision\n\t\t\t\t\t}\n\t\t\t\t\t\"app.oam.dev/component\":
              context.name\n\t\t\t\t}\n\t\t\t\tif parameter.annotations != _|_ {\n\t\t\t\t\tannotations:
              parameter.annotations\n\t\t\t\t}\n\t\t\t}\n\n\t\t\tspec: {\n\t\t\t\tcontainers:
              [{\n\t\t\t\t\tname:  context.name\n\t\t\t\t\timage: parameter.image\n\t\t\t\t\tif
              parameter[\"port\"] != _|_ && parameter[\"ports\"] == _|_ {\n\t\t\t\t\t\tports:
              [{\n\t\t\t\t\t\t\tcontainerPort: parameter.port\n\t\t\t\t\t\t}]\n\t\t\t\t\t}\n\t\t\t\t\tif
              parameter[\"ports\"] != _|_ {\n\t\t\t\t\t\tports: [ for v in parameter.ports
              {\n\t\t\t\t\t\t\t{\n\t\t\t\t\t\t\t\tcontainerPort: v.port\n\t\t\t\t\t\t\t\tprotocol:
              \     v.protocol\n\t\t\t\t\t\t\t\tif v.name != _|_ {\n\t\t\t\t\t\t\t\t\tname:
              v.name\n\t\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\t\tif v.name == _|_ {\n\t\t\t\t\t\t\t\t\tname:
              \"port-\" + strconv.FormatInt(v.port, 10)\n\t\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\t}}]\n\t\t\t\t\t}\n\n\t\t\t\t\tif
              parameter[\"imagePullPolicy\"] != _|_ {\n\t\t\t\t\t\timagePullPolicy:
              parameter.imagePullPolicy\n\t\t\t\t\t}\n\n\t\t\t\t\tif parameter[\"cmd\"]
              != _|_ {\n\t\t\t\t\t\tcommand: parameter.cmd\n\t\t\t\t\t}\n\n\t\t\t\t\tif
              parameter[\"env\"] != _|_ {\n\t\t\t\t\t\tenv: parameter.env\n\t\t\t\t\t}\n\n\t\t\t\t\tif
              context[\"config\"] != _|_ {\n\t\t\t\t\t\tenv: context.config\n\t\t\t\t\t}\n\n\t\t\t\t\tif
              parameter[\"cpu\"] != _|_ {\n\t\t\t\t\t\tresources: {\n\t\t\t\t\t\t\tlimits:
              cpu:   parameter.cpu\n\t\t\t\t\t\t\trequests: cpu: parameter.cpu\n\t\t\t\t\t\t}\n\t\t\t\t\t}\n\n\t\t\t\t\tif
              parameter[\"memory\"] != _|_ {\n\t\t\t\t\t\tresources: {\n\t\t\t\t\t\t\tlimits:
              memory:   parameter.memory\n\t\t\t\t\t\t\trequests: memory: parameter.memory\n\t\t\t\t\t\t}\n\t\t\t\t\t}\n\n\t\t\t\t\tif
              parameter[\"volumes\"] != _|_ && parameter[\"volumeMounts\"] == _|_
              {\n\t\t\t\t\t\tvolumeMounts: [ for v in parameter.volumes {\n\t\t\t\t\t\t\t{\n\t\t\t\t\t\t\t\tmountPath:
              v.mountPath\n\t\t\t\t\t\t\t\tname:      v.name\n\t\t\t\t\t\t\t}}]\n\t\t\t\t\t}\n\n\t\t\t\t\tif
              parameter[\"volumeMounts\"] != _|_ {\n\t\t\t\t\t\tvolumeMounts: mountsArray.pvc
              + mountsArray.configMap + mountsArray.secret + mountsArray.emptyDir
              + mountsArray.hostPath\n\t\t\t\t\t}\n\n\t\t\t\t\tif parameter[\"livenessProbe\"]
              != _|_ {\n\t\t\t\t\t\tlivenessProbe: parameter.livenessProbe\n\t\t\t\t\t}\n\n\t\t\t\t\tif
              parameter[\"readinessProbe\"] != _|_ {\n\t\t\t\t\t\treadinessProbe:
              parameter.readinessProbe\n\t\t\t\t\t}\n\n\t\t\t\t}]\n\n\t\t\t\tif parameter[\"hostAliases\"]
              != _|_ {\n\t\t\t\t\t// +patchKey=ip\n\t\t\t\t\thostAliases: parameter.hostAliases\n\t\t\t\t}\n\n\t\t\t\tif
              parameter[\"imagePullSecrets\"] != _|_ {\n\t\t\t\t\timagePullSecrets:
              [ for v in parameter.imagePullSecrets {\n\t\t\t\t\t\tname: v\n\t\t\t\t\t},\n\t\t\t\t\t]\n\t\t\t\t}\n\n\t\t\t\tif
              parameter[\"volumes\"] != _|_ && parameter[\"volumeMounts\"] == _|_
              {\n\t\t\t\t\tvolumes: [ for v in parameter.volumes {\n\t\t\t\t\t\t{\n\t\t\t\t\t\t\tname:
              v.name\n\t\t\t\t\t\t\tif v.type == \"pvc\" {\n\t\t\t\t\t\t\t\tpersistentVolumeClaim:
              claimName: v.claimName\n\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\tif v.type ==
              \"configMap\" {\n\t\t\t\t\t\t\t\tconfigMap: {\n\t\t\t\t\t\t\t\t\tdefaultMode:
              v.defaultMode\n\t\t\t\t\t\t\t\t\tname:        v.cmName\n\t\t\t\t\t\t\t\t\tif
              v.items != _|_ {\n\t\t\t\t\t\t\t\t\t\titems: v.items\n\t\t\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\tif
              v.type == \"secret\" {\n\t\t\t\t\t\t\t\tsecret: {\n\t\t\t\t\t\t\t\t\tdefaultMode:
              v.defaultMode\n\t\t\t\t\t\t\t\t\tsecretName:  v.secretName\n\t\t\t\t\t\t\t\t\tif
              v.items != _|_ {\n\t\t\t\t\t\t\t\t\t\titems: v.items\n\t\t\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\tif
              v.type == \"emptyDir\" {\n\t\t\t\t\t\t\t\temptyDir: medium: v.medium\n\t\t\t\t\t\t\t}\n\t\t\t\t\t\t}\n\t\t\t\t\t}]\n\t\t\t\t}\n\n\t\t\t\tif
              parameter[\"volumeMounts\"] != _|_ {\n\t\t\t\t\tvolumes: volumesArray.pvc
              + volumesArray.configMap + volumesArray.secret + volumesArray.emptyDir
              + volumesArray.hostPath\n\t\t\t\t}\n\t\t\t}\n\t\t}\n\t}\n}\nexposePorts:
              [\n\tfor v in parameter.ports if v.expose == true {\n\t\tport:       v.port\n\t\ttargetPort:
              v.port\n\t\tif v.name != _|_ {\n\t\t\tname: v.name\n\t\t}\n\t\tif v.name
              == _|_ {\n\t\t\tname: \"port-\" + strconv.FormatInt(v.port, 10)\n\t\t}\n\t},\n]\noutputs:
              {\n\tif len(exposePorts) != 0 {\n\t\twebserviceExpose: {\n\t\t\tapiVersion:
              \"v1\"\n\t\t\tkind:       \"Service\"\n\t\t\tmetadata: name: context.name\n\t\t\tspec:
              {\n\t\t\t\tselector: \"app.oam.dev/component\": context.name\n\t\t\t\tports:
              exposePorts\n\t\t\t\ttype:  parameter.exposeType\n\t\t\t}\n\t\t}\n\t}\n}\nparameter:
              {\n\t// +usage=Specify the labels in the workload\n\tlabels?: [string]:
              string\n\n\t// +usage=Specify the annotations in the workload\n\tannotations?:
              [string]: string\n\n\t// +usage=Which image would you like to use for
              your service\n\t// +short=i\n\timage: string\n\n\t// +usage=Specify
              image pull policy for your service\n\timagePullPolicy?: \"Always\" |
              \"Never\" | \"IfNotPresent\"\n\n\t// +usage=Specify image pull secrets
              for your service\n\timagePullSecrets?: [...string]\n\n\t// +ignore\n\t//
              +usage=Deprecated field, please use ports instead\n\t// +short=p\n\tport?:
              int\n\n\t// +usage=Which ports do you want customer traffic sent to,
              defaults to 80\n\tports?: [...{\n\t\t// +usage=Number of port to expose
              on the pod's IP address\n\t\tport: int\n\t\t// +usage=Name of the port\n\t\tname?:
              string\n\t\t// +usage=Protocol for port. Must be UDP, TCP, or SCTP\n\t\tprotocol:
              *\"TCP\" | \"UDP\" | \"SCTP\"\n\t\t// +usage=Specify if the port should
              be exposed\n\t\texpose: *false | bool\n\t}]\n\n\t// +ignore\n\t// +usage=Specify
              what kind of Service you want. options: \"ClusterIP\", \"NodePort\",
              \"LoadBalancer\", \"ExternalName\"\n\texposeType: *\"ClusterIP\" | \"NodePort\"
              | \"LoadBalancer\" | \"ExternalName\"\n\n\t// +ignore\n\t// +usage=If
              addRevisionLabel is true, the revision label will be added to the underlying
              pods\n\taddRevisionLabel: *false | bool\n\n\t// +usage=Commands to run
              in the container\n\tcmd?: [...string]\n\n\t// +usage=Define arguments
              by using environment variables\n\tenv?: [...{\n\t\t// +usage=Environment
              variable name\n\t\tname: string\n\t\t// +usage=The value of the environment
              variable\n\t\tvalue?: string\n\t\t// +usage=Specifies a source the value
              of this var should come from\n\t\tvalueFrom?: {\n\t\t\t// +usage=Selects
              a key of a secret in the pod's namespace\n\t\t\tsecretKeyRef?: {\n\t\t\t\t//
              +usage=The name of the secret in the pod's namespace to select from\n\t\t\t\tname:
              string\n\t\t\t\t// +usage=The key of the secret to select from. Must
              be a valid secret key\n\t\t\t\tkey: string\n\t\t\t}\n\t\t\t// +usage=Selects
              a key of a config map in the pod's namespace\n\t\t\tconfigMapKeyRef?:
              {\n\t\t\t\t// +usage=The name of the config map in the pod's namespace
              to select from\n\t\t\t\tname: string\n\t\t\t\t// +usage=The key of the
              config map to select from. Must be a valid secret key\n\t\t\t\tkey:
              string\n\t\t\t}\n\t\t}\n\t}]\n\n\t// +usage=Number of CPU units for
              the service, like \n\tcpu?: string\n\n\t//
              +usage=Specifies the attributes of the memory resource required for
              the container.\n\tmemory?: string\n\n\tvolumeMounts?: {\n\t\t// +usage=Mount
              PVC type volume\n\t\tpvc?: [...{\n\t\t\tname:      string\n\t\t\tmountPath:
              string\n\t\t\t// +usage=The name of the PVC\n\t\t\tclaimName: string\n\t\t}]\n\t\t//
              +usage=Mount ConfigMap type volume\n\t\tconfigMap?: [...{\n\t\t\tname:
              \       string\n\t\t\tmountPath:   string\n\t\t\tdefaultMode: *420 |
              int\n\t\t\tcmName:      string\n\t\t\titems?: [...{\n\t\t\t\tkey:  string\n\t\t\t\tpath:
              string\n\t\t\t\tmode: *511 | int\n\t\t\t}]\n\t\t}]\n\t\t// +usage=Mount
              Secret type volume\n\t\tsecret?: [...{\n\t\t\tname:        string\n\t\t\tmountPath:
              \  string\n\t\t\tdefaultMode: *420 | int\n\t\t\tsecretName:  string\n\t\t\titems?:
              [...{\n\t\t\t\tkey:  string\n\t\t\t\tpath: string\n\t\t\t\tmode: *511
              | int\n\t\t\t}]\n\t\t}]\n\t\t// +usage=Mount EmptyDir type volume\n\t\temptyDir?:
              [...{\n\t\t\tname:      string\n\t\t\tmountPath: string\n\t\t\tmedium:
              \   *\"\" | \"Memory\"\n\t\t}]\n\t\t// +usage=Mount HostPath type volume\n\t\thostPath?:
              [...{\n\t\t\tname:      string\n\t\t\tmountPath: string\n\t\t\tpath:
              \     string\n\t\t}]\n\t}\n\n\t// +usage=Deprecated field, use volumeMounts
              instead.\n\tvolumes?: [...{\n\t\tname:      string\n\t\tmountPath: string\n\t\t//
              +usage=Specify volume type, options: \"pvc\",\"configMap\",\"secret\",\"emptyDir\"\n\t\ttype:
              \"pvc\" | \"configMap\" | \"secret\" | \"emptyDir\"\n\t\tif type ==
              \"pvc\" {\n\t\t\tclaimName: string\n\t\t}\n\t\tif type == \"configMap\"
              {\n\t\t\tdefaultMode: *420 | int\n\t\t\tcmName:      string\n\t\t\titems?:
              [...{\n\t\t\t\tkey:  string\n\t\t\t\tpath: string\n\t\t\t\tmode: *511
              | int\n\t\t\t}]\n\t\t}\n\t\tif type == \"secret\" {\n\t\t\tdefaultMode:
              *420 | int\n\t\t\tsecretName:  string\n\t\t\titems?: [...{\n\t\t\t\tkey:
              \ string\n\t\t\t\tpath: string\n\t\t\t\tmode: *511 | int\n\t\t\t}]\n\t\t}\n\t\tif
              type == \"emptyDir\" {\n\t\t\tmedium: *\"\" | \"Memory\"\n\t\t}\n\t}]\n\n\t//
              +usage=Instructions for assessing whether the container is alive.\n\tlivenessProbe?:
              #HealthProbe\n\n\t// +usage=Instructions for assessing whether the container
              is in a suitable state to serve traffic.\n\treadinessProbe?: #HealthProbe\n\n\t//
              +usage=Specify the hostAliases to add\n\thostAliases?: [...{\n\t\tip:
              string\n\t\thostnames: [...string]\n\t}]\n}\n#HealthProbe: {\n\n\t//
              +usage=Instructions for assessing container health by executing a command.
              Either this attribute or the httpGet attribute or the tcpSocket attribute
              MUST be specified. This attribute is mutually exclusive with both the
              httpGet attribute and the tcpSocket attribute.\n\texec?: {\n\t\t// +usage=A
              command to be executed inside the container to assess its health. Each
              space delimited token of the command is a separate array element. Commands
              exiting 0 are considered to be successful probes, whilst all other exit
              codes are considered failures.\n\t\tcommand: [...string]\n\t}\n\n\t//
              +usage=Instructions for assessing container health by executing an HTTP
              GET request. Either this attribute or the exec attribute or the tcpSocket
              attribute MUST be specified. This attribute is mutually exclusive with
              both the exec attribute and the tcpSocket attribute.\n\thttpGet?: {\n\t\t//
              +usage=The endpoint, relative to the port, to which the HTTP GET request
              should be directed.\n\t\tpath: string\n\t\t// +usage=The TCP socket
              within the container to which the HTTP GET request should be directed.\n\t\tport:
              int\n\t\thttpHeaders?: [...{\n\t\t\tname:  string\n\t\t\tvalue: string\n\t\t}]\n\t}\n\n\t//
              +usage=Instructions for assessing container health by probing a TCP
              socket. Either this attribute or the exec attribute or the httpGet attribute
              MUST be specified. This attribute is mutually exclusive with both the
              exec attribute and the httpGet attribute.\n\ttcpSocket?: {\n\t\t// +usage=The
              TCP socket within the container that should be probed to assess container
              health.\n\t\tport: int\n\t}\n\n\t// +usage=Number of seconds after the
              container is started before the first probe is initiated.\n\tinitialDelaySeconds:
              *0 | int\n\n\t// +usage=How often, in seconds, to execute the probe.\n\tperiodSeconds:
              *10 | int\n\n\t// +usage=Number of seconds after which the probe times
              out.\n\ttimeoutSeconds: *1 | int\n\n\t// +usage=Minimum consecutive
              successes for the probe to be considered successful after having failed.\n\tsuccessThreshold:
              *1 | int\n\n\t// +usage=Number of consecutive failures required to determine
              the container is not alive (liveness probe) or not ready (readiness
              probe).\n\tfailureThreshold: *3 | int\n}\n"
        status:
          customStatus: "ready: {\n\treadyReplicas: *0 | int\n} & {\n\tif context.output.status.readyReplicas
            != _|_ {\n\t\treadyReplicas: context.output.status.readyReplicas\n\t}\n}\nmessage:
            \"Ready:\\(ready.readyReplicas)/\\(context.output.spec.replicas)\""
          healthPolicy: "ready: {\n\tupdatedReplicas:    *0 | int\n\treadyReplicas:
            \     *0 | int\n\treplicas:           *0 | int\n\tobservedGeneration:
            *0 | int\n} & {\n\tif context.output.status.updatedReplicas != _|_ {\n\t\tupdatedReplicas:
            context.output.status.updatedReplicas\n\t}\n\tif context.output.status.readyReplicas
            != _|_ {\n\t\treadyReplicas: context.output.status.readyReplicas\n\t}\n\tif
            context.output.status.replicas != _|_ {\n\t\treplicas: context.output.status.replicas\n\t}\n\tif
            context.output.status.observedGeneration != _|_ {\n\t\tobservedGeneration:
            context.output.status.observedGeneration\n\t}\n}\nisHealth: (context.output.spec.replicas
            == ready.readyReplicas) && (context.output.spec.replicas == ready.updatedReplicas)
            && (context.output.spec.replicas == ready.replicas) && (ready.observedGeneration
            == context.output.metadata.generation || ready.observedGeneration > context.output.metadata.generation)"
        workload:
          definition:
            apiVersion: apps/v1
            kind: Deployment
          type: deployments.apps
      status: {}
status: {}
`
		Expect(yaml.Unmarshal([]byte(apprevYaml), &apprev)).To(Succeed())
		// simulate 1.2 version that WorkflowStepDefinitions are not patched in appliacation revision
		apprev.ObjectMeta.OwnerReferences[0].UID = app.ObjectMeta.UID
		Expect(k8sClient.Create(ctx, &apprev)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		// prepare handler
		_handler, err := NewAppHandler(ctx, reconciler, &app, nil)
		Expect(err).Should(Succeed())
		handler = _handler

	})

	It("Test currentAppRevIsNew func", func() {
		By("Backport 1.2 version that WorkflowStepDefinitions are not patched to application revision")
		// generate appfile
		appfile, err := appfile.NewApplicationParser(reconciler.Client, reconciler.dm, reconciler.pd).GenerateAppFile(ctx, &app)
		ctx = util.SetNamespaceInCtx(ctx, app.Namespace)
		Expect(err).To(Succeed())
		Expect(handler.PrepareCurrentAppRevision(ctx, appfile)).Should(Succeed())

		// prepare apprev
		thisWSD := handler.currentAppRev.Spec.WorkflowStepDefinitions
		Expect(len(thisWSD) > 0 && func() bool {
			expected := appfile.RelatedWorkflowStepDefinitions
			for i, w := range thisWSD {
				expW := expected[i]
				if !reflect.DeepEqual(w, expW) {
					fmt.Printf("appfile wsd:%s apprev wsd%s", w.Name, expW.Name)
					return false
				}
			}
			return true
		}()).Should(BeTrue())
	})
})

func TestDeepEqualAppInRevision(t *testing.T) {
	oldRev := &v1beta1.ApplicationRevision{}
	newRev := &v1beta1.ApplicationRevision{}
	newRev.Spec.Application.Spec.Workflow = &v1beta1.Workflow{
		Steps: []workflowv1alpha1.WorkflowStep{{
			WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
				Type: "deploy",
				Name: "deploy",
			},
		}},
	}
	require.False(t, deepEqualAppInRevision(oldRev, newRev))
	metav1.SetMetaDataAnnotation(&oldRev.Spec.Application.ObjectMeta, oam.AnnotationKubeVelaVersion, "v1.6.0-alpha.5")
	require.False(t, deepEqualAppInRevision(oldRev, newRev))
	metav1.SetMetaDataAnnotation(&oldRev.Spec.Application.ObjectMeta, oam.AnnotationKubeVelaVersion, "v1.5.0")
	require.True(t, deepEqualAppInRevision(oldRev, newRev))
}
