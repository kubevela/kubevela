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

// Package kubeconfig reports whether a Kubernetes client configuration can be
// resolved, without terminating the process when it cannot.
//
// It exists because the shared client singletons in kubevela/pkg are built on
// controller-runtime's config.GetConfigOrDie, which calls os.Exit(1) when no
// kubeconfig is present. That is not a panic and not a returned error, so a
// caller cannot recover from it, log it, or fall back: the process is simply
// gone. Callers that are able to run without a cluster should consult Check
// before reaching for anything that resolves a client singleton.
package kubeconfig

import (
	"sync/atomic"

	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// assumed records that a client configuration was supplied out of band. See
// AssumeAvailable.
var assumed atomic.Bool

// AssumeAvailable records that a Kubernetes client configuration has been
// supplied directly, bypassing the environment. Call it alongside
// singleton.KubeConfig.Set, as the envtest harnesses do: a singleton that has
// already been populated never reaches GetConfigOrDie, so it cannot exit the
// process, and the work this package guards is safe to attempt.
//
// This has to be recorded explicitly because the singleton exposes no way to
// ask whether it has been set. Without it, a suite that supplies an envtest
// config would still be told to skip, and would silently lose behaviour that
// depends on cluster access, such as loading external CUE packages.
//
// The returned function restores the state the call displaced rather than
// clearing the assumption outright, so calls nest. Call it once the supplied
// configuration is no longer valid, typically when the harness tears its
// control plane down.
func AssumeAvailable() (restore func()) {
	return restoreAssumption(assumed.Swap(true))
}

// AssumeUnavailable withdraws any outstanding assumption until the returned
// function is called, so Check consults the environment again. It is the
// inverse of AssumeAvailable, for tests that exercise the skip path from
// inside a suite that has already supplied a config. Without it such a test
// has to clear the suite-wide assumption and remember to reinstate it by hand.
func AssumeUnavailable() (restore func()) {
	return restoreAssumption(assumed.Swap(false))
}

// restoreAssumption puts back the value an Assume call displaced. Restoring
// the previous value rather than a fixed one is what lets the calls nest: an
// inner scope ending must not cancel an outer scope that is still open. A
// restore that stored false unconditionally would leave a suite which assumed
// once in TestMain silently skipping the work this package guards.
func restoreAssumption(previous bool) func() {
	return func() { assumed.Store(previous) }
}

// Check returns nil when using the shared client singletons is safe: either a
// configuration was supplied out of band (see AssumeAvailable) or a REST config
// can be resolved from the ambient environment (--kubeconfig, $KUBECONFIG,
// ~/.kube/config, or in-cluster service account). Otherwise it returns the
// resolution error.
//
// It resolves the config exactly the way config.GetConfigOrDie does, but hands
// back the failure instead of exiting, so callers can decide what to do. A nil
// return does not promise the cluster is reachable, only that a configuration
// exists to describe it. Reachability still surfaces as an ordinary error on
// the first request.
func Check() error {
	if assumed.Load() {
		return nil
	}
	_, err := config.GetConfig()
	return err
}

// Available reports whether Check succeeds. It is a convenience for call sites
// that only branch on the outcome and do not log the reason.
func Available() bool {
	return Check() == nil
}

// AvailableFor reports whether Check succeeds, logging a warning that names the
// work being skipped when it does not. Degrading silently would leave an
// operator with no explanation for why, say, external CUE packages are missing.
//
// Note that this inspects the ambient environment, not the client singletons.
// A process that populates singleton.KubeConfig directly, as the envtest suites
// do, has a working client even though nothing resolvable exists on disk, and
// will be told to skip. That is the safe direction to be wrong in: the singleton
// offers no way to ask whether it has already been set, and guessing wrong the
// other way exits the process.
func AvailableFor(purpose string) bool {
	if err := Check(); err != nil {
		klog.Warningf("no usable kubeconfig, skipping %s: %v", purpose, err)
		return false
	}
	return true
}
