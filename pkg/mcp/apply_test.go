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
	"strings"
	"time"

	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
)

// promptRecorder answers elicitation prompts with a fixed action and keeps
// what the user was shown.
type promptRecorder struct {
	action   string
	messages []string
}

func (p *promptRecorder) clientOptions() *sdk.ClientOptions {
	return &sdk.ClientOptions{ElicitationHandler: func(_ context.Context, req *sdk.ElicitRequest) (*sdk.ElicitResult, error) {
		p.messages = append(p.messages, req.Params.Message)
		return &sdk.ElicitResult{Action: p.action}, nil
	}}
}

func whoAmI(name string) WhoAmIFunc {
	return func(context.Context, client.Client) (string, error) { return name, nil }
}

// writeOptions allows writes, with direct mode only in team-a.
func writeOptions() Options {
	return Options{AllowWrite: true, DirectNamespaces: []string{"team-a"}, WhoAmI: whoAmI("alice@example.com")}
}

var _ = Describe("apply_application", func() {
	var (
		h      *harness
		prompt *promptRecorder
	)

	BeforeEach(func() { prompt = &promptRecorder{action: "accept"} })

	AfterEach(func() {
		if h != nil {
			h.close()
			h = nil
		}
	})

	diff := func(doc string) DiffOutput {
		var out DiffOutput
		res := h.call("diff_application", map[string]any{"application": doc}, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		return out
	}

	apply := func(doc, token, mode string) (*sdk.CallToolResult, ApplyOutput) {
		var out ApplyOutput
		res := h.call("apply_application", map[string]any{"application": doc, "token": token, "mode": mode}, &out)
		return res, out
	}

	liveApp := func(name string) *v1beta1.Application {
		app := &v1beta1.Application{}
		if err := h.cli.Get(context.Background(), client.ObjectKey{Namespace: "team-a", Name: name}, app); err != nil {
			return nil
		}
		return app
	}

	Context("direct mode", func() {
		It("asks the user with a server-written prompt and writes only after they accept", func() {
			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), platformFixtures()...)
			doc := shopDoc("nginx:1.27", 2, "")
			d := diff(doc)
			res, out := apply(doc, d.Token, ModeDirect)
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			Expect(out.Mode).To(Equal(ModeDirect))
			Expect(out.Applied).To(BeTrue())
			Expect(prompt.messages).To(HaveLen(1))
			Expect(prompt.messages[0]).To(ContainSubstring("apply Application shop to team-a, direct mode"))
			Expect(prompt.messages[0]).To(ContainSubstring("ADD Deployment team-a/shop"))
			Expect(prompt.messages[0]).To(ContainSubstring("Risk LOW: new-application"))

			app := liveApp("shop")
			Expect(app).NotTo(BeNil())
			Expect(app.Annotations).To(HaveKeyWithValue(annoInitiator, "alice@example.com"), "the forged initiator in the document is replaced")
			Expect(app.Annotations).To(HaveKeyWithValue(annoDiffToken, d.Token))
			Expect(app.Annotations).To(HaveKeyWithValue(annoRisk, RiskLow))
			Expect(app.Annotations).To(HaveKeyWithValue("filter.oam.dev/annotation-keys", annoInitiator+","+annoDiffToken+","+annoRisk),
				"the server's records stay on the Application and are not copied onto rendered resources")
			Expect(out.ResourceVersion).To(Equal(app.ResourceVersion))
		})

		It("writes nothing when the user declines", func() {
			prompt.action = "decline"
			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), platformFixtures()...)
			doc := shopDoc("nginx:1.27", 2, "")
			_, out := apply(doc, diff(doc).Token, ModeDirect)
			Expect(out.Applied).To(BeFalse())
			Expect(out.Declined).To(BeTrue())
			Expect(liveApp("shop")).To(BeNil())
		})

		It("is refused on a client that cannot prompt, not downgraded", func() {
			h = startHarness(writeOptions(), platformFixtures()...)
			doc := shopDoc("nginx:1.27", 2, "")
			res, _ := apply(doc, diff(doc).Token, ModeDirect)
			Expect(errorText(res)).To(ContainSubstring("needs a client that can ask the user to confirm"))
			Expect(liveApp("shop")).To(BeNil())
		})

		It("updates an existing Application, keeping its other annotations", func() {
			objs := append(platformFixtures(), shopRunning()...)
			objs[len(objs)-2].(*v1beta1.Application).Annotations = map[string]string{"owner": "team-a", annoInitiator: "old@example.com", "filter.oam.dev/annotation-keys": "secret.example/x," + annoRisk}
			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), objs...)
			doc := shopDoc("nginx:1.27", 2, "")
			d := diff(doc)
			Expect(d.Risk.Class).To(Equal(RiskLow))
			_, out := apply(doc, d.Token, ModeDirect)
			Expect(out.Applied).To(BeTrue())
			Expect(prompt.messages[0]).To(ContainSubstring("spec.template.spec.containers[0].image  nginx:1.26 -> nginx:1.27"))
			app := liveApp("shop")
			Expect(string(app.Spec.Components[0].Properties.Raw)).To(ContainSubstring("nginx:1.27"))
			Expect(app.Annotations).To(HaveKeyWithValue("owner", "team-a"))
			Expect(app.Annotations).To(HaveKeyWithValue(annoInitiator, "alice@example.com"))
			Expect(app.Annotations).To(HaveKeyWithValue("filter.oam.dev/annotation-keys", "secret.example/x,"+annoRisk+","+annoInitiator+","+annoDiffToken),
				"the user's filter entries are kept, in order, without duplicates")
		})

		It("refuses a diff against a revision other than the one running", func() {
			objs := append(platformFixtures(), shopRunning()...)
			objs[len(objs)-2].(*v1beta1.Application).Status.LatestRevision = &oamcommon.Revision{Name: "shop-v5", Revision: 5}
			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), objs...)
			doc := shopDoc("nginx:1.27", 2, "")
			var d DiffOutput
			res := h.call("diff_application", map[string]any{"application": doc, "revision": "shop-v1"}, &d)
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			Expect(d.Base).To(Equal("shop-v1"))
			res, _ = apply(doc, d.Token, ModeDirect)
			Expect(errorText(res)).To(ContainSubstring("the running revision is shop-v5"))
			Expect(prompt.messages).To(BeEmpty(), "the user is never asked about a change computed against another revision")
			Expect(string(liveApp("shop").Spec.Components[0].Properties.Raw)).To(ContainSubstring("nginx:1.26"))
			_, out := apply(doc, d.Token, ModeGitOps)
			Expect(out.YAML).NotTo(BeEmpty(), "gitops mode may keep an explicit revision")
		})

		It("refuses a diff against a revision when no Application exists", func() {
			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), append(platformFixtures(), shopRunning()[1])...)
			doc := shopDoc("nginx:1.27", 2, "")
			var d DiffOutput
			res := h.call("diff_application", map[string]any{"application": doc, "revision": "shop-v1"}, &d)
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			res, _ = apply(doc, d.Token, ModeDirect)
			Expect(errorText(res)).To(ContainSubstring("does not exist"))
			Expect(prompt.messages).To(BeEmpty())
			Expect(liveApp("shop")).To(BeNil())
		})
	})

	Context("no-op and metadata-only applies", func() {
		It("returns nothing to apply without prompting or writing", func() {
			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), append(platformFixtures(), shopRunning()...)...)
			doc := shopDoc("nginx:1.26", 2, "")
			doc = strings.Replace(doc, "  annotations:\n    mcp.oam.dev/initiator: forged@example.com\n", "", 1)
			before := liveApp("shop").ResourceVersion
			_, out := apply(doc, diff(doc).Token, ModeDirect)
			Expect(out.Applied).To(BeFalse())
			Expect(out.Next).To(ContainSubstring("Nothing to apply"))
			Expect(prompt.messages).To(BeEmpty())
			Expect(liveApp("shop").ResourceVersion).To(Equal(before))
		})

		It("still prompts for a label-only change, which reaches the rendered objects", func() {
			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), append(platformFixtures(), shopRunning()...)...)
			doc := strings.Replace(shopDoc("nginx:1.26", 2, ""), "  annotations:\n", "  labels:\n    team: a\n  annotations:\n", 1)
			_, out := apply(doc, diff(doc).Token, ModeDirect)
			Expect(out.Applied).To(BeTrue())
			Expect(prompt.messages).To(HaveLen(1))
			Expect(prompt.messages[0]).To(ContainSubstring("metadata.labels.team"))
			Expect(liveApp("shop").Labels).To(HaveKeyWithValue("team", "a"))
		})

		It("words the risk line when no rule fired", func() {
			app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "team-a"}}
			text := confirmationText(app, diffRecord{base: "shop-v1", risk: Risk{Class: RiskLow}, createdAt: time.Now()})
			Expect(text).To(ContainSubstring("no rendered resources change"))
			Expect(text).To(ContainSubstring("Risk LOW: no rendered resource changes"))
			Expect(text).NotTo(ContainSubstring("Risk LOW: \n"))
		})
	})

	Context("kubectl interplay", func() {
		It("rewrites kubectl's last-applied record so a later kubectl apply merges from the new intent", func() {
			objs := append(platformFixtures(), shopRunning()...)
			objs[len(objs)-2].(*v1beta1.Application).Annotations = map[string]string{
				"kubectl.kubernetes.io/last-applied-configuration": `{"spec":{"components":[{"properties":{"image":"nginx:1.26"}}]}}`,
			}
			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), objs...)
			doc := shopDoc("nginx:1.27", 2, "")
			_, out := apply(doc, diff(doc).Token, ModeDirect)
			Expect(out.Applied).To(BeTrue())
			record := liveApp("shop").Annotations["kubectl.kubernetes.io/last-applied-configuration"]
			Expect(record).To(ContainSubstring("nginx:1.27"))
			Expect(record).NotTo(ContainSubstring("nginx:1.26"))
			Expect(record).NotTo(ContainSubstring("mcp.oam.dev"), "the record holds what was submitted, not the server's stamps")
		})

		It("does not add a kubectl record to an Application kubectl never applied", func() {
			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), append(platformFixtures(), shopRunning()...)...)
			doc := shopDoc("nginx:1.27", 2, "")
			res, out := apply(doc, diff(doc).Token, ModeDirect)
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			Expect(out.Applied).To(BeTrue())
			Expect(liveApp("shop").Annotations).NotTo(HaveKey("kubectl.kubernetes.io/last-applied-configuration"))
		})
	})

	Context("mode policy", func() {
		It("defaults to gated, which falls back to gitops and writes nothing", func() {
			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), platformFixtures()...)
			doc := shopDoc("nginx:1.27", 2, "")
			res, out := apply(doc, diff(doc).Token, "")
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			Expect(out.Mode).To(Equal(ModeGitOps))
			Expect(out.Applied).To(BeFalse())
			Expect(out.ModeReasons).To(ContainElement(ContainSubstring("hold the change for approval")))
			Expect(out.YAML).To(ContainSubstring("kind: Application"))
			Expect(out.YAML).To(ContainSubstring("image: nginx:1.27"))
			Expect(out.YAML).NotTo(ContainSubstring("mcp.oam.dev"))
			Expect(out.YAML).NotTo(ContainSubstring("status:"))
			Expect(prompt.messages).To(BeEmpty())
			Expect(liveApp("shop")).To(BeNil())
		})

		It("never goes direct outside the direct allowlist", func() {
			opts := writeOptions()
			opts.DirectNamespaces = []string{"dev"}
			h = startHarnessWithClient(opts, prompt.clientOptions(), platformFixtures()...)
			doc := shopDoc("nginx:1.27", 2, "")
			_, out := apply(doc, diff(doc).Token, ModeDirect)
			Expect(out.Mode).To(Equal(ModeGitOps))
			Expect(out.ModeReasons).To(ContainElement(ContainSubstring("not on this server's direct allowlist")))
			Expect(liveApp("shop")).To(BeNil())
		})

		It("never applies a HIGH risk change directly", func() {
			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), append(platformFixtures(), shopRunning()...)...)
			doc := shopDoc("evil.io/nginx:1.26", 2, "")
			d := diff(doc)
			Expect(d.Risk.Class).To(Equal(RiskHigh))
			_, out := apply(doc, d.Token, ModeDirect)
			Expect(out.Mode).To(Equal(ModeGitOps))
			Expect(out.ModeReasons).To(ContainElement(ContainSubstring("HIGH risk changes are never applied directly")))
			Expect(prompt.messages).To(BeEmpty())
			Expect(string(liveApp("shop").Spec.Components[0].Properties.Raw)).To(ContainSubstring("nginx:1.26"))
		})

		It("lets a namespace label or the document only tighten the mode", func() {
			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), append(platformFixtures(), namespace("team-a", map[string]string{agentWriteModeKey: ModeGitOps}))...)
			doc := shopDoc("nginx:1.27", 2, "")
			_, out := apply(doc, diff(doc).Token, ModeDirect)
			Expect(out.Mode).To(Equal(ModeGitOps))
			Expect(out.ModeReasons).To(ContainElement(ContainSubstring("namespace team-a is labelled")))
			h.close()

			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), append(platformFixtures(), namespace("team-a", map[string]string{agentWriteModeKey: ModeDirect}))...)
			doc = strings.Replace(shopDoc("nginx:1.27", 2, ""), "  annotations:\n", "  annotations:\n    "+agentWriteModeKey+": gitops\n", 1)
			_, out = apply(doc, diff(doc).Token, ModeDirect)
			Expect(out.Mode).To(Equal(ModeGitOps), "a loosening label is ignored and the document tightens")
		})

		It("treats an unrecognised write-mode value as gitops, never as no policy", func() {
			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), append(platformFixtures(), namespace("team-a", map[string]string{agentWriteModeKey: "readonly"}))...)
			doc := shopDoc("nginx:1.27", 2, "")
			_, out := apply(doc, diff(doc).Token, ModeDirect)
			Expect(out.Mode).To(Equal(ModeGitOps))
			Expect(out.ModeReasons).To(ContainElement(ContainSubstring(`unrecognised value "readonly"`)))
			Expect(prompt.messages).To(BeEmpty())
			Expect(liveApp("shop")).To(BeNil())
			h.close()

			objs := append(platformFixtures(), shopRunning()...)
			objs[len(objs)-2].(*v1beta1.Application).Annotations = map[string]string{agentWriteModeKey: "git-ops"}
			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), objs...)
			_, out = apply(doc, diff(doc).Token, ModeDirect)
			Expect(out.Mode).To(Equal(ModeGitOps))
			Expect(out.ModeReasons).To(ContainElement(ContainSubstring(`unrecognised value "git-ops"`)))
			Expect(prompt.messages).To(BeEmpty())
			Expect(string(liveApp("shop").Spec.Components[0].Properties.Raw)).To(ContainSubstring("nginx:1.26"))
			h.close()

			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), platformFixtures()...)
			doc = strings.Replace(shopDoc("nginx:1.27", 2, ""), "  annotations:\n", "  annotations:\n    "+agentWriteModeKey+": read-only\n", 1)
			_, out = apply(doc, diff(doc).Token, ModeDirect)
			Expect(out.Mode).To(Equal(ModeGitOps))
			Expect(out.ModeReasons).To(ContainElement(ContainSubstring(`the submitted Application is annotated`)))
			Expect(out.ModeReasons).To(ContainElement(ContainSubstring(`unrecognised value "read-only"`)))
			Expect(prompt.messages).To(BeEmpty())
			Expect(liveApp("shop")).To(BeNil())
		})

		It("refuses to start with a production namespace on the direct allowlist", func() {
			_, err := NewServer(velacmd.NewTestFactory(nil, nil), Options{AllowWrite: true, DirectNamespaces: []string{"payments-prod"}})
			Expect(err).To(MatchError(ContainSubstring("matches the production pattern")))
			_, err = NewServer(velacmd.NewTestFactory(nil, nil), Options{AllowWrite: true, DirectNamespaces: []string{"dev"}})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("token binding", func() {
		It("requires a token for this exact document and uses it once", func() {
			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), platformFixtures()...)
			doc := shopDoc("nginx:1.27", 2, "")
			d := diff(doc)
			res, _ := apply(doc+"\n", d.Token, ModeDirect)
			Expect(errorText(res)).To(ContainSubstring("issued for a different document"))
			res, _ = apply(doc, "sha256:unknown", ModeDirect)
			Expect(errorText(res)).To(ContainSubstring("unknown or expired"))
			_, out := apply(doc, d.Token, ModeDirect)
			Expect(out.Applied).To(BeTrue(), "a mismatch does not burn the token")
			res, _ = apply(doc, d.Token, ModeDirect)
			Expect(errorText(res)).To(ContainSubstring("unknown or expired"), "a token is used once")
		})

		It("rejects a confirmation the server never asked for", func() {
			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), platformFixtures()...)
			doc := shopDoc("nginx:1.27", 2, "")
			d := diff(doc)
			for _, state := range []string{"", "made-up-nonce"} {
				res, err := h.session.CallTool(h.ctx, &sdk.CallToolParams{
					Name:           "apply_application",
					Arguments:      map[string]any{"application": doc, "token": d.Token, "mode": ModeDirect},
					InputResponses: sdk.InputResponseMap{confirmInputKey: &sdk.ElicitResult{Action: "accept"}},
					RequestState:   state,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(errorText(res)).To(ContainSubstring("does not answer a prompt this server issued"), "state %q", state)
			}
			Expect(liveApp("shop")).To(BeNil())
			_, out := apply(doc, d.Token, ModeDirect)
			Expect(out.Applied).To(BeTrue(), "the token survives a forged answer and works through a real prompt")
		})

		It("refuses when the Application changed after the diff", func() {
			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), append(platformFixtures(), shopRunning()...)...)
			doc := shopDoc("nginx:1.27", 2, "")
			d := diff(doc)
			app := liveApp("shop")
			app.Labels = map[string]string{"touched": "by-someone-else"}
			Expect(h.cli.Update(context.Background(), app)).To(Succeed())
			res, _ := apply(doc, d.Token, ModeDirect)
			Expect(errorText(res)).To(ContainSubstring("changed since the diff"))
			Expect(prompt.messages).To(BeEmpty(), "the user is never asked about a stale change")
		})

		It("lets status updates by the controller through but refuses a spec change", func() {
			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), append(platformFixtures(), shopRunning()...)...)
			doc := shopDoc("nginx:1.27", 2, "")
			d := diff(doc)
			app := liveApp("shop")
			before := app.ResourceVersion
			app.Status.Phase = "runningWorkflow"
			app.Annotations = map[string]string{"oam.dev/kubevela-version": "v1.11.0"}
			Expect(h.cli.Update(context.Background(), app)).To(Succeed())
			Expect(liveApp("shop").ResourceVersion).NotTo(Equal(before), "the update moved the resourceVersion, as a status write does")
			_, out := apply(doc, d.Token, ModeDirect)
			Expect(out.Applied).To(BeTrue(), "a status write and a controller annotation are not a change of intent")

			d = diff(shopDoc("nginx:1.28", 2, ""))
			app = liveApp("shop")
			app.Generation++
			Expect(h.cli.Update(context.Background(), app)).To(Succeed())
			res, _ := apply(shopDoc("nginx:1.28", 2, ""), d.Token, ModeDirect)
			Expect(errorText(res)).To(ContainSubstring("spec changed since the diff"))
		})

		It("refuses when an Application appeared after a diff against nothing", func() {
			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), platformFixtures()...)
			doc := shopDoc("nginx:1.27", 2, "")
			d := diff(doc)
			Expect(h.cli.Create(context.Background(), &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "team-a"}})).To(Succeed())
			res, _ := apply(doc, d.Token, ModeDirect)
			Expect(errorText(res)).To(ContainSubstring("was created since the diff"))
		})

		It("refuses when the Application was deleted and recreated under the same name", func() {
			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), append(platformFixtures(), shopRunning()...)...)
			doc := shopDoc("nginx:1.27", 2, "")
			d := diff(doc)
			old := liveApp("shop")
			Expect(h.cli.Delete(context.Background(), old)).To(Succeed())
			again := old.DeepCopy()
			again.ResourceVersion, again.UID = "", "recreated"
			Expect(h.cli.Create(context.Background(), again)).To(Succeed())
			Expect(liveApp("shop").Generation).To(Equal(old.Generation), "the replacement matches on everything but its UID")
			res, _ := apply(doc, d.Token, ModeDirect)
			Expect(errorText(res)).To(ContainSubstring("deleted and recreated since the diff"))
			Expect(prompt.messages).To(BeEmpty())
		})
	})

	Context("what a direct write must not trust", func() {
		It("refuses direct for a diff that used local definitions, which the token does not bind", func() {
			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), append(platformFixtures(), shopRunning()...)...)
			local := `apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: webservice
  namespace: vela-system
spec:
  schematic:
    cue:
      template: |
        output: {apiVersion: "apps/v1", kind: "Deployment", spec: template: spec: containers: [{image: parameter.image}]}
        parameter: image: string
`
			doc := shopDoc("nginx:1.27", 2, "")
			var d DiffOutput
			res := h.call("diff_application", map[string]any{"application": doc, "definitions": []string{local}}, &d)
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			res, _ = apply(doc, d.Token, ModeDirect)
			Expect(errorText(res)).To(ContainSubstring("local definitions"))
			Expect(prompt.messages).To(BeEmpty())
			_, out := apply(doc, d.Token, ModeGitOps)
			Expect(out.YAML).NotTo(BeEmpty(), "the same token still produces a pull request")
		})

		It("refuses direct when the Application renders into a namespace outside the allowlist", func() {
			shared := &v1beta1.ComponentDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "shared-config", Namespace: types.DefaultKubeVelaNS},
				Spec: v1beta1.ComponentDefinitionSpec{Schematic: &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: `output: {apiVersion: "v1", kind: "ConfigMap", metadata: {name: context.name, namespace: "shared"}, data: a: "b"}
parameter: {}
`}}},
			}
			opts := writeOptions()
			opts.Namespaces = []string{"team-a"}
			h = startHarnessWithClient(opts, prompt.clientOptions(), append(platformFixtures(), shared)...)
			doc := `apiVersion: core.oam.dev/v1beta1
kind: Application
metadata: {name: cfg, namespace: team-a}
spec:
  components:
  - {name: cfg, type: shared-config}
`
			d := diff(doc)
			res, _ := apply(doc, d.Token, ModeDirect)
			Expect(errorText(res)).To(ContainSubstring("namespace shared"))
			Expect(prompt.messages).To(BeEmpty())
		})

		// renderingDef is a component definition whose output is the given
		// CUE object, for specs about where rendered objects land.
		renderingDef := func(name, output string) *v1beta1.ComponentDefinition {
			return &v1beta1.ComponentDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: types.DefaultKubeVelaNS},
				Spec:       v1beta1.ComponentDefinitionSpec{Schematic: &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: "output: " + output + "\nparameter: {}\n"}}},
			}
		}
		appOf := func(componentType string) string {
			return "apiVersion: core.oam.dev/v1beta1\nkind: Application\nmetadata: {name: cfg, namespace: team-a}\nspec:\n  components:\n  - {name: cfg, type: " + componentType + "}\n"
		}

		It("refuses direct when a rendered namespace is not on the direct allowlist, even on an unrestricted server", func() {
			// DirectNamespaces are the only namespaces where direct mode may
			// write, which covers every namespace the render writes into, not
			// only the Application's own.
			def := renderingDef("payments-config", `{apiVersion: "v1", kind: "ConfigMap", metadata: {name: context.name, namespace: "payments"}, data: a: "b"}`)
			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), append(platformFixtures(), def)...)
			doc := appOf("payments-config")
			d := diff(doc)
			Expect(d.Risk.Class).NotTo(Equal(RiskHigh))
			res, _ := apply(doc, d.Token, ModeDirect)
			Expect(errorText(res)).To(ContainSubstring("namespace payments, which is not on this server's direct allowlist"))
			Expect(prompt.messages).To(BeEmpty())
			Expect(liveApp("cfg")).To(BeNil())
		})

		It("refuses direct when the Application renders a cluster-scoped object", func() {
			def := renderingDef("priority", `{apiVersion: "scheduling.k8s.io/v1", kind: "PriorityClass", metadata: name: context.name, value: 1000}`)
			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), append(platformFixtures(), def)...)
			doc := appOf("priority")
			d := diff(doc)
			Expect(d.Risk.Class).NotTo(Equal(RiskHigh))
			res, _ := apply(doc, d.Token, ModeDirect)
			Expect(errorText(res)).To(ContainSubstring("cluster-scoped PriorityClass cfg"))
			Expect(prompt.messages).To(BeEmpty())
			Expect(liveApp("cfg")).To(BeNil())
		})

		It("refuses direct when the cluster cannot say whether a rendered kind is namespaced", func() {
			def := renderingDef("widget", `{apiVersion: "example.com/v1", kind: "Widget", metadata: name: context.name}`)
			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), append(platformFixtures(), def)...)
			doc := appOf("widget")
			d := diff(doc)
			Expect(d.Risk.Class).NotTo(Equal(RiskHigh))
			Expect(d.ChangeSet.Resources[0].Namespace).To(Equal("team-a"), "the listing is unchanged")
			res, _ := apply(doc, d.Token, ModeDirect)
			Expect(errorText(res)).To(ContainSubstring("whether Widget is namespaced"))
			Expect(prompt.messages).To(BeEmpty())
			Expect(liveApp("cfg")).To(BeNil())
		})

		Context("topology placement", func() {
			// runningWith is shop running with the given policies in both the
			// live Application and its revision, so they are unchanged.
			// deployStep is the definition the workflow a topology implies uses.
			deployStep := func() client.Object {
				return &v1beta1.WorkflowStepDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: "deploy", Namespace: types.DefaultKubeVelaNS},
					Spec:       v1beta1.WorkflowStepDefinitionSpec{Schematic: &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: "parameter: {}\n"}}},
				}
			}
			runningWith := func(policies ...v1beta1.AppPolicy) []client.Object {
				objs := shopRunning()
				objs[0].(*v1beta1.Application).Spec.Policies = policies
				objs[1].(*v1beta1.ApplicationRevision).Spec.Application.Spec.Policies = policies
				return append(append(platformFixtures(), objs...), deployStep())
			}
			policiesYAML := func(props string) string {
				return "  policies:\n  - name: where\n    type: topology\n    properties: " + props + "\n"
			}
			topology := func(props string) v1beta1.AppPolicy {
				return v1beta1.AppPolicy{Name: "where", Type: "topology", Properties: raw(props)}
			}
			refused := func(props, want string) {
				h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), runningWith(topology(props))...)
				doc := shopDoc("nginx:1.27", 2, policiesYAML(props))
				d := diff(doc)
				Expect(d.ChangeSet.PlacementChanged).To(BeFalse(), "the placement is unchanged, so only the gate can stop it")
				Expect(d.Risk.Class).To(Equal(RiskLow))
				res, _ := apply(doc, d.Token, ModeDirect)
				Expect(errorText(res)).To(ContainSubstring(want))
				Expect(prompt.messages).To(BeEmpty())
				Expect(string(liveApp("shop").Spec.Components[0].Properties.Raw)).To(ContainSubstring("nginx:1.26"))
			}

			It("refuses an unchanged topology that places resources outside the direct allowlist", func() {
				refused(`{"namespace":"prod-payments"}`, "topology policy where places resources in namespace prod-payments, which is not on this server's direct allowlist")
			})

			It("refuses a topology that selects clusters by name", func() {
				refused(`{"clusters":["local"]}`, "topology policy where selects clusters")
			})

			It("refuses a topology that selects clusters by label", func() {
				refused(`{"clusterLabelSelector":{"region":"eu"}}`, "topology policy where selects clusters")
			})

			It("refuses a deploy step whose policy is not declared in the Application", func() {
				shared := &v1alpha1.Policy{
					ObjectMeta: metav1.ObjectMeta{Name: "shared-placement", Namespace: "team-a"},
					Type:       "topology",
					Properties: raw(`{"namespace":"prod-payments"}`),
				}
				h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), append(append(platformFixtures(), shopRunning()...), deployStep(), shared)...)
				doc := shopDoc("nginx:1.27", 2, `  workflow:
    steps:
    - name: ship
      type: deploy
      properties:
        policies: ["shared-placement"]
`)
				d := diff(doc)
				Expect(d.Risk.Class).NotTo(Equal(RiskHigh))
				res, _ := apply(doc, d.Token, ModeDirect)
				Expect(errorText(res)).To(ContainSubstring("policy shared-placement, which the Application does not declare"))
				Expect(prompt.messages).To(BeEmpty())
				Expect(string(liveApp("shop").Spec.Components[0].Properties.Raw)).To(ContainSubstring("nginx:1.26"))
			})

			It("refuses a workflow that refers to an external Workflow object, whose steps the diff cannot see", func() {
				flow := &wfTypesv1alpha1.Workflow{
					ObjectMeta:   metav1.ObjectMeta{Name: "shared-flow", Namespace: "team-a"},
					WorkflowSpec: wfTypesv1alpha1.WorkflowSpec{Steps: stepsOf("deploy")},
				}
				h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), append(append(platformFixtures(), shopRunning()...), deployStep(), flow)...)
				doc := shopDoc("nginx:1.27", 2, "  workflow:\n    ref: shared-flow\n")
				d := diff(doc)
				Expect(d.Risk.Class).NotTo(Equal(RiskHigh))
				res, _ := apply(doc, d.Token, ModeDirect)
				Expect(errorText(res)).To(ContainSubstring("the workflow refers to Workflow shared-flow"))
				Expect(prompt.messages).To(BeEmpty())
				Expect(string(liveApp("shop").Spec.Components[0].Properties.Raw)).To(ContainSubstring("nginx:1.26"))
			})

			It("refuses a policy of the deprecated env-binding type, whose placement is not read", func() {
				envs := v1beta1.AppPolicy{Name: "envs", Type: "env-binding", Properties: raw(`{"envs":[]}`)}
				h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), runningWith(envs)...)
				doc := shopDoc("nginx:1.27", 2, "  policies:\n  - name: envs\n    type: env-binding\n    properties: {\"envs\":[]}\n")
				d := diff(doc)
				Expect(d.Risk.Class).NotTo(Equal(RiskHigh))
				res, _ := apply(doc, d.Token, ModeDirect)
				Expect(errorText(res)).To(ContainSubstring("policy envs is of the deprecated type env-binding"))
				Expect(prompt.messages).To(BeEmpty())
				Expect(string(liveApp("shop").Spec.Components[0].Properties.Raw)).To(ContainSubstring("nginx:1.26"))
			})

			It("lets an inline topology whose namespace is on the direct allowlist go direct", func() {
				props := `{"namespace":"team-a"}`
				h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), runningWith(topology(props))...)
				doc := shopDoc("nginx:1.27", 2, policiesYAML(props))
				d := diff(doc)
				res, out := apply(doc, d.Token, ModeDirect)
				Expect(res.IsError).To(BeFalse(), toJSON(res))
				Expect(out.Applied).To(BeTrue())
				Expect(string(liveApp("shop").Spec.Components[0].Properties.Raw)).To(ContainSubstring("nginx:1.27"))
			})
		})

		It("refuses direct when the workflow runs steps the diff cannot see into, even on an unrestricted server", func() {
			// Direct mode may write only into DirectNamespaces, and the diff
			// cannot see where an apply-object or webhook step writes.
			opts := writeOptions()
			applyObject := &v1beta1.WorkflowStepDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "apply-object", Namespace: types.DefaultKubeVelaNS},
				Spec:       v1beta1.WorkflowStepDefinitionSpec{Schematic: &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: "parameter: {}\n"}}},
			}
			h = startHarnessWithClient(opts, prompt.clientOptions(), append(append(platformFixtures(), shopRunning()...), applyObject)...)
			doc := shopDoc("nginx:1.27", 2, `  workflow:
    steps:
    - name: deploy
      type: apply-component
      properties:
        component: shop
    - name: extra
      type: apply-object
`)
			d := diff(doc)
			res, _ := apply(doc, d.Token, ModeDirect)
			Expect(errorText(res)).To(ContainSubstring("the workflow runs apply-object"))
			Expect(prompt.messages).To(BeEmpty())
		})

		It("shows Application-level changes in the prompt and does not treat them as a no-op", func() {
			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), append(platformFixtures(), shopRunning()...)...)
			doc := shopDoc("nginx:1.26", 2, `  workflow:
    steps:
    - name: deploy
      type: apply-component
      properties:
        component: shop
`)
			d := diff(doc)
			Expect(d.ChangeSet.Resources).To(BeEmpty(), "the workflow change renders the same objects")
			Expect(rulesOf(d.Risk)).To(ContainElement("workflow-changed"))
			_, out := apply(doc, d.Token, ModeDirect)
			Expect(out.Applied).To(BeTrue())
			Expect(prompt.messages).To(HaveLen(1))
			Expect(prompt.messages[0]).To(ContainSubstring("the workflow changes"))
		})

		It("leaves controller and server annotations out of the GitOps document", func() {
			h = startHarnessWithClient(writeOptions(), prompt.clientOptions(), platformFixtures()...)
			doc := shopDoc("nginx:1.27", 2, "") + `
`
			doc = strings.Replace(doc, "    mcp.oam.dev/initiator: forged@example.com", "    mcp.oam.dev/initiator: forged@example.com\n    oam.dev/kubevela-version: v1.11.0\n    team: shop", 1)
			d := diff(doc)
			_, out := apply(doc, d.Token, ModeGitOps)
			Expect(out.YAML).To(ContainSubstring("team: shop"))
			Expect(out.YAML).NotTo(ContainSubstring("kubevela-version"))
			Expect(out.YAML).NotTo(ContainSubstring("mcp.oam.dev"))
		})
	})

	Context("concurrent writes during the apply", func() {
		// raceOnce makes one write to the live Application just before the
		// server's first update, the way a reconcile can land between the
		// server's read and its write.
		raceOnce := func(change func(*v1beta1.Application)) (*interceptor.Funcs, *bool) {
			raced := false
			return &interceptor.Funcs{Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				if _, ok := obj.(*v1beta1.Application); ok && !raced {
					raced = true
					live := &v1beta1.Application{}
					if err := c.Get(ctx, client.ObjectKeyFromObject(obj), live); err != nil {
						return err
					}
					change(live)
					if err := c.Update(ctx, live); err != nil {
						return err
					}
				}
				return c.Update(ctx, obj, opts...)
			}}, &raced
		}

		It("retries past a status write by the controller", func() {
			funcs, raced := raceOnce(func(app *v1beta1.Application) { app.Status.Phase = "runningWorkflow" })
			h = startHarnessIntercepted(writeOptions(), prompt.clientOptions(), funcs, append(platformFixtures(), shopRunning()...)...)
			doc := shopDoc("nginx:1.27", 2, "")
			d := diff(doc)
			res, out := apply(doc, d.Token, ModeDirect)
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			Expect(*raced).To(BeTrue(), "the status write landed between the read and the update")
			Expect(out.Applied).To(BeTrue())
			Expect(string(liveApp("shop").Spec.Components[0].Properties.Raw)).To(ContainSubstring("nginx:1.27"))
		})

		It("refuses when the running revision moves during the apply", func() {
			funcs, raced := raceOnce(func(app *v1beta1.Application) {
				app.Status.LatestRevision = &oamcommon.Revision{Name: "shop-v2", Revision: 2}
			})
			h = startHarnessIntercepted(writeOptions(), prompt.clientOptions(), funcs, append(platformFixtures(), shopRunning()...)...)
			doc := shopDoc("nginx:1.27", 2, "")
			d := diff(doc)
			res, out := apply(doc, d.Token, ModeDirect)
			Expect(*raced).To(BeTrue())
			Expect(out.Applied).To(BeFalse())
			Expect(errorText(res)).To(ContainSubstring("the running revision is shop-v2"))
			Expect(string(liveApp("shop").Spec.Components[0].Properties.Raw)).NotTo(ContainSubstring("nginx:1.27"))
		})

		It("still refuses a spec change that lands during the apply", func() {
			funcs, raced := raceOnce(func(app *v1beta1.Application) { app.Generation++ })
			h = startHarnessIntercepted(writeOptions(), prompt.clientOptions(), funcs, append(platformFixtures(), shopRunning()...)...)
			doc := shopDoc("nginx:1.27", 2, "")
			d := diff(doc)
			res, out := apply(doc, d.Token, ModeDirect)
			Expect(*raced).To(BeTrue())
			Expect(out.Applied).To(BeFalse())
			Expect(errorText(res)).To(ContainSubstring("spec changed since the diff"))
			Expect(string(liveApp("shop").Spec.Components[0].Properties.Raw)).NotTo(ContainSubstring("nginx:1.27"))
		})
	})
})
