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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// cloudResource builds an unstructured provisioner resource with conditions.
func cloudResource(apiVersion, kind, name string, generation int64, conditions []map[string]any, status map[string]any) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetAPIVersion(apiVersion)
	u.SetKind(kind)
	u.SetNamespace("team-a")
	u.SetName(name)
	u.SetGeneration(generation)
	if status == nil {
		status = map[string]any{}
	}
	if conditions != nil {
		conds := make([]any, 0, len(conditions))
		for _, c := range conditions {
			conds = append(conds, any(c))
		}
		status["conditions"] = conds
	}
	_ = unstructured.SetNestedMap(u.Object, status, "status")
	return u
}

// diagnoseApp is an Application whose tracker records the given resources.
func diagnoseApp(resources ...*unstructured.Unstructured) []client.Object {
	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "team-a", Generation: 3},
		Spec: v1beta1.ApplicationSpec{Components: []oamcommon.ApplicationComponent{
			{Name: "orders-db", Type: "webservice", Properties: raw(`{"image":"nginx"}`)},
		}},
		Status: oamcommon.AppStatus{Phase: oamcommon.ApplicationRunning, ObservedGeneration: 3,
			Services: []oamcommon.ApplicationComponentStatus{{Name: "orders-db", Healthy: true}}},
	}
	rt := &v1beta1.ResourceTracker{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "orders-team-a",
			Labels: map[string]string{oam.LabelAppName: "orders", oam.LabelAppNamespace: "team-a"},
		},
		Spec: v1beta1.ResourceTrackerSpec{Type: v1beta1.ResourceTrackerTypeVersioned, ApplicationGeneration: 3},
	}
	objs := append([]client.Object{}, platformFixtures()...)
	objs = append(objs, app, rt)
	for _, r := range resources {
		rt.Spec.ManagedResources = append(rt.Spec.ManagedResources, v1beta1.ManagedResource{
			ClusterObjectReference: oamcommon.ClusterObjectReference{ObjectReference: corev1.ObjectReference{
				APIVersion: r.GetAPIVersion(), Kind: r.GetKind(), Namespace: r.GetNamespace(), Name: r.GetName(),
			}},
			OAMObjectReference: oamcommon.OAMObjectReference{Component: "orders-db"},
		})
		objs = append(objs, r)
	}
	return objs
}

