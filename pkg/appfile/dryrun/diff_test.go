/*
Copyright 2021 The KubeVela Authors.

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

package dryrun

import (
	"bytes"
	"context"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
)

var _ = Describe("Test Live-Diff", func() {
	appMultiChangesYAML := readDataFromFile("./testdata/diff-input-app-multichanges.yaml")
	appNoChangeYAML := readDataFromFile("./testdata/diff-input-app-nochange.yaml")
	appOnlyAddYAML := readDataFromFile("./testdata/diff-input-app-onlyadd.yaml")
	appOnlyModifYAML := readDataFromFile("./testdata/diff-input-app-onlymodif.yaml")
	appOnlyRemoveYAML := readDataFromFile("./testdata/diff-input-app-onlyremove.yaml")

	appMultiChanges := new(v1beta1.Application)
	appNoChange := new(v1beta1.Application)
	appOnlyAdd := new(v1beta1.Application)
	appOnlyModif := new(v1beta1.Application)
	appOnlyRemove := new(v1beta1.Application)

	origAppRevYAML := readDataFromFile("./testdata/diff-apprevision.yaml")
	originalAppRev := new(v1beta1.ApplicationRevision)

	diffAndPrint := func(app *v1beta1.Application) string {
		By("Execute Live-diff")
		diffResult, err := diffOpt.Diff(context.Background(), app, originalAppRev)
		Expect(err).Should(BeNil())
		Expect(diffResult).ShouldNot(BeNil())

		By("Print diff result into buffer")
		buff := &bytes.Buffer{}
		reportOpt := NewReportDiffOption(10, buff)
		reportOpt.PrintDiffReport(diffResult)

		return buff.String()
	}

	BeforeEach(func() {
		By("Prepare AppRevision data")
		Expect(yaml.Unmarshal([]byte(origAppRevYAML), originalAppRev)).Should(Succeed())
	})

	It("Test app containing multiple changes(add/modify/remove/no)", func() {
		Expect(yaml.Unmarshal([]byte(appMultiChangesYAML), appMultiChanges)).Should(Succeed())
		diffResultStr := diffAndPrint(appMultiChanges)
		Expect(diffResultStr).Should(SatisfyAll(
			ContainSubstring("Application (livediff-demo) has been modified(*)"),
			ContainSubstring("Component (myweb-1) has been modified(*)"),
			ContainSubstring("Component (myweb-1) / Trait (myingress/service) has been modified(*)"),
			ContainSubstring("Component (myweb-1) / Trait (myingress/ingress) has been modified(*)"),
			ContainSubstring("Component (myweb-2) has no change"),
			ContainSubstring("Component (myweb-2) / Trait (myingress/service) has been added(+)"),
			ContainSubstring("Component (myweb-2) / Trait (myingress/ingress) has been added(+)"),
			ContainSubstring("Component (myweb-3) has been added(+)"),
			ContainSubstring("Component (myweb-3) / Trait (myingress/service) has been added(+)"),
			ContainSubstring("Component (myweb-3) / Trait (myingress/ingress) has been added(+)"),
		))
	})

	It("Test no change", func() {
		Expect(yaml.Unmarshal([]byte(appNoChangeYAML), appNoChange)).Should(Succeed())
		diffResultStr := diffAndPrint(appNoChange)
		Expect(diffResultStr).Should(SatisfyAll(
			ContainSubstring("Application (livediff-demo) has no change"),
			ContainSubstring("Component (myweb-1) has no change"),
			ContainSubstring("Component (myweb-1) / Trait (myingress/service) has no change"),
			ContainSubstring("Component (myweb-1) / Trait (myingress/ingress) has no change"),
			ContainSubstring("Component (myweb-2) has no change"),
		))
		Expect(diffResultStr).ShouldNot(SatisfyAny(
			ContainSubstring("added"),
			ContainSubstring("removed"),
			ContainSubstring("modified"),
		))
	})

	It("Test only added change", func() {
		Expect(yaml.Unmarshal([]byte(appOnlyAddYAML), appOnlyAdd)).Should(Succeed())
		diffResultStr := diffAndPrint(appOnlyAdd)
		Expect(diffResultStr).Should(SatisfyAll(
			ContainSubstring("Application (livediff-demo) has been modified"),
			ContainSubstring("Component (myweb-1) has no change"),
			ContainSubstring("Component (myweb-1) / Trait (myingress/service) has no change"),
			ContainSubstring("Component (myweb-1) / Trait (myingress/ingress) has no change"),
			ContainSubstring("Component (myweb-2) has no change"),
			ContainSubstring("Component (myweb-2) / Trait (myingress/service) has been added"),
			ContainSubstring("Component (myweb-2) / Trait (myingress/ingress) has been added"),
			ContainSubstring("Component (myweb-3) has been added"),
			ContainSubstring("Component (myweb-3) / Trait (myingress/service) has been added"),
			ContainSubstring("Component (myweb-3) / Trait (myingress/ingress) has been added"),
		))
		Expect(diffResultStr).ShouldNot(SatisfyAny(
			ContainSubstring("removed"),
		))
	})

	It("Test only modified change", func() {
		Expect(yaml.Unmarshal([]byte(appOnlyModifYAML), appOnlyModif)).Should(Succeed())
		diffResultStr := diffAndPrint(appOnlyModif)
		Expect(diffResultStr).Should(SatisfyAll(
			ContainSubstring("Application (livediff-demo) has been modified"),
			ContainSubstring("Component (myweb-1) has been modified"),
			ContainSubstring("Component (myweb-1) / Trait (myingress/service) has been modified"),
			ContainSubstring("Component (myweb-1) / Trait (myingress/ingress) has been modified"),
			ContainSubstring("Component (myweb-2) has no change"),
		))
		Expect(diffResultStr).ShouldNot(SatisfyAny(
			ContainSubstring("removed"),
			ContainSubstring("added"),
		))
	})

	It("Test only removed change", func() {
		Expect(yaml.Unmarshal([]byte(appOnlyRemoveYAML), appOnlyRemove)).Should(Succeed())
		diffResultStr := diffAndPrint(appOnlyRemove)
		Expect(diffResultStr).Should(SatisfyAll(
			ContainSubstring("Application (livediff-demo) has been modified"),
			ContainSubstring("Component (myweb-1) has no change"),
			ContainSubstring("Component (myweb-1) / Trait (myingress/service) has no change"),
			ContainSubstring("Component (myweb-1) / Trait (myingress/ingress) has no change"),
			ContainSubstring("Component (myweb-2) has been removed"),
		))
		Expect(diffResultStr).ShouldNot(SatisfyAny(
			ContainSubstring("added"),
		))
	})

	It("Test renderless diff", func() {
		liveDiffOpt := LiveDiffOption{
			DryRun: NewDryRunOption(k8sClient, cfg, dm, pd, nil, false),
			Parser: appfile.NewApplicationParser(k8sClient, dm, pd),
		}
		applyFile := func(filename string, ns string) {
			bs, err := os.ReadFile("./testdata/" + filename)
			Expect(err).Should(Succeed())
			un := &unstructured.Unstructured{}
			Expect(yaml.Unmarshal(bs, un)).Should(Succeed())
			un.SetNamespace(ns)
			Expect(k8sClient.Create(context.Background(), un)).Should(Succeed())
		}
		ctx := context.Background()
		applyFile("diff-input-app-with-externals.yaml", "default")
		applyFile("diff-apprevision.yaml", "default")
		app := &v1beta1.Application{}
		apprev := &v1beta1.ApplicationRevision{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "livediff-demo"}, app)).Should(Succeed())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "livediff-demo-v1"}, apprev)).Should(Succeed())
		reverse := false
		runDiff := func() string {
			a, b := LiveDiffObject{Application: app}, LiveDiffObject{ApplicationRevision: apprev}
			if reverse {
				a, b = b, a
			}
			de, err := liveDiffOpt.RenderlessDiff(ctx, a, b)
			Expect(err).Should(Succeed())
			buff := &bytes.Buffer{}
			reportOpt := NewReportDiffOption(-1, buff)
			reportOpt.PrintDiffReport(de)
			return buff.String()
		}
		Expect(runDiff()).Should(ContainSubstring("\"myworker\" not found"))
		applyFile("td-myingress.yaml", "vela-system")
		applyFile("cd-myworker.yaml", "vela-system")
		applyFile("wd-ref-objects.yaml", "vela-system")
		Expect(runDiff()).Should(ContainSubstring("\"deploy-livediff-demo\" not found"))
		applyFile("external-workflow.yaml", "default")
		Expect(runDiff()).Should(ContainSubstring("topology-local not found"))
		applyFile("external-policy.yaml", "default")
		Expect(runDiff()).Should(ContainSubstring("deployments.apps \"livediff-demo\" not found"))
		applyFile("livediff-demo-deploy.yaml", "default")
		e := runDiff()
		_ = e
		Expect(runDiff()).Should(SatisfyAll(
			ContainSubstring("Application (livediff-demo) has been modified(*)"),
			ContainSubstring("External Policy (topology-local) has been added(+)"),
			ContainSubstring("External Workflow (deploy-livediff-demo) has been added(+)"),
			ContainSubstring("Referred Object (apps/v1 Deployment default/livediff-demo) has been added(+)"),
		))
		reverse = true
		Expect(runDiff()).Should(SatisfyAll(
			ContainSubstring("Application (livediff-demo) has been modified(*)"),
			ContainSubstring("External Policy (topology-local) has been removed(-)"),
			ContainSubstring("External Workflow (deploy-livediff-demo) has been removed(-)"),
			ContainSubstring("Referred Object (apps/v1 Deployment default/livediff-demo) has been removed(-)"),
		))
	})

})
