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

package ginkgotest

import (
	"strings"
	"testing"

	"github.com/kubevela/pkg/cue/cuex"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/definition/cuetest"
)

func TestGinkgotest(t *testing.T) {
	// Renders must not load external packages from a cluster.
	cuex.EnableExternalPackageForDefaultCompiler = false
	RegisterFailHandler(Fail)
	RunSpecs(t, "Definition tests")
}

var _ = DescribeDefinitions("../testdata/defs/scaler_test.cue")

// The legacy definition only renders once the upgrader rewrites it.
var _ = DescribeDefinitions("../testdata/upgrade", WithUpgrades())

// Workflow steps run against a local API server, stopped when the suite ends.
var _ = DescribeDefinitions("../testdata/steps")

// Hooks build what the cases run on, around the file and each case.
var _ = DescribeDefinitions("../testdata/hooks")

// A known upgrader dependency passes, with its reason in the report.
var _ = DescribeDefinitions("../testdata/upgrade-known")

var _ = ReportAfterSuite("known upgrader dependencies are reported", func(r Report) {
	found := false
	for _, s := range r.SpecReports {
		if s.LeafNodeText != "legacy / known upgrader dependency" || s.State == types.SpecStateSkipped {
			continue
		}
		found = true
		Expect(s.State).To(Equal(types.SpecStatePassed))
		var entries []string
		for _, e := range s.ReportEntries {
			entries = append(entries, e.Name+": "+e.StringRepresentation())
		}
		Expect(entries).To(ContainElement("@upgrade: args use list addition"))
		Expect(s.Labels()).To(ContainElement("upgrade"))
	}
	c := r.SuiteConfig
	if c.LabelFilter == "" && len(c.FocusStrings) == 0 && len(c.SkipStrings) == 0 && len(c.FocusFiles) == 0 && len(c.SkipFiles) == 0 {
		Expect(found).To(BeTrue())
	}
})

// Each case spec carries its labels and its CUE location, so Ginkgo's own
// --label-filter, --focus and --focus-file select definition tests.
var _ = ReportAfterSuite("definition specs are labelled", func(r Report) {
	labels := map[string][]string{}
	var pending []string
	for _, s := range r.SpecReports {
		if strings.HasSuffix(s.LeafNodeLocation.FileName, "scaler_test.cue") {
			labels[s.LeafNodeText] = s.Labels()
			if s.State == types.SpecStatePending {
				pending = append(pending, s.LeafNodeText)
			}
		}
	}
	if r.SuiteConfig.LabelFilter != "" || len(r.SuiteConfig.FocusStrings) > 0 || len(r.SuiteConfig.FocusFiles) > 0 {
		return
	}
	Expect(labels).To(Equal(map[string][]string{
		"scaler / patches replicas":       {"render", "trait"},
		"scaler / healthy once scaled":    {"status", "trait"},
		"scaler / unhealthy below target": {"status", "trait"},
		"scaler / pending on purpose":     {"render", "trait"},
	}))
	Expect(pending).To(Equal([]string{"scaler / pending on purpose"}))
})

var _ = Context("Pass", func() {
	It("reports each failure by path", func() {
		suites, err := cuetest.Load("../testdata/defs/web_test.cue")
		Expect(err).NotTo(HaveOccurred())
		var failing *cuetest.Case
		for _, c := range suites[0].Cases {
			if c.Name == "fails on purpose" {
				failing = c
			}
		}
		Expect(failing).NotTo(BeNil())
		m := Pass()
		ok, err := m.Match(failing)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeFalse())
		Expect(m.FailureMessage(failing)).To(And(
			MatchRegexp(`^\.\./testdata/defs/web_test\.cue:\d+: `),
			ContainSubstring("output.spec.replicas: expected 2, got 1"),
		))
	})

	It("rejects anything but a case", func() {
		_, err := Pass().Match("nope")
		Expect(err).To(MatchError(ContainSubstring("*cuetest.Case")))
	})
})

// --focus and --skip see the same text here as under vela def test, which is
// the case's own FullText.
var _ = ReportAfterSuite("specs have the text vela def test filters", func(r Report) {
	texts := map[string]bool{}
	for _, path := range []string{"../testdata/defs/scaler_test.cue", "../testdata/upgrade", "../testdata/steps", "../testdata/hooks", "../testdata/upgrade-known"} {
		suites, err := cuetest.Load(path)
		Expect(err).NotTo(HaveOccurred())
		for _, s := range suites {
			for _, c := range s.Cases {
				texts[c.FullText()] = true
			}
		}
	}
	for _, spec := range r.SpecReports {
		if spec.LeafNodeType == types.NodeTypeIt && strings.HasSuffix(spec.LeafNodeLocation.FileName, "_test.cue") {
			Expect(texts).To(HaveKey(spec.FullText()))
		}
	}
})
