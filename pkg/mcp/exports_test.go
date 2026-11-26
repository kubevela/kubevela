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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// fieldExport builds an ACK FieldExport copying .status.endpoint of the
// DBCluster orders-db into key host of the Secret orders-db.
func fieldExport(name, synced string) *unstructured.Unstructured {
	fe := cloudResource("services.k8s.aws/v1alpha1", "FieldExport", name, 1,
		[]map[string]any{{"type": "ACK.ResourceSynced", "status": synced}}, nil)
	_ = unstructured.SetNestedMap(fe.Object, map[string]any{
		"from": map[string]any{
			"path":     ".status.endpoint",
			"resource": map[string]any{"group": "rds.services.k8s.aws", "kind": "DBCluster", "name": "orders-db"},
		},
		"to": map[string]any{"kind": "secret", "name": "orders-db", "key": "host"},
	}, "spec")
	return fe
}

func resourceNamed(out DiagnoseApplicationOutput, kind, name string) *ResourceDiagnosis {
	for i := range out.Resources {
		if out.Resources[i].Kind == kind && out.Resources[i].Name == name {
			return &out.Resources[i]
		}
	}
	return nil
}

var _ = Describe("diagnose_application exporters", func() {
	var h *harness

	AfterEach(func() {
		if h != nil {
			h.close()
			h = nil
		}
	})

	args := map[string]any{"name": "orders", "namespace": "team-a"}

	It("describes a synced FieldExport as a written export served by its source's controller", func() {
		db := cloudResource("rds.services.k8s.aws/v1alpha1", "DBCluster", "orders-db", 1,
			[]map[string]any{{"type": "ACK.ResourceSynced", "status": "True"}}, nil)
		fe := fieldExport("orders-db-conn-host", "True")
		ctrl := ackControllerDeployment("rds", "http://aws-emulator.floci.svc.cluster.local:4566")
		h = startHarness(Options{}, append(diagnoseApp(db, fe), ctrl)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))

		Expect(out.Healthy).To(BeTrue())
		exp := resourceNamed(out, "FieldExport", "orders-db-conn-host")
		Expect(exp).NotTo(BeNil())
		Expect(exp.Ready).To(BeTrue())
		Expect(exp.Export).NotTo(BeNil())
		Expect(*exp.Export).To(Equal(ExportDetail{
			SourceKind: "DBCluster", SourceName: "orders-db", SourcePath: ".status.endpoint",
			TargetKind: "Secret", TargetName: "orders-db", TargetKey: "host", Written: true,
		}))
		Expect(exp.Summary).To(ContainSubstring("exports .status.endpoint of DBCluster orders-db into Secret orders-db key host: written"))
		// the FieldExport lives in the common group; its controller is the
		// source service's, so the rds endpoint override applies to it
		Expect(exp.EndpointOverride).To(Equal("http://aws-emulator.floci.svc.cluster.local:4566"))
		Expect(exp.Notes).NotTo(ContainElement(ContainSubstring("endpoint provenance unknown")))
		// the DBCluster is not an exporter
		Expect(resourceNamed(out, "DBCluster", "orders-db").Export).To(BeNil())
	})

	It("ties an unwritten FieldExport to the state of its source", func() {
		db := cloudResource("rds.services.k8s.aws/v1alpha1", "DBCluster", "orders-db", 1,
			[]map[string]any{{"type": "ACK.ResourceSynced", "status": "False", "reason": "creating"}}, nil)
		fe := fieldExport("orders-db-conn-host", "False")
		h = startHarness(Options{}, diagnoseApp(fe, db)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))

		Expect(out.Healthy).To(BeFalse())
		exp := resourceNamed(out, "FieldExport", "orders-db-conn-host")
		Expect(exp).NotTo(BeNil())
		Expect(exp.Export.Written).To(BeFalse())
		Expect(exp.Summary).To(ContainSubstring("not written yet"))
		Expect(exp.Notes).To(ContainElement("source DBCluster orders-db is NotReady; the export cannot complete until the source field exists"))
		Expect(out.Findings).To(ContainElement(SatisfyAll(
			HaveField("Severity", SeverityWarning),
			HaveField("Message", ContainSubstring(`FieldExport orders-db-conn-host has not written Secret orders-db key host because its source DBCluster orders-db is NotReady`)),
			HaveField("Evidence", "FieldExport team-a/orders-db-conn-host in cluster local"),
		)))
	})

	It("says when an unwritten FieldExport's source is ready, so the exporter itself is the problem", func() {
		db := cloudResource("rds.services.k8s.aws/v1alpha1", "DBCluster", "orders-db", 1,
			[]map[string]any{{"type": "ACK.ResourceSynced", "status": "True"}}, nil)
		fe := fieldExport("orders-db-conn-host", "False")
		h = startHarness(Options{}, diagnoseApp(db, fe)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))

		exp := resourceNamed(out, "FieldExport", "orders-db-conn-host")
		Expect(exp.Notes).To(ContainElement(ContainSubstring("source DBCluster orders-db is Ready; the exporter's own conditions name the problem")))
		Expect(out.Findings).NotTo(ContainElement(HaveField("Message", ContainSubstring("because its source"))))
	})

	It("reports a source that is not among the Application's resources", func() {
		fe := fieldExport("orders-db-conn-host", "False")
		h = startHarness(Options{}, diagnoseApp(fe)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))

		Expect(out.Findings).To(ContainElement(HaveField("Message", ContainSubstring("its source DBCluster orders-db is not among this Application's resources"))))
	})

	It("reports a Crossplane connection Secret as an export written when the resource is Ready", func() {
		cp := cloudResource("rds.aws.upbound.io/v1beta1", "Cluster", "orders-db", 1,
			[]map[string]any{{"type": "Ready", "status": "True"}, {"type": "Synced", "status": "True"}}, nil)
		_ = unstructured.SetNestedMap(cp.Object, map[string]any{
			"writeConnectionSecretToRef": map[string]any{"name": "orders-db-conn", "namespace": "team-a"},
		}, "spec")
		h = startHarness(Options{}, diagnoseApp(cp)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))

		r := resourceNamed(out, "Cluster", "orders-db")
		Expect(r.Export).NotTo(BeNil())
		Expect(*r.Export).To(Equal(ExportDetail{SourceKind: "Cluster", SourceName: "orders-db", TargetKind: "Secret", TargetName: "orders-db-conn", Written: true}))
		Expect(r.Summary).To(ContainSubstring("exports into Secret orders-db-conn: written"))
	})

	It("reads the connection Secret of a Terraform Configuration, which writes no conditions", func() {
		tf := cloudResource("terraform.core.oam.dev/v1beta2", "Configuration", "orders-db", 1, nil,
			map[string]any{"apply": map[string]any{"state": "Available"}})
		_ = unstructured.SetNestedMap(tf.Object, map[string]any{
			"writeConnectionSecretToRef": map[string]any{"name": "orders-db-conn", "namespace": "team-a"},
		}, "spec")
		h = startHarness(Options{}, diagnoseApp(tf)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))

		r := resourceNamed(out, "Configuration", "orders-db")
		Expect(r.Provider).To(Equal(providerKubernetes), "no conditions: settled from the tree")
		Expect(r.Export).NotTo(BeNil())
		Expect(r.Export.TargetName).To(Equal("orders-db-conn"))
		Expect(r.Export.Written).To(Equal(r.Ready))
		Expect(r.Summary).To(ContainSubstring("exports into Secret orders-db-conn"))
	})

	It("notes that a not-ready Crossplane resource writes its connection Secret on readiness", func() {
		cp := cloudResource("rds.aws.upbound.io/v1beta1", "Cluster", "orders-db", 1,
			[]map[string]any{{"type": "Ready", "status": "False", "reason": "Creating"}, {"type": "Synced", "status": "True"}}, nil)
		_ = unstructured.SetNestedMap(cp.Object, map[string]any{
			"publishConnectionDetailsTo": map[string]any{"name": "orders-db-conn"},
		}, "spec")
		h = startHarness(Options{}, diagnoseApp(cp)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))

		r := resourceNamed(out, "Cluster", "orders-db")
		Expect(r.Export.Written).To(BeFalse())
		Expect(r.Notes).To(ContainElement("connection Secret orders-db-conn is written when this resource is Ready"))
	})
})
