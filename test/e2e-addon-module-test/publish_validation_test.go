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

// This file holds the CLI-only checks of the manual scenarios: everything
// "vela module publish --dry-run" enforces against a positional OCI reference
// (scenario 17), and the dry-run annotations and tag rules of scenario 01.
// None of it touches a registry or the cluster.
package addonmoduletest

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// offlineOCIRef is a positional OCI reference that is never contacted: a dry
// run validates and packages, then stops before the push.
const offlineOCIRef = "oci://registry.invalid/modules"

var _ = Describe("Module publish validation (scenario 17, offline)", func() {
	// invalidModules maps each tree under testdata/modules/invalid to the
	// fragment of the parser error "vela module publish" must print for it.
	invalidModules := map[string]string{
		"bad-api-version":          `apiVersion "version-one" in v1/_version.cue is invalid`,
		"bad-aux-type":             `unsupported auxiliary file type: notes.txt`,
		"bad-name":                 `module name "Bad_Name" in _module.cue is invalid`,
		"bad-semver":               `version "1.0" in _module.cue is not a valid semver`,
		"duplicate-line":           `duplicate line "v1" (from directory v1-copy)`,
		"line-without-definitions": `v1/definitions: no such file or directory`,
		"missing-def-name":         `definition rendered from v1/definitions/noname.yaml has an empty metadata.name`,
		"no-lines":                 `module declares no API lines`,
		"stray-file":               `unsupported definition file type: README.md`,
	}

	for dir, want := range invalidModules {
		It("refuses the invalid module "+dir, func() {
			out, err := runVela("module", "publish", testdataPath("modules", "invalid", dir), offlineOCIRef, "--dry-run")
			Expect(err).Should(HaveOccurred(), "publish of %s must fail\noutput:\n%s", dir, out)
			Expect(out).Should(ContainSubstring(want))
			Expect(out).ShouldNot(ContainSubstring("Would publish"), "an invalid module must never reach the push step")
		})
	}

	It("dry-runs a scaffolded module cleanly", func() {
		tmp, err := os.MkdirTemp("", "module-init-")
		Expect(err).ShouldNot(HaveOccurred())
		DeferCleanup(func() { _ = os.RemoveAll(tmp) })

		runVelaSucceed("module", "init", "demo-kit", "--path", tmp)
		for _, f := range []string{"_module.cue", filepath.Join("v1", "_version.cue"), filepath.Join("v1", "definitions", "example.cue")} {
			_, statErr := os.Stat(filepath.Join(tmp, "demo-kit", f))
			Expect(statErr).ShouldNot(HaveOccurred(), "scaffold must contain %s", f)
		}

		out := runVelaSucceed("module", "publish", filepath.Join(tmp, "demo-kit"), offlineOCIRef, "--dry-run")
		Expect(out).Should(ContainSubstring("Would publish"))
		Expect(out).Should(ContainSubstring("modules.oam.dev/lines: v1"))
	})

	It("refuses a --version override that is not semver", func() {
		out, err := runVela("module", "publish", testdataPath("modules", "widget-kit-1.0.0"), offlineOCIRef, "--dry-run", "--version", "v1.0.1")
		Expect(err).Should(HaveOccurred(), out)
		Expect(out).Should(ContainSubstring("not a valid semver"))
	})
})

var _ = Describe("Module publish dry run (scenario 01, offline)", func() {
	It("prints the target and the line annotations, with disabled lines left out of enabled-lines", func() {
		out := runVelaSucceed("module", "publish", testdataPath("modules", "widget-kit-1.0.0"), offlineOCIRef, "--dry-run")
		Expect(out).Should(ContainSubstring("Would publish registry.invalid/modules/widget-kit:1.0.0"))
		Expect(out).Should(ContainSubstring("modules.oam.dev/module: widget-kit"))
		Expect(out).Should(ContainSubstring("modules.oam.dev/lines: v1,v1beta1,v2"))
		Expect(out).Should(ContainSubstring("modules.oam.dev/enabled-lines: v1,v2"))

		out = runVelaSucceed("module", "publish", testdataPath("modules", "widget-kit-1.1.0"), offlineOCIRef, "--dry-run")
		Expect(out).Should(ContainSubstring("widget-kit:1.1.0"))
		Expect(out).Should(ContainSubstring("modules.oam.dev/enabled-lines: v1,v2"))

		// 1.2.0 switches v2 off: it ships (lines) but is not enabled.
		out = runVelaSucceed("module", "publish", testdataPath("modules", "widget-kit-1.2.0"), offlineOCIRef, "--dry-run")
		Expect(out).Should(ContainSubstring("widget-kit:1.2.0"))
		Expect(out).Should(ContainSubstring("modules.oam.dev/lines: v1,v1beta1,v2"))
		Expect(out).Should(ContainSubstring("modules.oam.dev/enabled-lines: v1\n"))
	})

	It("warns when --version retags an artifact whose _module.cue says otherwise", func() {
		out := runVelaSucceed("module", "publish", testdataPath("modules", "widget-kit-1.0.0"), offlineOCIRef, "--dry-run", "--version", "1.0.1-rc1")
		Expect(out).Should(ContainSubstring("still declares version 1.0.0"))
		Expect(out).Should(ContainSubstring("widget-kit:1.0.1-rc1"))
	})

	It("reports every module fixture of this suite as publishable", func() {
		for _, dir := range []string{"gadget-kit-1.0.0", "probe-kit-1.0.0-a", "probe-kit-1.0.0-b"} {
			out := runVelaSucceed("module", "publish", testdataPath("modules", dir), offlineOCIRef, "--dry-run")
			Expect(out).Should(ContainSubstring("Would publish"), dir)
		}
	})
})
