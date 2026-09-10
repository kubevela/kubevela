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

package registry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// A path is relative to the registry's configured root. Nothing checked that,
// so `../` reached whatever the backend put next to it.
func TestReadFileRefusesPathsOutsideTheRegistry(t *testing.T) {
	for _, path := range []string{
		"/etc/passwd",
		"../secrets.yaml",
		"addons/../../secrets.yaml",
		"..",
	} {
		err := validateRegistryPath(path)
		require.Error(t, err, "path %q must be refused", path)
	}
	for _, path := range []string{
		"addons/redis/metadata.yaml",
		"./addons/redis/metadata.yaml",
		"addons/redis/../mysql/metadata.yaml",
	} {
		require.NoError(t, validateRegistryPath(path), "path %q must be allowed", path)
	}
}
