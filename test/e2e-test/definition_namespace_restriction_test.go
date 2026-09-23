/*
 Copyright 2026. The KubeVela Authors.

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

package controllers_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Definition namespace restrictions", func() {
	ctx := context.Background()

	const restrictedTemplate = `
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	spec: {
		replicas: 1
		selector: matchLabels: "app": context.name
		template: {
			metadata: labels: "app": context.name
			spec: containers: [{
				name:  context.name
				image: parameter.image
			}]
		}
	}
}
parameter: image: string
`

	var allowedNS, deniedNS string
	var allowedNamespace, deniedNamespace corev1.Namespace
	// Definitions these specs put in vela-system. They outlive the namespaces, so
	// AfterEach removes them.
	var systemDefs []client.Object

	createNamespace := func(name string) corev1.Namespace {
		ns := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
		Eventually(func() error {
			return k8sClient.Create(ctx, &ns)
		}, time.Second*3, time.Microsecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		return ns
	}

	// patterns nil leaves the definition unrestricted.
	restrictedCompDef := func(namespace, name string, patterns []string, annotation string) *v1beta1.ComponentDefinition {
		var restrictions *common.DefinitionRestrictions
		if patterns != nil {
			restrictions = &common.DefinitionRestrictions{Namespaces: patterns}
		}
		cd := &v1beta1.ComponentDefinition{
			TypeMeta: metav1.TypeMeta{Kind: "ComponentDefinition", APIVersion: "core.oam.dev/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{
				Name: name, Namespace: namespace,
			},
			Spec: v1beta1.ComponentDefinitionSpec{
				Workload:     common.WorkloadTypeDescriptor{Definition: common.WorkloadGVK{APIVersion: "apps/v1", Kind: "Deployment"}},
				Restrictions: restrictions,
				Schematic:    &common.Schematic{CUE: &common.CUE{Template: restrictedTemplate}},
			},
		}
		if annotation != "" {
			cd.Annotations = map[string]string{oam.AnnotationRestrictNamespaces: annotation}
		}
		return cd
	}

	appUsing := func(namespace, name, compType string) *v1beta1.Application {
		return &v1beta1.Application{
			TypeMeta:   metav1.TypeMeta{Kind: "Application", APIVersion: "core.oam.dev/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{{
					Name:       "web",
					Type:       compType,
					Properties: util.Object2RawExtension(map[string]string{"image": "nginx:alpine"}),
				}},
			},
		}
	}

	BeforeEach(func() {
		// The specs restrict on "tenant-*", so one namespace matches and one does not.
		allowedNS = randomNamespaceName("tenant-nsrestrict")
		deniedNS = randomNamespaceName("other-nsrestrict")
		allowedNamespace = createNamespace(allowedNS)
		deniedNamespace = createNamespace(deniedNS)
	})

	AfterEach(func() {
		By("Clean up resources after a test")
		for _, def := range systemDefs {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, def))).Should(Succeed())
		}
		systemDefs = nil
		for _, ns := range []string{allowedNS, deniedNS} {
			_ = k8sClient.DeleteAllOf(ctx, &v1beta1.Application{}, client.InNamespace(ns))
			_ = k8sClient.DeleteAllOf(ctx, &v1beta1.ComponentDefinition{}, client.InNamespace(ns))
		}
		Expect(k8sClient.Delete(ctx, &allowedNamespace, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(Succeed())
		Expect(k8sClient.Delete(ctx, &deniedNamespace, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(Succeed())
	})

	It("admits an Application in a namespace the definition allows", func() {
		By("Installing a definition restricted to tenant namespaces in the allowed namespace")
		cd := restrictedCompDef(allowedNS, "restricted-web", []string{"tenant-*"}, "")
		Expect(k8sClient.Create(ctx, cd)).Should(Succeed())

		By("Creating an Application in that namespace")
		app := appUsing(allowedNS, "allowed-app", "restricted-web")
		Eventually(func() error {
			return k8sClient.Create(ctx, app)
		}, 15*time.Second, time.Second).Should(Succeed())
	})

	It("denies an Application in a namespace the definition excludes", func() {
		By("Installing the same definition in the excluded namespace")
		cd := restrictedCompDef(deniedNS, "restricted-web", []string{"tenant-*"}, "")
		Expect(k8sClient.Create(ctx, cd)).Should(Succeed())

		By("The webhook rejects an Application there, naming the component")
		// One Create, not Eventually: a retry would see AlreadyExists and pass even
		// if the Application had been admitted.
		err := k8sClient.Create(ctx, appUsing(deniedNS, "denied-app", "restricted-web"))
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).Should(ContainSubstring("spec.components[0].type"))
		Expect(err.Error()).Should(ContainSubstring("is restricted"))
		// The allowed namespaces are not the author's business.
		Expect(err.Error()).ShouldNot(ContainSubstring("tenant-*"))
	})

	It("restricts through the annotation as well as the spec", func() {
		By("Installing a definition restricted only by annotation")
		cd := restrictedCompDef(deniedNS, "annotated-web", nil, "tenant-*")
		Expect(k8sClient.Create(ctx, cd)).Should(Succeed())

		err := k8sClient.Create(ctx, appUsing(deniedNS, "annotated-app", "annotated-web"))
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).Should(ContainSubstring("spec.components[0].type"))
		Expect(err.Error()).Should(ContainSubstring("is restricted"))
	})

	// A shared definition in vela-system, reached through GetDefinition's
	// system-namespace fallback rather than found beside the Application.
	It("restricts a shared definition reached from vela-system", func() {
		By("Installing a restricted definition in vela-system")
		cd := restrictedCompDef(oam.SystemDefinitionNamespace, "shared-restricted-web", []string{"tenant-*"}, "")
		systemDefs = append(systemDefs, cd)
		Expect(k8sClient.Create(ctx, cd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		By("An Application outside the pattern is rejected")
		err := k8sClient.Create(ctx, appUsing(deniedNS, "shared-denied-app", "shared-restricted-web"))
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).Should(ContainSubstring("is restricted"))

		By("An Application matching it is admitted")
		Eventually(func() error {
			return k8sClient.Create(ctx, appUsing(allowedNS, "shared-allowed-app", "shared-restricted-web"))
		}, 15*time.Second, time.Second).Should(Succeed())
	})

	// A namespace that belongs to a tenant but is not named for one still matches.
	It("restricts by namespace label, whatever the namespace is called", func() {
		By("Labelling the excluded namespace so it qualifies")
		Eventually(func() error {
			got := &corev1.Namespace{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: deniedNS}, got); err != nil {
				return err
			}
			if got.Labels == nil {
				got.Labels = map[string]string{}
			}
			got.Labels["tenant"] = "true"
			return k8sClient.Update(ctx, got)
		}, 15*time.Second, time.Second).Should(Succeed())

		By("Installing a definition that selects on that label")
		cd := restrictedCompDef(oam.SystemDefinitionNamespace, "label-selected-web", nil, "")
		cd.Spec.Restrictions = &common.DefinitionRestrictions{
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"tenant": "true"}},
		}
		systemDefs = append(systemDefs, cd)
		Expect(k8sClient.Create(ctx, cd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		By("The labelled namespace is admitted despite not matching any name glob")
		Eventually(func() error {
			return k8sClient.Create(ctx, appUsing(deniedNS, "label-allowed-app", "label-selected-web"))
		}, 15*time.Second, time.Second).Should(Succeed())

		By("The unlabelled namespace is rejected without naming the selector")
		err := k8sClient.Create(ctx, appUsing(allowedNS, "label-denied-app", "label-selected-web"))
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).Should(ContainSubstring("is restricted"))
		Expect(err.Error()).ShouldNot(ContainSubstring("tenant=true"))
	})

	It("refuses a definition whose restriction is not a valid pattern", func() {
		cd := restrictedCompDef(allowedNS, "malformed-web", []string{"tenant-["}, "")
		err := k8sClient.Create(ctx, cd)
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).Should(ContainSubstring("tenant-["))
	})

	// Enforcement is on write only, so an Application admitted before the
	// restriction keeps reconciling, but cannot be edited afterwards.
	It("leaves an Application admitted before the restriction running, but frozen", func() {
		By("Installing an unrestricted definition and an Application that uses it")
		cd := restrictedCompDef(deniedNS, "later-restricted", nil, "")
		Expect(k8sClient.Create(ctx, cd)).Should(Succeed())

		app := appUsing(deniedNS, "grandfathered-app", "later-restricted")
		Eventually(func() error {
			return k8sClient.Create(ctx, app)
		}, 15*time.Second, time.Second).Should(Succeed())

		By("Waiting for it to reconcile")
		Eventually(func() error {
			got := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: app.Name, Namespace: deniedNS}, got); err != nil {
				return err
			}
			if got.Status.Phase != common.ApplicationRunning {
				return fmt.Errorf("application is %q, not running yet", got.Status.Phase)
			}
			return nil
		}, 90*time.Second, 2*time.Second).Should(Succeed())

		By("Restricting the definition to namespaces this Application is not in")
		Eventually(func() error {
			got := &v1beta1.ComponentDefinition{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: cd.Name, Namespace: deniedNS}, got); err != nil {
				return err
			}
			got.Spec.Restrictions = &common.DefinitionRestrictions{Namespaces: []string{"tenant-*"}}
			return k8sClient.Update(ctx, got)
		}, 15*time.Second, time.Second).Should(Succeed())

		By("It keeps reconciling, and the Deployment it dispatched stays up")
		Consistently(func() error {
			got := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: app.Name, Namespace: deniedNS}, got); err != nil {
				return err
			}
			if got.Status.Phase != common.ApplicationRunning {
				return fmt.Errorf("application fell out of running: %q", got.Status.Phase)
			}
			return k8sClient.Get(ctx, client.ObjectKey{Name: "web", Namespace: deniedNS}, &appsv1.Deployment{})
		}, 30*time.Second, 5*time.Second).Should(Succeed())

		By("But editing it is now rejected, which is what forces the issue")
		// Retry until the update is refused for the restriction specifically: a bare
		// ShouldNot(Succeed()) would also pass on a resourceVersion conflict.
		Eventually(func() error {
			got := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: app.Name, Namespace: deniedNS}, got); err != nil {
				return err
			}
			got.Spec.Components[0].Properties = util.Object2RawExtension(map[string]string{"image": "nginx:1.25"})
			err := k8sClient.Update(ctx, got)
			if err == nil {
				return fmt.Errorf("update was admitted, but the definition is restricted")
			}
			if !strings.Contains(err.Error(), "is restricted") {
				return fmt.Errorf("refused for another reason: %w", err)
			}
			return nil
		}, 30*time.Second, time.Second).Should(Succeed())
	})
})
