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

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
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
			apply(doc, diff(doc).Token, ModeDirect)
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
			Expect(out.ModeReasons).To(ContainElement(ContainSubstring("controller hold")))
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
