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

package config

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/pflag"
)

func TestConfigSpecs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "core app config")
}

var _ = Describe("WorkflowConfig HTTP deny template name", func() {
	It("defaults to the vela-core chart template", func() {
		Expect(NewWorkflowConfig().HTTPDenyConfigTemplateName).To(Equal(VelaCoreHTTPDenyConfigTemplate))
	})

	It("is set by --workflow-http-deny-config-template-name", func() {
		c := NewWorkflowConfig()
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		c.AddFlags(fs)

		Expect(fs.Parse([]string{"--workflow-http-deny-config-template-name=custom-deny"})).To(Succeed())
		Expect(c.HTTPDenyConfigTemplateName).To(Equal("custom-deny"))
	})

	It("can be set to empty to turn discovery off", func() {
		c := NewWorkflowConfig()
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		c.AddFlags(fs)

		Expect(fs.Parse([]string{"--workflow-http-deny-config-template-name="})).To(Succeed())
		Expect(c.HTTPDenyConfigTemplateName).To(BeEmpty())
	})
})
