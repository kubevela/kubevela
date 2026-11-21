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

	It("stamps the diagnosis with the controller's endpoint override", func() {
		db := cloudResource("rds.services.k8s.aws/v1alpha1", "DBCluster", "orders-db", 1,
			[]map[string]any{{"type": "ACK.ResourceSynced", "status": "True"}}, nil)
		ctrl := ackControllerDeployment("rds", "http://aws-emulator.floci.svc.cluster.local:4566")
		h = startHarness(Options{}, append(diagnoseApp(db), ctrl)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Resources).To(HaveLen(1))
		Expect(out.Resources[0].EndpointOverride).To(Equal("http://aws-emulator.floci.svc.cluster.local:4566"))
		Expect(out.Evidence).To(ContainElement(ContainSubstring("AWS_ENDPOINT_URL=http://aws-emulator.floci.svc.cluster.local:4566")))
	})

	It("redacts credentials in the controller's endpoint override", func() {
		db := cloudResource("rds.services.k8s.aws/v1alpha1", "DBCluster", "orders-db", 1,
			[]map[string]any{{"type": "ACK.ResourceSynced", "status": "True"}}, nil)
		ctrl := ackControllerDeployment("rds", "http://admin:s3cr3t@localstack:4566")
		h = startHarness(Options{}, append(diagnoseApp(db), ctrl)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Resources).To(HaveLen(1))
		Expect(out.Resources[0].EndpointOverride).To(Equal("http://admin:" + redactedValue + "@localstack:4566"))
		Expect(toJSON(out)).NotTo(ContainSubstring("s3cr3t"))
	})

	It("notes endpoint provenance as unknown when the controller Deployment is absent", func() {
		db := cloudResource("rds.services.k8s.aws/v1alpha1", "DBCluster", "orders-db", 1,
			[]map[string]any{{"type": "ACK.ResourceSynced", "status": "True"}}, nil)
		h = startHarness(Options{}, diagnoseApp(db)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Resources[0].EndpointOverride).To(BeEmpty())
		Expect(out.Resources[0].Notes).To(ContainElement(ContainSubstring("endpoint provenance unknown")))
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

	It("classifies an ACK throttle as a cloud-owned cause and points at the fleet sweep", func() {
		db := cloudResource("rds.services.k8s.aws/v1alpha1", "DBCluster", "orders-db", 1,
			[]map[string]any{
				{"type": "ACK.ResourceSynced", "status": "Unknown", "reason": "operation error RDS: ModifyDBCluster, https response error StatusCode: 400, RequestID: 1, Throttling: Rate exceeded"},
				{"type": "ACK.Recoverable", "status": "True", "message": "Throttling: Rate exceeded", "lastTransitionTime": "2026-11-15T09:12:00Z"},
			}, nil)
		h = startHarness(Options{}, append(diagnoseApp(db), ackControllerDeployment("rds", ""))...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		c := out.Resources[0].Cause
		Expect(c).NotTo(BeNil())
		Expect(c.Class).To(Equal(causeThrottled))
		Expect(c.Owner).To(Equal(ownerExternal))
		Expect(c.Scope).To(Equal(scopeThisResource))
		Expect(c.Evidence).To(ContainSubstring("ACK.Recoverable message: Throttling: Rate exceeded"))
		Expect(c.Controller).NotTo(BeNil())
		Expect(c.Controller.Namespace).To(Equal("ack-system"))
		Expect(c.Controller.Name).To(Equal("ack-rds-rds-chart"))
		Expect(c.Controller.LabelSelector).To(Equal("app.kubernetes.io/name=rds-chart"))
		Expect(c.Controller.APIGroup).To(Equal("rds.services.k8s.aws"))
		Expect(c.Controller.Cluster).To(Equal(localCluster))
		Expect(c.Controller.Window.Basis).To(Equal("condition transition"))
		Expect(c.Controller.Window.From).To(Equal(time.Date(2026, 11, 15, 9, 12, 0, 0, time.UTC)))
		Expect(out.Findings).To(ContainElement(SatisfyAll(
			HaveField("Severity", SeverityError),
			HaveField("Message", ContainSubstring("cause throttled (Throttling) — owner: external")),
			HaveField("Observed", ContainSubstring("Rate exceeded")),
		)))
		Expect(out.NextChecks).To(ContainElement(ContainSubstring("diagnose_controllers when this server offers it: provider aws, family ack, window 1h")))
	})

	It("names no controller handle when the ACK controller Deployment is absent, in the provenance note", func() {
		db := cloudResource("rds.services.k8s.aws/v1alpha1", "DBCluster", "orders-db", 1,
			[]map[string]any{{"type": "ACK.Recoverable", "status": "True", "message": "Throttling: Rate exceeded"}}, nil)
		h = startHarness(Options{}, diagnoseApp(db)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		r := out.Resources[0]
		Expect(r.Cause).NotTo(BeNil())
		Expect(r.Cause.Controller).To(BeNil())
		Expect(r.Notes).To(ContainElement(ContainSubstring("no controller handle")))
		provenance := 0
		for _, n := range r.Notes {
			if strings.Contains(n, "provenance") {
				provenance++
			}
		}
		Expect(provenance).To(Equal(1))
	})

	It("falls back to the hour before the read when the condition records no transition", func() {
		db := cloudResource("rds.services.k8s.aws/v1alpha1", "DBCluster", "orders-db", 1,
			[]map[string]any{{"type": "ACK.Recoverable", "status": "True", "message": "Throttling: Rate exceeded"}}, nil)
		h = startHarness(Options{}, append(diagnoseApp(db), ackControllerDeployment("rds", ""))...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		c := out.Resources[0].Cause
		Expect(c).NotTo(BeNil())
		Expect(c.Since).To(BeNil())
		Expect(c.Controller).NotTo(BeNil())
		Expect(c.Controller.Window.Basis).To(Equal("fallback: one hour before this read"))
	})

	It("points at the fleet sweep once however many resources are throttled", func() {
		throttled := []map[string]any{{"type": "ACK.Recoverable", "status": "True", "message": "Throttling: Rate exceeded"}}
		db1 := cloudResource("rds.services.k8s.aws/v1alpha1", "DBCluster", "orders-db", 1, throttled, nil)
		db2 := cloudResource("rds.services.k8s.aws/v1alpha1", "DBCluster", "orders-db-2", 1, throttled, nil)
		h = startHarness(Options{}, append(diagnoseApp(db1, db2), ackControllerDeployment("rds", ""))...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Resources).To(HaveLen(2))
		for _, r := range out.Resources {
			Expect(r.Cause).NotTo(BeNil())
			Expect(r.Cause.Class).To(Equal(causeThrottled))
		}
		sweeps := 0
		for _, n := range out.NextChecks {
			if strings.Contains(n, "diagnose_controllers") {
				sweeps++
			}
		}
		Expect(sweeps).To(Equal(1))
	})

	It("classifies a Terminal invalid parameter as application-spec and does not point at the sweep", func() {
		db := cloudResource("rds.services.k8s.aws/v1alpha1", "DBInstance", "orders-db-1", 1,
			[]map[string]any{
				{"type": "ACK.ResourceSynced", "status": "False", "reason": "resource is in terminal condition", "message": "Resource not synced"},
				{"type": "ACK.Terminal", "status": "True", "message": "api error InvalidParameterCombination: Cannot find version 16.3 for aurora-postgresql"},
			}, nil)
		h = startHarness(Options{}, append(diagnoseApp(db), ackControllerDeployment("rds", ""))...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		c := out.Resources[0].Cause
		Expect(c).NotTo(BeNil())
		Expect(c.Class).To(Equal(causeInvalid))
		Expect(c.Owner).To(Equal(ownerApplicationSpec))
		Expect(c.Controller).NotTo(BeNil())
		Expect(c.Controller.Name).To(Equal("ack-rds-rds-chart"))
		Expect(out.NextChecks).NotTo(ContainElement(ContainSubstring("diagnose_controllers")))

		h2 := startHarness(Options{}, diagnoseApp(db)...)
		defer h2.close()
		var bare DiagnoseApplicationOutput
		res = h2.call("diagnose_application", args, &bare)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(bare.Resources[0].Cause.Controller).To(BeNil())
		Expect(bare.Resources[0].Notes).To(ContainElement(ContainSubstring("no controller handle")))
	})

	It("leaves unrecognised text unclassified with no owner and no cause finding", func() {
		db := cloudResource("rds.services.k8s.aws/v1alpha1", "DBCluster", "orders-db", 1,
			[]map[string]any{{"type": "ACK.Recoverable", "status": "True", "message": "something the controller made up"}}, nil)
		h = startHarness(Options{}, diagnoseApp(db)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Resources[0].Cause.Class).To(Equal(causeUnclassified))
		Expect(out.Resources[0].Cause.Owner).To(BeEmpty())
		Expect(out.Findings).NotTo(ContainElement(HaveField("Message", ContainSubstring("— owner:"))))
	})

	It("classifies a Crossplane resource from a warning event when its Synced message is bare", func() {
		b := cloudResource("s3.aws.upbound.io/v1beta1", "Bucket", "assets", 1,
			[]map[string]any{
				{"type": "Synced", "status": "False", "reason": "ReconcileError"},
				{"type": "Ready", "status": "False"},
			}, nil)
		ev := &corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "assets.1", Namespace: "team-a"},
			InvolvedObject: corev1.ObjectReference{APIVersion: "s3.aws.upbound.io/v1beta1", Kind: "Bucket", Name: "assets", Namespace: "team-a"},
			Type:           corev1.EventTypeWarning, Reason: "CannotObserveExternalResource",
			Message: "cannot run refresh: api error SlowDown: Please reduce your request rate.",
			Count:   3, LastTimestamp: metav1.Now(),
		}
		h = startHarness(Options{}, append(diagnoseApp(b), ev)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Resources[0].Cause).NotTo(BeNil())
		Expect(out.Resources[0].Cause.Class).To(Equal(causeThrottled))
		Expect(out.Resources[0].Cause.Evidence).To(ContainSubstring("Event CannotObserveExternalResource"))
		Expect(out.NextChecks).To(ContainElement(ContainSubstring("diagnose_controllers when this server offers it: provider aws, family crossplane, window 1h")))
		Expect(out.Resources[0].Notes).To(ContainElement(ContainSubstring("no controller handle")))
		Expect(out.Findings).To(ContainElement(HaveField("Message", ContainSubstring("cause throttled (SlowDown) — owner: external"))))
	})

	It("reads past a newer warning event no rule recognises to the older one that classifies", func() {
		b := cloudResource("s3.aws.upbound.io/v1beta1", "Bucket", "assets", 1,
			[]map[string]any{
				{"type": "Synced", "status": "False", "reason": "ReconcileError"},
				{"type": "Ready", "status": "False"},
			}, nil)
		involved := corev1.ObjectReference{APIVersion: "s3.aws.upbound.io/v1beta1", Kind: "Bucket", Name: "assets", Namespace: "team-a"}
		older := &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: "assets.1", Namespace: "team-a"}, InvolvedObject: involved,
			Type: corev1.EventTypeWarning, Reason: "CannotObserveExternalResource",
			Message: "cannot run refresh: api error SlowDown: Please reduce your request rate.",
			Count:   3, LastTimestamp: metav1.NewTime(time.Date(2026, 11, 15, 9, 40, 0, 0, time.UTC)),
		}
		newer := &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: "assets.2", Namespace: "team-a"}, InvolvedObject: involved,
			Type: corev1.EventTypeWarning, Reason: "CannotObserveExternalResource",
			Message: "cannot run refresh: the provider returned no body",
			Count:   1, LastTimestamp: metav1.NewTime(time.Date(2026, 11, 15, 9, 41, 0, 0, time.UTC)),
		}
		h = startHarness(Options{}, append(diagnoseApp(b), newer, older)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		c := out.Resources[0].Cause
		Expect(c).NotTo(BeNil())
		Expect(c.Class).To(Equal(causeThrottled), "the older event classifies; the newer one does not hide it")
		Expect(c.Evidence).To(ContainSubstring("SlowDown"))
		Expect(c.Since).NotTo(BeNil())
		Expect(*c.Since).To(Equal(time.Date(2026, 11, 15, 9, 40, 0, 0, time.UTC)), "the classifying event's own time dates the cause")
	})

	It("does not classify a failure from an event that predates it", func() {
		// Synced left True at 10:00. The SlowDown event was last seen at 09:40,
		// during an earlier failure that was resolved in between.
		b := cloudResource("s3.aws.upbound.io/v1beta1", "Bucket", "assets", 1,
			[]map[string]any{
				{"type": "Synced", "status": "False", "reason": "ReconcileError", "lastTransitionTime": "2026-11-15T10:00:00Z"},
				{"type": "Ready", "status": "False"},
			}, nil)
		involved := corev1.ObjectReference{APIVersion: "s3.aws.upbound.io/v1beta1", Kind: "Bucket", Name: "assets", Namespace: "team-a"}
		earlier := &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: "assets.1", Namespace: "team-a"}, InvolvedObject: involved,
			Type: corev1.EventTypeWarning, Reason: "CannotObserveExternalResource",
			Message: "cannot run refresh: api error SlowDown: Please reduce your request rate.",
			Count:   3, LastTimestamp: metav1.NewTime(time.Date(2026, 11, 15, 9, 40, 0, 0, time.UTC)),
		}
		current := &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: "assets.2", Namespace: "team-a"}, InvolvedObject: involved,
			Type: corev1.EventTypeWarning, Reason: "CannotObserveExternalResource",
			Message: "cannot run refresh: the provider returned no body",
			Count:   1, LastTimestamp: metav1.NewTime(time.Date(2026, 11, 15, 10, 1, 0, 0, time.UTC)),
		}
		h = startHarness(Options{}, append(diagnoseApp(b), current, earlier)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		if c := out.Resources[0].Cause; c != nil {
			Expect(c.Class).NotTo(Equal(causeThrottled), "the earlier failure's event must not classify this one")
			Expect(c.Evidence).NotTo(ContainSubstring("SlowDown"))
		}
		Expect(out.Findings).NotTo(ContainElement(HaveField("Message", ContainSubstring("SlowDown"))))
	})

	It("replaces an unclassified Crossplane cause with the one its warning event classifies", func() {
		ts := "2026-11-15T09:12:00Z"
		b := cloudResource("s3.aws.upbound.io/v1beta1", "Bucket", "assets", 1,
			[]map[string]any{
				{"type": "Synced", "status": "False", "reason": "ReconcileError", "message": "connect failed: Invalid provider config", "lastTransitionTime": ts},
				{"type": "Ready", "status": "False"},
			}, nil)
		ev := &corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "assets.1", Namespace: "team-a"},
			InvolvedObject: corev1.ObjectReference{APIVersion: "s3.aws.upbound.io/v1beta1", Kind: "Bucket", Name: "assets", Namespace: "team-a"},
			Type:           corev1.EventTypeWarning, Reason: "CannotConnectToProvider",
			Message: "cannot get AWS config: ExpiredTokenException: The security token included in the request is expired",
			Count:   2, LastTimestamp: metav1.NewTime(time.Date(2026, 11, 15, 9, 40, 0, 0, time.UTC)),
		}
		h = startHarness(Options{}, append(diagnoseApp(b), ev)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		c := out.Resources[0].Cause
		Expect(c).NotTo(BeNil())
		Expect(c.Class).To(Equal(causeAuth))
		Expect(c.Code).To(Equal("ExpiredTokenException"))
		Expect(c.Evidence).To(ContainSubstring("Event CannotConnectToProvider on Bucket/assets"))
		// the event is the evidence, so its last timestamp dates the cause,
		// not the Synced transition at 09:12
		Expect(c.Since).NotTo(BeNil())
		Expect(*c.Since).To(Equal(time.Date(2026, 11, 15, 9, 40, 0, 0, time.UTC)))
		Expect(out.Findings).To(ContainElement(HaveField("Message", ContainSubstring("cause auth (ExpiredTokenException) — owner: platform"))))
	})

	It("does not read a stale warning event once Synced is True", func() {
		b := cloudResource("s3.aws.upbound.io/v1beta1", "Bucket", "assets", 1,
			[]map[string]any{
				{"type": "Synced", "status": "True"},
				{"type": "Ready", "status": "False"},
			}, nil)
		ev := &corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "assets.1", Namespace: "team-a"},
			InvolvedObject: corev1.ObjectReference{APIVersion: "s3.aws.upbound.io/v1beta1", Kind: "Bucket", Name: "assets", Namespace: "team-a"},
			Type:           corev1.EventTypeWarning, Reason: "CannotObserveExternalResource",
			Message: "cannot run refresh: api error SlowDown: Please reduce your request rate.",
			Count:   3, LastTimestamp: metav1.Now(),
		}
		h = startHarness(Options{}, append(diagnoseApp(b), ev)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Resources[0].Cause).To(BeNil())
		Expect(out.Findings).NotTo(ContainElement(HaveField("Message", ContainSubstring("— owner:"))))
	})

	It("matches an event to a resource only in the same namespace and cluster", func() {
		r := ResourceDiagnosis{Cluster: localCluster, Namespace: "team-a", Kind: "Bucket", Name: "assets"}
		Expect(eventMatchesResource(EventSummary{Cluster: localCluster, Namespace: "team-a", Object: "Bucket/assets"}, r)).To(BeTrue())
		Expect(eventMatchesResource(EventSummary{Cluster: "prod-east", Namespace: "team-a", Object: "Bucket/assets"}, r)).To(BeFalse())
		Expect(eventMatchesResource(EventSummary{Cluster: localCluster, Namespace: "team-b", Object: "Bucket/assets"}, r)).To(BeFalse())
		Expect(eventMatchesResource(EventSummary{Cluster: localCluster, Namespace: "team-a", Object: "Bucket/other"}, r)).To(BeFalse())
		clusterScoped := ResourceDiagnosis{Cluster: localCluster, Kind: "Bucket", Name: "assets"}
		Expect(eventMatchesResource(EventSummary{Cluster: localCluster, Object: "Bucket/assets"}, clusterScoped)).To(BeTrue())
		Expect(eventMatchesResource(EventSummary{Cluster: localCluster, Namespace: "team-a", Object: "Bucket/assets"}, clusterScoped)).To(BeFalse())
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

		stale := cloudResource("rds.services.k8s.aws/v1alpha1", "DBCluster", "orders-db", 2,
			[]map[string]any{
				{"type": "ACK.ResourceSynced", "status": "True"},
				{"type": "ACK.ReferencesResolved", "status": "False", "message": "the referenced resource has 'ACK.Terminal' condition 'True'"},
			}, nil)
		h3 := startHarness(Options{}, diagnoseApp(stale)...)
		defer h3.close()
		res = h3.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Resources[0].Ready).To(BeFalse(), "a stale Synced=True must not outrank a broken reference")
		Expect(out.Resources[0].State).To(Equal(stateNotReady))
		Expect(out.Resources[0].Notes).To(ContainElement(ContainSubstring("stale")))
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

	It("reads a cert-manager Certificate by the standard-conditions convention and classifies the ACME limit", func() {
		// condition reason and message as cert-manager's documented issuance failure writes them
		cert := cloudResource("cert-manager.io/v1", "Certificate", "tls-cert", 1,
			[]map[string]any{
				{"type": "Issuing", "status": "False", "reason": "Failed", "message": "The certificate request has failed to complete and will be retried: Failed to wait for order resource \"tls-cert-1\" to become ready: order is in \"errored\" state: Failed to create Order: 429 urn:ietf:params:acme:error:rateLimited: too many certificates (5) already issued for this exact set of domains in the last 168 hours", "lastTransitionTime": "2026-11-16T08:00:00Z"},
				{"type": "Ready", "status": "False", "reason": "Failed", "message": "The certificate request has failed to complete and will be retried: Failed to wait for order resource \"tls-cert-1\" to become ready: order is in \"errored\" state: Failed to create Order: 429 urn:ietf:params:acme:error:rateLimited: too many certificates (5) already issued for this exact set of domains in the last 168 hours", "lastTransitionTime": "2026-11-16T08:00:00Z"},
			}, nil)
		h = startHarness(Options{}, diagnoseApp(cert)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Healthy).To(BeFalse())
		r := out.Resources[0]
		Expect(r.Provider).To(Equal(providerStandardConditions))
		Expect(r.State).To(Equal(stateNotReady))
		Expect(r.Summary).To(Equal("Ready is False; reason Failed"))
		Expect(r.Cause).NotTo(BeNil())
		Expect(r.Cause.Class).To(Equal(causeThrottled))
		Expect(r.Cause.Code).To(Equal("rateLimited"))
		Expect(r.Cause.Owner).To(Equal(ownerExternal))
		Expect(r.Cause.Evidence).To(HavePrefix("Ready message: "))
		Expect(r.Cause.Controller).To(BeNil())
		Expect(r.Notes).To(ContainElement("no controller handle: the standard-conditions family names no controller; a profile for cert-manager.io can name it"))
		Expect(out.NextChecks).To(ContainElement(ContainSubstring("diagnose_controllers when this server offers it: family standard-conditions, group cert-manager.io, window 1h")))
		Expect(out.Findings).To(ContainElement(HaveField("Message", ContainSubstring("cause throttled (rateLimited) — owner: external"))))
		Expect(out.Evidence).To(ContainElement(ContainSubstring("Certificate team-a/tls-cert")))
	})

	It("calls a standard-conditions resource ready only on Ready True", func() {
		cert := cloudResource("cert-manager.io/v1", "Certificate", "tls-cert", 1,
			[]map[string]any{{"type": "Ready", "status": "True", "reason": "Ready", "message": "Certificate is up to date and has not expired"}}, nil)
		h = startHarness(Options{}, diagnoseApp(cert)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Healthy).To(BeTrue())
		Expect(out.Resources[0].Provider).To(Equal(providerStandardConditions))
		Expect(out.Resources[0].Ready).To(BeTrue())
		Expect(out.Resources[0].Cause).To(BeNil())
	})

	It("reports a resource with no Ready condition unknown, never healthy, and lists it", func() {
		// a Gateway writes Accepted and Programmed, as the Gateway API documents
		gw := cloudResource("gateway.networking.k8s.io/v1", "Gateway", "edge", 1,
			[]map[string]any{
				{"type": "Accepted", "status": "True", "reason": "Accepted", "message": "Gateway is accepted"},
				{"type": "Programmed", "status": "False", "reason": "AddressNotAssigned", "message": "no address has been assigned to the Gateway"},
			}, nil)
		h = startHarness(Options{}, diagnoseApp(gw)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(out.Healthy).To(BeFalse())
		r := out.Resources[0]
		Expect(r.Provider).To(Equal(providerStandardConditions))
		Expect(r.Ready).To(BeFalse())
		Expect(r.State).To(Equal(stateUnknown))
		Expect(r.Summary).To(ContainSubstring("its controller writes Accepted, Programmed"))
		Expect(out.Unknowns).To(ContainElement(ContainSubstring("Gateway team-a/edge in cluster local: readiness unknown, it writes no Ready condition (Accepted, Programmed)")))
		// the newest False condition with a message is read, and its text matches no rule
		Expect(r.Cause).NotTo(BeNil())
		Expect(r.Cause.Class).To(Equal(causeUnclassified))
		Expect(r.Cause.Evidence).To(HavePrefix("Programmed reason: "))
		Expect(out.Findings).NotTo(ContainElement(HaveField("Message", ContainSubstring("— owner:"))))
	})

	It("keeps the tree's health for a custom resource that writes no conditions, and says so", func() {
		widget := cloudResource("widgets.example.com/v1", "Widget", "w", 1, nil, map[string]any{"phase": "Active"})
		h = startHarness(Options{}, diagnoseApp(widget)...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		r := out.Resources[0]
		Expect(r.Provider).To(Equal(providerKubernetes))
		Expect(r.Ready).To(BeTrue())
		Expect(r.Summary).To(HavePrefix("resource tree health: Healthy"))
		Expect(r.Notes).To(ContainElement(ContainSubstring("writes no conditions: kept the resource tree's health")))
		Expect(r.Cause).To(BeNil())
		Expect(out.Evidence).To(ContainElement(ContainSubstring("Widget team-a/w")))
	})

	It("maps API groups to families, built-in groups to kubernetes and other custom resources to standard-conditions", func() {
		Expect(providerOf("rds.services.k8s.aws/v1alpha1")).To(Equal(providerACK))
		Expect(providerOf("sql.cnrm.cloud.google.com/v1beta1")).To(Equal(providerConfigConnector))
		Expect(providerOf("s3.aws.upbound.io/v1beta1")).To(Equal(providerCrossplane))
		Expect(providerOf("apps/v1")).To(Equal(providerKubernetes))
		Expect(providerOf("v1")).To(Equal(providerKubernetes))
		Expect(providerOf("networking.k8s.io/v1")).To(Equal(providerKubernetes))
		Expect(providerOf("internal.apiserver.k8s.io/v1alpha1")).To(Equal(providerKubernetes))
		Expect(providerOf("lifecycle.k8s.io/v1alpha1")).To(Equal(providerKubernetes))
		Expect(providerOf("gateway.networking.k8s.io/v1")).To(Equal(providerStandardConditions))
		Expect(providerOf("cert-manager.io/v1")).To(Equal(providerStandardConditions))
		Expect(providerOf("cilium.io/v2")).To(Equal(providerStandardConditions))
		Expect(providerOf("storage.azure.com/v1api20230101")).To(Equal(providerStandardConditions))
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

var _ = DescribeTable("providerOf maps an API group to the family that interprets it",
	func(apiVersion, want string) {
		Expect(providerOf(apiVersion)).To(Equal(want))
	},
	Entry("ACK", "rds.services.k8s.aws/v1alpha1", providerACK),
	Entry("Config Connector", "sql.cnrm.cloud.google.com/v1beta1", providerConfigConnector),
	Entry("Crossplane v2 namespaced", "dynamodb.aws.m.upbound.io/v1beta1", providerCrossplane),
	Entry("Crossplane community", "database.aws.crossplane.io/v1beta1", providerCrossplane),
	Entry("a group that only contains a Crossplane suffix", "custom.crossplane.io.example.com/v1", providerStandardConditions),
	Entry("a group that only contains an Upbound suffix", "custom.upbound.io.example.com/v1", providerStandardConditions),
	Entry("core", "apps/v1", providerKubernetes),
)
