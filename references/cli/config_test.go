/*
Copyright 2022 The KubeVela Authors.

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
	"io"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cmd"
	"github.com/oam-dev/kubevela/pkg/config"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

var _ = Describe("Test the commands of the config", func() {
	var arg cmd.Factory
	BeforeEach(func() {
		arg = cmd.NewTestFactory(cfg, k8sClient)
	})

	It("Test apply a template", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := TemplateCommandGroup(arg, "", util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"apply", "-f", "./test-data/config-templates/image-registry.cue", "--name", "test"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(buffer.String()).Should(Equal("the config template test applied successfully\n"))
	})

	It("Test apply a new template", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := TemplateCommandGroup(arg, "", util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"apply", "-f", "./test-data/config-templates/image-registry.cue", "--name", "test2"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(buffer.String()).Should(Equal("the config template test2 applied successfully\n"))
	})

	It("Test applying a template notes a legacy ConfigMap it shadows", func() {
		legacy := &v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: types.DefaultKubeVelaNS, Name: config.TemplateConfigMapNamePrefix + "shadowed"}}
		Expect(k8sClient.Create(context.TODO(), legacy)).Should(Succeed())
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(context.TODO(), legacy))).Should(Succeed())
			Expect(client.IgnoreNotFound(k8sClient.Delete(context.TODO(), &configv1alpha1.ConfigTemplate{ObjectMeta: metav1.ObjectMeta{Namespace: types.DefaultKubeVelaNS, Name: "shadowed"}}))).Should(Succeed())
		})

		buffer := bytes.NewBuffer(nil)
		cmd := TemplateCommandGroup(arg, "", util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"apply", "-f", "./test-data/config-templates/image-registry.cue", "--name", "shadowed"})
		Expect(cmd.Execute()).Should(Succeed())
		Expect(buffer.String()).Should(Equal("the config template shadowed applied successfully\n" +
			"note: legacy template ConfigMap config-template-shadowed in vela-system is now shadowed by the ConfigTemplate CR; run \"vela config-template migrate shadowed -n vela-system\" to adopt it\n"))
	})

	It("Test applying a template fails when the ConfigTemplate CRD is not installed", func() {
		noCRD := cmd.NewTestFactory(cfg, fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).Build())
		buffer := bytes.NewBuffer(nil)
		cmd := TemplateCommandGroup(noCRD, "", util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"apply", "-f", "./test-data/config-templates/image-registry.cue", "--name", "no-crd"})
		Expect(cmd.Execute()).Should(MatchError(errConfigTemplateCRDMissing))
	})

	It("Test list the templates", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := TemplateCommandGroup(arg, "", util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"list", "-A"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(strings.Contains(buffer.String(), "vela-system")).Should(Equal(true))
		Expect(strings.Contains(buffer.String(), "test")).Should(Equal(true))
		Expect(strings.Contains(buffer.String(), "\n")).Should(Equal(true))
	})

	It("Test listing templates merges both backends and marks a shadowed legacy one", func() {
		inf := config.NewConfigFactory(k8sClient)
		body, err := os.ReadFile("./test-data/config-templates/image-registry.cue")
		Expect(err).Should(BeNil())
		for _, name := range []string{"legacy-only", "shadow-me"} {
			t, err := inf.ParseTemplate(context.TODO(), name, body)
			Expect(err).Should(BeNil())
			Expect(inf.CreateOrUpdateConfigTemplate(context.TODO(), "default", t)).Should(Succeed())
		}
		apply := TemplateCommandGroup(arg, "", util.IOStreams{In: os.Stdin, Out: io.Discard, ErrOut: io.Discard})
		apply.SetArgs([]string{"apply", "-f", "./test-data/config-templates/image-registry.cue", "--name", "shadow-me", "-n", "default"})
		Expect(apply.Execute()).Should(Succeed())
		DeferCleanup(func() {
			for _, name := range []string{"legacy-only", "shadow-me"} {
				cm := &v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: config.TemplateConfigMapNamePrefix + name}}
				Expect(client.IgnoreNotFound(k8sClient.Delete(context.TODO(), cm))).Should(Succeed())
			}
			ct := &configv1alpha1.ConfigTemplate{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "shadow-me"}}
			Expect(client.IgnoreNotFound(k8sClient.Delete(context.TODO(), ct))).Should(Succeed())
		})

		list := func(extra ...string) []string {
			buffer := bytes.NewBuffer(nil)
			cmd := TemplateCommandGroup(arg, "", util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
			cmd.SetArgs(append([]string{"list", "-n", "default"}, extra...))
			Expect(cmd.Execute()).Should(Succeed())
			return strings.Split(strings.TrimRight(buffer.String(), "\n"), "\n")
		}

		// merged: CR rows first, then legacy rows, the legacy twin marked shadowed
		lines := list()
		Expect(lines).Should(HaveLen(4))
		Expect(lines[0]).Should(MatchRegexp(`^NAME\s+ALIAS\s+SCOPE\s+SENSITIVE\s+SOURCE\s+CREATED-TIME`))
		Expect(lines[1]).Should(MatchRegexp(`^shadow-me\s.*\scrd\s+\d{4}-`))
		Expect(lines[2]).Should(MatchRegexp(`^legacy-only\s.*\slegacy\s+\d{4}-`))
		Expect(lines[3]).Should(MatchRegexp(`^shadow-me\s.*\slegacy \(shadowed\)\s+\d{4}-`))

		lines = list("--config-mode", "crd")
		Expect(lines).Should(HaveLen(2))
		Expect(lines[1]).Should(MatchRegexp(`^shadow-me\s.*\scrd\s+\d{4}-`))

		// legacy only: no CR rows, so nothing to shadow
		lines = list("--config-mode", "legacy")
		Expect(lines).Should(HaveLen(3))
		Expect(lines[1]).Should(MatchRegexp(`^legacy-only\s.*\slegacy\s+\d{4}-`))
		Expect(lines[2]).Should(MatchRegexp(`^shadow-me\s.*\slegacy\s+\d{4}-`))
	})

	It("Test listing templates rejects an unknown --config-mode before any API call", func() {
		cmd := TemplateCommandGroup(arg, "", util.IOStreams{In: os.Stdin, Out: io.Discard, ErrOut: io.Discard})
		cmd.SetArgs([]string{"list", "--config-mode", "auto"})
		Expect(cmd.Execute()).Should(MatchError(`invalid --config-mode "auto": use legacy or crd`))
	})

	It("Test listing templates on a cluster without the CRD shows legacy rows only", func() {
		noCRDClient := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).Build()
		inf := config.NewConfigFactory(noCRDClient)
		body, err := os.ReadFile("./test-data/config-templates/image-registry.cue")
		Expect(err).Should(BeNil())
		t, err := inf.ParseTemplate(context.TODO(), "only-legacy-here", body)
		Expect(err).Should(BeNil())
		Expect(inf.CreateOrUpdateConfigTemplate(context.TODO(), "default", t)).Should(Succeed())

		buffer := bytes.NewBuffer(nil)
		cmd := TemplateCommandGroup(cmd.NewTestFactory(cfg, noCRDClient), "", util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"list", "-n", "default"})
		Expect(cmd.Execute()).Should(Succeed())
		lines := strings.Split(strings.TrimRight(buffer.String(), "\n"), "\n")
		Expect(lines).Should(HaveLen(2))
		Expect(lines[1]).Should(MatchRegexp(`^only-legacy-here\s.*\slegacy\s+\d{4}-`))
	})

	It("Test --config-mode is rejected on every command but list", func() {
		for _, args := range [][]string{
			{"config-template", "apply", "-f", "./test-data/config-templates/image-registry.cue"},
			{"config-template", "show", "test"},
			{"config-template", "delete", "test"},
			{"config", "create", "x", "-t", "test"},
			{"config", "delete", "test"},
			{"config", "distribute", "test"},
		} {
			root := NewCommandWithIOStreams(util.IOStreams{In: os.Stdin, Out: io.Discard, ErrOut: io.Discard})
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(append(args, "--config-mode", "crd"))
			Expect(root.Execute()).Should(MatchError(ContainSubstring("unknown flag: --config-mode")), strings.Join(args, " "))
		}
	})

	It("Test show the templates", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := TemplateCommandGroup(arg, "", util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"show", "test2"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(line(buffer.String())).Should(Equal(24))
	})

	It("Test create the config with the args", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := ConfigCommandGroup(arg, "", util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"create", "test", "--template=test", "registry=test.kubevela.net", "auth.username=yueda", "auth.password=yueda123", "useHTTP=true"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(buffer.String()).Should(Equal("the config test applied successfully\n"))
	})

	It("Test create the config with the file", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := ConfigCommandGroup(arg, "", util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"create", "testfile", "--template=test2", "--namespace=default", "-f", "./test-data/config/registry.yaml"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(buffer.String()).Should(Equal("the config testfile applied successfully\n"))
	})

	It("Test create the config without the template", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := ConfigCommandGroup(arg, "", util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"create", "without-template", "--namespace=default", "-f", "./test-data/config/registry.yaml"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(buffer.String()).Should(Equal("the config without-template applied successfully\n"))
	})

	It("Test creating and distributing the config", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := ConfigCommandGroup(arg, "", util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"create", "distribution", "--namespace=default", "-f", "./test-data/config/registry.yaml", "--target", "test"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(buffer.String()).Should(Equal("the config distribution applied successfully\n"))
	})

	It("Test list the configs", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := ConfigCommandGroup(arg, "", util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"list", "-A"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(line(buffer.String())).Should(Equal(5))
	})

	It("Test list the configs with the namespace filter", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := ConfigCommandGroup(arg, "", util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"list", "-n", "default"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(line(buffer.String())).Should(Equal(4))
	})

	It("Test list the configs with the template filter", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := ConfigCommandGroup(arg, "", util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"list", "-A", "-t", "test2"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(line(buffer.String())).Should(Equal(2))
	})

	It("Test listing configs merges both backends with SOURCE and an empty legacy PHASE", func() {
		inf := config.NewConfigFactory(k8sClient)
		item, err := inf.ParseConfig(context.TODO(),
			config.NamespacedName{Name: "test2", Namespace: types.DefaultKubeVelaNS},
			config.Metadata{
				NamespacedName: config.NamespacedName{Name: "legacy-cfg", Namespace: "default"},
				Properties: map[string]interface{}{
					"registry": "docker.io",
					"auth":     map[string]interface{}{"username": "u", "password": "p"},
					"useHTTP":  true,
				},
			})
		Expect(err).Should(BeNil())
		Expect(inf.CreateOrUpdateConfig(context.TODO(), item, "default")).Should(Succeed())
		DeferCleanup(func() {
			secret := &v1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "legacy-cfg"}}
			Expect(client.IgnoreNotFound(k8sClient.Delete(context.TODO(), secret))).Should(Succeed())
		})

		list := func(extra ...string) []string {
			buffer := bytes.NewBuffer(nil)
			cmd := ConfigCommandGroup(arg, "", util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
			cmd.SetArgs(append([]string{"list", "-n", "default"}, extra...))
			Expect(cmd.Execute()).Should(Succeed())
			return strings.Split(strings.TrimRight(buffer.String(), "\n"), "\n")
		}

		// merged: the three CR configs created above, then the legacy Secret
		lines := list()
		Expect(lines).Should(HaveLen(5))
		Expect(lines[0]).Should(MatchRegexp(`^NAME\s+ALIAS\s+SOURCE\s+PHASE\s+DISTRIBUTION\s+TEMPLATE\s+CREATED-TIME\s+DESCRIPTION`))
		for _, l := range lines[1:4] {
			Expect(l).Should(MatchRegexp(`\scrd\s`))
		}
		// alias, PHASE and DISTRIBUTION are all empty, so SOURCE is followed directly by TEMPLATE
		Expect(lines[4]).Should(MatchRegexp(`^legacy-cfg\s+legacy\s+vela-system/test2\s+\d{4}-`))

		Expect(list("--config-mode", "crd")).Should(HaveLen(4))
		Expect(list("--config-mode", "legacy")).Should(HaveLen(2))
		// the template filter applies to both backends: testfile (CR) and legacy-cfg
		Expect(list("-t", "test2")).Should(HaveLen(3))
	})

	It("Test dry run the config", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := ConfigCommandGroup(arg, "", util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"create", "testfile", "--template=test", "-f", "./test-data/config/registry.yaml", "--dry-run"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		var secret v1.Secret
		Expect(yaml.Unmarshal(buffer.Bytes(), &secret)).Should(BeNil())
		Expect(secret.Name).Should(Equal("testfile"))
		Expect(secret.Labels["config.oam.dev/type"]).Should(Equal("test"))
		Expect(secret.Labels["config.oam.dev/catalog"]).Should(Equal("velacore-config"))
		Expect(string(secret.Type)).Should(Equal("kubernetes.io/dockerconfigjson"))
	})

	It("Distribute a config", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := ConfigCommandGroup(arg, "", util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"distribute", "testfile", "-t", "test"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(buffer.String()).Should(Equal("the distribution distribute-testfile applied successfully\n"))
	})

	It("Recall a config", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := ConfigCommandGroup(arg, "", util.IOStreams{In: strings.NewReader("y\n"), Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"distribute", "testfile", "--recall"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(buffer.String()).Should(Equal("Do you want to recall this config (y/n)the distribution distribute-testfile deleted successfully\n"))
	})

	It("Test delete a config", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := ConfigCommandGroup(arg, "", util.IOStreams{In: strings.NewReader("y\n"), Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"delete", "distribution", "-n", "default"})
		assumeYes = false
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(buffer.String()).Should(Equal("Do you want to delete this config (y/n)the config distribution deleted successfully\n"))
	})

	It("Test delete a template", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := TemplateCommandGroup(arg, "", util.IOStreams{In: strings.NewReader("y\n"), Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"delete", "test"})
		assumeYes = false
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(buffer.String()).Should(Equal("Do you want to delete this template (y/n)the config template test deleted successfully\n"))
	})

	It("Test distributing a CRD-backed config sets the Config as owner of the distribution", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := ConfigCommandGroup(arg, "", util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"create", "crd-dist", "--template=test2", "--namespace=default", "-f", "./test-data/config/registry.yaml", "--target", "test"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())

		var cfgObj configv1alpha1.Config
		Expect(k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "crd-dist"}, &cfgObj)).Should(BeNil())

		var app v1beta1.Application
		Expect(k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "distribute-crd-dist"}, &app)).Should(BeNil())
		Expect(metav1.IsControlledBy(&app, &cfgObj)).Should(BeTrue())
	})

	It("Test --not-recall is refused for a CRD-backed config with an existing distribution", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := ConfigCommandGroup(arg, "", util.IOStreams{In: strings.NewReader("y\n"), Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"delete", "crd-dist", "-n", "default", "--not-recall"})
		assumeYes = false
		err := cmd.Execute()
		Expect(err).ShouldNot(BeNil())
		Expect(err.Error()).Should(ContainSubstring("not-recall is not supported"))

		// nothing was touched
		var cfgObj configv1alpha1.Config
		Expect(k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "crd-dist"}, &cfgObj)).Should(BeNil())
		var app v1beta1.Application
		Expect(k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "distribute-crd-dist"}, &app)).Should(BeNil())
	})

	It("Test deleting a CRD-backed config still recalls its distribution by default", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := ConfigCommandGroup(arg, "", util.IOStreams{In: strings.NewReader("y\n"), Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"delete", "crd-dist", "-n", "default"})
		assumeYes = false
		err := cmd.Execute()
		Expect(err).Should(BeNil())

		var cfgObj configv1alpha1.Config
		err = k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "crd-dist"}, &cfgObj)
		Expect(apierrors.IsNotFound(err)).Should(BeTrue())
		var app v1beta1.Application
		err = k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "distribute-crd-dist"}, &app)
		Expect(apierrors.IsNotFound(err)).Should(BeTrue())
	})

	// The CLI no longer produces legacy configs (create writes a Config CR once the
	// CRDs are installed), so these cases seed one through the factory.
	It("Test --not-recall still works for a legacy config (no owner reference involved)", func() {
		seedLegacyConfigWithDistribution(k8sClient, "legacy-dist")
		DeferCleanup(func() {
			app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "distribute-legacy-dist"}}
			Expect(client.IgnoreNotFound(k8sClient.Delete(context.TODO(), app))).Should(Succeed())
		})

		buffer := bytes.NewBuffer(nil)
		cmd := ConfigCommandGroup(arg, "", util.IOStreams{In: strings.NewReader("y\n"), Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"delete", "legacy-dist", "-n", "default", "--not-recall"})
		assumeYes = false
		Expect(cmd.Execute()).Should(BeNil())

		// the config is gone, but its distribution was intentionally left alone
		var secret v1.Secret
		err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "legacy-dist"}, &secret)
		Expect(apierrors.IsNotFound(err)).Should(BeTrue())
		var app v1beta1.Application
		Expect(k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "distribute-legacy-dist"}, &app)).Should(BeNil())
	})

	It("Test deleting a legacy config recalls its distribution by default", func() {
		seedLegacyConfigWithDistribution(k8sClient, "legacy-recall")

		buffer := bytes.NewBuffer(nil)
		cmd := ConfigCommandGroup(arg, "", util.IOStreams{In: strings.NewReader("y\n"), Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"delete", "legacy-recall", "-n", "default"})
		assumeYes = false
		Expect(cmd.Execute()).Should(Succeed())
		Expect(buffer.String()).Should(HaveSuffix("the config legacy-recall deleted successfully\n"))

		var secret v1.Secret
		err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "legacy-recall"}, &secret)
		Expect(apierrors.IsNotFound(err)).Should(BeTrue())
		var app v1beta1.Application
		err = k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "distribute-legacy-recall"}, &app)
		Expect(apierrors.IsNotFound(err)).Should(BeTrue())
	})

	It("Test deleting a config that neither backend holds reports not found", func() {
		cmd := ConfigCommandGroup(arg, "", util.IOStreams{In: strings.NewReader("y\n"), Out: io.Discard, ErrOut: io.Discard})
		cmd.SetArgs([]string{"delete", "nothing-here", "-n", "default"})
		assumeYes = false
		Expect(cmd.Execute()).Should(MatchError("the config nothing-here not found"))
	})

	It("Test deleting a template held only as a legacy ConfigMap", func() {
		seedLegacyTemplate(k8sClient, "default", "legacy-del")

		buffer := bytes.NewBuffer(nil)
		cmd := TemplateCommandGroup(arg, "", util.IOStreams{In: strings.NewReader("y\n"), Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"delete", "legacy-del", "-n", "default"})
		assumeYes = false
		Expect(cmd.Execute()).Should(Succeed())
		Expect(buffer.String()).Should(Equal("Do you want to delete this template (y/n)the config template legacy-del deleted successfully\n"))

		var cm v1.ConfigMap
		err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: config.TemplateConfigMapNamePrefix + "legacy-del"}, &cm)
		Expect(apierrors.IsNotFound(err)).Should(BeTrue())
	})

	It("Test deleting a template held by both backends removes the CR and reports the legacy twin", func() {
		seedLegacyTemplate(k8sClient, "default", "del-both")
		apply := TemplateCommandGroup(arg, "", util.IOStreams{In: os.Stdin, Out: io.Discard, ErrOut: io.Discard})
		apply.SetArgs([]string{"apply", "-f", "./test-data/config-templates/image-registry.cue", "--name", "del-both", "-n", "default"})
		Expect(apply.Execute()).Should(Succeed())
		DeferCleanup(func() {
			cm := &v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: config.TemplateConfigMapNamePrefix + "del-both"}}
			Expect(client.IgnoreNotFound(k8sClient.Delete(context.TODO(), cm))).Should(Succeed())
		})

		del := func() string {
			buffer := bytes.NewBuffer(nil)
			cmd := TemplateCommandGroup(arg, "", util.IOStreams{In: strings.NewReader("y\n"), Out: buffer, ErrOut: buffer})
			cmd.SetArgs([]string{"delete", "del-both", "-n", "default"})
			assumeYes = false
			Expect(cmd.Execute()).Should(Succeed())
			return buffer.String()
		}

		// first delete: the CR goes, the ConfigMap stays and the user is told
		Expect(del()).Should(Equal("Do you want to delete this template (y/n)the config template del-both deleted successfully\n" +
			"note: legacy template ConfigMap config-template-del-both in default remains and is visible again\n"))
		var ct configv1alpha1.ConfigTemplate
		err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "del-both"}, &ct)
		Expect(apierrors.IsNotFound(err)).Should(BeTrue())
		var cm v1.ConfigMap
		Expect(k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: config.TemplateConfigMapNamePrefix + "del-both"}, &cm)).Should(Succeed())

		// second delete: the ConfigMap goes, no note
		Expect(del()).Should(Equal("Do you want to delete this template (y/n)the config template del-both deleted successfully\n"))
		err = k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: config.TemplateConfigMapNamePrefix + "del-both"}, &cm)
		Expect(apierrors.IsNotFound(err)).Should(BeTrue())
	})

	It("Test deleting a template that neither backend holds reports not found", func() {
		cmd := TemplateCommandGroup(arg, "", util.IOStreams{In: strings.NewReader("y\n"), Out: io.Discard, ErrOut: io.Discard})
		cmd.SetArgs([]string{"delete", "nothing-here", "-n", "default"})
		assumeYes = false
		Expect(cmd.Execute()).Should(MatchError("the config template nothing-here not found"))
	})

	It("Test deleting a legacy template on a cluster without the CRD", func() {
		noCRDClient := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).Build()
		seedLegacyTemplate(noCRDClient, "default", "legacy-no-crd")

		buffer := bytes.NewBuffer(nil)
		cmd := TemplateCommandGroup(cmd.NewTestFactory(cfg, noCRDClient), "", util.IOStreams{In: strings.NewReader("y\n"), Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"delete", "legacy-no-crd", "-n", "default"})
		assumeYes = false
		Expect(cmd.Execute()).Should(Succeed())
		Expect(buffer.String()).Should(HaveSuffix("the config template legacy-no-crd deleted successfully\n"))
		var cm v1.ConfigMap
		err := noCRDClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: config.TemplateConfigMapNamePrefix + "legacy-no-crd"}, &cm)
		Expect(apierrors.IsNotFound(err)).Should(BeTrue())
	})

	It("Test creating a config against a legacy template writes a Config CR", func() {
		seedLegacyTemplate(k8sClient, "default", "legacy-tpl")
		DeferCleanup(func() {
			cm := &v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: config.TemplateConfigMapNamePrefix + "legacy-tpl"}}
			Expect(client.IgnoreNotFound(k8sClient.Delete(context.TODO(), cm))).Should(Succeed())
			cfgObj := &configv1alpha1.Config{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "created-on-legacy"}}
			Expect(client.IgnoreNotFound(k8sClient.Delete(context.TODO(), cfgObj))).Should(Succeed())
		})
		// the template is held only as a ConfigMap
		var ct configv1alpha1.ConfigTemplate
		Expect(apierrors.IsNotFound(k8sClient.Get(context.TODO(), client.ObjectKey{Namespace: "default", Name: "legacy-tpl"}, &ct))).Should(BeTrue())

		buffer := bytes.NewBuffer(nil)
		cmd := ConfigCommandGroup(arg, "", util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"create", "created-on-legacy", "--template=default/legacy-tpl", "--namespace=default", "-f", "./test-data/config/registry.yaml"})
		Expect(cmd.Execute()).Should(Succeed())
		Expect(buffer.String()).Should(Equal("the config created-on-legacy applied successfully\n"))

		var cfgObj configv1alpha1.Config
		Expect(k8sClient.Get(context.TODO(), client.ObjectKey{Namespace: "default", Name: "created-on-legacy"}, &cfgObj)).Should(Succeed())
		Expect(cfgObj.Spec.TemplateRef).ShouldNot(BeNil())
		Expect(cfgObj.Spec.TemplateRef.Name).Should(Equal("legacy-tpl"))
		Expect(cfgObj.Spec.TemplateRef.Namespace).Should(Equal("default"))
		// the CLI wrote no legacy Secret; the controller renders one from the CR
		var secret v1.Secret
		Expect(apierrors.IsNotFound(k8sClient.Get(context.TODO(), client.ObjectKey{Namespace: "default", Name: "created-on-legacy"}, &secret))).Should(BeTrue())
	})

	It("Test creating a config fails when the Config CRD is not installed", func() {
		noCRDClient := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).Build()
		seedLegacyTemplate(noCRDClient, "default", "legacy-no-crd")

		cmd := ConfigCommandGroup(cmd.NewTestFactory(cfg, noCRDClient), "", util.IOStreams{In: os.Stdin, Out: io.Discard, ErrOut: io.Discard})
		cmd.SetArgs([]string{"create", "x", "--template=default/legacy-no-crd", "--namespace=default", "-f", "./test-data/config/registry.yaml"})
		Expect(cmd.Execute()).Should(MatchError(errConfigCRDMissing))
		// and nothing was written as a legacy Secret instead
		var secret v1.Secret
		Expect(apierrors.IsNotFound(noCRDClient.Get(context.TODO(), client.ObjectKey{Namespace: "default", Name: "x"}, &secret))).Should(BeTrue())
	})

	It("Test show renders a legacy template", func() {
		seedLegacyTemplate(k8sClient, "default", "legacy-show")
		DeferCleanup(func() {
			cm := &v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: config.TemplateConfigMapNamePrefix + "legacy-show"}}
			Expect(client.IgnoreNotFound(k8sClient.Delete(context.TODO(), cm))).Should(Succeed())
		})

		buffer := bytes.NewBuffer(nil)
		cmd := TemplateCommandGroup(arg, "", util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"show", "legacy-show", "-n", "default"})
		Expect(cmd.Execute()).Should(Succeed())
		Expect(buffer.String()).Should(ContainSubstring("registry"))
		Expect(line(buffer.String())).Should(Equal(24))
	})
})

// seedLegacyTemplate writes template name as a legacy ConfigMap in ns through the
// factory, the way a pre-CRD CLI did.
func seedLegacyTemplate(cli client.Client, ns, name string) {
	inf := config.NewConfigFactory(cli)
	body, err := os.ReadFile("./test-data/config-templates/image-registry.cue")
	Expect(err).Should(BeNil())
	t, err := inf.ParseTemplate(context.TODO(), name, body)
	Expect(err).Should(BeNil())
	Expect(inf.CreateOrUpdateConfigTemplate(context.TODO(), ns, t)).Should(Succeed())
}

// seedLegacyConfigWithDistribution writes config name in default as a legacy
// Secret from the vela-system test2 template, plus a distribution Application
// with no owner reference, the way a pre-CRD CLI did.
func seedLegacyConfigWithDistribution(cli client.Client, name string) {
	inf := config.NewConfigFactory(cli)
	item, err := inf.ParseConfig(context.Background(),
		config.NamespacedName{Name: "test2", Namespace: types.DefaultKubeVelaNS},
		config.Metadata{
			NamespacedName: config.NamespacedName{Name: name, Namespace: "default"},
			Properties: map[string]interface{}{
				"registry": "docker.io",
				"auth":     map[string]interface{}{"username": "zhangsan", "password": "lisi"},
				"useHTTP":  true,
			},
		})
	Expect(err).Should(BeNil())
	Expect(inf.CreateOrUpdateConfig(context.Background(), item, "default")).Should(BeNil())
	Expect(inf.CreateOrUpdateDistribution(context.Background(), "default", config.DefaultDistributionName(name), &config.CreateDistributionSpec{
		Targets: []*config.ClusterTarget{{ClusterName: types.ClusterLocalName, Namespace: "test"}},
		Configs: []*config.NamespacedName{{Name: name, Namespace: "default"}},
	})).Should(BeNil())
}

func line(data string) int {
	return len(strings.Split(strings.TrimRight(data, "\n"), "\n"))
}
