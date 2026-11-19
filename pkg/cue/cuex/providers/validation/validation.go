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

// Package validation carries the marker that tells CueX providers they are
// evaluating an Application for admission rather than rendering it for real.
//
// It sits beside the providers rather than inside any one of them because the
// marker is not about addons or modules: the addon provider, the module
// provider and the appfile validation path all need the same signal, and
// keeping it here means shared appfile code does not import a package it has no
// other business with.
package validation

import "context"

// validationOnlyKey marks a context as admission-time validation, in which a
// provider must not reach a remote registry.
type validationOnlyKey struct{}

// WithValidationOnly returns a context on which a provider renders a
// placeholder instead of fetching and resolving from a remote registry. It
// mirrors helm.WithDryRun, which the same validation path sets for the helm
// provider.
//
// Resolving one package is a registry listing plus one request per file in it,
// and an Application resolves each of its components separately. The apiserver
// caps an admission request at admissionWebhookTimeout (10s by default) and the
// Application webhook runs with failurePolicy: Fail, so an Application with a
// handful of addon or module components cannot fit in the budget, and every
// apply in the cluster would depend on the registry being reachable and on the
// rate-limit budget of whoever hosts it.
//
// Resolution therefore belongs to the controller, which has no admission
// deadline and reports failures through the Application status. Admission keeps
// only what it can answer locally: that the component's CUE compiles, that its
// parameters typecheck, and whatever checks its own validator makes.
func WithValidationOnly(ctx context.Context) context.Context {
	return context.WithValue(ctx, validationOnlyKey{}, true)
}

// IsValidationOnly reports whether ctx forbids remote registry access.
func IsValidationOnly(ctx context.Context) bool {
	v, _ := ctx.Value(validationOnlyKey{}).(bool)
	return v
}
