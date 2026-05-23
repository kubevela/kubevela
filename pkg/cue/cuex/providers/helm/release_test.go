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
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	kubefake "helm.sh/helm/v3/pkg/kube/fake"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/client-go/kubernetes"
	clientsetfake "k8s.io/client-go/kubernetes/fake"
)

// corruptingDriver wraps a real driver and replaces Get / Query results for a
// configured release name with a synthetic decode-style error, simulating the
// production case where .data.release on a helm release Secret has been
// mutated to invalid base64 / gzip / json bytes.
type corruptingDriver struct {
	driver.Driver
	corruptName string
}

func (c *corruptingDriver) Get(key string) (*release.Release, error) {
	if c.corruptName != "" && (key == c.corruptName || keyStartsWithRelease(key, c.corruptName)) {
		return nil, errors.New("invalid character '\\x00' looking for beginning of value")
	}
	return c.Driver.Get(key)
}

func (c *corruptingDriver) Query(labels map[string]string) ([]*release.Release, error) {
	if c.corruptName != "" && labels["name"] == c.corruptName {
		return nil, errors.New("invalid character '\\x00' looking for beginning of value")
	}
	return c.Driver.Query(labels)
}

func keyStartsWithRelease(key, releaseName string) bool {
	prefix := "sh.helm.release.v1." + releaseName + "."
	return len(key) >= len(prefix) && key[:len(prefix)] == prefix
}

// fakeActionConfig builds an action.Configuration backed by helm's printing
// fake KubeClient and an in-memory release storage driver. This lets us
// exercise installOrUpgradeChart without a real cluster.
func fakeActionConfig() *action.Configuration {
	return &action.Configuration{
		Releases:     storage.Init(driver.NewMemory()),
		KubeClient:   &kubefake.PrintingKubeClient{Out: io.Discard},
		Capabilities: chartutil.DefaultCapabilities,
		Log:          func(format string, v ...interface{}) {},
	}
}

// installProviderWithFake returns a fresh Provider whose two seams
// (actionConfigFactory and kubeClientFactory) are bound to the supplied
// fakes, so the dispatcher cannot reach the active cluster. Multiple test
// specs do not share storage state because each call constructs its own
// fake action.Configuration + clientset.
func installProviderWithFake(cfg *action.Configuration) *Provider {
	p := NewProviderWithConfig(nil)
	p.actionConfigFactory = func(string) (*action.Configuration, error) { return cfg, nil }
	cs := clientsetfake.NewSimpleClientset()
	p.kubeClientFactory = func() (kubernetes.Interface, error) { return cs, nil }
	return p
}

