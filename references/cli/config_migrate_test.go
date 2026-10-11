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

package cli

import (
	"bytes"
	"context"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cmd"
	"github.com/oam-dev/kubevela/pkg/config"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

var _ = Describe("Test the migrate commands of the config", func() {
	var arg cmd.Factory
	ctx := context.Background()
	const ns = "default"

	BeforeEach(func() {
		arg = cmd.NewTestFactory(cfg, k8sClient)
	})

	// seedLegacyConfigFrom writes config name in ns as a legacy Secret from the legacy
	// template tpl in ns, the way a pre-CRD CLI did.
	seedLegacyConfigFrom := func(tpl, name string) {
		inf := config.NewConfigFactory(k8sClient)
		item, err := inf.ParseConfig(ctx,
			config.NamespacedName{Name: tpl, Namespace: ns},
			config.Metadata{
				NamespacedName: config.NamespacedName{Name: name, Namespace: ns},
				Properties: map[string]interface{}{
					"registry": "docker.io",
					"auth":     map[string]interface{}{"username": "u", "password": "p"},
					"useHTTP":  true,
				},
			})
		Expect(err).Should(BeNil())
		Expect(inf.CreateOrUpdateConfig(ctx, item, ns)).Should(Succeed())
	}

	cleanup := func(templates, configs []string) {
		for _, name := range configs {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &configv1alpha1.Config{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name}}))).Should(Succeed())
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &v1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name}}))).Should(Succeed())
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &v1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name + "-properties"}}))).Should(Succeed())
		}
		for _, name := range templates {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &configv1alpha1.ConfigTemplate{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name}}))).Should(Succeed())
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: config.TemplateConfigMapNamePrefix + name}}))).Should(Succeed())
		}
	}

	run := func(group func(cmd.Factory, string, util.IOStreams) *cobra.Command, args ...string) (string, error) {
		buffer := bytes.NewBuffer(nil)
		c := group(arg, "", util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		c.SetArgs(args)
		err := c.Execute()
		return buffer.String(), err
	}
	lines := func(out string) []string {
		return strings.Split(strings.TrimRight(out, "\n"), "\n")
	}

	It("Test migrating a template adopts it and its configs, and the lists show each once", func() {
		seedLegacyTemplate(k8sClient, ns, "mig-cli")
		seedLegacyConfigFrom("mig-cli", "mig-cli-cfg")
		DeferCleanup(func() { cleanup([]string{"mig-cli"}, []string{"mig-cli-cfg"}) })
		var before v1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "mig-cli-cfg"}, &before)).Should(Succeed())

		out, err := run(TemplateCommandGroup, "migrate", "mig-cli", "-n", ns)
		Expect(err).Should(Succeed())
		got := lines(out)
		Expect(got).Should(HaveLen(4))
		Expect(got[0]).Should(MatchRegexp(`^KIND\s+NAME\s+NAMESPACE\s+RESULT\s+REASON`))
		Expect(got[1]).Should(MatchRegexp(`^ConfigMap\s+mig-cli\s+default\s+adopted\s+ConfigTemplate mig-cli created\s*$`))
		Expect(got[2]).Should(MatchRegexp(`^Secret\s+mig-cli-cfg\s+default\s+adopted\s+Config mig-cli-cfg created\s*$`))
		Expect(got[3]).Should(Equal("adopted 2, skipped 0, conflict 0, failed 0"))

		var after v1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "mig-cli-cfg"}, &after)).Should(Succeed())
		Expect(after.UID).Should(Equal(before.UID))
		Expect(metav1.GetControllerOf(&after)).ShouldNot(BeNil())
		Expect(metav1.GetControllerOf(&after).Kind).Should(Equal("Config"))

		By("a second run skips everything")
		out, err = run(TemplateCommandGroup, "migrate", "mig-cli", "-n", ns)
		Expect(err).Should(Succeed())
		Expect(out).Should(ContainSubstring("adopted 0, skipped 2, conflict 0, failed 0"))
		Expect(out).Should(MatchRegexp(`Secret\s+mig-cli-cfg\s+default\s+skipped\s+already migrated`))

		By("the merged template list has one row, legacy mode marks the ConfigMap migrated")
		out, err = run(TemplateCommandGroup, "list", "-n", ns)
		Expect(err).Should(Succeed())
		Expect(strings.Count(out, "mig-cli")).Should(Equal(1))
		Expect(out).Should(MatchRegexp(`(?m)^mig-cli\s.*\scrd\s+\d{4}-`))
		out, err = run(TemplateCommandGroup, "list", "-n", ns, "--config-mode", "legacy")
		Expect(err).Should(Succeed())
		Expect(out).Should(MatchRegexp(`(?m)^mig-cli\s.*\slegacy \(migrated\)\s+\d{4}-`))

		By("the config list shows the config once, as crd")
		out, err = run(ConfigCommandGroup, "list", "-n", ns, "-t", "mig-cli")
		Expect(err).Should(Succeed())
		Expect(strings.Count(out, "mig-cli-cfg")).Should(Equal(1))
		Expect(out).Should(MatchRegexp(`(?m)^mig-cli-cfg\s+.*\scrd\s`))

		By("apply on the migrated name prints no shadow note")
		out, err = run(TemplateCommandGroup, "apply", "-f", "./test-data/config-templates/image-registry.cue", "--name", "mig-cli", "-n", ns)
		Expect(err).Should(Succeed())
		Expect(out).Should(Equal("the config template mig-cli applied successfully\n"))

		By("once the CR is gone the ConfigMap is plain legacy again")
		Expect(k8sClient.Delete(ctx, &configv1alpha1.ConfigTemplate{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "mig-cli"}})).Should(Succeed())
		out, err = run(TemplateCommandGroup, "list", "-n", ns, "--config-mode", "legacy")
		Expect(err).Should(Succeed())
		Expect(out).Should(MatchRegexp(`(?m)^mig-cli\s.*\slegacy\s+\d{4}-`))
		out, err = run(TemplateCommandGroup, "list", "-n", ns)
		Expect(err).Should(Succeed())
		Expect(out).Should(MatchRegexp(`(?m)^mig-cli\s.*\slegacy\s+\d{4}-`))
	})

	It("Test migrate --dry-run prints the CRs and writes nothing", func() {
		seedLegacyTemplate(k8sClient, ns, "mig-dry")
		seedLegacyConfigFrom("mig-dry", "mig-dry-cfg")
		DeferCleanup(func() { cleanup([]string{"mig-dry"}, []string{"mig-dry-cfg"}) })

		out, err := run(TemplateCommandGroup, "migrate", "mig-dry", "-n", ns, "--dry-run")
		Expect(err).Should(Succeed())
		Expect(strings.Count(out, "---\n")).Should(Equal(2))
		Expect(out).Should(ContainSubstring("kind: ConfigTemplate\n"))
		Expect(out).Should(ContainSubstring("kind: Config\n"))
		Expect(out).Should(ContainSubstring("config.oam.dev/created-by: migration"))
		Expect(out).Should(MatchRegexp(`ConfigMap\s+mig-dry\s+default\s+would adopt\s+ConfigTemplate mig-dry created`))
		Expect(out).Should(MatchRegexp(`Secret\s+mig-dry-cfg\s+default\s+would adopt\s+Config mig-dry-cfg created`))
		Expect(out).Should(HaveSuffix("adopted 2, skipped 0, conflict 0, failed 0\n"))
		Expect(client.IgnoreNotFound(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "mig-dry"}, &configv1alpha1.ConfigTemplate{}))).Should(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "mig-dry"}, &configv1alpha1.ConfigTemplate{})).ShouldNot(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "mig-dry-cfg"}, &configv1alpha1.Config{})).ShouldNot(Succeed())
	})

	It("Test migrate exits non-zero on a conflict", func() {
		seedLegacyTemplate(k8sClient, ns, "mig-clash")
		seedLegacyConfigFrom("mig-clash", "mig-clash-cfg")
		DeferCleanup(func() { cleanup([]string{"mig-clash"}, []string{"mig-clash-cfg"}) })
		handmade := &configv1alpha1.Config{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "mig-clash-cfg"}, Spec: configv1alpha1.ConfigSpec{Alias: "mine"}}
		Expect(k8sClient.Create(ctx, handmade)).Should(Succeed())

		out, err := run(TemplateCommandGroup, "migrate", "mig-clash", "-n", ns)
		Expect(err).Should(MatchError("1 object(s) in conflict; see the table above"))
		Expect(out).Should(MatchRegexp(`Secret\s+mig-clash-cfg\s+default\s+conflict\s+Config mig-clash-cfg exists and was not created by migration`))
		Expect(out).Should(ContainSubstring("adopted 1, skipped 0, conflict 1, failed 0"))

		By("the Events are on the API server before the command returns")
		var events v1.EventList
		Expect(k8sClient.List(ctx, &events, client.InNamespace(ns), client.MatchingFields{"reason": "MigrationConflict"})).Should(Succeed())
		var conflictMessages []string
		for _, e := range events.Items {
			if e.InvolvedObject.Kind == "Secret" && e.InvolvedObject.Name == "mig-clash-cfg" {
				conflictMessages = append(conflictMessages, e.Type+" "+e.Source.Component+" "+e.Message)
			}
		}
		Expect(conflictMessages).Should(ConsistOf("Warning vela-cli Config mig-clash-cfg exists and was not created by migration"))
		Expect(k8sClient.List(ctx, &events, client.InNamespace(ns), client.MatchingFields{"reason": "MigrationAdopted"})).Should(Succeed())
		var adopted []string
		for _, e := range events.Items {
			if e.InvolvedObject.Kind == "ConfigMap" && e.InvolvedObject.Name == config.TemplateConfigMapNamePrefix+"mig-clash" {
				adopted = append(adopted, e.Message)
			}
		}
		Expect(adopted).Should(ConsistOf("ConfigTemplate mig-clash created"))
	})

	It("Test migrate argument rules and not-found", func() {
		_, err := run(TemplateCommandGroup, "migrate", "x", "--all", "-n", ns)
		Expect(err).Should(MatchError("specify a template name or --all, not both"))
		_, err = run(TemplateCommandGroup, "migrate", "-n", ns)
		Expect(err).Should(MatchError("specify a template name or --all"))
		_, err = run(TemplateCommandGroup, "migrate", "no-such-template", "-n", ns)
		Expect(err).Should(MatchError("the config template no-such-template not found"))
		_, err = run(ConfigCommandGroup, "migrate", "x", "--all", "-n", ns)
		Expect(err).Should(MatchError("specify a config name or --all, not both"))
		_, err = run(ConfigCommandGroup, "migrate", "-n", ns)
		Expect(err).Should(MatchError("specify a config name or --all"))
		_, err = run(ConfigCommandGroup, "migrate", "nope", "-n", ns)
		Expect(err).Should(MatchError("the config nope not found"))
	})

	It("Test each migrate command probes only the CRDs it writes", func() {
		templateOnly := cmd.NewTestFactory(cfg, fakeClientServing(configv1alpha1.ConfigTemplateGroupVersionKind))
		configOnly := cmd.NewTestFactory(cfg, fakeClientServing(configv1alpha1.ConfigGroupVersionKind))
		neither := cmd.NewTestFactory(cfg, fakeClientServing())
		execute := func(group func(cmd.Factory, string, util.IOStreams) *cobra.Command, f cmd.Factory) error {
			c := group(f, "", util.IOStreams{In: os.Stdin, Out: bytes.NewBuffer(nil), ErrOut: bytes.NewBuffer(nil)})
			c.SetArgs([]string{"migrate", "x", "-n", ns})
			return c.Execute()
		}
		// template migration writes both kinds
		Expect(execute(TemplateCommandGroup, neither)).Should(MatchError(errConfigTemplateCRDMissing))
		Expect(execute(TemplateCommandGroup, templateOnly)).Should(MatchError(errConfigCRDMissing))
		Expect(execute(TemplateCommandGroup, configOnly)).Should(MatchError(errConfigTemplateCRDMissing))
		// config migration writes Configs only, so a missing ConfigTemplate CRD does not block it
		Expect(execute(ConfigCommandGroup, neither)).Should(MatchError(errConfigCRDMissing))
		Expect(execute(ConfigCommandGroup, templateOnly)).Should(MatchError(errConfigCRDMissing))
		Expect(execute(ConfigCommandGroup, configOnly)).Should(MatchError("the config x not found"))
	})

	It("Test config migrate adopts one config while its template stays legacy, then --all the rest", func() {
		seedLegacyTemplate(k8sClient, ns, "mig-stay")
		seedLegacyConfigFrom("mig-stay", "mig-first")
		seedLegacyConfigFrom("mig-stay", "mig-second")
		DeferCleanup(func() { cleanup([]string{"mig-stay"}, []string{"mig-first", "mig-second"}) })

		out, err := run(ConfigCommandGroup, "migrate", "mig-first", "-n", ns)
		Expect(err).Should(Succeed())
		got := lines(out)
		Expect(got).Should(HaveLen(3))
		Expect(got[1]).Should(MatchRegexp(`^Secret\s+mig-first\s+default\s+adopted\s+Config mig-first created\s*$`))
		Expect(got[2]).Should(Equal("adopted 1, skipped 0, conflict 0, failed 0"))
		var cfgCR configv1alpha1.Config
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "mig-first"}, &cfgCR)).Should(Succeed())
		Expect(cfgCR.Spec.TemplateRef).Should(Equal(&configv1alpha1.ConfigTemplateReference{Name: "mig-stay", Namespace: ns}))
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "mig-stay"}, &configv1alpha1.ConfigTemplate{})).ShouldNot(Succeed())

		out, err = run(TemplateCommandGroup, "list", "-n", ns, "--config-mode", "legacy")
		Expect(err).Should(Succeed())
		Expect(out).Should(MatchRegexp(`(?m)^mig-stay\s.*\slegacy\s+\d{4}-`))

		out, err = run(ConfigCommandGroup, "migrate", "--all", "-n", ns, "--dry-run")
		Expect(err).Should(Succeed())
		Expect(out).Should(MatchRegexp(`Secret\s+mig-first\s+default\s+skipped\s+already migrated`))
		Expect(out).Should(MatchRegexp(`Secret\s+mig-second\s+default\s+would adopt\s+Config mig-second created`))

		out, err = run(ConfigCommandGroup, "migrate", "--all", "-n", ns)
		Expect(err).Should(Succeed())
		Expect(out).Should(MatchRegexp(`Secret\s+mig-second\s+default\s+adopted\s+Config mig-second created`))
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "mig-second"}, &configv1alpha1.Config{})).Should(Succeed())
	})

	It("Test a ConfigMap adopted by a generated ConfigTemplate counts as migrated in both list modes", func() {
		seedLegacyTemplate(k8sClient, ns, "mig-gen")
		DeferCleanup(func() { cleanup([]string{"mig-gen"}, nil) })
		out, err := run(TemplateCommandGroup, "migrate", "mig-gen", "-n", ns)
		Expect(err).Should(Succeed())
		Expect(out).Should(ContainSubstring("adopted 1, skipped 0"))
		// a SourceDefinition takes the template over: the CR is now a generated one,
		// hidden from the CR rows like Factory.ListTemplates hides it
		var ct configv1alpha1.ConfigTemplate
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "mig-gen"}, &ct)).Should(Succeed())
		ct.Labels[types.LabelSourceDefinitionName] = "some-source"
		Expect(k8sClient.Update(ctx, &ct)).Should(Succeed())

		out, err = run(TemplateCommandGroup, "list", "-n", ns)
		Expect(err).Should(Succeed())
		Expect(out).ShouldNot(ContainSubstring("mig-gen"), "merged: the CR row is hidden and the legacy row is migrated, not plain legacy")
		out, err = run(TemplateCommandGroup, "list", "-n", ns, "--config-mode", "legacy")
		Expect(err).Should(Succeed())
		Expect(out).Should(MatchRegexp(`(?m)^mig-gen\s.*\slegacy \(migrated\)\s+\d{4}-`))
		out, err = run(TemplateCommandGroup, "list", "-n", ns, "--config-mode", "crd")
		Expect(err).Should(Succeed())
		Expect(out).ShouldNot(ContainSubstring("mig-gen"))
	})

	It("Test config-template migrate --all adopts a whole namespace, dry run first", func() {
		seedLegacyTemplate(k8sClient, ns, "mig-all-a")
		seedLegacyTemplate(k8sClient, ns, "mig-all-b")
		seedLegacyConfigFrom("mig-all-a", "mig-all-a-cfg")
		seedLegacyConfigFrom("mig-all-b", "mig-all-b-cfg")
		inf := config.NewConfigFactory(k8sClient)
		noTpl, err := inf.ParseConfig(ctx, config.NamespacedName{}, config.Metadata{
			NamespacedName: config.NamespacedName{Name: "mig-all-none", Namespace: ns},
			Properties:     map[string]interface{}{"key": "value"},
		})
		Expect(err).Should(Succeed())
		Expect(inf.CreateOrUpdateConfig(ctx, noTpl, ns)).Should(Succeed())
		DeferCleanup(func() {
			cleanup([]string{"mig-all-a", "mig-all-b"}, []string{"mig-all-a-cfg", "mig-all-b-cfg", "mig-all-none"})
		})

		out, err := run(TemplateCommandGroup, "migrate", "--all", "-n", ns, "--dry-run")
		Expect(err).Should(Succeed())
		Expect(strings.Count(out, "would adopt")).Should(BeNumerically(">=", 5))
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "mig-all-a"}, &configv1alpha1.ConfigTemplate{})).ShouldNot(Succeed())

		out, err = run(TemplateCommandGroup, "migrate", "--all", "-n", ns)
		Expect(err).Should(Succeed())
		got := lines(out)
		// templates in name order, each followed by its configs; then what is left in the namespace
		Expect(got[1]).Should(MatchRegexp(`^ConfigMap\s+mig-all-a\s+default\s+adopted\s+ConfigTemplate mig-all-a created`))
		Expect(got[2]).Should(MatchRegexp(`^Secret\s+mig-all-a-cfg\s+default\s+adopted\s+Config mig-all-a-cfg created`))
		Expect(got[3]).Should(MatchRegexp(`^ConfigMap\s+mig-all-b\s+default\s+adopted\s+ConfigTemplate mig-all-b created`))
		Expect(got[4]).Should(MatchRegexp(`^Secret\s+mig-all-b-cfg\s+default\s+adopted\s+Config mig-all-b-cfg created`))
		Expect(out).Should(MatchRegexp(`Secret\s+mig-all-none\s+default\s+adopted\s+Config mig-all-none created`))
		Expect(out).Should(MatchRegexp(`adopted [5-9][0-9]*, skipped \d+, conflict 0, failed 0`))
		var none configv1alpha1.Config
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "mig-all-none"}, &none)).Should(Succeed())
		Expect(none.Spec.TemplateRef).Should(BeNil())

		out, err = run(TemplateCommandGroup, "migrate", "--all", "-n", ns)
		Expect(err).Should(Succeed())
		Expect(out).Should(MatchRegexp(`adopted 0, skipped [5-9][0-9]*, conflict 0, failed 0`))
		Expect(out).ShouldNot(ContainSubstring(" adopted "))
	})

	It("Test the migrate commands are registered", func() {
		for _, group := range []func(cmd.Factory, string, util.IOStreams) *cobra.Command{TemplateCommandGroup, ConfigCommandGroup} {
			c := group(arg, "", util.IOStreams{In: os.Stdin, Out: bytes.NewBuffer(nil), ErrOut: bytes.NewBuffer(nil)})
			names := []string{}
			for _, sub := range c.Commands() {
				names = append(names, sub.Name())
			}
			Expect(names).Should(ContainElement("migrate"))
		}
		Expect(types.DefaultKubeVelaNS).Should(Equal("vela-system"))
	})
})