var _ = Describe("diagnose_application", func() {
	var h *harness

	AfterEach(func() {
		if h != nil {
			h.close()
			h = nil
		}
	})

	args := map[string]any{"name": "orders", "namespace": "team-a"}

	It("reports a synced ACK resource ready, with the staleness caveat after a spec change", func() {
		db := cloudResource("rds.services.k8s.aws/v1alpha1", "DBCluster", "orders-db", 3,
			[]map[string]any{{"type": "ACK.ResourceSynced", "status": "True"}},
			map[string]any{"ackResourceMetadata": map[string]any{"arn": "arn:aws:rds:us-east-1:000000000000:cluster:orders-db"}})
		h = startHarness(Options{}, diagnoseApp(db)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Healthy).To(BeTrue())
		Expect(out.Resources).To(HaveLen(1))
		Expect(out.Resources[0].Provider).To(Equal(providerACK))
		Expect(out.Resources[0].Ready).To(BeTrue())
		Expect(out.Resources[0].State).To(Equal(stateReady))
		Expect(out.Resources[0].Notes).To(ContainElement(ContainSubstring("can be stale")))
		Expect(out.Evidence).To(ContainElement(ContainSubstring("DBCluster team-a/orders-db")))
	})

	It("calls a Terminal ACK resource halted, not merely unsynced", func() {
		db := cloudResource("rds.services.k8s.aws/v1alpha1", "DBCluster", "orders-db", 1,
			[]map[string]any{
				{"type": "ACK.ResourceSynced", "status": "False"},
				{"type": "ACK.Terminal", "status": "True", "message": "InvalidSubnet: one or more subnets do not exist"},
			}, nil)
		h = startHarness(Options{}, diagnoseApp(db)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Healthy).To(BeFalse())
		Expect(out.Resources[0].State).To(Equal(stateHalted))
		Expect(out.Findings).NotTo(BeEmpty())
		Expect(out.Findings[0].Severity).To(Equal(SeverityError))
		Expect(out.Findings[0].Observed).To(ContainSubstring("InvalidSubnet"))
		Expect(out.NextChecks).To(ContainElement(ContainSubstring("Terminal message")))
	})

	It("names the convergence-loop signature when the provider has the resource but sync never turns true", func() {
		db := cloudResource("rds.services.k8s.aws/v1alpha1", "DBCluster", "orders-db", 2,
			[]map[string]any{{"type": "ACK.ResourceSynced", "status": "False",
				"lastTransitionTime": time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339)}},
			map[string]any{"ackResourceMetadata": map[string]any{"arn": "arn:aws:rds:us-east-1:000000000000:cluster:orders-db"}, "status": "available"})
		h = startHarness(Options{}, diagnoseApp(db)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Resources[0].State).To(Equal(stateNotReady))
		Expect(out.Resources[0].Summary).To(ContainSubstring("computes differently"))
		Expect(out.NextChecks).To(ContainElement(ContainSubstring("ACK rds controller's logs")))
	})

	It("applies the Config Connector readiness reason, not only the condition", func() {
		sql := cloudResource("sql.cnrm.cloud.google.com/v1beta1", "SQLInstance", "orders-db", 1,
			[]map[string]any{{"type": "Ready", "status": "False", "reason": "UpdateFailed", "message": "googleapi: Error 403"}}, nil)
		h = startHarness(Options{}, diagnoseApp(sql)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Resources[0].Provider).To(Equal(providerConfigConnector))
		Expect(out.Resources[0].State).To(Equal(stateNotReady))
		Expect(out.Resources[0].Summary).To(ContainSubstring("UpdateFailed"))
		Expect(out.NextChecks).To(ContainElement(ContainSubstring("cnrm-controller-manager")))

		live := &unstructured.Unstructured{}
		live.SetAPIVersion("sql.cnrm.cloud.google.com/v1beta1")
		live.SetKind("SQLInstance")
		Expect(h.cli.Get(h.ctx, client.ObjectKey{Namespace: "team-a", Name: "orders-db"}, live)).To(Succeed())
		Expect(unstructured.SetNestedSlice(live.Object,
			[]any{map[string]any{"type": "Ready", "status": "True", "reason": "UpToDate"}}, "status", "conditions")).To(Succeed())
		Expect(h.cli.Update(h.ctx, live)).To(Succeed())
		res = h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Resources[0].Ready).To(BeTrue())
	})

	It("blames the unresolved reference, not the controller, when references block reconciliation", func() {
		db := cloudResource("rds.services.k8s.aws/v1alpha1", "DBCluster", "orders-db", 1,
			[]map[string]any{{"type": "ACK.ReferencesResolved", "status": "False",
				"message": "the referenced resource has 'ACK.Terminal' condition 'True'. Cannot be referenced. resource:DBSubnetGroup"}}, nil)
		h = startHarness(Options{}, diagnoseApp(db)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Resources[0].State).To(Equal(stateNotReady))
		Expect(out.Resources[0].Summary).To(ContainSubstring("reference resolution failed"))

		pending := cloudResource("rds.services.k8s.aws/v1alpha1", "DBInstance", "orders-db", 1,
			[]map[string]any{{"type": "ACK.ReferencesResolved", "status": "Unknown",
				"message": "the referenced resource is not synced yet. resource:DBCluster"}}, nil)
		h2 := startHarness(Options{}, diagnoseApp(pending)...)
		defer h2.close()
		res = h2.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Resources[0].State).To(Equal(stateNotReady))
		Expect(out.Resources[0].Summary).To(ContainSubstring("has not completed"))
	})

	It("requires both Ready and Synced from a Crossplane resource", func() {
		table := cloudResource("dynamodb.aws.upbound.io/v1beta1", "Table", "orders-db", 1,
			[]map[string]any{
				{"type": "Ready", "status": "False"},
				{"type": "Synced", "status": "True"},
			}, nil)
		h = startHarness(Options{}, diagnoseApp(table)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Resources[0].Provider).To(Equal(providerCrossplane))
		Expect(out.Resources[0].State).To(Equal(stateNotReady))
		Expect(out.Resources[0].Summary).To(ContainSubstring("Ready is False"))
	})

	It("does not call a Crossplane resource ready on a lone positive condition", func() {
		table := cloudResource("dynamodb.aws.upbound.io/v1beta1", "Table", "orders-db", 1,
			[]map[string]any{{"type": "Ready", "status": "True"}}, nil)
		h = startHarness(Options{}, diagnoseApp(table)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Resources[0].Ready).To(BeFalse())
		Expect(out.Resources[0].State).To(Equal(stateUnknown))
		Expect(out.Resources[0].Summary).To(ContainSubstring("no Synced condition yet"))
	})

	It("settles plain Kubernetes resources from the tree without live reads", func() {
		h = startHarness(Options{}, managedFixtures()...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", map[string]any{"name": "web", "namespace": "team-a"}, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		// the fixture manages a Deployment and a Secret: the Secret is never
		// diagnosed, so exactly the Deployment remains
		Expect(out.Resources).To(HaveLen(1))
		Expect(out.Resources[0].Kind).To(Equal("Deployment"))
		Expect(out.Resources[0].Provider).To(Equal(providerKubernetes))
		Expect(out.Resources[0].Ready).To(BeTrue())
		// a live read leaves a "<Kind> ns/name ... resourceVersion" evidence
		// line; the tree path must not
		Expect(toJSON(out.Evidence)).NotTo(ContainSubstring("Deployment team-a/web"))
	})

	It("never reports healthy when the inventory could not be read", func() {
		funcs := &interceptor.Funcs{List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			if _, ok := list.(*v1beta1.ResourceTrackerList); ok {
				return fmt.Errorf("resourcetrackers is unavailable")
			}
			return c.List(ctx, list, opts...)
		}}
		h = startHarnessIntercepted(Options{}, nil, funcs, managedFixtures()...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", map[string]any{"name": "web", "namespace": "team-a"}, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Healthy).To(BeFalse(), "an unread inventory can never prove health")
		Expect(toJSON(out.Unknowns)).To(ContainSubstring("managed resources"))
	})

	It("bounds live reads and says what it did not read", func() {
		old := maxDiagnoseReads
		maxDiagnoseReads = 1
		DeferCleanup(func() { maxDiagnoseReads = old })
		one := cloudResource("rds.services.k8s.aws/v1alpha1", "DBCluster", "orders-db", 1,
			[]map[string]any{{"type": "ACK.ResourceSynced", "status": "True"}}, nil)
		two := cloudResource("rds.services.k8s.aws/v1alpha1", "DBInstance", "orders-db-1", 1,
			[]map[string]any{{"type": "ACK.ResourceSynced", "status": "True"}}, nil)
		h = startHarness(Options{}, diagnoseApp(one, two)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Meta.Truncated).To(BeTrue())
		Expect(out.Unknowns).To(ContainElement(ContainSubstring("bound")))
	})
})
