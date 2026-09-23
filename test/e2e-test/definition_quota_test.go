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
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// warningRecorder keeps the admission warnings a request came back with. The
// default handler only logs them, and a warn threshold has nothing else to show
// for itself.
type warningRecorder struct {
	mu       sync.Mutex
	warnings []string
}

func (w *warningRecorder) HandleWarningHeader(code int, _ string, message string) {
	if code != 299 || message == "" {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.warnings = append(w.warnings, message)
}

func (w *warningRecorder) take() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	got := w.warnings
	w.warnings = nil
	return got
}

var _ = Describe("Definition usage quota", func() {
	ctx := context.Background()

	const quotaTemplate = `
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

	var tenantNS string
	var tenantNamespace corev1.Namespace
	var warnings *warningRecorder
	var warnClient client.Client

	createNamespace := func(name string) corev1.Namespace {
		ns := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
		Eventually(func() error {
			return k8sClient.Create(ctx, &ns)
		}, time.Second*3, time.Microsecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		return ns
	}

	quotaCompDef := func(namespace, name string, quota ...common.NamespaceQuota) *v1beta1.ComponentDefinition {
		return &v1beta1.ComponentDefinition{
			TypeMeta: metav1.TypeMeta{Kind: "ComponentDefinition", APIVersion: "core.oam.dev/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{
				Name: name, Namespace: namespace,
			},
			Spec: v1beta1.ComponentDefinitionSpec{
				Workload:     common.WorkloadTypeDescriptor{Definition: common.WorkloadGVK{APIVersion: "apps/v1", Kind: "Deployment"}},
				Restrictions: &common.DefinitionRestrictions{Quota: quota},
				Schematic:    &common.Schematic{CUE: &common.CUE{Template: quotaTemplate}},
			},
		}
	}

	// appWith builds an Application with count components, all of compType.
	appWith := func(namespace, name, compType string, count int) *v1beta1.Application {
		comps := make([]common.ApplicationComponent, 0, count)
		for i := 0; i < count; i++ {
			comps = append(comps, common.ApplicationComponent{
				Name:       fmt.Sprintf("web-%d", i),
				Type:       compType,
				Properties: util.Object2RawExtension(map[string]string{"image": "nginx:alpine"}),
			})
		}
		return &v1beta1.Application{
			TypeMeta:   metav1.TypeMeta{Kind: "Application", APIVersion: "core.oam.dev/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec:       v1beta1.ApplicationSpec{Components: comps},
		}
	}

	// appWithTrait builds an Application of one component carrying count copies of
	// the capped trait, so a namespace can be filled from a single Application.
	appWithTrait := func(namespace, name, compType string, count int) *v1beta1.Application {
		comp := common.ApplicationComponent{
			Name:       "web",
			Type:       compType,
			Properties: util.Object2RawExtension(map[string]string{"image": "nginx:alpine"}),
		}
		for i := 0; i < count; i++ {
			comp.Traits = append(comp.Traits, common.ApplicationTrait{
				Type:       "capped-label",
				Properties: util.Object2RawExtension(map[string]string{}),
			})
		}
		return &v1beta1.Application{
			TypeMeta:   metav1.TypeMeta{Kind: "Application", APIVersion: "core.oam.dev/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec:       v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{comp}},
		}
	}

	// refusedEventually retries a create under a fresh name until it is refused for
	// the reasons given. The webhook reads definitions and counts their use through
	// a cache, so neither is visible the instant it is written. A fresh name each
	// time means an admitted attempt is never misread as AlreadyExists, and each
	// one admitted only brings the namespace closer to the limit, so this converges
	// instead of passing on a stale read.
	refusedEventually := func(prefix string, build func(name string) *v1beta1.Application, wants ...string) {
		var err error
		attempt := 0
		Eventually(func() error {
			attempt++
			if err = k8sClient.Create(ctx, build(fmt.Sprintf("%s-%d", prefix, attempt))); err == nil {
				return fmt.Errorf("attempt %d was admitted; the webhook does not see it yet", attempt)
			}
			for _, want := range wants {
				if !strings.Contains(err.Error(), want) {
					return fmt.Errorf("refused, but not for %q: %w", want, err)
				}
			}
			return nil
		}, 60*time.Second, 2*time.Second).Should(Succeed())
	}

	int32p := func(n int32) *int32 { return &n }

	BeforeEach(func() {
		tenantNS = randomNamespaceName("tenant-quota")
		tenantNamespace = createNamespace(tenantNS)

		// A second client, so the 299 warning headers can be read back rather than
		// only logged.
		warnings = &warningRecorder{}
		cfg := rest.CopyConfig(config.GetConfigOrDie())
		cfg.WarningHandler = warnings
		var err error
		warnClient, err = client.New(cfg, client.Options{Scheme: scheme})
		Expect(err).Should(BeNil())
	})

	AfterEach(func() {
		By("Clean up resources after a test")
		_ = k8sClient.DeleteAllOf(ctx, &v1beta1.Application{}, client.InNamespace(tenantNS))
		_ = k8sClient.DeleteAllOf(ctx, &v1beta1.ComponentDefinition{}, client.InNamespace(tenantNS))
		Expect(k8sClient.Delete(ctx, &tenantNamespace, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(Succeed())
	})

	It("refuses the component that takes a namespace over the limit", func() {
		By("Installing a definition that allows three components per namespace")
		Expect(k8sClient.Create(ctx, quotaCompDef(tenantNS, "capped-web",
			common.NamespaceQuota{Limit: int32p(3)}))).Should(Succeed())

		By("Filling the namespace to the limit, across two Applications")
		Eventually(func() error {
			return k8sClient.Create(ctx, appWith(tenantNS, "app-1", "capped-web", 2))
		}, 15*time.Second, time.Second).Should(Succeed())
		Expect(k8sClient.Create(ctx, appWith(tenantNS, "app-2", "capped-web", 1))).Should(Succeed())

		By("The next component is refused, naming the field it came in on")
		refusedEventually("app-overflow", func(name string) *v1beta1.Application {
			return appWith(tenantNS, name, "capped-web", 1)
		}, "spec.components[0].type", `quota for component type "capped-web"`)

		By("An edit that changes nothing is still admitted at the limit")
		// The count excludes the Application being edited; without that it would
		// fail its own quota.
		Eventually(func() error {
			got := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: "app-1", Namespace: tenantNS}, got); err != nil {
				return err
			}
			got.Spec.Components[0].Properties = util.Object2RawExtension(map[string]string{"image": "nginx:1.25"})
			return k8sClient.Update(ctx, got)
		}, 30*time.Second, time.Second).Should(Succeed())

		By("Widening the quota lets the refused Application through")
		Eventually(func() error {
			got := &v1beta1.ComponentDefinition{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: "capped-web", Namespace: tenantNS}, got); err != nil {
				return err
			}
			got.Spec.Restrictions = &common.DefinitionRestrictions{
				Quota: []common.NamespaceQuota{{Limit: int32p(10)}},
			}
			return k8sClient.Update(ctx, got)
		}, 15*time.Second, time.Second).Should(Succeed())

		Eventually(func() error {
			return k8sClient.Create(ctx, appWith(tenantNS, "app-3", "capped-web", 1))
		}, 30*time.Second, 2*time.Second).Should(Succeed())
	})

	It("admits with a warning once a namespace reaches the warn threshold", func() {
		By("Installing a warn-only quota, which never refuses")
		Expect(k8sClient.Create(ctx, quotaCompDef(tenantNS, "advisory-web",
			common.NamespaceQuota{Warn: int32p(2)}))).Should(Succeed())

		By("Staying below the threshold says nothing")
		Eventually(func() error {
			return warnClient.Create(ctx, appWith(tenantNS, "quiet-app", "advisory-web", 1))
		}, 15*time.Second, time.Second).Should(Succeed())
		Expect(warnings.take()).Should(BeEmpty())

		By("Crossing it is admitted, with a warning saying where the namespace stands")
		Expect(warnClient.Create(ctx, appWith(tenantNS, "loud-app", "advisory-web", 3))).Should(Succeed())
		Expect(warnings.take()).Should(ContainElement(SatisfyAll(
			ContainSubstring(`ComponentDefinition "advisory-web"`),
			ContainSubstring("using 4"),
		)))
	})

	It("never applies a quota to the system namespace", func() {
		By("Installing a definition in vela-system that forbids its type outright")
		cd := quotaCompDef(types.DefaultKubeVelaNS, "addon-only-web",
			common.NamespaceQuota{Limit: int32p(0)})
		Expect(k8sClient.Create(ctx, cd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		defer func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, cd))).Should(Succeed())
		}()

		By("An Application in vela-system is admitted anyway, as an addon's would be")
		app := appWith(types.DefaultKubeVelaNS, "addon-app", "addon-only-web", 2)
		Eventually(func() error {
			return k8sClient.Create(ctx, app)
		}, 15*time.Second, time.Second).Should(Succeed())
		defer func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, app))).Should(Succeed())
		}()

		By("While the same definition forbids it everywhere else")
		err := k8sClient.Create(ctx, appWith(tenantNS, "tenant-app", "addon-only-web", 1))
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).Should(ContainSubstring(`quota for component type "addon-only-web"`))
	})

	It("counts traits against their own quota, apart from components", func() {
		By("A trait limited to two uses per namespace")
		trait := &v1beta1.TraitDefinition{
			TypeMeta:   metav1.TypeMeta{Kind: "TraitDefinition", APIVersion: "core.oam.dev/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{Name: "capped-label", Namespace: tenantNS},
			Spec: v1beta1.TraitDefinitionSpec{
				AppliesToWorkloads: []string{"deployments.apps"},
				Restrictions:       &common.DefinitionRestrictions{Quota: []common.NamespaceQuota{{Limit: int32p(2)}}},
				Schematic: &common.Schematic{CUE: &common.CUE{Template: `
patch: spec: template: metadata: labels: "e2e-quota": "yes"
`}},
			},
		}
		Expect(k8sClient.Create(ctx, trait)).Should(Succeed())
		Expect(k8sClient.Create(ctx, quotaCompDef(tenantNS, "plain-web"))).Should(Succeed())

		By("Two traits fill the namespace")
		Eventually(func() error {
			return k8sClient.Create(ctx, appWithTrait(tenantNS, "traited-1", "plain-web", 2))
		}, 15*time.Second, time.Second).Should(Succeed())

		By("A third is refused, at the trait's own field path")
		refusedEventually("traited-over", func(name string) *v1beta1.Application {
			return appWithTrait(tenantNS, name, "plain-web", 1)
		}, "traits[0].type", `quota for trait type "capped-label"`)
	})

	It("refuses a quota on a kind nothing counts", func() {
		policy := &v1beta1.PolicyDefinition{
			TypeMeta:   metav1.TypeMeta{Kind: "PolicyDefinition", APIVersion: "core.oam.dev/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{Name: "quota-on-policy", Namespace: tenantNS},
			Spec: v1beta1.PolicyDefinitionSpec{
				Restrictions: &common.DefinitionRestrictions{Quota: []common.NamespaceQuota{{Limit: int32p(2)}}},
				Schematic:    &common.Schematic{CUE: &common.CUE{Template: `output: {}`}},
			},
		}
		err := k8sClient.Create(ctx, policy)
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).Should(ContainSubstring("quota is not supported on PolicyDefinition"))
	})

	It("lifts every quota from a namespace annotated as exempt", func() {
		By("A definition that forbids its type outright")
		Expect(k8sClient.Create(ctx, quotaCompDef(tenantNS, "locked-web",
			common.NamespaceQuota{Limit: int32p(0)}))).Should(Succeed())

		By("It is refused while the namespace is ordinary")
		refusedEventually("blocked", func(name string) *v1beta1.Application {
			return appWith(tenantNS, name, "locked-web", 1)
		}, `quota for component type "locked-web"`)

		By("Annotating the namespace lifts it")
		Eventually(func() error {
			ns := &corev1.Namespace{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: tenantNS}, ns); err != nil {
				return err
			}
			if ns.Annotations == nil {
				ns.Annotations = map[string]string{}
			}
			ns.Annotations[oam.AnnotationQuotaExempt] = "true"
			return k8sClient.Update(ctx, ns)
		}, 15*time.Second, time.Second).Should(Succeed())

		Eventually(func() error {
			return k8sClient.Create(ctx, appWith(tenantNS, "allowed", "locked-web", 3))
		}, 30*time.Second, 2*time.Second).Should(Succeed())
	})

	It("refuses a quota whose default entry shadows the ones after it", func() {
		cd := quotaCompDef(tenantNS, "shadowed-quota",
			common.NamespaceQuota{Limit: int32p(5)},
			common.NamespaceQuota{Namespaces: []string{"tenant-*"}, Limit: int32p(1)})
		err := k8sClient.Create(ctx, cd)
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).Should(ContainSubstring("matches every namespace"))
	})

	It("refuses a quota that could never warn before it refuses", func() {
		cd := quotaCompDef(tenantNS, "malformed-quota",
			common.NamespaceQuota{Warn: int32p(9), Limit: int32p(3)})
		err := k8sClient.Create(ctx, cd)
		Expect(err).Should(HaveOccurred())
		Expect(strings.ToLower(err.Error())).Should(ContainSubstring("would never warn"))
	})
})
