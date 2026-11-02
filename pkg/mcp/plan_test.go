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
	"context"
	"fmt"
	"strings"

	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
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
				Application: v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "team-a", Annotations: map[string]string{"oam.dev/kubevela-version": "v1.11.0", "mcp.oam.dev/risk": "LOW", "mcp.oam.dev/initiator": "bob@example.com"}}, Spec: spec},
			}},
		},
	}
}

// bucketCUE is what vela def gen-module writes for a defkit component that
// emits an ACK S3 Bucket. It is kept verbatim so the test feeds the server
// the same text an author's generated file contains.
const bucketCUE = `"aws-s3-bucket": {
	type: "component"
	annotations: {}
	labels: {}
	description: "S3 bucket provisioned by the ACK S3 controller"
	attributes: {
		workload: {
			definition: {
				apiVersion: "s3.services.k8s.aws/v1alpha1"
				kind:       "Bucket"
			}
			type: "buckets.s3.services.k8s.aws"
		}
	}
}
template: {
	output: {
		apiVersion: "s3.services.k8s.aws/v1alpha1"
		kind:       "Bucket"
		metadata: {
			name: context.name
		}
		spec: {
			name: parameter.bucketName
			versioning: {
				status: parameter.versioning
			}
		}
	}
	parameter: {
		// +usage=Globally unique S3 bucket name
		bucketName!: string
		// +usage=Enabled or Suspended
		versioning: *"Suspended" | string
	}
}
`

