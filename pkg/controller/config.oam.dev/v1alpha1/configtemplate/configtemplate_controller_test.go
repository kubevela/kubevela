/*
Copyright 2026 The KubeVela Authors.

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

package configtemplate

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
)

// usernameTemplate renders one Secret from one parameter: enough for a schema.
const usernameTemplate = `
template: {
	parameter: { username: string }
	output: { apiVersion: "v1", kind: "Secret", stringData: { username: parameter.username } }
}
`

// eventuallyTemplatePhase waits for the ConfigTemplate at key to reach want, leaving it in got.
func eventuallyTemplatePhase(ctx context.Context, key client.ObjectKey, want configv1alpha1.ConfigTemplatePhase, got *configv1alpha1.ConfigTemplate) {
	Eventually(func() configv1alpha1.ConfigTemplatePhase {
		Expect(k8sClient.Get(ctx, key, got)).Should(Succeed())
		return got.Status.Phase
	}, 15*time.Second, time.Second).Should(Equal(want))
}

var _ = Describe("ConfigTemplate controller", func() {
	ctx := context.Background()

	It("should extract a schema and set Phase=Available for a valid CUE template", func() {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "valid-template", Namespace: "default"},
			Spec: configv1alpha1.ConfigTemplateSpec{
				Template: `
template: {
	parameter: {
		username: string
	}
	output: {
		apiVersion: "v1"
		kind: "Secret"
		stringData: {
			username: parameter.username
		}
	}
}
`,
			},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())

		var got configv1alpha1.ConfigTemplate
		eventuallyTemplatePhase(ctx, client.ObjectKeyFromObject(ct), configv1alpha1.ConfigTemplatePhaseAvailable, &got)
		Expect(got.Status.Schema).ShouldNot(BeNil())
		Expect(got.Status.Schema.Raw).ShouldNot(BeEmpty())
		Expect(got.Status.GetCondition(condition.TypeSynced).Status).Should(BeEquivalentTo("True"))
	})

	It("should set Phase=Error for an invalid CUE template", func() {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "invalid-template", Namespace: "default"},
			Spec: configv1alpha1.ConfigTemplateSpec{
				Template: `this is not valid cue {{{`,
			},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())

		var got configv1alpha1.ConfigTemplate
		eventuallyTemplatePhase(ctx, client.ObjectKeyFromObject(ct), configv1alpha1.ConfigTemplatePhaseError, &got)
		Expect(got.Status.GetCondition(condition.TypeSynced).Message).ShouldNot(BeEmpty())
	})

	It("should do nothing (no-op) when DeletionTimestamp is set", func() {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "ct-deleting",
				Namespace:  "default",
				Finalizers: []string{"test.oam.dev/protect"},
			},
			Spec: configv1alpha1.ConfigTemplateSpec{
				Template: usernameTemplate,
			},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())

		eventuallyTemplatePhase(ctx, client.ObjectKeyFromObject(ct), configv1alpha1.ConfigTemplatePhaseAvailable, ct)

		Expect(k8sClient.Delete(ctx, ct)).Should(Succeed())
		Eventually(func() bool {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), ct)).Should(Succeed())
			return ct.DeletionTimestamp != nil
		}, 15*time.Second, time.Second).Should(BeTrue())
		Expect(ct.Status.Phase).Should(Equal(configv1alpha1.ConfigTemplatePhaseAvailable))

		patch := client.MergeFrom(ct.DeepCopy())
		ct.Finalizers = nil
		Expect(k8sClient.Patch(ctx, ct, patch)).Should(Succeed())
		Eventually(func() bool {
			return apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), ct))
		}, 15*time.Second, time.Second).Should(BeTrue())
	})

	It("should drop the schema when a template's CUE stops parsing", func() {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "goes-bad", Namespace: "default"},
			Spec: configv1alpha1.ConfigTemplateSpec{
				Template: usernameTemplate,
			},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())
		var got configv1alpha1.ConfigTemplate
		eventuallyTemplatePhase(ctx, client.ObjectKeyFromObject(ct), configv1alpha1.ConfigTemplatePhaseAvailable, &got)
		Expect(got.Status.Schema).ShouldNot(BeNil())

		patch := client.MergeFrom(got.DeepCopy())
		got.Spec.Template = `this is not valid cue {{{`
		Expect(k8sClient.Patch(ctx, &got, patch)).Should(Succeed())
		eventuallyTemplatePhase(ctx, client.ObjectKeyFromObject(ct), configv1alpha1.ConfigTemplatePhaseError, &got)
		Expect(got.Status.Schema).Should(BeNil(), "a stale schema would make the broken template usable")
	})

	It("should set Phase=Error, schema kept, for a system template outside vela-system", func() {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "system-misplaced", Namespace: "default"},
			Spec: configv1alpha1.ConfigTemplateSpec{
				Scope:    configv1alpha1.ConfigTemplateScopeSystem,
				Template: usernameTemplate,
			},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, ct))).Should(Succeed()) })

		var got configv1alpha1.ConfigTemplate
		eventuallyTemplatePhase(ctx, client.ObjectKeyFromObject(ct), configv1alpha1.ConfigTemplatePhaseError, &got)
		Expect(got.Status.GetCondition(condition.TypeSynced).Message).Should(ContainSubstring("scope system is only valid in vela-system"))
		// the error is about placement, not the CUE: the schema is still extracted
		Expect(got.Status.Schema).ShouldNot(BeNil())
		Expect(got.Status.Schema.Raw).ShouldNot(BeEmpty())
	})

	It("should set Phase=Available for a system template in vela-system", func() {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vela-system"}}
		if err := k8sClient.Create(ctx, ns); err != nil {
			Expect(apierrors.IsAlreadyExists(err)).Should(BeTrue())
		}
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "system-placed", Namespace: "vela-system"},
			Spec: configv1alpha1.ConfigTemplateSpec{
				Scope:    configv1alpha1.ConfigTemplateScopeSystem,
				Template: usernameTemplate,
			},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())

		var got configv1alpha1.ConfigTemplate
		eventuallyTemplatePhase(ctx, client.ObjectKeyFromObject(ct), configv1alpha1.ConfigTemplatePhaseAvailable, &got)
	})
})

// This suite installs the chart CRDs and no webhook: what is refused here is the CRD's CEL rule.
var _ = Describe("ConfigTemplate sensitive rule in the CRD", func() {
	ctx := context.Background()

	newTemplate := func(name string, sensitive bool) *configv1alpha1.ConfigTemplate {
		return &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: usernameTemplate, Sensitive: sensitive},
		}
	}

	It("should refuse turning sensitive off without a webhook", func() {
		ct := newTemplate("sensitive-off", true)
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, ct))).Should(Succeed()) })

		patch := client.MergeFrom(ct.DeepCopy())
		ct.Spec.Sensitive = false
		err := k8sClient.Patch(ctx, ct, patch)
		Expect(apierrors.IsInvalid(err)).Should(BeTrue(), "got %v", err)
		Expect(err.Error()).Should(ContainSubstring("sensitive cannot be changed from true to false"))

		var got configv1alpha1.ConfigTemplate
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), &got)).Should(Succeed())
		Expect(got.Spec.Sensitive).Should(BeTrue())
	})

	It("should admit turning sensitive on, and other edits of a sensitive template", func() {
		ct := newTemplate("sensitive-on", false)
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, ct))).Should(Succeed()) })

		patch := client.MergeFrom(ct.DeepCopy())
		ct.Spec.Sensitive = true
		Expect(k8sClient.Patch(ctx, ct, patch)).Should(Succeed())

		patch = client.MergeFrom(ct.DeepCopy())
		ct.Spec.Description = "still sensitive"
		Expect(k8sClient.Patch(ctx, ct, patch)).Should(Succeed())

		var got configv1alpha1.ConfigTemplate
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), &got)).Should(Succeed())
		Expect(got.Spec.Sensitive).Should(BeTrue())
		Expect(got.Spec.Description).Should(Equal("still sensitive"))
	})
})