// minimalChart returns a tiny in-memory chart that helm SDK accepts.
func minimalChart(name, version string) *chart.Chart {
	return &chart.Chart{
		Metadata: &chart.Metadata{
			Name:       name,
			Version:    version,
			APIVersion: chart.APIVersionV2,
		},
		Templates: []*chart.File{
			{
				Name: "templates/cm.yaml",
				Data: []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}-cm
  namespace: {{ .Release.Namespace }}
data:
  key: value
`),
			},
		},
		Values: map[string]interface{}{},
	}
}

var _ = Describe("release", func() {

	Describe("computeReleaseFingerprint", func() {
		It("should be deterministic for same inputs", func() {
			ch := &chart.Chart{Metadata: &chart.Metadata{Version: "1.2.3"}}
			values := map[string]interface{}{"replicas": 2}

			fp1 := computeReleaseFingerprint(ch, values)
			fp2 := computeReleaseFingerprint(ch, values)
			Expect(fp1).To(Equal(fp2))
		})

		It("should differ for different values", func() {
			ch := &chart.Chart{Metadata: &chart.Metadata{Version: "1.2.3"}}
			fp1 := computeReleaseFingerprint(ch, map[string]interface{}{"replicas": 2})
			fp2 := computeReleaseFingerprint(ch, map[string]interface{}{"replicas": 3})
			Expect(fp1).ToNot(Equal(fp2))
		})

		It("should encode the chart version", func() {
			ch := &chart.Chart{Metadata: &chart.Metadata{Version: "1.2.3"}}
			fp := computeReleaseFingerprint(ch, map[string]interface{}{"replicas": 2})
			Expect(fp).To(ContainSubstring("1.2.3"))
		})

		It("should differ for different chart versions", func() {
			ch1 := &chart.Chart{Metadata: &chart.Metadata{Version: "1.0.0"}}
			ch2 := &chart.Chart{Metadata: &chart.Metadata{Version: "2.0.0"}}
			values := map[string]interface{}{"key": "val"}

			fp1 := computeReleaseFingerprint(ch1, values)
			fp2 := computeReleaseFingerprint(ch2, values)
			Expect(fp1).ToNot(Equal(fp2))
		})

		It("should handle nil chart metadata", func() {
			fp := computeReleaseFingerprint(nil, map[string]interface{}{"replicas": 2})
			Expect(fp).ToNot(BeEmpty())
		})

		It("should treat nil values and empty map as equivalent", func() {
			// Helm stores release.Config as nil when no values were supplied
			// at install time, but mergeValues returns an empty map for the
			// same logical input. Without normalising the two, the dedup
			// check at the call site would mis-fire on every reconcile and
			// trigger spurious helm upgrades for releases installed with
			// empty/optional valuesFrom sources.
			ch := &chart.Chart{Metadata: &chart.Metadata{Version: "1.2.3"}}
			fpNil := computeReleaseFingerprint(ch, nil)
			fpEmpty := computeReleaseFingerprint(ch, map[string]interface{}{})
			Expect(fpNil).To(Equal(fpEmpty))
		})
	})

	Describe("installOrUpgradeChart with fake action config", func() {
		const (
			relName = "rel"
			relNS   = "ns"
		)

		It("installs a fresh release when none exists", func() {
			cfg := fakeActionConfig()
			p := installProviderWithFake(cfg)
			ch := minimalChart("c", "1.0.0")

			manifest, _, version, err := p.installOrUpgradeChart(
				context.Background(), ch, relName, relNS,
				map[string]interface{}{}, nil, nil)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(version).To(Equal(1))
			Expect(manifest).To(ContainSubstring("kind: ConfigMap"))

			// The cache MUST be populated after a successful install.
			cacheKey := releaseCacheKey(relNS, relName)
			Expect(p.releaseVersions[cacheKey]).To(Equal(1))
		})

		It("short-circuits when an existing release has a matching fingerprint", func() {
			cfg := fakeActionConfig()
			p := installProviderWithFake(cfg)
			ch := minimalChart("c", "1.0.0")
			values := map[string]interface{}{"replicas": 2}

			velaCtx := &ContextParams{AppName: "app", AppNamespace: relNS, Name: "comp"}
			// First install creates the release.
			_, _, _, err := p.installOrUpgradeChart(
				context.Background(), ch, relName, relNS, values, nil, velaCtx)
			Expect(err).ShouldNot(HaveOccurred())

			// Second call with the same chart+values must NOT bump the revision.
			_, _, v2, err := p.installOrUpgradeChart(
				context.Background(), ch, relName, relNS, values, nil, velaCtx)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(v2).To(Equal(1), "fingerprint dedup should keep revision at 1")
		})

		It("upgrades when the chart version changes", func() {
			cfg := fakeActionConfig()
			p := installProviderWithFake(cfg)
			velaCtx := &ContextParams{AppName: "app", AppNamespace: relNS, Name: "comp"}

			_, _, v1, err := p.installOrUpgradeChart(
				context.Background(), minimalChart("c", "1.0.0"), relName, relNS,
				map[string]interface{}{}, nil, velaCtx)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(v1).To(Equal(1))

			_, _, v2, err := p.installOrUpgradeChart(
				context.Background(), minimalChart("c", "1.0.1"), relName, relNS,
				map[string]interface{}{}, nil, velaCtx)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(v2).To(Equal(2))
		})

		It("upgrades when values change", func() {
			cfg := fakeActionConfig()
			p := installProviderWithFake(cfg)
			velaCtx := &ContextParams{AppName: "app", AppNamespace: relNS, Name: "comp"}
			ch := minimalChart("c", "1.0.0")

			_, _, v1, err := p.installOrUpgradeChart(
				context.Background(), ch, relName, relNS,
				map[string]interface{}{"replicas": 2}, nil, velaCtx)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(v1).To(Equal(1))

			_, _, v2, err := p.installOrUpgradeChart(
				context.Background(), ch, relName, relNS,
				map[string]interface{}{"replicas": 3}, nil, velaCtx)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(v2).To(Equal(2))
		})

		It("forces upgrade to adopt an existing release that lacks KubeVela labels", func() {
			cfg := fakeActionConfig()
			// Pre-seed the storage with a release that has no app.oam.dev/* labels.
			vanilla := &release.Release{
				Name:      relName,
				Namespace: relNS,
				Version:   1,
				Info:      &release.Info{Status: release.StatusDeployed},
				Chart:     minimalChart("c", "1.0.0"),
				Config:    map[string]interface{}{},
				// Labels is intentionally empty — simulates a vanilla helm install.
			}
			Expect(cfg.Releases.Create(vanilla)).To(Succeed())

			p := installProviderWithFake(cfg)
			velaCtx := &ContextParams{AppName: "app", AppNamespace: relNS, Name: "comp"}

			_, _, version, err := p.installOrUpgradeChart(
				context.Background(), minimalChart("c", "1.0.0"), relName, relNS,
				map[string]interface{}{}, nil, velaCtx)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(version).To(Equal(2), "adoption should force-upgrade the existing release")

			adopted, err := cfg.Releases.Get(relName, 2)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(adopted.Labels).To(HaveKeyWithValue("app.oam.dev/name", "app"))
			Expect(adopted.Labels).To(HaveKeyWithValue("app.oam.dev/namespace", relNS))
			Expect(adopted.Labels).To(HaveKeyWithValue("app.oam.dev/component", "comp"))
		})

		It("falls through to fresh install when Get returns a corruption-style error", func() {
			// Reproduces the contract validated by the e2e test
			// "Helmchart Self-Healing / should recover from corrupted Helm
			// release secret": when helm's storage driver returns a non-
			// NotFound error (e.g. the release Secret's .data.release has
			// been mutated to garbage and helm cannot decode it), the
			// dispatcher must NOT surface that error to the caller. Instead
			// it must clear the in-memory cache, call freshInstall, and let
			// the orphan-state retry path clean the corrupted secret.
			mem := driver.NewMemory()
			pre := &release.Release{
				Name: "rel", Namespace: relNS, Version: 1,
				Info:   &release.Info{Status: release.StatusDeployed},
				Chart:  minimalChart("c", "1.0.0"),
				Config: map[string]interface{}{},
			}
			Expect(mem.Create("rel.v1", pre)).To(Succeed())

			cfg := &action.Configuration{
				Releases:     storage.Init(&corruptingDriver{Driver: mem, corruptName: relName}),
				KubeClient:   &kubefake.PrintingKubeClient{Out: io.Discard},
				Capabilities: chartutil.DefaultCapabilities,
				Log:          func(format string, v ...interface{}) {},
			}
			p := installProviderWithFake(cfg)

			// Pre-seed the in-memory cache so the dispatcher would short-
			// circuit on it if the Get-failure handling did not clear it.
			cacheKey := releaseCacheKey(relNS, relName)
			p.releaseMu.Lock()
			p.releaseFingerprints[cacheKey] = "stale|stalehash"
			p.releaseManifests[cacheKey] = "stale-manifest"
			p.releaseVersions[cacheKey] = 999
			p.releaseMu.Unlock()

			_, _, version, err := p.installOrUpgradeChart(
				context.Background(), minimalChart("c", "1.0.0"), relName, relNS,
				map[string]interface{}{}, nil, nil)

			Expect(err).ShouldNot(HaveOccurred(), "dispatcher MUST swallow the corruption error and recover")
			Expect(version).To(Equal(1), "freshInstall path must run and produce a new revision 1")

			// The fresh install repopulates the cache with the new fingerprint
			// computed from the rendered chart+values. The stale fingerprint
			// the test pre-seeded MUST be gone.
			p.releaseMu.Lock()
			fp := p.releaseFingerprints[cacheKey]
			p.releaseMu.Unlock()
			Expect(fp).NotTo(Equal("stale|stalehash"), "stale fingerprint must be replaced by the fresh install's fingerprint")
			Expect(fp).NotTo(BeEmpty(), "fresh install must repopulate the cache")
		})

		It("honors publishVersion pin when chart version and pin label match", func() {
			cfg := fakeActionConfig()
			pinValue := "v42"
			pinned := &release.Release{
				Name:      relName,
				Namespace: relNS,
				Version:   1,
				Info:      &release.Info{Status: release.StatusDeployed, Notes: "pinned"},
				Chart:     minimalChart("c", "1.0.0"),
				Config:    map[string]interface{}{"replicas": 7},
				Manifest:  "pinned-manifest",
				Labels: map[string]string{
					"app.oam.dev/name":           "app",
					"app.oam.dev/namespace":      relNS,
					"app.oam.dev/component":      "comp",
					"app.oam.dev/publishVersion": pinValue,
				},
			}
			Expect(cfg.Releases.Create(pinned)).To(Succeed())

			p := installProviderWithFake(cfg)
			velaCtx := &ContextParams{
				AppName: "app", AppNamespace: relNS, Name: "comp",
				PublishVersion: pinValue,
			}

			// Submit a different values map; pin must hold so revision stays at 1.
			manifest, _, version, err := p.installOrUpgradeChart(
				context.Background(), minimalChart("c", "1.0.0"), relName, relNS,
				map[string]interface{}{"replicas": 99}, nil, velaCtx)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(version).To(Equal(1), "pin must suppress the upgrade")
			Expect(manifest).To(Equal("pinned-manifest"))
		})
	})

})
