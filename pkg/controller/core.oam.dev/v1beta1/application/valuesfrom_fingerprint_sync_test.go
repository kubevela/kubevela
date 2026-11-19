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

package application

import "testing"

// TestDefaultValuesFromKeyMatchesHelmProvider locks defaultValuesFromKey to
// the literal value the helm provider's defaultValuesKey uses at
// pkg/cue/cuex/providers/helm/helm.go. The two packages cannot import each
// other (circular), and the helm provider's constant is unexported, so the
// fingerprint helper has to duplicate the literal. If the helm provider
// changes its default, this test catches the drift at package-test time
// instead of leaving a silent fingerprint bug in production.
func TestDefaultValuesFromKeyMatchesHelmProvider(t *testing.T) {
	const helmProviderDefault = "values.yaml"
	if defaultValuesFromKey != helmProviderDefault {
		t.Fatalf("defaultValuesFromKey %q diverged from helm provider default %q — keep them in sync (see pkg/cue/cuex/providers/helm/helm.go)", defaultValuesFromKey, helmProviderDefault)
	}
}
