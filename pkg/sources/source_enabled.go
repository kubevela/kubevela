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

package sources

import (
	utilfeature "k8s.io/apiserver/pkg/util/feature"

	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// ExpressionsEnabledFor reports whether $( ) property expressions are read for
// an Application carrying these annotations.
//
// One function because admission and render must agree exactly. They reach it by
// different routes - the webhook holds the Application, a render reads
// context.appAnnotations - and if they ever disagreed the result would be an
// Application admitted and then failing to render, or worse, one admitted with an
// expression that is then written into a workload verbatim.
//
// Three states, from two gates:
//
//	EnableCelExpressions=false                      off, for everyone
//	EnableCelExpressions=true, RequireOptIn=true     annotated Applications only
//	EnableCelExpressions=true, RequireOptIn=false    every Application
func ExpressionsEnabledFor(annotations map[string]string) bool {
	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableCelExpressions) {
		return false
	}
	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.RequireCelExpressionOptIn) {
		return true
	}
	return annotations[oam.AnnotationCelExpressions] == "true"
}
