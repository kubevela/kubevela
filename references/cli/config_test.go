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
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cmd"
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

	// --config-mode is a persistent flag registered only on the root command tree
	// (see cli.go's NewCommand), not on ConfigCommandGroup, which is all these tests
	// build - so it can't be passed via SetArgs here. Set the package-level var
	// directly instead, and always restore it so other tests aren't affected.
	withConfigMode := func(mode string, fn func()) {
		prev := configMode
		configMode = mode
		defer func() { configMode = prev }()
		fn()
	}

	It("Test distributing a CRD-backed config sets the Config as owner of the distribution", func() {
		withConfigMode("crd", func() {
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
	})

	It("Test --not-recall is refused for a CRD-backed config with an existing distribution", func() {
		withConfigMode("crd", func() {
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
	})

	It("Test deleting a CRD-backed config still recalls its distribution by default", func() {
		withConfigMode("crd", func() {
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
	})

	It("Test --not-recall still works for a legacy config (no owner reference involved)", func() {
		withConfigMode("legacy", func() {
			buffer := bytes.NewBuffer(nil)
			cmd := ConfigCommandGroup(arg, "", util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
			cmd.SetArgs([]string{"create", "legacy-dist", "--template=test2", "--namespace=default", "-f", "./test-data/config/registry.yaml", "--target", "test"})
			Expect(cmd.Execute()).Should(BeNil())

			buffer2 := bytes.NewBuffer(nil)
			cmd2 := ConfigCommandGroup(arg, "", util.IOStreams{In: strings.NewReader("y\n"), Out: buffer2, ErrOut: buffer2})
			cmd2.SetArgs([]string{"delete", "legacy-dist", "-n", "default", "--not-recall"})
			assumeYes = false
			Expect(cmd2.Execute()).Should(BeNil())

			// the config is gone, but its distribution was intentionally left alone
			var secret v1.Secret
			err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "legacy-dist"}, &secret)
			Expect(apierrors.IsNotFound(err)).Should(BeTrue())
			var app v1beta1.Application
			Expect(k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "distribute-legacy-dist"}, &app)).Should(BeNil())
		})
	})
})

func line(data string) int {
	return len(strings.Split(strings.TrimRight(data, "\n"), "\n"))
}
