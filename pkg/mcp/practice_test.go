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

package mcp

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func namespace(name string, labels map[string]string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}}
}

func deployment(ns, name string, managed bool) *appsv1.Deployment {
	d := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
	if managed {
		d.Labels = map[string]string{oam.LabelAppName: name, oam.LabelAppNamespace: ns, oam.LabelAppComponent: name}
	}
	return d
}

func allowAll(allowed bool) AccessReviewFunc {
	return func(context.Context, client.Client, string, string, schema.GroupResource) (bool, error) {
		return allowed, nil
	}
}

// practiceFixtures has one namespace per delivery style.
func practiceFixtures() []client.Object {
	return append(platformFixtures(),
		namespace("plain", nil),
		deployment("plain", "api", false),
		deployment("plain", "worker", false),
		namespace("vela", map[string]string{deliveryDefaultLabel: StyleKubeVela}),
		&v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "vela"}},
		deployment("vela", "web", true),
		namespace("both", nil),
		&v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "both"}},
		deployment("both", "web", true),
		deployment("both", "legacy", false),
		namespace("fresh", map[string]string{deliveryDefaultLabel: "something-else"}),
		namespace("batch", nil),
		&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "migrate", Namespace: "batch"}},
		namespace("copied", nil),
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "legacy", Namespace: "copied", Labels: map[string]string{oam.LabelAppName: "legacy"}}},
		namespace("cron", nil),
		&v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "report", Namespace: "cron"}},
		&batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: "report", Namespace: "cron", Labels: map[string]string{oam.LabelAppName: "report", oam.LabelAppNamespace: "cron", oam.LabelAppComponent: "report"}}},
		&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "report-29001", Namespace: "cron",
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "batch/v1", Kind: "CronJob", Name: "report", UID: "cron-uid"}}}},
	)
}

var _ = Describe("delivery practice", func() {
	var h *harness

	AfterEach(func() {
		if h != nil {
			h.close()
			h = nil
		}
	})

	DescribeTable("reports how a namespace delivers workloads",
		func(ns, style string, apps, workloads, managed int, declared string) {
			h = startHarness(Options{AccessReview: allowAll(true)}, practiceFixtures()...)
			var out GetPlatformInfoOutput
			res := h.call("get_platform_info", map[string]any{"namespace": ns}, &out)
			Expect(res.IsError).To(BeFalse())
			Expect(out.Practice).NotTo(BeNil())
			Expect(out.Practice.Namespace).To(Equal(ns))
			Expect(out.Practice.Style).To(Equal(style))
			Expect(out.Practice.Applications).To(Equal(apps))
			Expect(out.Practice.Workloads).To(Equal(workloads))
			Expect(out.Practice.WorkloadsManagedByApplications).To(Equal(managed))
			Expect(out.Practice.DeclaredDefault).To(Equal(declared))
			Expect(*out.Practice.CanCreatePlainWorkloads).To(BeTrue())
		},
		Entry("plain Kubernetes", "plain", StyleKubernetes, 0, 2, 0, ""),
		Entry("KubeVela with a declared default", "vela", StyleKubeVela, 1, 1, 1, StyleKubeVela),
		Entry("both side by side", "both", StyleMixed, 1, 2, 1, ""),
		Entry("blank, ignoring an unknown declared value", "fresh", StyleEmpty, 0, 0, 0, ""),
		Entry("a plain Job is existing practice, not a blank start", "batch", StyleKubernetes, 0, 1, 0, ""),
		Entry("a Job an Application's CronJob started is counted through its owner", "cron", StyleKubeVela, 1, 1, 1, ""),
		Entry("a workload carrying only the name label is not taken for Application-managed", "copied", StyleKubernetes, 0, 1, 0, ""),
	)

	It("reports the style as unknown when a count could not be read", func() {
		funcs := &interceptor.Funcs{List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			if _, ok := list.(*appsv1.StatefulSetList); ok {
				return errors.New("forbidden")
			}
			return c.List(ctx, list, opts...)
		}}
		h = startHarnessIntercepted(Options{AccessReview: allowAll(true)}, nil, funcs, practiceFixtures()...)
		var out GetPlatformInfoOutput
		h.call("get_platform_info", map[string]any{"namespace": "plain"}, &out)
		Expect(out.Practice.Style).To(Equal(StyleUnknown))
		Expect(out.Meta.Warnings).To(ContainElement(ContainSubstring("delivery style is unknown")))
	})

	It("omits the report without a namespace and reports a denied access review", func() {
		h = startHarness(Options{AccessReview: allowAll(false)}, practiceFixtures()...)
		var out GetPlatformInfoOutput
		h.call("get_platform_info", nil, &out)
		Expect(out.Practice).To(BeNil())
		h.call("get_platform_info", map[string]any{"namespace": "plain"}, &out)
		Expect(*out.Practice.CanCreatePlainWorkloads).To(BeFalse())
	})

	It("degrades to a warning when the access review fails", func() {
		failing := func(context.Context, client.Client, string, string, schema.GroupResource) (bool, error) {
			return false, errors.New("token=abc review failed")
		}
		h = startHarness(Options{AccessReview: failing}, practiceFixtures()...)
		var out GetPlatformInfoOutput
		h.call("get_platform_info", map[string]any{"namespace": "plain"}, &out)
		Expect(out.Practice.CanCreatePlainWorkloads).To(BeNil())
		Expect(out.Meta.Warnings).To(ContainElement(ContainSubstring("access review unavailable")))
		Expect(toJSON(out)).NotTo(ContainSubstring("abc"))
	})

	It("builds a SelfSubjectAccessReview the scheme can encode", func() {
		cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
		allowed, err := selfAccessReview(context.Background(), cli, "plain", "create", schema.GroupResource{Group: "apps", Resource: "deployments"})
		// The fake client demands a name the real API server does not need for
		// access reviews; what matters here is that the kind is registered.
		Expect(runtime.IsNotRegisteredError(err)).To(BeFalse(), "%v", err)
		Expect(allowed).To(BeFalse(), "the fake client does not evaluate RBAC")
	})

	It("applies the namespace allowlist", func() {
		h = startHarness(Options{Namespaces: []string{"vela"}, AccessReview: allowAll(true)}, practiceFixtures()...)
		res := h.call("get_platform_info", map[string]any{"namespace": "plain"}, nil)
		Expect(errorText(res)).To(ContainSubstring("outside this server's allowed namespaces"))
	})
})

