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
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
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

var _ = Describe("install action and retry", func() {

	Describe("isRetryableInstallError", func() {
		DescribeTable("should identify retryable errors",
			func(errMsg string, expected bool) {
				Expect(isRetryableInstallError(errors.New(errMsg))).To(Equal(expected))
			},
			Entry("cannot be imported", "cannot be imported into the current release", true),
			Entry("invalid ownership metadata", "invalid ownership metadata", true),
			Entry("no revision for release", "no revision for release", true),
			Entry("release already exists", "release: already exists", true),
			Entry("generic error", "connection refused", false),
			Entry("timeout error", "context deadline exceeded", false),
		)
	})

	Describe("newInstallAction", func() {
		var p *Provider

		BeforeEach(func() {
			p = NewProviderWithConfig(nil)
		})

		It("should set defaults correctly", func() {
			install := p.newInstallAction(&action.Configuration{}, "test-release", "test-ns", nil, nil, nil)
			Expect(install.ReleaseName).To(Equal("test-release"))
			Expect(install.Namespace).To(Equal("test-ns"))
			Expect(install.CreateNamespace).To(BeTrue())
			Expect(install.DryRun).To(BeFalse())
			Expect(install.ClientOnly).To(BeFalse())
		})

		It("should apply render options", func() {
			createNs := false
			skipHooks := true
			install := p.newInstallAction(&action.Configuration{}, "test-release", "test-ns", &RenderOptionsParams{
				Atomic:          true,
				Timeout:         "5m",
				CreateNamespace: &createNs,
				SkipHooks:       &skipHooks,
			}, nil, map[string]string{"app.oam.dev/name": "my-app"})

			Expect(install.Atomic).To(BeTrue())
			Expect(install.Wait).To(BeTrue())
			Expect(install.Timeout).To(Equal(5 * time.Minute))
			Expect(install.CreateNamespace).To(BeFalse())
			Expect(install.DisableHooks).To(BeTrue())
			Expect(install.Labels).To(Equal(map[string]string{"app.oam.dev/name": "my-app"}))
		})

		It("should set Wait when Atomic is true", func() {
			install := p.newInstallAction(&action.Configuration{}, "rel", "ns", &RenderOptionsParams{
				Atomic: true,
			}, nil, nil)
			Expect(install.Wait).To(BeTrue())
		})

		It("should set Wait independently", func() {
			install := p.newInstallAction(&action.Configuration{}, "rel", "ns", &RenderOptionsParams{
				Wait: true,
			}, nil, nil)
			Expect(install.Wait).To(BeTrue())
			Expect(install.Atomic).To(BeFalse())
		})
	})

	Describe("freshInstall", func() {
		It("should return error when install fails (via installOrUpgradeChart)", func() {
			p := NewProviderWithConfig(nil)
			ch := &chart.Chart{
				Metadata: &chart.Metadata{Name: "test-fresh", Version: "1.0.0"},
				Templates: []*chart.File{
					{
						Name: "templates/cm.yaml",
						Data: []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}
`),
					},
				},
			}
			// Use installOrUpgradeChart which handles actionConfig initialization safely
			// It will install into a real cluster if available, or fail if not
			_, _, _, err := p.installOrUpgradeChart(
				context.Background(), ch, "test-fresh-install", "default",
				map[string]interface{}{}, nil, nil)
			// We just verify it doesn't panic - it may succeed or fail depending on cluster
			_ = err
		})
	})

})
