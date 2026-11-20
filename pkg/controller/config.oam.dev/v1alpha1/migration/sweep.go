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

// Package migration runs the legacy config adoption sweep at controller startup
// when the AutoMigrateLegacyConfigs feature gate is on.
package migration

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	"github.com/oam-dev/kubevela/pkg/config"
	"github.com/oam-dev/kubevela/pkg/monitor/metrics"
)

const (
	// DefaultAttempts is how many times the sweep runs when rows keep failing.
	DefaultAttempts = 3
	// DefaultInterval is the pause between attempts.
	DefaultInterval = 30 * time.Second
	// DefaultReadyTimeout caps the wait for the webhook server. After it the sweep
	// runs anyway and the rows say what failed.
	DefaultReadyTimeout = 2 * time.Minute

	readyPollInterval = 2 * time.Second
)

// StartupSweep adopts every legacy config template and config in the cluster once,
// when the manager starts. It runs on the leader only, after the caches synced,
// and never fails the manager: list errors and panics are logged, Start returns nil.
//
// The log messages, Event reasons and the metric are a contract with the manual
// test scenarios; change them together.
type StartupSweep struct {
	// Client reads and writes directly. Use an uncached client: the manager's client
	// would start cluster-wide informers for Secrets and ConfigMaps on the first list.
	Client client.Client
	// Recorder receives the migration Events on the legacy objects; nil records nothing.
	Recorder record.EventRecorder
	// Ready is the webhook server's StartedChecker. Every Config create goes through
	// the validating webhook served by this process, so the sweep waits for it. nil
	// skips the wait.
	Ready healthz.Checker
	// Attempts is how many times to run while rows fail; 0 means DefaultAttempts.
	Attempts int
	// Interval is the pause between attempts; 0 means DefaultInterval.
	Interval time.Duration
	// ReadyTimeout caps the wait for Ready; 0 means DefaultReadyTimeout.
	ReadyTimeout time.Duration
	// Now and Sleep are replaceable for tests.
	Now   func() time.Time
	Sleep func(context.Context, time.Duration)
}

// NeedLeaderElection makes the manager start the sweep on the leader only.
func (s *StartupSweep) NeedLeaderElection() bool { return true }

// Start runs the sweep. It blocks until the sweep is over or ctx is cancelled and
// returns nil in every case, so a sweep that cannot run is read in the log, not in
// a crash loop.
func (s *StartupSweep) Start(ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			klog.ErrorS(fmt.Errorf("%v", r), "legacy config migration sweep panicked", "stack", string(debug.Stack()))
		}
		err = nil
	}()
	s.defaults()
	klog.InfoS("starting legacy config migration sweep", "attempts", s.Attempts, "interval", s.Interval)
	s.waitForWebhook(ctx)
	s.run(ctx)
	return nil
}

func (s *StartupSweep) defaults() {
	if s.Attempts <= 0 {
		s.Attempts = DefaultAttempts
	}
	if s.Interval <= 0 {
		s.Interval = DefaultInterval
	}
	if s.ReadyTimeout <= 0 {
		s.ReadyTimeout = DefaultReadyTimeout
	}
	if s.Now == nil {
		s.Now = time.Now
	}
	if s.Sleep == nil {
		s.Sleep = sleepContext
	}
}

// waitForWebhook polls Ready until it passes, the timeout is reached or ctx ends.
func (s *StartupSweep) waitForWebhook(ctx context.Context) {
	if s.Ready == nil {
		return
	}
	deadline := s.Now().Add(s.ReadyTimeout)
	logged := false
	for ctx.Err() == nil {
		if err := s.Ready(&http.Request{}); err == nil {
			return
		}
		if !logged {
			klog.InfoS("waiting for the webhook server before the migration sweep", "timeout", s.ReadyTimeout)
			logged = true
		}
		if !s.Now().Before(deadline) {
			klog.InfoS("webhook server not ready in time; running the migration sweep anyway", "timeout", s.ReadyTimeout)
			return
		}
		s.Sleep(ctx, readyPollInterval)
	}
}

// run performs the attempts. A repeat only touches what failed: adoption is
// idempotent and everything adopted earlier comes back skipped.
func (s *StartupSweep) run(ctx context.Context) {
	migrator := config.NewMigrator(s.Client)
	opts := config.MigrationOptions{Recorder: s.Recorder}
	var report config.MigrationReport
	attempt := 0
	for attempt < s.Attempts && ctx.Err() == nil {
		attempt++
		if attempt > 1 {
			s.Sleep(ctx, s.Interval)
			if ctx.Err() != nil {
				break
			}
		}
		started := s.Now()
		var err error
		report, err = migrator.MigrateAll(ctx, opts)
		if err != nil {
			klog.ErrorS(err, "legacy config migration sweep attempt could not list legacy objects", "attempt", attempt)
			continue
		}
		klog.InfoS("legacy config migration sweep attempt", "attempt", attempt,
			"adopted", report.Count(config.MigrationAdopted), "skipped", report.Count(config.MigrationSkipped),
			"conflict", report.Count(config.MigrationConflict), "failed", report.Count(config.MigrationFailed),
			"duration", s.Now().Sub(started))
		logProblems(&report)
		if !report.HasFailure() {
			break
		}
	}
	for _, outcome := range []config.MigrationOutcome{config.MigrationAdopted, config.MigrationSkipped, config.MigrationConflict, config.MigrationFailed} {
		metrics.ConfigMigrationCounter.WithLabelValues(string(outcome)).Add(float64(report.Count(outcome)))
	}
	klog.InfoS("legacy config migration sweep finished",
		"adopted", report.Count(config.MigrationAdopted), "skipped", report.Count(config.MigrationSkipped),
		"conflict", report.Count(config.MigrationConflict), "failed", report.Count(config.MigrationFailed),
		"attempts", attempt)
}

// logProblems logs the conflict and failed rows. Adopted rows are Events only; a
// large cluster would otherwise log thousands of lines at startup.
func logProblems(report *config.MigrationReport) {
	for _, res := range report.Results {
		switch res.Outcome {
		case config.MigrationConflict:
			klog.InfoS("legacy config migration: conflict", "kind", res.Kind, "namespace", res.Namespace, "name", res.Name, "reason", res.Reason)
		case config.MigrationFailed:
			klog.InfoS("legacy config migration: failed", "kind", res.Kind, "namespace", res.Namespace, "name", res.Name, "reason", res.Reason)
		default:
		}
	}
}

func sleepContext(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
