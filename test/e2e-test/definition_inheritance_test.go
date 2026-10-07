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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// A parent that renders a Deployment and reports health from its replicas.
const inheritParentTemplate = `
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: name: context.name
	spec: {
		replicas: parameter.replicas
		selector: matchLabels: "app.oam.dev/component": context.name
		template: {
			metadata: labels: "app.oam.dev/component": context.name
			spec: containers: [{
				name:  context.name
				image: parameter.image
			}]
		}
	}
}

parameter: {
	image:    string
	replicas: *1 | int
}
`

// A child that supplies part of the parent's parameters, overlays a label, and
// adds a resource of its own.
const inheritChildTemplate = `
$super: properties: {image: parameter.image}

output: metadata: labels: "tenant.oam.dev/name": parameter.tenant

outputs: tenantConfig: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: {
		name:      context.name + "-tenant"
		namespace: context.namespace
	}
	data: tenant: parameter.tenant
}

parameter: {
	image:  string
	tenant: string
}
`

func componentDefinitionIn(namespace, name, extends, template string, status *common.Status) *v1beta1.ComponentDefinition {
	return &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: v1beta1.ComponentDefinitionSpec{
			Extends: extends,
			Workload: common.WorkloadTypeDescriptor{
				Definition: common.WorkloadGVK{APIVersion: "apps/v1", Kind: "Deployment"},
			},
			Status:    status,
			Schematic: &common.Schematic{CUE: &common.CUE{Template: template}},
		},
	}
}

