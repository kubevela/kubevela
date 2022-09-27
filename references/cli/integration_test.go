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

var _ = Describe("Test the commands of the integration", func() {
	arg := cmd.NewTestFactory(cfg, k8sClient)
	It("Test apply a template", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := IntegrationCommandGroup(arg, util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"template", "apply", "-f", "../../vela-templates/integration-templates/image-registry.cue", "--name", "test"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(buffer.String()).Should(Equal("the integration template test applied successfully\n"))
	})

	It("Test apply a new template", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := IntegrationCommandGroup(arg, util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"template", "apply", "-f", "../../vela-templates/integration-templates/image-registry.cue", "--name", "test2"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(buffer.String()).Should(Equal("the integration template test2 applied successfully\n"))
	})

	It("Test list the templates", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := IntegrationCommandGroup(arg, util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"template", "list", "-A"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(strings.Contains(buffer.String(), "vela-system")).Should(Equal(true))
		Expect(strings.Contains(buffer.String(), "test")).Should(Equal(true))
		Expect(strings.Contains(buffer.String(), "\n")).Should(Equal(true))
	})

	It("Test create the integration with the args", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := IntegrationCommandGroup(arg, util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"apply", "--template=test", "--name=test", "registry=test.kubevela.net", "auth.username=yueda", "auth.password=yueda123", "useHTTP=true"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(buffer.String()).Should(Equal("the integration test applied successfully\n"))
	})

	It("Test create the integration with the file", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := IntegrationCommandGroup(arg, util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"apply", "--template=test2", "--name=testfile", "--namespace=default", "-f", "./test-data/integration/registry.yaml"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(buffer.String()).Should(Equal("the integration testfile applied successfully\n"))
	})

	It("Test create the integration without the template", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := IntegrationCommandGroup(arg, util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"apply", "--name=without-template", "--namespace=default", "-f", "./test-data/integration/registry.yaml"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(buffer.String()).Should(Equal("the integration without-template applied successfully\n"))
	})

	It("Test list the integrations", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := IntegrationCommandGroup(arg, util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"list", "-A"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(line(buffer.String())).Should(Equal(4))
	})

	It("Test list the integrations with the namespace filter", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := IntegrationCommandGroup(arg, util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"list", "-n", "default"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(line(buffer.String())).Should(Equal(3))
	})

	It("Test list the integrations with the template filter", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := IntegrationCommandGroup(arg, util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"list", "-A", "-t", "test2"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(line(buffer.String())).Should(Equal(2))
	})

	It("Test dry run the integration", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := IntegrationCommandGroup(arg, util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"apply", "--template=test", "--name=testfile", "-f", "./test-data/integration/registry.yaml", "--dry-run"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		var secret v1.Secret
		Expect(yaml.Unmarshal(buffer.Bytes(), &secret)).Should(BeNil())
		Expect(secret.Name).Should(Equal("testfile"))
		Expect(secret.Labels["config.oam.dev/type"]).Should(Equal("test"))
		Expect(secret.Labels["config.oam.dev/catalog"]).Should(Equal("integration"))
		Expect(string(secret.Type)).Should(Equal("kubernetes.io/dockerconfigjson"))
	})

	It("Test delete a template", func() {
		buffer := bytes.NewBuffer(nil)
		cmd := IntegrationCommandGroup(arg, util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer})
		cmd.SetArgs([]string{"template", "delete", "test"})
		err := cmd.Execute()
		Expect(err).Should(BeNil())
		Expect(buffer.String()).Should(Equal("the integration template test deleted successfully\n"))
	})
})

func line(data string) int {
	return len(strings.Split(strings.TrimRight(data, "\n"), "\n"))
}
