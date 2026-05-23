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

var _ = Describe("applyCommonUpgradeOptions", func() {
	var upgrade *action.Upgrade

	BeforeEach(func() {
		upgrade = action.NewUpgrade(&action.Configuration{})
	})

	It("is a no-op when opts is nil", func() {
		applyCommonUpgradeOptions(upgrade, nil)
		Expect(upgrade.SkipCRDs).To(BeFalse())
		Expect(upgrade.Force).To(BeFalse())
		Expect(upgrade.Atomic).To(BeFalse())
		Expect(upgrade.CleanupOnFail).To(BeFalse())
		Expect(upgrade.Recreate).To(BeFalse())
		Expect(upgrade.MaxHistory).To(BeZero())
	})

	It("sets SkipCRDs=true when includeCRDs is explicitly false", func() {
		applyCommonUpgradeOptions(upgrade, &RenderOptionsParams{IncludeCRDs: boolPtr(false)})
		Expect(upgrade.SkipCRDs).To(BeTrue())
	})

	It("sets Force=true when force is true", func() {
		applyCommonUpgradeOptions(upgrade, &RenderOptionsParams{Force: true})
		Expect(upgrade.Force).To(BeTrue())
	})

	It("sets Recreate=true when recreatePods is true", func() {
		applyCommonUpgradeOptions(upgrade, &RenderOptionsParams{RecreatePods: true})
		Expect(upgrade.Recreate).To(BeTrue())
	})

	It("sets CleanupOnFail=true when cleanupOnFail is true", func() {
		applyCommonUpgradeOptions(upgrade, &RenderOptionsParams{CleanupOnFail: true})
		Expect(upgrade.CleanupOnFail).To(BeTrue())
	})

	It("sets MaxHistory when greater than zero", func() {
		applyCommonUpgradeOptions(upgrade, &RenderOptionsParams{MaxHistory: 5})
		Expect(upgrade.MaxHistory).To(Equal(5))
	})

	It("leaves MaxHistory at zero when not set", func() {
		applyCommonUpgradeOptions(upgrade, &RenderOptionsParams{})
		Expect(upgrade.MaxHistory).To(BeZero())
	})

	It("sets Atomic and Wait when atomic is true", func() {
		applyCommonUpgradeOptions(upgrade, &RenderOptionsParams{Atomic: true})
		Expect(upgrade.Atomic).To(BeTrue())
		Expect(upgrade.Wait).To(BeTrue())
	})

	It("parses Timeout from a duration string", func() {
		applyCommonUpgradeOptions(upgrade, &RenderOptionsParams{Timeout: "45s"})
		Expect(upgrade.Timeout).To(Equal(45 * time.Second))
	})

	It("applies all upgrade-only fields together", func() {
		applyCommonUpgradeOptions(upgrade, &RenderOptionsParams{
			IncludeCRDs:   boolPtr(false),
			Force:         true,
			CleanupOnFail: true,
			RecreatePods:  true,
			MaxHistory:    3,
		})
		Expect(upgrade.SkipCRDs).To(BeTrue())
		Expect(upgrade.Force).To(BeTrue())
		Expect(upgrade.CleanupOnFail).To(BeTrue())
		Expect(upgrade.Recreate).To(BeTrue())
		Expect(upgrade.MaxHistory).To(Equal(3))
	})
})
