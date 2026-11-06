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

package migration

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	legacyconfig "github.com/oam-dev/kubevela/pkg/config"
	"github.com/oam-dev/kubevela/pkg/monitor/metrics"
)

// sweepTemplateCUE is a legacy template with one same-namespace output object.
func sweepTemplateCUE(name string) string {
	return fmt.Sprintf(`
metadata: {
	name:      %q
	alias:     "sweep"
	scope:     "namespace"
	sensitive: false
}
context: {
	name:      string
	namespace: string
}
template: {
	parameter: value: string
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {name: context.name, namespace: context.namespace}
		stringData: value: parameter.value
	}
	outputs: {
		local: {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: {name: context.name + "-out", namespace: context.namespace}
			data: value: parameter.value
		}
	}
}
`, name)
}

// counts reads the four series of the migration counter.
type counts struct{ adopted, skipped, conflict, failed float64 }

func readCounts() counts {
	read := func(result legacyconfig.MigrationOutcome) float64 {
		return testutil.ToFloat64(metrics.ConfigMigrationCounter.WithLabelValues(string(result)))
	}
	return counts{
		adopted: read(legacyconfig.MigrationAdopted), skipped: read(legacyconfig.MigrationSkipped),
		conflict: read(legacyconfig.MigrationConflict), failed: read(legacyconfig.MigrationFailed),
	}
}

// drain returns every Event the fake recorder holds, as "Type Reason Message" lines.
func drain(rec *record.FakeRecorder) []string {
	var events []string
	for {
		select {
		case ev := <-rec.Events:
			events = append(events, ev)
		default:
			return events
		}
	}
}

func withReason(events []string, reason string) []string {
	var out []string
	for _, ev := range events {
		if strings.Contains(ev, " "+reason+" ") {
			out = append(out, ev)
		}
	}
	return out
}