var _ = Describe("ComponentDefinition inheritance", func() {
	ctx := context.Background()

	var namespace string
	var ns corev1.Namespace

	BeforeEach(func() {
		namespace = randomNamespaceName("inherit-e2e-test")
		ns = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		Eventually(func() error {
			return k8sClient.Create(ctx, &ns)
		}, time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})

	AfterEach(func() {
		By("clean up the namespace")
		Expect(k8sClient.Delete(ctx, &ns)).Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
	})

	It("renders a child on top of what it extends", func() {
		By("apply the parent and the child")
		parent := componentDefinitionIn(namespace, "inherit-parent", "", inheritParentTemplate, nil)
		Expect(k8sClient.Create(ctx, parent)).Should(Succeed())

		child := componentDefinitionIn(namespace, "inherit-child", "inherit-parent", inheritChildTemplate, nil)
		Expect(k8sClient.Create(ctx, child)).Should(Succeed())

		By("apply an application naming only the child")
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "inherit-app", Namespace: namespace},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{{
					Name:       "inherited",
					Type:       "inherit-child",
					Properties: &runtime.RawExtension{Raw: []byte(`{"image":"nginx:1.27","tenant":"acme"}`)},
				}},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())

		By("the parent's workload is rendered, carrying the child's overlay")
		deploy := &appsv1.Deployment{}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: "inherited", Namespace: namespace}, deploy)
		}, time.Second*60, time.Second*2).Should(Succeed())

		Expect(deploy.Spec.Template.Spec.Containers[0].Image).Should(Equal("nginx:1.27"))
		Expect(deploy.Labels["tenant.oam.dev/name"]).Should(Equal("acme"))
		Expect(*deploy.Spec.Replicas).Should(BeEquivalentTo(1), "the parent's default reached the render")

		By("the child's own output is rendered beside it")
		cm := &corev1.ConfigMap{}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: "inherited-tenant", Namespace: namespace}, cm)
		}, time.Second*60, time.Second*2).Should(Succeed())
		Expect(cm.Data["tenant"]).Should(Equal("acme"))

		By("the revision records what the render went through")
		revs := &v1beta1.ApplicationRevisionList{}
		Eventually(func() error {
			return k8sClient.List(ctx, revs, client.InNamespace(namespace))
		}, time.Second*30, time.Second*2).Should(Succeed())
		Expect(revs.Items).ShouldNot(BeEmpty())

		defs := revs.Items[0].Spec.ComponentDefinitions
		Expect(defs).Should(HaveKey("inherit-child"))
		Expect(defs).Should(HaveKey("inherit-parent"), "the ancestor is recorded, or a replay drifts")
	})

	It("composes health with the policy it extends", func() {
		By("a parent whose health depends on its replicas being ready")
		parent := componentDefinitionIn(namespace, "health-parent", "", inheritParentTemplate, &common.Status{
			HealthPolicy: `isHealth: context.output.status.readyReplicas == context.output.spec.replicas`,
			CustomStatus: `message: "parent says \(context.output.spec.replicas)"`,
		})
		Expect(k8sClient.Create(ctx, parent)).Should(Succeed())

		By("a child adding a condition of its own, and extending the message")
		child := componentDefinitionIn(namespace, "health-child", "health-parent", inheritChildTemplate, &common.Status{
			HealthPolicy: `isHealth: context.output.spec.replicas > 0`,
			CustomStatus: `message: "\($super.message), child agrees"`,
		})
		Expect(k8sClient.Create(ctx, child)).Should(Succeed())

		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "health-app", Namespace: namespace},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{{
					Name:       "checked",
					Type:       "health-child",
					Properties: &runtime.RawExtension{Raw: []byte(`{"image":"example.invalid/never:pull","tenant":"acme"}`)},
				}},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())

		By("the parent's check fails, so the pair does, and the message carries both")
		Eventually(func(g Gomega) {
			got := &v1beta1.Application{}
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "health-app", Namespace: namespace}, got)).Should(Succeed())
			g.Expect(got.Status.Services).ShouldNot(BeEmpty())
			g.Expect(got.Status.Services[0].Healthy).Should(BeFalse())
			g.Expect(got.Status.Services[0].Message).Should(ContainSubstring("parent says"))
			g.Expect(got.Status.Services[0].Message).Should(ContainSubstring("child agrees"))
		}, time.Second*90, time.Second*3).Should(Succeed())
	})

	It("refuses a definition that never calls what it extends", func() {
		parent := componentDefinitionIn(namespace, "lonely-parent", "", inheritParentTemplate, nil)
		Expect(k8sClient.Create(ctx, parent)).Should(Succeed())

		child := componentDefinitionIn(namespace, "lonely-child", "lonely-parent",
			`output: metadata: labels: x: "y"`+"\n"+`parameter: {}`, nil)

		err := k8sClient.Create(ctx, child)
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).Should(ContainSubstring("never calls it"))
	})

	It("refuses a parent in another namespace", func() {
		child := componentDefinitionIn(namespace, "reaching-child", "webservice",
			"$super: properties: {image: parameter.image}\nparameter: {image: string}", nil)

		err := k8sClient.Create(ctx, child)
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).Should(ContainSubstring("its own namespace"))
	})

	It("refuses an application naming an abstract definition", func() {
		By("a base that carries the platform's rules, marked extend-only")
		base := componentDefinitionIn(namespace, "abstract-base", "", inheritParentTemplate, nil)
		base.Spec.Abstract = true
		Expect(k8sClient.Create(ctx, base)).Should(Succeed())

		By("and a definition that extends it, which is the way through")
		paved := componentDefinitionIn(namespace, "abstract-paved", "abstract-base", inheritChildTemplate, nil)
		Expect(k8sClient.Create(ctx, paved)).Should(Succeed())

		By("naming the base directly is refused")
		direct := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "direct-app", Namespace: namespace},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{{
					Name:       "straight-at-it",
					Type:       "abstract-base",
					Properties: &runtime.RawExtension{Raw: []byte(`{"image":"nginx:1.27"}`)},
				}},
			},
		}
		err := k8sClient.Create(ctx, direct)
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).Should(ContainSubstring("abstract and cannot be used directly"))

		By("while what extends it is accepted")
		viaChild := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "paved-app", Namespace: namespace},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{{
					Name:       "the-paved-road",
					Type:       "abstract-paved",
					Properties: &runtime.RawExtension{Raw: []byte(`{"image":"nginx:1.27","tenant":"acme"}`)},
				}},
			},
		}
		Expect(k8sClient.Create(ctx, viaChild)).Should(Succeed())

		deploy := &appsv1.Deployment{}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: "the-paved-road", Namespace: namespace}, deploy)
		}, time.Second*60, time.Second*2).Should(Succeed())
	})
})
