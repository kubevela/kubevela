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
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// Check returns nil when a Kubernetes REST config can be resolved from the
// ambient environment (--kubeconfig, $KUBECONFIG, ~/.kube/config, or in-cluster
// service account), and the resolution error otherwise.
//
// It resolves the config exactly the way config.GetConfigOrDie does, but hands
// back the failure instead of exiting, so callers can decide what to do. A nil
// return does not promise the cluster is reachable, only that a configuration
// exists to describe it. Reachability still surfaces as an ordinary error on
// the first request.
func Check() error {
	_, err := config.GetConfig()
	return err
}

// Available reports whether Check succeeds. It is a convenience for call sites
// that only branch on the outcome and do not log the reason.
func Available() bool {
	return Check() == nil
}
