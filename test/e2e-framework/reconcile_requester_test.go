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

package framework

import (
	"context"
	"testing"
	"time"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// requesterHarness drives a reconcileRequester on a fake clock and counts the
// reconciles it requests.
type requesterHarness struct {
	t     *testing.T
	ctx   context.Context
	c     client.Client
	obj   *corev1.ConfigMap
	clock time.Time
	r     *reconcileRequester
}

func newRequesterHarness(t *testing.T) *requesterHarness {
	obj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"}}
	c := fake.NewClientBuilder().WithObjects(obj).Build()
	h := &requesterHarness{t: t, ctx: context.Background(), c: c, obj: obj, clock: time.Unix(0, 0)}
	h.r = &reconcileRequester{
		framework: &Framework{Client: c},
		ctx:       h.ctx,
		obj:       obj,
		settle:    settlePeriod,
		now:       func() time.Time { return h.clock },
	}
	return h
}

// pollFor advances the clock in one-second polls, as EventuallyReconciled does,
// and returns how many reconciles were requested.
func (h *requesterHarness) pollFor(d time.Duration) int {
	g := gomega.NewWithT(h.t)
	requested := 0
	for end := h.clock.Add(d); h.clock.Before(end); h.clock = h.clock.Add(time.Second) {
		before := h.stamp()
		h.r.request(g)
		if h.stamp() != before {
			requested++
		}
	}
	return requested
}

func (h *requesterHarness) stamp() string {
	current := &corev1.ConfigMap{}
	gomega.NewWithT(h.t).Expect(h.c.Get(h.ctx, client.ObjectKeyFromObject(h.obj), current)).To(gomega.Succeed())
	return current.Annotations["app.oam.dev/requestreconcile"]
}

// controllerWrites stands in for the controller writing the object as a
// reconcile finishes.
func (h *requesterHarness) controllerWrites() {
	current := &corev1.ConfigMap{}
	g := gomega.NewWithT(h.t)
	g.Expect(h.c.Get(h.ctx, client.ObjectKeyFromObject(h.obj), current)).To(gomega.Succeed())
	current.Data = map[string]string{"written": h.clock.String()}
	g.Expect(h.c.Update(h.ctx, current)).To(gomega.Succeed())
}

func TestReconcileRequesterLeavesARequestedReconcileToFinish(t *testing.T) {
	h := newRequesterHarness(t)
	if n := h.pollFor(quietPeriod + time.Second); n != 1 {
		t.Fatalf("an object left alone for quietPeriod gets one request, got %d", n)
	}
	// The requested reconcile is still running: a second request would make
	// its write conflict, so none is sent while it has time to finish.
	if n := h.pollFor(settlePeriod - time.Second); n != 0 {
		t.Fatalf("requested %d more reconciles before the first one wrote the object", n)
	}
}

func TestReconcileRequesterRequestsAgainOnceTheControllerHasWritten(t *testing.T) {
	h := newRequesterHarness(t)
	h.pollFor(quietPeriod + time.Second)
	h.pollFor(5 * time.Second)
	h.controllerWrites()
	if n := h.pollFor(quietPeriod + time.Second); n != 1 {
		t.Fatalf("after the controller wrote and went quiet, expected one request, got %d", n)
	}
}

func TestReconcileRequesterRequestsAgainWhenTheReconcileWroteNothing(t *testing.T) {
	h := newRequesterHarness(t)
	h.pollFor(quietPeriod + time.Second)
	// A reconcile with nothing to change writes nothing; after settlePeriod the
	// request is taken to have been served and another is sent.
	if n := h.pollFor(settlePeriod + time.Second); n != 1 {
		t.Fatalf("expected one more request once settlePeriod passed with no write, got %d", n)
	}
}
