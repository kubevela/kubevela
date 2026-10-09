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

package controllers_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// The unit and envtest suites compile templates in the test binary. Only here is
// the compiler the one vela-core itself builds, from the packages that pod ships.
var _ = Describe("WorkflowStepDefinition CUE template validation E2E tests", func() {
	ctx := context.Background()

	var namespace string
	var ns corev1.Namespace

	newStepDef := func(name, template string) *v1beta1.WorkflowStepDefinition {
		return &v1beta1.WorkflowStepDefinition{
			TypeMeta: metav1.TypeMeta{
				Kind:       "WorkflowStepDefinition",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: v1beta1.WorkflowStepDefinitionSpec{
				Schematic: &common.Schematic{
					CUE: &common.CUE{Template: template},
				},
			},
		}
	}

	BeforeEach(func() {
		namespace = randomNamespaceName("wsd-cue-validation")
		ns = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}

		Eventually(func() error {
			return k8sClient.Create(ctx, &ns)
		}, time.Second*3, time.Microsecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})

	AfterEach(func() {
		By("Clean up resources after a test")
		Expect(k8sClient.DeleteAllOf(ctx, &v1beta1.WorkflowStepDefinition{}, client.InNamespace(namespace))).Should(Succeed())

		By(fmt.Sprintf("Delete the entire namespace %s", ns.Name))
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(Succeed())
	})

	// Issue #7277: an identically broken ComponentDefinition was already
	// rejected, while this was admitted and only failed once a workflow ran it.
	It("Should reject a WorkflowStepDefinition whose template imports a package it never uses", func() {
		err := k8sClient.Create(ctx, newStepDef("wsd-unused-import", `
import "vela/kube"

parameter: {
	name: string
}`))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`imported and not used: "vela/kube"`))
	})

	It("Should reject a WorkflowStepDefinition whose template imports an undefined package", func() {
		err := k8sClient.Create(ctx, newStepDef("wsd-unknown-package", `
import "vela/doesnotexist"

output: doesnotexist.#Foo`))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`builtin package "vela/doesnotexist" undefined`))
	})

	// vela/op exists only in the workflow compiler. The workload compiler that
	// ComponentDefinition uses rejects this, and most of the bundled step defs.
	It("Should admit a WorkflowStepDefinition importing vela/op", func() {
		stepDef := newStepDef("wsd-vela-op", `
import "vela/op"

wait: op.#ConditionalWait & {
	continue: true
}`)
		Expect(k8sClient.Create(ctx, stepDef)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, stepDef)).Should(Succeed())
	})

	// The cluster named here is unreachable, which admission never finds out.
	It("Should admit a WorkflowStepDefinition with concrete provider arguments without executing the provider", func() {
		stepDef := newStepDef("wsd-concrete-provider-args", `
import "vela/kube"

read: kube.#Read & {
	$params: {
		cluster: "some-unreachable-cluster"
		value: {
			apiVersion: "v1"
			kind:       "Secret"
			metadata: {
				name:      "does-not-exist"
				namespace: "does-not-exist"
			}
		}
	}
}`)
		Expect(k8sClient.Create(ctx, stepDef)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, stepDef)).Should(Succeed())
	})
})