func bucketDoc(bucket string) string {
	return fmt.Sprintf(`apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: logs
  namespace: team-a
spec:
  components:
  - name: logs
    type: aws-s3-bucket
    properties:
      bucketName: %s
`, bucket)
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

		It("renders against the CUE vela def gen-module generates for a defkit module", func() {
			h = startHarness(Options{}, platformFixtures()...)
			var out DryRunOutput
			res := h.call("dryrun_application", map[string]any{"application": bucketDoc("acme-logs"), "definitions": []string{bucketCUE}}, &out)
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			Expect(out.Resources).To(HaveLen(1))
			Expect(out.Resources[0].Kind).To(Equal("Bucket"))
			Expect(out.Resources[0].APIVersion).To(Equal("s3.services.k8s.aws/v1alpha1"))
			Expect(toJSON(out.Resources[0].Object)).To(ContainSubstring(`"name":"acme-logs"`))
			Expect(toJSON(out.Resources[0].Object)).To(ContainSubstring(`"status":"Suspended"`), "the defkit default applies")
			Expect(out.ServerCheck).To(BeNil(), "the server check runs only on request")
		})

		Context("with serverCheck", func() {
			// recordCreates captures the options of every create and can
			// reject one kind the way a strict API server rejects an unknown field.
			recordCreates := func(reject string) (*interceptor.Funcs, *[]client.CreateOptions) {
				var seen []client.CreateOptions
				return &interceptor.Funcs{Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					o := client.CreateOptions{}
					o.ApplyOptions(opts)
					seen = append(seen, o)
					if obj.GetObjectKind().GroupVersionKind().Kind == reject {
						return apierrors.NewBadRequest(`strict decoding error: unknown field "spec.nmae"`)
					}
					return c.Create(ctx, obj, opts...)
				}}, &seen
			}
			liveBucket := func() error {
				u := &unstructured.Unstructured{}
				u.SetAPIVersion("s3.services.k8s.aws/v1alpha1")
				u.SetKind("Bucket")
				return h.cli.Get(context.Background(), client.ObjectKey{Namespace: "team-a", Name: "logs"}, u)
			}

			It("sends each object as a strict dry run and stores nothing", func() {
				funcs, seen := recordCreates("")
				h = startHarnessIntercepted(Options{}, nil, funcs, platformFixtures()...)
				var out DryRunOutput
				res := h.call("dryrun_application", map[string]any{"application": bucketDoc("acme-logs"), "definitions": []string{bucketCUE}, "serverCheck": true}, &out)
				Expect(res.IsError).To(BeFalse(), toJSON(res))
				Expect(out.ServerCheck).NotTo(BeNil())
				Expect(out.ServerCheck.Objects).To(ConsistOf(ObjectCheck{Kind: "Bucket", Namespace: "team-a", Name: "logs", Accepted: true}))
				Expect(*seen).To(HaveLen(1))
				Expect((*seen)[0].DryRun).To(ConsistOf(metav1.DryRunAll))
				Expect((*seen)[0].FieldValidation).To(Equal("Strict"))
				Expect(apierrors.IsNotFound(liveBucket())).To(BeTrue(), "a dry run stores nothing")
			})

			It("reports the API server's reason when an object is rejected", func() {
				funcs, _ := recordCreates("Bucket")
				h = startHarnessIntercepted(Options{}, nil, funcs, platformFixtures()...)
				var out DryRunOutput
				h.call("dryrun_application", map[string]any{"application": bucketDoc("acme-logs"), "definitions": []string{bucketCUE}, "serverCheck": true}, &out)
				Expect(out.ServerCheck.Objects).To(HaveLen(1))
				Expect(out.ServerCheck.Objects[0].Accepted).To(BeFalse())
				Expect(out.ServerCheck.Objects[0].Message).To(ContainSubstring(`unknown field "spec.nmae"`))
			})

			It("checks an object that already exists as an update of the live copy", func() {
				existing := &unstructured.Unstructured{}
				existing.SetAPIVersion("s3.services.k8s.aws/v1alpha1")
				existing.SetKind("Bucket")
				existing.SetNamespace("team-a")
				existing.SetName("logs")
				Expect(unstructured.SetNestedField(existing.Object, "old-name", "spec", "name")).To(Succeed())
				h = startHarness(Options{}, append(platformFixtures(), existing)...)
				var out DryRunOutput
				h.call("dryrun_application", map[string]any{"application": bucketDoc("acme-logs"), "definitions": []string{bucketCUE}, "serverCheck": true}, &out)
				Expect(out.ServerCheck.Objects).To(ConsistOf(ObjectCheck{Kind: "Bucket", Namespace: "team-a", Name: "logs", Accepted: true}))
				live := &unstructured.Unstructured{}
				live.SetAPIVersion("s3.services.k8s.aws/v1alpha1")
				live.SetKind("Bucket")
				Expect(h.cli.Get(context.Background(), client.ObjectKey{Namespace: "team-a", Name: "logs"}, live)).To(Succeed())
				name, _, _ := unstructured.NestedString(live.Object, "spec", "name")
				Expect(name).To(Equal("old-name"), "the dry-run update left the live object alone")
			})

			It("checks and diffs the objects a policy renders, not only component output", func() {
			policyDef := `apiVersion: core.oam.dev/v1beta1
kind: PolicyDefinition
metadata:
  name: quota
  namespace: vela-system
spec:
  schematic:
    cue:
      template: |
        output: {apiVersion: "v1", kind: "ResourceQuota", metadata: name: "team-quota", spec: hard: pods: parameter.pods}
        parameter: pods: string
`
			funcs, seen := recordCreates("")
			h = startHarnessIntercepted(Options{}, nil, funcs, platformFixtures()...)
			doc := shopDoc("nginx:1.27", 1, `  policies:
  - name: quota
    type: quota
    properties:
      pods: "10"
`)
			var out DryRunOutput
			res := h.call("dryrun_application", map[string]any{"application": doc, "definitions": []string{policyDef}, "serverCheck": true}, &out)
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			Expect(out.Policies).To(ContainElement(HaveField("Kind", "ResourceQuota")))
			kinds := []string{}
			for _, c := range out.ServerCheck.Objects {
				kinds = append(kinds, c.Kind)
			}
			Expect(kinds).To(ContainElement("ResourceQuota"), "policy objects reach the API server check")
			Expect(len(*seen)).To(Equal(len(out.ServerCheck.Objects)))
		})

		It("does not send objects in a namespace outside the allowlist", func() {
				funcs, seen := recordCreates("")
				h = startHarnessIntercepted(Options{Namespaces: []string{"team-a"}}, nil, funcs, platformFixtures()...)
				elsewhere := strings.Replace(bucketCUE, "name: context.name", "name: context.name\n\t\t\tnamespace: \"shared\"", 1)
				Expect(elsewhere).NotTo(Equal(bucketCUE))
				var out DryRunOutput
				res := h.call("dryrun_application", map[string]any{"application": bucketDoc("acme-logs"), "definitions": []string{elsewhere}, "serverCheck": true}, &out)
				Expect(res.IsError).To(BeFalse(), toJSON(res))
				Expect(out.ServerCheck.Objects).To(BeEmpty())
				Expect(out.ServerCheck.Skipped).To(ConsistOf(ContainSubstring("Bucket shared/logs")))
				Expect(*seen).To(BeEmpty(), "nothing outside the allowlist reaches the API server")
			})
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

		Context("when the revision recorded its definitions", func() {
			// withRecorded stores a webservice snapshot in shop-v1, as the
			// controller does, so the base renders as what is running.
			withRecorded := func(template string) []client.Object {
				objs := shopRunning()
				rev := objs[1].(*v1beta1.ApplicationRevision)
				rev.Spec.ComponentDefinitions = map[string]*v1beta1.ComponentDefinition{"webservice": {
					ObjectMeta: metav1.ObjectMeta{Name: "webservice", Namespace: types.DefaultKubeVelaNS},
					Spec: v1beta1.ComponentDefinitionSpec{
						Version:   "2.1.0",
						Schematic: &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: template}},
					},
				}}
				return append(platformFixtures(), objs...)
			}

			It("renders the base with them and reports no change when nothing drifted", func() {
				h = startHarness(Options{}, withRecorded(webserviceTemplate)...)
				var out DiffOutput
				h.call("diff_application", map[string]any{"application": shopDoc("nginx:1.26", 2, "")}, &out)
				Expect(out.ChangeSet.Resources).To(BeEmpty())
				Expect(out.Evidence).To(ContainElement(ContainSubstring("with the definitions it recorded (1)")))
				Expect(out.Meta.Warnings).NotTo(ContainElement(ContainSubstring("changed since")))
			})

			It("shows what a changed definition will change on apply and says where it came from", func() {
				older := strings.Replace(webserviceTemplate, "\t\tports: [{containerPort: parameter.port}]\n", "", 1)
				Expect(older).NotTo(Equal(webserviceTemplate))
				h = startHarness(Options{}, withRecorded(older)...)
				var out DiffOutput
				h.call("diff_application", map[string]any{"application": shopDoc("nginx:1.26", 2, "")}, &out)
				Expect(out.ChangeSet.Resources).To(HaveLen(1), "the running objects lack the port today's definition adds")
				Expect(out.ChangeSet.Resources[0].Action).To(Equal(ActionModify))
				Expect(toJSON(out.ChangeSet.Resources[0].Fields)).To(ContainSubstring("ports"))
				Expect(out.Meta.Warnings).To(ContainElement(ContainSubstring("definition ComponentDefinition webservice changed since shop-v1")))
			})
		})

		It("refuses a revision that belongs to another Application", func() {
			other := &v1beta1.ApplicationRevision{ObjectMeta: metav1.ObjectMeta{Name: "billing-v4", Namespace: "team-a", Labels: map[string]string{oam.LabelAppName: "billing"}}}
			h = startHarness(Options{}, append(append(platformFixtures(), shopRunning()...), other)...)
			res := h.call("diff_application", map[string]any{"application": shopDoc("nginx:1.27", 2, ""), "revision": "billing-v4"}, nil)
			Expect(errorText(res)).To(ContainSubstring("belongs to another Application"))
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