var _ = Describe("definition alternatives", func() {
	var h *harness

	AfterEach(func() {
		if h != nil {
			h.close()
			h = nil
		}
	})

	postgres := func(name, defaultFor string) *v1beta1.ComponentDefinition {
		d := &v1beta1.ComponentDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   types.DefaultKubeVelaNS,
				Labels:      map[string]string{providesLabel: "postgres"},
				Annotations: map[string]string{types.AnnoDefinitionDescription: name + " database"},
			},
			Spec: v1beta1.ComponentDefinitionSpec{Schematic: &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: "parameter: {}\n"}}},
		}
		if defaultFor != "" {
			d.Annotations[defaultForAnnotation] = defaultFor
		}
		return d
	}

	fixtures := func() []client.Object {
		return append(platformFixtures(), postgres("cnpg-postgres", "dev, team-a"), postgres("aurora-postgres", ""))
	}

	It("filters by what a definition provides and marks the default for the namespace", func() {
		h = startHarness(Options{}, fixtures()...)
		var out ListDefinitionsOutput
		h.call("list_definitions", map[string]any{"provides": "postgres", "namespace": "dev"}, &out)
		Expect(out.Definitions).To(HaveLen(2))
		byName := map[string]DefinitionSummary{}
		for _, d := range out.Definitions {
			byName[d.Name] = d
		}
		Expect(byName["cnpg-postgres"].Default).To(BeTrue())
		Expect(byName["cnpg-postgres"].DefaultFor).To(Equal([]string{"dev", "team-a"}))
		Expect(byName["aurora-postgres"].Default).To(BeFalse())
		Expect(byName["aurora-postgres"].Provides).To(Equal("postgres"))
	})

	It("marks no default where the platform set none", func() {
		h = startHarness(Options{}, fixtures()...)
		var out ListDefinitionsOutput
		h.call("list_definitions", map[string]any{"provides": "postgres", "namespace": "prod"}, &out)
		Expect(out.Definitions).To(HaveLen(2))
		for _, d := range out.Definitions {
			Expect(d.Default).To(BeFalse(), d.Name)
		}
	})

	It("treats a star as every namespace and leaves unrelated definitions out of a provides filter", func() {
		objs := fixtures()
		objs[len(objs)-1].(*v1beta1.ComponentDefinition).Annotations[defaultForAnnotation] = "*"
		h = startHarness(Options{}, objs...)
		var out ListDefinitionsOutput
		h.call("list_definitions", map[string]any{"provides": "postgres"}, &out)
		Expect(out.Definitions).To(HaveLen(2), "both labelled definitions, and nothing unrelated")
		for _, d := range out.Definitions {
			Expect(d.Default).To(Equal(d.Name == "aurora-postgres"), d.Name)
			Expect(d.MatchedBy).To(Equal("label"))
		}
	})

	It("includes an unlabelled definition whose description mentions the capability, and says how it matched", func() {
		legacy := &v1beta1.ComponentDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "rds", Namespace: types.DefaultKubeVelaNS, Annotations: map[string]string{types.AnnoDefinitionDescription: "Managed PostgreSQL on RDS"}},
			Spec:       v1beta1.ComponentDefinitionSpec{Schematic: &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: "parameter: {}\n"}}},
		}
		h = startHarness(Options{}, append(fixtures(), legacy)...)
		var out ListDefinitionsOutput
		h.call("list_definitions", map[string]any{"provides": "postgres"}, &out)
		byName := map[string]DefinitionSummary{}
		for _, d := range out.Definitions {
			byName[d.Name] = d
		}
		Expect(byName).To(HaveKey("rds"))
		Expect(byName["rds"].MatchedBy).To(Equal("description"))
		Expect(byName["rds"].Description).To(ContainSubstring("PostgreSQL"))
		Expect(byName).NotTo(HaveKey("webservice"))
	})
})