var _ = Describe("The startup migration sweep", func() {
	ctx := context.Background()
	var fac legacyconfig.Factory
	var watchClient client.WithWatch
	noSleep := func(context.Context, time.Duration) {}

	ensureNamespace := func(name string) {
		err := k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}})
		Expect(client.IgnoreAlreadyExists(err)).Should(Succeed())
	}

	// seed writes a legacy template and one legacy config of it the way the 1.11
	// factory does, so the sweep sees the real legacy shape.
	seed := func(ns, tpl, cfgName string) {
		t, err := fac.ParseTemplate(ctx, tpl, []byte(sweepTemplateCUE(tpl)))
		Expect(err).Should(Succeed())
		Expect(fac.CreateOrUpdateConfigTemplate(ctx, ns, t)).Should(Succeed())
		item, err := fac.ParseConfig(ctx, legacyconfig.NamespacedName{Namespace: ns, Name: tpl}, legacyconfig.Metadata{
			NamespacedName: legacyconfig.NamespacedName{Namespace: ns, Name: cfgName},
			Alias:          "a",
			Properties:     map[string]interface{}{"value": "v-" + cfgName},
		})
		Expect(err).Should(Succeed())
		Expect(fac.CreateOrUpdateConfig(ctx, item, ns)).Should(Succeed())
		var secret corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: cfgName}, &secret)).Should(Succeed())
		Expect(secret.OwnerReferences).Should(BeEmpty())
	}

	eventuallyAvailable := func(ns, name string) *configv1alpha1.Config {
		var cfg configv1alpha1.Config
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &cfg)).Should(Succeed())
			g.Expect(cfg.Status.Phase).Should(Equal(configv1alpha1.ConfigPhaseAvailable))
		}, 30*time.Second, 200*time.Millisecond).Should(Succeed())
		return &cfg
	}

	BeforeEach(func() {
		fac = legacyconfig.NewConfigFactory(k8sClient)
		var err error
		watchClient, err = client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).Should(Succeed())
	})

	It("needs leader election", func() {
		Expect((&StartupSweep{}).NeedLeaderElection()).Should(BeTrue())
	})

	It("adopts the legacy objects once, counts them, and only skips on a second run", func() {
		const ns = "sweep-a"
		ensureNamespace(ns)
		seed(ns, "sw-tpl", "sw-cfg")
		rec := record.NewFakeRecorder(50)
		before := readCounts()

		sweep := &StartupSweep{Client: k8sClient, Recorder: rec, Interval: time.Millisecond, Sleep: noSleep}
		Expect(sweep.Start(ctx)).Should(Succeed())

		events := drain(rec)
		Expect(withReason(events, legacyconfig.EventMigrationAdopted)).Should(HaveLen(2), "template and config adopted: %v", events)
		Expect(withReason(events, legacyconfig.EventMigrationFailed)).Should(BeEmpty())
		var ct configv1alpha1.ConfigTemplate
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "sw-tpl"}, &ct)).Should(Succeed())
		cfg := eventuallyAvailable(ns, "sw-cfg")
		var secret corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "sw-cfg"}, &secret)).Should(Succeed())
		Expect(metav1.GetControllerOf(&secret)).ShouldNot(BeNil())
		Expect(metav1.GetControllerOf(&secret).UID).Should(Equal(cfg.UID))

		after := readCounts()
		Expect(after.adopted - before.adopted).Should(Equal(2.0))
		Expect(after.failed - before.failed).Should(Equal(0.0))

		By("a second sweep finds everything adopted")
		rec2 := record.NewFakeRecorder(50)
		second := &StartupSweep{Client: k8sClient, Recorder: rec2, Interval: time.Millisecond, Sleep: noSleep}
		Expect(second.Start(ctx)).Should(Succeed())
		Expect(withReason(drain(rec2), legacyconfig.EventMigrationAdopted)).Should(BeEmpty())
		again := readCounts()
		Expect(again.adopted - after.adopted).Should(Equal(0.0))
		Expect(again.skipped - after.skipped).Should(BeNumerically(">=", 2.0))
		var secretAgain corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: "sw-cfg"}, &secretAgain)).Should(Succeed())
		Expect(secretAgain.ResourceVersion).Should(Equal(secret.ResourceVersion))
	})

	It("adopts on the second attempt what failed on the first", func() {
		const ns = "sweep-b"
		ensureNamespace(ns)
		seed(ns, "sw-retry-tpl", "sw-retry-cfg")
		rec := record.NewFakeRecorder(50)
		before := readCounts()

		// The first Create of the Config is refused, as a flaky API server would; the
		// template adopts on attempt 1, the config on attempt 2.
		rejected := false
		flaky := interceptor.NewClient(watchClient, interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if cfg, ok := obj.(*configv1alpha1.Config); ok && cfg.Name == "sw-retry-cfg" && !rejected {
					rejected = true
					return errors.New("injected create failure")
				}
				return c.Create(ctx, obj, opts...)
			},
		})
		sweep := &StartupSweep{Client: flaky, Recorder: rec, Attempts: 3, Interval: time.Millisecond, Sleep: noSleep}
		Expect(sweep.Start(ctx)).Should(Succeed())

		events := drain(rec)
		Expect(withReason(events, legacyconfig.EventMigrationFailed)).Should(HaveLen(1), "%v", events)
		Expect(withReason(events, legacyconfig.EventMigrationAdopted)).Should(HaveLen(2), "%v", events)
		eventuallyAvailable(ns, "sw-retry-cfg")

		after := readCounts()
		Expect(after.failed-before.failed).Should(Equal(0.0), "the final attempt had no failed row")
		Expect(after.adopted-before.adopted).Should(Equal(1.0), "the final attempt adopted the config only")
	})

	It("gives up on the last attempt and counts the row as failed", func() {
		const ns = "sweep-c"
		ensureNamespace(ns)
		seed(ns, "sw-stuck-tpl", "sw-stuck-cfg")
		rec := record.NewFakeRecorder(50)
		before := readCounts()

		stuck := interceptor.NewClient(watchClient, interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if cfg, ok := obj.(*configv1alpha1.Config); ok && cfg.Name == "sw-stuck-cfg" {
					return errors.New("always refused")
				}
				return c.Create(ctx, obj, opts...)
			},
		})
		sweep := &StartupSweep{Client: stuck, Recorder: rec, Attempts: 2, Interval: time.Millisecond, Sleep: noSleep}
		Expect(sweep.Start(ctx)).Should(Succeed(), "a failing sweep never fails the manager")

		Expect(withReason(drain(rec), legacyconfig.EventMigrationFailed)).Should(HaveLen(2), "one per attempt")
		after := readCounts()
		Expect(after.failed - before.failed).Should(Equal(1.0))
	})

	It("waits for the webhook server and proceeds once it is up", func() {
		calls := 0
		ready := func(*http.Request) error {
			calls++
			if calls < 3 {
				return errors.New("not yet")
			}
			return nil
		}
		sweep := &StartupSweep{Client: k8sClient, Ready: ready, Sleep: noSleep}
		Expect(sweep.Start(ctx)).Should(Succeed())
		Expect(calls).Should(Equal(3))
	})

	It("stops waiting for the webhook server at the cap and sweeps anyway", func() {
		const ns = "sweep-d"
		ensureNamespace(ns)
		seed(ns, "sw-late-tpl", "sw-late-cfg")
		rec := record.NewFakeRecorder(50)
		never := func(*http.Request) error { return errors.New("never ready") }
		now := time.Now()
		clock := func() time.Time { now = now.Add(time.Second); return now }
		sweep := &StartupSweep{Client: k8sClient, Recorder: rec, Ready: never, ReadyTimeout: 5 * time.Second, Now: clock, Sleep: noSleep}
		Expect(sweep.Start(ctx)).Should(Succeed())
		Expect(withReason(drain(rec), legacyconfig.EventMigrationAdopted)).Should(HaveLen(2))
	})

	It("returns nil when the context ends before the sweep", func() {
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		never := func(*http.Request) error { return errors.New("never ready") }
		sweep := &StartupSweep{Client: k8sClient, Ready: never, Sleep: noSleep}
		Expect(sweep.Start(cancelled)).Should(Succeed())
	})
})
