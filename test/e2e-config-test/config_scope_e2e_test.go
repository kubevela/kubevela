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

package config_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/yaml"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
)

// The scope rules of issue #7410 section 6, as a user bound to one namespace. The admission
// webhooks are on in this suite (the CRD specs rely on them); the AuthenticateApplication
// gate is off, so the create-config spec exercises the answer for an Application with no user.
// The gate-on case, where the step reviews the template for the Application's user before it
// writes and the write is that user's, needs a second install with the gate on and is covered
// by the provider suite and by manual testing.
var _ = Describe("Template scope and cross-namespace access as a non-admin user", Label("core-config"), func() {
	ctx := context.Background()

	var home, other string // alice's namespace, and another team's
	var alice string
	var asAlice client.Client

	BeforeEach(func() {
		home = randomNamespaceName("config-scope-home")
		other = randomNamespaceName("config-scope-other")
		for _, name := range []string{home, other} {
			ns := createNamespace(ctx, name)
			DeferCleanup(func() {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationBackground)))).To(Succeed())
			})
		}

		By("alice: configs and templates in her namespace, nothing anywhere else")
		alice = home + "-alice"
		role := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{Name: "scope-user", Namespace: home},
			Rules: []rbacv1.PolicyRule{
				{APIGroups: []string{configv1alpha1.Group}, Resources: []string{"configs", "configtemplates"}, Verbs: []string{"get", "list", "create", "update", "patch", "delete"}},
				{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get", "list"}},
			},
		}
		Expect(k8sClient.Create(ctx, role)).Should(Succeed())
		Expect(k8sClient.Create(ctx, bindingFor(home, "scope-user", alice))).Should(Succeed())

		cfg := config.GetConfigOrDie()
		cfg.Impersonate = rest.ImpersonationConfig{UserName: alice, Groups: []string{"scope-users"}}
		var err error
		asAlice, err = client.New(cfg, client.Options{Scheme: support.Scheme})
		Expect(err).ToNot(HaveOccurred())
	})

	// availableTemplate writes a template as the admin and waits for the controller.
	availableTemplate := func(name, namespace string, scope configv1alpha1.ConfigTemplateScope) *configv1alpha1.ConfigTemplate {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: configE2ECUETemplate, Scope: scope},
		}
		Expect(k8sClient.Create(ctx, ct)).Should(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, ct))).Should(Succeed()) })
		Eventually(func() configv1alpha1.ConfigTemplatePhase {
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(ct), ct)
			return ct.Status.Phase
		}, configE2ETimeout, configE2EPollInterval).Should(Equal(configv1alpha1.ConfigTemplatePhaseAvailable))
		return ct
	}

	configFrom := func(name string, tmpl *configv1alpha1.ConfigTemplate) *configv1alpha1.Config {
		return &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: home},
			Spec: configv1alpha1.ConfigSpec{
				TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: tmpl.Name, Namespace: tmpl.Namespace},
				Properties:  configE2ERawExtension(map[string]string{"username": alice}),
			},
		}
	}

	eventuallyAvailable := func(cfg *configv1alpha1.Config) {
		Eventually(func() configv1alpha1.ConfigPhase {
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), cfg)
			return cfg.Status.Phase
		}, configE2ETimeout, configE2EPollInterval).Should(Equal(configv1alpha1.ConfigPhaseAvailable))
	}

	It("a system template in vela-system is usable from any namespace, with no grant on it", func() {
		shared := availableTemplate(home+"-shared", types.DefaultKubeVelaNS, configv1alpha1.ConfigTemplateScopeSystem)
		cfg := configFrom("from-shared", shared)
		Expect(asAlice.Create(ctx, cfg)).Should(Succeed(), "alice has nothing in vela-system and needs nothing")
		eventuallyAvailable(cfg)
	})

	It("a namespace-scoped template in another namespace needs the author to be able to get it", func() {
		theirs := availableTemplate("theirs", other, configv1alpha1.ConfigTemplateScopeNamespace)
		cfg := configFrom("from-theirs", theirs)

		By("denied at admission, as alice")
		err := asAlice.Create(ctx, cfg)
		Expect(apierrors.IsForbidden(err)).Should(BeTrue(), fmt.Sprintf("%v", err))
		Expect(err.Error()).Should(ContainSubstring(fmt.Sprintf("config template %s/theirs is namespace-scoped: a config in %s may use it only if its author can get it", other, home)))
		Expect(err.Error()).Should(ContainSubstring(alice + " may not"))
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(cfg), &configv1alpha1.Config{}))).Should(BeTrue(), "nothing written")

		By("the grant the message names: get on that ConfigTemplate")
		Expect(k8sClient.Create(ctx, &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{Name: "read-theirs", Namespace: other},
			Rules:      []rbacv1.PolicyRule{{APIGroups: []string{configv1alpha1.Group}, Resources: []string{"configtemplates"}, ResourceNames: []string{"theirs"}, Verbs: []string{"get"}}},
		})).Should(Succeed())
		Expect(k8sClient.Create(ctx, bindingFor(other, "read-theirs", alice))).Should(Succeed())
		Eventually(func() error { return asAlice.Create(ctx, configFrom("from-theirs", theirs)) }, configE2ETimeout, configE2EPollInterval).Should(Succeed(), "RBAC takes a moment to be seen")
		eventuallyAvailable(cfg)
	})

	It("a system template cannot be created outside vela-system", func() {
		misplaced := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "misplaced", Namespace: home},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: configE2ECUETemplate, Scope: configv1alpha1.ConfigTemplateScopeSystem},
		}
		err := k8sClient.Create(ctx, misplaced)
		Expect(apierrors.IsForbidden(err)).Should(BeTrue(), fmt.Sprintf("%v", err))
		Expect(err.Error()).Should(ContainSubstring(fmt.Sprintf("config template %s/misplaced: scope system is only valid in vela-system", home)))

		By("nor turned into one on update")
		mine := availableTemplate("mine", home, configv1alpha1.ConfigTemplateScopeNamespace)
		mine.Spec.Scope = configv1alpha1.ConfigTemplateScopeSystem
		err = asAlice.Update(ctx, mine)
		Expect(apierrors.IsForbidden(err)).Should(BeTrue(), fmt.Sprintf("%v", err))
		Expect(err.Error()).Should(ContainSubstring("scope system is only valid in vela-system"))
	})

	It("create-config without a user applies the rule itself: a system template is fine, a namespace-scoped one elsewhere is refused before anything is written", func() {
		shared := availableTemplate(home+"-step-shared", types.DefaultKubeVelaNS, configv1alpha1.ConfigTemplateScopeSystem)
		theirs := availableTemplate("step-theirs", other, configv1alpha1.ConfigTemplateScopeNamespace)

		By("from the other team's namespace-scoped template: the step's refusal names the gate and the grant")
		refused := applyStepApplication(ctx, home, "step-from-theirs", "cfg-from-theirs", other+"/"+theirs.Name)
		Eventually(func() string {
			var got v1beta1.Application
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(refused), &got); err != nil || got.Status.Workflow == nil {
				return ""
			}
			for _, step := range got.Status.Workflow.Steps {
				if step.Name == "write" && step.Phase == "failed" {
					return step.Message
				}
			}
			return ""
		}, 2*time.Minute, configE2EPollInterval).Should(SatisfyAll(
			ContainSubstring(fmt.Sprintf("config template %s/%s is namespace-scoped", other, theirs.Name)),
			ContainSubstring("the AuthenticateApplication feature gate is off"),
		))
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Namespace: home, Name: "cfg-from-theirs"}, &configv1alpha1.Config{}))).Should(BeTrue(), "nothing written")

		By("from the shared system template: written and rendered")
		app := applyStepApplication(ctx, home, "step-from-shared", "cfg-from-shared", types.DefaultKubeVelaNS+"/"+shared.Name)
		Eventually(func() common.ApplicationPhase {
			var got v1beta1.Application
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(app), &got)
			return got.Status.Phase
		}, 2*time.Minute, configE2EPollInterval).Should(Equal(common.ApplicationRunning))
		cfg := &configv1alpha1.Config{ObjectMeta: metav1.ObjectMeta{Namespace: home, Name: "cfg-from-shared"}}
		eventuallyAvailable(cfg)
	})
})

func bindingFor(namespace, role, user string) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: role + "-" + user, Namespace: namespace},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.UserKind, Name: user, APIGroup: rbacv1.GroupName}},
		RoleRef:    rbacv1.RoleRef{Kind: "Role", Name: role, APIGroup: rbacv1.GroupName},
	}
}

// applyStepApplication is an Application with one create-config step, applied as the admin
// (with the gate off every Application runs as the controller) and deleted with the spec.
func applyStepApplication(ctx context.Context, namespace, name, configName, template string) *v1beta1.Application {
	manifest := fmt.Sprintf(`
kind: Application
apiVersion: core.oam.dev/v1beta1
metadata:
  name: %s
  namespace: %s
spec:
  components: []
  workflow:
    steps:
    - name: write
      type: create-config
      properties:
        name: %s
        namespace: %s
        template: %s
        config:
          username: step
`, name, namespace, configName, namespace, template)
	var app v1beta1.Application
	Expect(yaml.Unmarshal([]byte(manifest), &app)).Should(Succeed())
	Expect(k8sClient.Create(ctx, &app)).Should(Succeed())
	DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &app))).Should(Succeed()) })
	return &app
}
