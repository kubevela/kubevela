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

package addon

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPushOCI exercises the real pushOCI implementation (the existing
// push_oci_test.go tests only cover routing via the ociPushFn test seam, never
// pushOCI itself). A closed loopback port makes the final network push fail
// fast and deterministically, so the test still walks the whole function --
// chart loading, packaging, and building the OCI ref -- without depending on
// a live registry.
func TestPushOCI(t *testing.T) {
	t.Run("bad chart path fails before contacting the registry", func(t *testing.T) {
		p := &PushCmd{ChartName: "/this/this/not/a/chart"}
		err := p.pushOCI(&OCIAddonSource{URL: "oci://127.0.0.1:1/addon"})
		require.Error(t, err)
	})

	t.Run("valid chart pushes to an unreachable registry and returns a wrapped error", func(t *testing.T) {
		var out bytes.Buffer
		p := &PushCmd{
			ChartName:    "testdata/charts/sample-1.0.1.tgz",
			ChartVersion: "9.9.9",
			AppVersion:   "9.9.9",
			Out:          &out,
		}

		err := p.pushOCI(&OCIAddonSource{URL: "oci://127.0.0.1:1/addon"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to push OCI addon")
		assert.Contains(t, out.String(), "Pushing")
	})
}
