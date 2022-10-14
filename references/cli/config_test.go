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
	"os"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"

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
		cmd := TemplateCommandGroup(arg, util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"apply", "-f", "./test-data/config-templates/image-registry.cue", "--name", "test"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(buffer.String()).Should(Equal("the config template test applied successfully\n"))
	})

	It("Test apply a new template", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := TemplateCommandGroup(arg, util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"apply", "-f", "./test-data/config-templates/image-registry.cue", "--name", "test2"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(buffer.String()).Should(Equal("the config template test2 applied successfully\n"))
	})

	It("Test list the templates", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := TemplateCommandGroup(arg, util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"list", "-A"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(strings.Contains(buffer.String(), "vela-system")).Should(Equal(true))
		Expect(strings.Contains(buffer.String(), "test")).Should(Equal(true))
		Expect(strings.Contains(buffer.String(), "\n")).Should(Equal(true))
	})

	It("Test show the templates", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := TemplateCommandGroup(arg, util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"show", "test2"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(line(buffer.String())).Should(Equal(24))
	})

	It("Test create the config with the args", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := ConfigCommandGroup(arg, util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"create", "test", "--template=test", "registry=test.kubevela.net", "auth.username=yueda", "auth.password=yueda123", "useHTTP=true"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(buffer.String()).Should(Equal("the config test applied successfully\n"))
	})

	It("Test create the config with the file", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := ConfigCommandGroup(arg, util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"create", "testfile", "--template=test2", "--namespace=default", "-f", "./test-data/config/registry.yaml"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(buffer.String()).Should(Equal("the config testfile applied successfully\n"))
	})

	It("Test create the config without the template", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := ConfigCommandGroup(arg, util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"create", "without-template", "--namespace=default", "-f", "./test-data/config/registry.yaml"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(buffer.String()).Should(Equal("the config without-template applied successfully\n"))
	})

	It("Test creating and distributing the config", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := ConfigCommandGroup(arg, util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"create", "distribution", "--namespace=default", "-f", "./test-data/config/registry.yaml", "--target", "test"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(buffer.String()).Should(Equal("the config distribution applied successfully\n"))
	})

	It("Test list the configs", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := ConfigCommandGroup(arg, util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"list", "-A"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(line(buffer.String())).Should(Equal(5))
	})

	It("Test list the configs with the namespace filter", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := ConfigCommandGroup(arg, util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"list", "-n", "default"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(line(buffer.String())).Should(Equal(4))
	})

	It("Test list the configs with the template filter", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := ConfigCommandGroup(arg, util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"list", "-A", "-t", "test2"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(line(buffer.String())).Should(Equal(2))
	})

	It("Test dry run the config", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := ConfigCommandGroup(arg, util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
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
		cmd := ConfigCommandGroup(arg, util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"distribute", "testfile", "-t", "test"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(buffer.String()).Should(Equal("the distribution distribute-testfile applied successfully\n"))
	})

	It("Recall a config", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := ConfigCommandGroup(arg, util.IOStreams{In: strings.NewReader("y\n"), Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"distribute", "testfile", "--recall"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(buffer.String()).Should(Equal("Do you want to recall this config (y/n)the distribution distribute-testfile deleted successfully\n"))
	})

	It("Test delete a config", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := ConfigCommandGroup(arg, util.IOStreams{In: strings.NewReader("y\n"), Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"delete", "distribution", "-n", "default"})
		assumeYes = false
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(buffer.String()).Should(Equal("Do you want to delete this config (y/n)the config distribution deleted successfully\n"))
	})

	It("Test delete a template", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := TemplateCommandGroup(arg, util.IOStreams{In: strings.NewReader("y\n"), Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"delete", "test"})
		assumeYes = false
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(buffer.String()).Should(Equal("Do you want to delete this template (y/n)the config template test deleted successfully\n"))
	})
})

func line(data string) int {
	return len(strings.Split(strings.TrimRight(data, "\n"), "\n"))
}
