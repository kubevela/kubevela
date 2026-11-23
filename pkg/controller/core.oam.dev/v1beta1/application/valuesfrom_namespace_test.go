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

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The fingerprint reads values sources where the helm provider does: the Application's namespace
// only, so a change to the source a release uses is the change that triggers a rollout.
func TestValuesFromSourcesAreFingerprintedInTheApplicationNamespace(t *testing.T) {
	ns, err := resolveNamespace(valuesFromRef{Kind: "ConfigMap", Name: "v"}, "team-a", "release-ns")
	require.NoError(t, err)
	require.Equal(t, "team-a", ns, "no namespace means the Application's")

	ns, err = resolveNamespace(valuesFromRef{Kind: "ConfigMap", Name: "v", Namespace: "team-a"}, "team-a", "release-ns")
	require.NoError(t, err)
	require.Equal(t, "team-a", ns)

	_, err = resolveNamespace(valuesFromRef{Kind: "Secret", Name: "v", Namespace: "release-ns"}, "team-a", "release-ns")
	require.Error(t, err, "the release namespace is not a source")
}
