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

package component

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/registry"
)

// TestOCIClientCacheReusesLogin pins the fix for the handshake storm: listing a
// catalog resolves every addon, and without reuse each of those built a new
// client and logged in again, which real registries reject once the catalog
// holds more than a couple of addons.
func TestOCIClientCacheReusesLogin(t *testing.T) {
	ociClientCache.Lock()
	ociClientCache.clients = map[string]*registry.Client{}
	ociClientCache.Unlock()

	first, err := NewOCIClientWithPlainHTTP("reg.example.com", "", "", false)
	require.NoError(t, err)
	second, err := NewOCIClientWithPlainHTTP("reg.example.com", "", "", false)
	require.NoError(t, err)
	assert.Same(t, first, second, "the same host and credentials must reuse one client")

	other, err := NewOCIClientWithPlainHTTP("other.example.com", "", "", false)
	require.NoError(t, err)
	assert.NotSame(t, first, other, "different hosts must not share a client")

	// Credentialed clients log in, so assert the keying rather than build one.
	// A rotated credential must miss the cache: an ECR login token lasts 12
	// hours, and reusing the client holding the stale one would fail every pull.
	assert.NotEqual(t,
		ociClientCacheKey("reg.example.com", "AWS", "old-token", false),
		ociClientCacheKey("reg.example.com", "AWS", "new-token", false),
		"a rotated credential must not reuse the client holding the stale token")
	assert.NotEqual(t,
		ociClientCacheKey("reg.example.com", "u", "p", false),
		ociClientCacheKey("reg.example.com", "u", "p", true),
		"plain HTTP and TLS clients must be keyed apart")
	assert.Equal(t,
		ociClientCacheKey("reg.example.com", "u", "p", false),
		ociClientCacheKey("reg.example.com", "u", "p", false),
		"the same inputs must produce the same key")
}

func TestOCIClientCacheIsBounded(t *testing.T) {
	ociClientCache.Lock()
	ociClientCache.clients = map[string]*registry.Client{}
	ociClientCache.Unlock()

	for i := 0; i < ociClientCacheLimit*2; i++ {
		_, err := NewOCIClientWithPlainHTTP(fmt.Sprintf("reg%d.example.com", i), "", "", false)
		require.NoError(t, err)
	}

	ociClientCache.Lock()
	size := len(ociClientCache.clients)
	ociClientCache.Unlock()
	assert.LessOrEqual(t, size, ociClientCacheLimit, "the cache must stay bounded as credentials rotate")
}

// TestIsDockerHubHost pins the alias list, including the port-stripping and
// case-folding, since a miss here silently turns Docker Hub's deterministic
// catalog 401 back into a hard failure.
func TestIsDockerHubHost(t *testing.T) {
	for _, host := range []string{
		"docker.io",
		"index.docker.io",
		"registry-1.docker.io",
		"DOCKER.IO",
		"registry-1.docker.io:443",
	} {
		assert.True(t, IsDockerHubHost(host), "%q should be Docker Hub", host)
	}
	for _, host := range []string{
		"",
		"ghcr.io",
		"776719623202.dkr.ecr.us-west-2.amazonaws.com",
		"notdocker.io",
		"docker.io.evil.com",
		"localhost:5000",
	} {
		assert.False(t, IsDockerHubHost(host), "%q should not be Docker Hub", host)
	}
}

// TestPullOCIChartFilesRejectsNonOCI pins that a registry which is not OCI
// backed is refused by name rather than being dialled. Reaching the transport
// with no OCI source would surface as a connection error naming nothing the
// caller configured.
func TestPullOCIChartFilesRejectsNonOCI(t *testing.T) {
	_, err := PullOCIChartFiles(context.Background(), Registry{Name: "git-reg"}, "s3", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git-reg")
}
