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

package mcp

import (
	"fmt"
	"strings"

	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func stepsOf(types ...string) []wfTypesv1alpha1.WorkflowStep {
	steps := make([]wfTypesv1alpha1.WorkflowStep, 0, len(types))
	for i, t := range types {
		steps = append(steps, wfTypesv1alpha1.WorkflowStep{WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{Name: fmt.Sprintf("s%d", i), Type: t}})
	}
	return steps
}

// shopDoc is an Application document as a user would write it.
func shopDoc(image string, replicas int, extra string) string {
	return strings.TrimSpace(fmt.Sprintf(`
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: shop
  namespace: team-a
  annotations:
    mcp.oam.dev/initiator: forged@example.com
spec:
  components:
  - name: shop
    type: webservice
    properties:
      image: %s
    traits:
    - type: scaler
      properties:
        replicas: %d
%s`, image, replicas, extra))
}

// shopRunning is the shop Application as deployed at image nginx:1.26 with
// two replicas, recorded in revision shop-v1.
func shopRunning() []client.Object {
	spec := v1beta1.ApplicationSpec{Components: []oamcommon.ApplicationComponent{{
		Name: "shop", Type: "webservice", Properties: raw(`{"image":"nginx:1.26"}`),
		Traits: []oamcommon.ApplicationTrait{{Type: "scaler", Properties: raw(`{"replicas":2}`)}},
	}}}
	return []client.Object{
		&v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "team-a", Generation: 1},
			Spec:       spec,
			Status:     oamcommon.AppStatus{LatestRevision: &oamcommon.Revision{Name: "shop-v1", Revision: 1}},
		},
		&v1beta1.ApplicationRevision{
			ObjectMeta: metav1.ObjectMeta{Name: "shop-v1", Namespace: "team-a"},
			Spec: v1beta1.ApplicationRevisionSpec{ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
				// The controller stamps its version on the Application it stores
				// in a revision; renders of the base carry it, the submitted
				// document does not.
				Application: v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "team-a", Annotations: map[string]string{"oam.dev/kubevela-version": "v1.11.0"}}, Spec: spec},
			}},
		},
	}
}

