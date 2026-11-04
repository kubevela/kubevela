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

package application

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// The whole admission path against a real API server: the request decoded,
// the policies and definitions read from it, the response returned.
var _ = Describe("Applications are judged by the context their policies declare", func() {
	var objs []client.Object
	var h *ValidatingHandler

	request := func(op admissionv1.Operation, compType string) admission.Request {
		app := ctxApp(compType, nil)
		app.TypeMeta = metav1.TypeMeta{APIVersion: "core.oam.dev/v1beta1", Kind: "Application"}
		app.Namespace = "default"
		raw, err := json.Marshal(app)
		Expect(err).NotTo(HaveOccurred())
		req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: op,
			Namespace: "default",
			Resource:  metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1beta1", Resource: "applications"},
			Object:    runtime.RawExtension{Raw: raw},
		}}
		if op == admissionv1.Update {
			req.OldObject = runtime.RawExtension{Raw: raw}
		}
		return req
	}

	BeforeEach(func() {
		was := map[string]bool{}
		for _, gate := range []featuregate.Feature{features.EnableApplicationScopedPolicies, features.EnableGlobalPolicies} {
			was[string(gate)] = utilfeature.DefaultMutableFeatureGate.Enabled(gate)
			Expect(utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{string(gate): true})).To(Succeed())
		}
		DeferCleanup(func() { Expect(utilfeature.DefaultMutableFeatureGate.SetFromMap(was)).To(Succeed()) })
		h = &ValidatingHandler{Client: k8sClient, Decoder: decoder}
		objs = []client.Object{
			appPolicyDef(oam.SystemDefinitionNamespace, "ctx-identity", teamString, true, 0),
			reader("ctx-reads-team", "team: context.custom.team"),
			reader("ctx-reads-mesh", "mesh: context.custom.mesh"),
			reader("ctx-guards-mesh", "if context.custom.mesh != _|_ {mesh: context.custom.mesh}"),
			reader("ctx-reads-region", "region: context.custom.region"),
		}
		for _, o := range objs {
			Expect(k8sClient.Create(context.Background(), o)).To(Succeed())
		}
		DeferCleanup(func() {
			for _, o := range objs {
				_ = k8sClient.Delete(context.Background(), o)
			}
		})
	})

	for _, op := range []admissionv1.Operation{admissionv1.Create, admissionv1.Update} {
		op := op
		It("admits a component reading a declared key on "+string(op), func() {
			resp := h.Handle(context.Background(), request(op, "ctx-reads-team"))
			Expect(resp.Allowed).To(BeTrue(), "%v", resp.Result)
		})
		It("admits a guarded read of an optional key on "+string(op), func() {
			resp := h.Handle(context.Background(), request(op, "ctx-guards-mesh"))
			Expect(resp.Allowed).To(BeTrue(), "%v", resp.Result)
		})
		It("refuses an unguarded read of an optional key on "+string(op), func() {
			resp := h.Handle(context.Background(), request(op, "ctx-reads-mesh"))
			Expect(resp.Allowed).To(BeFalse())
			Expect(resp.Result.Message).To(ContainSubstring(`context.custom.mesh is optional in policy "ctx-identity"`))
		})
		It("refuses a read of a key no policy declares on "+string(op), func() {
			resp := h.Handle(context.Background(), request(op, "ctx-reads-region"))
			Expect(resp.Allowed).To(BeFalse())
			Expect(resp.Result.Message).To(ContainSubstring("reads context.custom.region, but no applicable policy declares it"))
		})
	}

	It("judges by the policy as it now stands", func() {
		pd := &v1beta1.PolicyDefinition{}
		Expect(k8sClient.Get(context.Background(), client.ObjectKey{Namespace: oam.SystemDefinitionNamespace, Name: "ctx-identity"}, pd)).To(Succeed())
		pd.Spec.Schematic.CUE.Template = "schema: ctx: close({team: string, mesh: string})\noutput: ctx: {team: \"a\", mesh: \"m\"}"
		Expect(k8sClient.Update(context.Background(), pd)).To(Succeed())
		resp := h.Handle(context.Background(), request(admissionv1.Create, "ctx-reads-mesh"))
		Expect(resp.Allowed).To(BeTrue(), "%v", resp.Result)
	})
})
