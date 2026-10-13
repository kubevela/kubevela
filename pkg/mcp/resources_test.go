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
	"io"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// managedFixtures is the web Application with a versioned ResourceTracker
// that owns a Deployment and a Secret, the Deployment's ReplicaSet and two Pods, plus
// an unmanaged Deployment in the same namespace.
func managedFixtures() []client.Object {
	app := webApp()
	labels := map[string]string{oam.LabelAppName: "web", oam.LabelAppComponent: "web"}
	rt := &v1beta1.ResourceTracker{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "web-team-a",
			Labels: map[string]string{oam.LabelAppName: "web", oam.LabelAppNamespace: "team-a"},
		},
		Spec: v1beta1.ResourceTrackerSpec{
			// Versioned and matching the Application generation, so it is the
			// current tracker and its resources report Latest.
			Type:                  v1beta1.ResourceTrackerTypeVersioned,
			ApplicationGeneration: 3,
			ManagedResources: []v1beta1.ManagedResource{
				{
					ClusterObjectReference: oamcommon.ClusterObjectReference{ObjectReference: corev1.ObjectReference{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "team-a", Name: "web"}},
					OAMObjectReference:     oamcommon.OAMObjectReference{Component: "web"},
				},
				{
					ClusterObjectReference: oamcommon.ClusterObjectReference{ObjectReference: corev1.ObjectReference{APIVersion: "v1", Kind: "Secret", Namespace: "team-a", Name: "web-creds"}},
					OAMObjectReference:     oamcommon.OAMObjectReference{Component: "web"},
				},
			},
		},
	}
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "team-a", UID: "dep-uid", Labels: labels, ResourceVersion: "77"},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](2),
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "web", Image: "nginx"}}}},
		},
		Status: appsv1.DeploymentStatus{Replicas: 2, UpdatedReplicas: 2, ReadyReplicas: 2, AvailableReplicas: 2, ObservedGeneration: 1},
	}
	rsOwner := metav1.OwnerReference{APIVersion: "apps/v1", Kind: "Deployment", Name: "web", UID: "dep-uid", Controller: ptr.To(true)}
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{Name: "web-abc", Namespace: "team-a", UID: "rs-uid", Labels: labels, OwnerReferences: []metav1.OwnerReference{rsOwner}},
		Spec:       appsv1.ReplicaSetSpec{Replicas: ptr.To[int32](2), Selector: &metav1.LabelSelector{MatchLabels: labels}},
		Status:     appsv1.ReplicaSetStatus{Replicas: 2, ReadyReplicas: 2, AvailableReplicas: 2},
	}
	podOwner := metav1.OwnerReference{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "web-abc", UID: "rs-uid", Controller: ptr.To(true)}
	pod := func(name string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "team-a", UID: apitypes.UID("pod-" + name), Labels: labels, OwnerReferences: []metav1.OwnerReference{podOwner}},
			Spec:       corev1.PodSpec{RestartPolicy: corev1.RestartPolicyAlways, Containers: []corev1.Container{{Name: "web", Image: "nginx"}, {Name: "sidecar", Image: "envoy"}}},
			Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}, ContainerStatuses: []corev1.ContainerStatus{
				{Name: "web", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
				{Name: "sidecar", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
			}},
		}
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "web-creds", Namespace: "team-a", Labels: labels},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"password": []byte("hunter2")},
	}
	rogue := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "rogue", Namespace: "team-a"}}
	return append(platformFixtures(), app, rt, deploy, rs, pod("web-abc-1"), pod("web-abc-2"), secret, rogue)
}

