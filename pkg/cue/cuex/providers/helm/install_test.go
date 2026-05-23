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

package helm

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"helm.sh/helm/v3/pkg/action"
)

func boolPtr(b bool) *bool { return &b }

var _ = Describe("applyCommonInstallOptions", func() {
	var install *action.Install

	BeforeEach(func() {
		install = action.NewInstall(&action.Configuration{})
	})

	It("is a no-op when opts is nil", func() {
		applyCommonInstallOptions(install, nil)
		Expect(install.SkipCRDs).To(BeFalse())
		Expect(install.Force).To(BeFalse())
		Expect(install.Atomic).To(BeFalse())
		Expect(install.Wait).To(BeFalse())
	})

	It("sets SkipCRDs=true when includeCRDs is explicitly false", func() {
		applyCommonInstallOptions(install, &RenderOptionsParams{IncludeCRDs: boolPtr(false)})
		Expect(install.SkipCRDs).To(BeTrue())
	})

	It("sets SkipCRDs=false when includeCRDs is explicitly true", func() {
		applyCommonInstallOptions(install, &RenderOptionsParams{IncludeCRDs: boolPtr(true)})
		Expect(install.SkipCRDs).To(BeFalse())
	})

	It("leaves SkipCRDs at its zero value when includeCRDs is nil", func() {
		applyCommonInstallOptions(install, &RenderOptionsParams{})
		Expect(install.SkipCRDs).To(BeFalse())
	})

	It("sets Force=true when force is true", func() {
		applyCommonInstallOptions(install, &RenderOptionsParams{Force: true})
		Expect(install.Force).To(BeTrue())
	})

	It("sets Atomic and Wait when atomic is true", func() {
		applyCommonInstallOptions(install, &RenderOptionsParams{Atomic: true})
		Expect(install.Atomic).To(BeTrue())
		Expect(install.Wait).To(BeTrue())
	})

	It("sets Wait when wait=true is set without atomic", func() {
		applyCommonInstallOptions(install, &RenderOptionsParams{Wait: true})
		Expect(install.Wait).To(BeTrue())
		Expect(install.Atomic).To(BeFalse())
	})

	It("parses Timeout from a duration string", func() {
		applyCommonInstallOptions(install, &RenderOptionsParams{Timeout: "30s"})
		Expect(install.Timeout).To(Equal(30 * time.Second))
	})

	It("ignores an unparseable Timeout", func() {
		applyCommonInstallOptions(install, &RenderOptionsParams{Timeout: "not-a-duration"})
		Expect(install.Timeout).To(Equal(time.Duration(0)))
	})

	It("honors CreateNamespace=false override", func() {
		applyCommonInstallOptions(install, &RenderOptionsParams{CreateNamespace: boolPtr(false)})
		Expect(install.CreateNamespace).To(BeFalse())
	})

	It("sets DisableHooks=true when skipHooks is true", func() {
		applyCommonInstallOptions(install, &RenderOptionsParams{SkipHooks: boolPtr(true)})
		Expect(install.DisableHooks).To(BeTrue())
	})

	It("applies all enabled fields together", func() {
		applyCommonInstallOptions(install, &RenderOptionsParams{
			IncludeCRDs:     boolPtr(false),
			Force:           true,
			Atomic:          true,
			Wait:            true,
			Timeout:         "2m",
			CreateNamespace: boolPtr(true),
			SkipHooks:       boolPtr(true),
		})
		Expect(install.SkipCRDs).To(BeTrue())
		Expect(install.Force).To(BeTrue())
		Expect(install.Atomic).To(BeTrue())
		Expect(install.Wait).To(BeTrue())
		Expect(install.Timeout).To(Equal(2 * time.Minute))
		Expect(install.CreateNamespace).To(BeTrue())
		Expect(install.DisableHooks).To(BeTrue())
	})
})