var _ = Describe("plan tools", func() {
	var h *harness

	AfterEach(func() {
		if h != nil {
			h.close()
			h = nil
		}
	})

	Context("dryrun_application", func() {
		It("renders the objects a document would create without saving anything", func() {
			h = startHarness(Options{}, platformFixtures()...)
			var out DryRunOutput
			res := h.call("dryrun_application", map[string]any{"application": shopDoc("nginx:1.27", 3, "")}, &out)
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			Expect(out.Application).To(Equal("shop"))
			Expect(out.Components).To(Equal([]ComponentRef{{Name: "shop", Type: "webservice"}}))
			Expect(out.Resources).To(HaveLen(1))
			dep := out.Resources[0]
			Expect(dep.Kind).To(Equal("Deployment"))
			Expect(dep.Component).To(Equal("shop"))
			Expect(dep.Namespace).To(Equal("team-a"))
			Expect(toJSON(dep.Object)).To(ContainSubstring("nginx:1.27"))
			Expect(toJSON(dep.Object)).To(ContainSubstring(`"replicas":3`))
			Expect(out.Meta.Source).To(Equal(SourceRendered))

			var apps ListApplicationsOutput
			h.call("list_applications", map[string]any{"namespace": "team-a"}, &apps)
			Expect(apps.Applications).To(BeEmpty(), "dry-run must not create the Application")
		})

		It("renders against a local definition that overrides the cluster's", func() {
			h = startHarness(Options{}, platformFixtures()...)
			local := strings.TrimSpace(`
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: webservice
  namespace: vela-system
spec:
  schematic:
    cue:
      template: |
        output: {
          apiVersion: "batch/v1"
          kind: "Job"
          spec: template: spec: containers: [{image: parameter.image}]
        }
        parameter: image: string
`)
			var out DryRunOutput
			res := h.call("dryrun_application", map[string]any{"application": shopDoc("nginx:1.27", 1, ""), "definitions": []string{local}}, &out)
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			Expect(out.Resources[0].Kind).To(Equal("Job"))
			Expect(out.Evidence).To(ContainElement(ContainSubstring("1 local definitions took priority")))
		})

		It("rejects bad input", func() {
			h = startHarness(Options{}, platformFixtures()...)
			res := h.call("dryrun_application", map[string]any{"application": "kind: Deployment\nmetadata: {name: x}"}, nil)
			Expect(errorText(res)).To(ContainSubstring("expected Application"))
			res = h.call("dryrun_application", map[string]any{"application": "apiVersion: core.oam.dev/v1beta1\nkind: Application\nmetadata: {name: x}\nspec: {components: []}"}, nil)
			Expect(errorText(res)).To(ContainSubstring("namespace is required"))
			res = h.call("dryrun_application", map[string]any{"application": shopDoc("nginx:1.27", 1, ""), "definitions": []string{"kind: Secret\nmetadata: {name: s}"}}, nil)
			Expect(errorText(res)).To(ContainSubstring("only definition kinds are accepted"))
		})

		It("checks the allowlist before rendering or reading anything", func() {
			h = startHarness(Options{Namespaces: []string{"team-b"}}, platformFixtures()...)
			for _, tool := range []string{"dryrun_application", "diff_application"} {
				res := h.call(tool, map[string]any{"application": shopDoc("nginx:1.27", 1, "")}, nil)
				Expect(errorText(res)).To(ContainSubstring("outside this server's allowed namespaces"), tool)
			}
		})
	})

	Context("diff_application", func() {
		It("treats a missing Application as new and issues a token", func() {
			h = startHarness(Options{}, platformFixtures()...)
			var out DiffOutput
			res := h.call("diff_application", map[string]any{"application": shopDoc("nginx:1.27", 2, "")}, &out)
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			Expect(out.Base).To(Equal("new"))
			Expect(out.ChangeSet.NewApplication).To(BeTrue())
			Expect(out.ChangeSet.Resources).To(HaveLen(1))
			Expect(out.ChangeSet.Resources[0].Action).To(Equal(ActionAdd))
			Expect(out.Risk.Class).To(Equal(RiskLow))
			Expect(rulesOf(out.Risk)).To(ConsistOf("new-application"))
			Expect(out.Token).To(HavePrefix("sha256:"))
			Expect(out.ExpiresAt).NotTo(BeZero())
			Expect(out.Meta.Source).To(Equal(SourceRendered))
		})

		It("classifies an image tag bump against the running revision as LOW", func() {
			h = startHarness(Options{}, append(platformFixtures(), shopRunning()...)...)
			var out DiffOutput
			res := h.call("diff_application", map[string]any{"application": shopDoc("nginx:1.27", 2, "")}, &out)
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			Expect(out.Base).To(Equal("shop-v1"))
			Expect(out.ChangeSet.NewApplication).To(BeFalse())
			Expect(out.ChangeSet.Resources).To(HaveLen(1))
			rc := out.ChangeSet.Resources[0]
			Expect(rc.Action).To(Equal(ActionModify))
			Expect(rc.Fields).To(HaveLen(1))
			Expect(rc.Fields[0].Path).To(Equal("spec.template.spec.containers[0].image"))
			Expect(rc.Fields[0].From).To(Equal("nginx:1.26"))
			Expect(rc.Fields[0].To).To(Equal("nginx:1.27"))
			Expect(out.Risk.Class).To(Equal(RiskLow))
			Expect(rulesOf(out.Risk)).To(ConsistOf("image-tag"))
			Expect(out.Evidence).To(ContainElement(ContainSubstring("ApplicationRevision team-a/shop-v1")))
		})

		It("classifies a repository change plus scaling as HIGH and lists both rules", func() {
			h = startHarness(Options{}, append(platformFixtures(), shopRunning()...)...)
			var out DiffOutput
			h.call("diff_application", map[string]any{"application": shopDoc("evil.io/nginx:1.26", 5, "")}, &out)
			Expect(out.Risk.Class).To(Equal(RiskHigh))
			Expect(rulesOf(out.Risk)).To(ConsistOf("image-repository", "scaling"))
		})

		It("reports no change and no rules for an identical document", func() {
			h = startHarness(Options{}, append(platformFixtures(), shopRunning()...)...)
			var out DiffOutput
			h.call("diff_application", map[string]any{"application": shopDoc("nginx:1.26", 2, "")}, &out)
			Expect(out.ChangeSet.Resources).To(BeEmpty())
			Expect(out.Risk.Class).To(Equal(RiskLow))
			Expect(out.Risk.Rules).To(BeEmpty())
		})

		It("binds the token to the exact document and expires it", func() {
			h = startHarness(Options{}, append(platformFixtures(), shopRunning()...)...)
			var a, b DiffOutput
			h.call("diff_application", map[string]any{"application": shopDoc("nginx:1.27", 2, "")}, &a)
			h.call("diff_application", map[string]any{"application": shopDoc("nginx:1.27", 2, "\n")}, &b)
			Expect(a.Token).NotTo(Equal(b.Token), "a whitespace edit is a different document")
			d := newDeps(h.srv.deps.f, h.srv.deps.opts)
			d.diffs = h.srv.deps.diffs
			rec, ok := d.diffs.lookup(a.Token)
			Expect(ok).To(BeTrue())
			Expect(rec.base).To(Equal("shop-v1"))
			Expect(rec.risk.Class).To(Equal(RiskLow))
			Expect(rec.liveExists).To(BeTrue())
			Expect(rec.liveRV).NotTo(BeEmpty())
			_, ok = d.diffs.lookup("sha256:nope")
			Expect(ok).To(BeFalse())
		})

		It("strips server-owned annotations from the input", func() {
			h = startHarness(Options{}, platformFixtures()...)
			var out DryRunOutput
			h.call("dryrun_application", map[string]any{"application": shopDoc("nginx:1.27", 1, "")}, &out)
			Expect(toJSON(out)).NotTo(ContainSubstring("forged@example.com"))
		})
	})
})