var _ = Describe("get_managed_resource", func() {
	var h *harness

	AfterEach(func() {
		if h != nil {
			h.close()
			h = nil
		}
	})

	It("returns every managed resource with its live tree and health", func() {
		h = startHarness(Options{}, managedFixtures()...)
		var out GetManagedResourceOutput
		res := h.call("get_managed_resource", map[string]any{"name": "web", "namespace": "team-a"}, &out)
		Expect(res.IsError).To(BeFalse())
		Expect(out.Resources).To(HaveLen(2))
		var deploy, secret *ManagedResource
		for i := range out.Resources {
			switch out.Resources[i].Kind {
			case "Deployment":
				deploy = &out.Resources[i]
			case "Secret":
				secret = &out.Resources[i]
			}
		}
		Expect(deploy).NotTo(BeNil())
		Expect(deploy.Cluster).To(Equal("local"))
		Expect(deploy.Component).To(Equal("web"))
		Expect(deploy.Latest).To(BeTrue())
		Expect(deploy.Object).To(BeEmpty(), "no live object unless one resource was selected")
		Expect(deploy.Tree).To(HaveLen(4), "Deployment, ReplicaSet and two Pods")
		root := deploy.Tree[0]
		Expect(root.Kind).To(Equal("Deployment"))
		Expect(root.Depth).To(Equal(0))
		Expect(root.Parent).To(BeEmpty())
		Expect(root.Health.Status).To(Equal("Healthy"))
		rs := deploy.Tree[1]
		Expect(rs.Kind).To(Equal("ReplicaSet"))
		Expect(rs.Depth).To(Equal(1))
		Expect(rs.Parent).To(Equal("Deployment/web"))
		pod := deploy.Tree[2]
		Expect(pod.Kind).To(Equal("Pod"))
		Expect(pod.Depth).To(Equal(2))
		Expect(pod.Parent).To(Equal("ReplicaSet/web-abc"))
		Expect(pod.Health.Status).To(Equal("Healthy"))
		Expect(deploy.Tree[3].Kind).To(Equal("Pod"))
		Expect(secret).NotTo(BeNil())
		Expect(secret.Object).To(BeEmpty())
	})

	It("returns the live object for a single selection, redacted", func() {
		h = startHarness(Options{}, managedFixtures()...)
		var out GetManagedResourceOutput
		h.call("get_managed_resource", map[string]any{"name": "web", "namespace": "team-a", "kind": "Deployment", "resourceName": "web"}, &out)
		Expect(out.Resources).To(HaveLen(1))
		obj := out.Resources[0].Object
		Expect(obj).To(HaveKeyWithValue("kind", "Deployment"))
		meta := obj["metadata"].(map[string]any)
		Expect(meta).NotTo(HaveKey("managedFields"))
		Expect(out.Evidence).To(ContainElement(ContainSubstring("Deployment team-a/web in cluster local resourceVersion 77")))
	})

	It("returns only metadata for a Secret", func() {
		h = startHarness(Options{}, managedFixtures()...)
		var out GetManagedResourceOutput
		h.call("get_managed_resource", map[string]any{"name": "web", "namespace": "team-a", "kind": "Secret", "resourceName": "web-creds"}, &out)
		Expect(out.Resources).To(HaveLen(1))
		obj := out.Resources[0].Object
		Expect(obj).To(HaveKey("metadata"))
		Expect(obj).NotTo(HaveKey("data"))
		Expect(obj).NotTo(HaveKey("stringData"))
		Expect(out.Resources[0].ObjectNote).To(ContainSubstring("metadata only"))
	})

	It("refuses objects the application does not manage", func() {
		h = startHarness(Options{}, managedFixtures()...)
		res := h.call("get_managed_resource", map[string]any{"name": "web", "namespace": "team-a", "kind": "Deployment", "resourceName": "rogue"}, nil)
		Expect(errorText(res)).To(ContainSubstring("not managed by application team-a/web"))
	})

	It("filters by component and by kind", func() {
		h = startHarness(Options{}, managedFixtures()...)
		var out GetManagedResourceOutput
		h.call("get_managed_resource", map[string]any{"name": "web", "namespace": "team-a", "component": "db"}, &out)
		Expect(out.Resources).To(BeEmpty())
		h.call("get_managed_resource", map[string]any{"name": "web", "namespace": "team-a", "kind": "secret"}, &out)
		Expect(out.Resources).To(HaveLen(1))
		Expect(out.Resources[0].Kind).To(Equal("Secret"))
	})
})

var _ = Describe("get_logs", func() {
	var (
		h        *harness
		requests []string
	)

	fakeLogs := func(_ context.Context, _ *rest.Config, cluster, namespace, pod string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
		requests = append(requests, cluster+"/"+namespace+"/"+pod+"/"+opts.Container)
		body := "started\nAuthorization: Bearer eyJhbGciOi.payload.sig\npassword=hunter2 user=app\nAKIAABCDEFGHIJKLMNOP used\nready\n"
		if opts.Previous {
			body = "crashed: panic\n"
		}
		return io.NopCloser(strings.NewReader(body)), nil
	}

	BeforeEach(func() { requests = nil })

	AfterEach(func() {
		if h != nil {
			h.close()
			h = nil
		}
	})

	It("tails every pod of the application with secrets removed", func() {
		h = startHarness(Options{PodLogs: fakeLogs}, managedFixtures()...)
		var out GetLogsOutput
		res := h.call("get_logs", map[string]any{"name": "web", "namespace": "team-a"}, &out)
		Expect(res.IsError).To(BeFalse())
		Expect(out.Pods).To(HaveLen(2))
		Expect(requests).To(ConsistOf("local/team-a/web-abc-1/web", "local/team-a/web-abc-2/web"))
		p := out.Pods[0]
		Expect(p.Pod).To(Equal("web-abc-1"))
		Expect(p.Component).To(Equal("web"))
		Expect(p.Container).To(Equal("web"))
		Expect(p.Containers).To(Equal([]string{"web", "sidecar"}))
		Expect(p.Phase).To(Equal("Running"))
		Expect(p.Lines).To(Equal([]string{
			"started",
			"Authorization: Bearer " + redactedValue,
			"password=" + redactedValue + " user=app",
			"AKIA" + redactedValue + " used",
			"ready",
		}))
		Expect(p.Truncated).To(BeFalse())
		Expect(out.Meta.Truncated).To(BeFalse())
		Expect(out.Evidence).To(ContainElement("Pod team-a/web-abc-1 container web in cluster local"))
	})

	It("selects one pod and container and honours previous", func() {
		h = startHarness(Options{PodLogs: fakeLogs}, managedFixtures()...)
		var out GetLogsOutput
		h.call("get_logs", map[string]any{"name": "web", "namespace": "team-a", "pod": "web-abc-2", "container": "sidecar", "previous": true, "tailLines": 10}, &out)
		Expect(out.Pods).To(HaveLen(1))
		Expect(requests).To(Equal([]string{"local/team-a/web-abc-2/sidecar"}))
		Expect(out.Pods[0].Lines).To(Equal([]string{"crashed: panic"}))
	})

	It("refuses pods the application does not manage and reports empty matches", func() {
		h = startHarness(Options{PodLogs: fakeLogs}, managedFixtures()...)
		res := h.call("get_logs", map[string]any{"name": "web", "namespace": "team-a", "pod": "other"}, nil)
		Expect(errorText(res)).To(ContainSubstring("not managed by application"))
		res = h.call("get_logs", map[string]any{"name": "web", "namespace": "team-a", "component": "db"}, nil)
		Expect(errorText(res)).To(ContainSubstring("no pods matching"))
	})
})
