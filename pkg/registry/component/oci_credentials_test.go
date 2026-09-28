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
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOCICredentialsFileCarriesTheCredential pins the shape the helm registry
// client reads: an auths entry keyed by host, holding a base64 "user:password".
// The entry is what keeps the Docker config machinery away from its platform
// credential helper, so an empty or differently shaped file would put the
// keychain back in the path.
func TestOCICredentialsFileCarriesTheCredential(t *testing.T) {
	path, err := writeOCICredentialsFile("key", "reg.example.com", "AWS", "token")
	require.NoError(t, err)
	defer removeOCICredentialsFile(path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var doc struct {
		Auths map[string]struct {
			Auth          string `json:"auth"`
			IdentityToken string `json:"identitytoken"`
		} `json:"auths"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))

	entry, ok := doc.Auths["reg.example.com"]
	require.True(t, ok, "the credential must be keyed by the registry host")
	decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
	require.NoError(t, err)
	assert.Equal(t, "AWS:token", string(decoded))
}

// TestOCICredentialsFileUsesIdentityTokenWithoutUsername pins the other shape.
// Decoding an auth string requires a non-empty user, so a credential with no
// username has to travel as an identity token or it is rejected on read.
func TestOCICredentialsFileUsesIdentityTokenWithoutUsername(t *testing.T) {
	path, err := writeOCICredentialsFile("key", "reg.example.com", "", "token")
	require.NoError(t, err)
	defer removeOCICredentialsFile(path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var doc struct {
		Auths map[string]struct {
			Auth          string `json:"auth"`
			IdentityToken string `json:"identitytoken"`
		} `json:"auths"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))

	entry := doc.Auths["reg.example.com"]
	assert.Equal(t, "token", entry.IdentityToken)
	assert.Empty(t, entry.Auth)
}

// TestOCICredentialsFileIsPrivate keeps the credential off other users of the
// machine. The file holds a registry password in plain text.
func TestOCICredentialsFileIsPrivate(t *testing.T) {
	path, err := writeOCICredentialsFile("key", "reg.example.com", "u", "p")
	require.NoError(t, err)
	defer removeOCICredentialsFile(path)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

// TestOCIAuthHostname pins the Docker Hub canonicalisation. oras resolves the
// hostname before looking a credential up, so writing one under the raw host
// would store a credential the lookup never finds, and the pull would go out
// anonymous.
func TestOCIAuthHostname(t *testing.T) {
	for _, alias := range []string{"index.docker.io", "docker.io", "registry-1.docker.io"} {
		assert.Equal(t, "https://index.docker.io/v1/", ociAuthHostname(alias), alias)
	}
	assert.Equal(t, "reg.example.com", ociAuthHostname("reg.example.com"))
	assert.Equal(t, "123.dkr.ecr.us-west-2.amazonaws.com", ociAuthHostname("123.dkr.ecr.us-west-2.amazonaws.com"))
}

// TestCredentialedOCIClientNeedsNoCredentialHelper is the regression this
// change exists for. Building a client for a credentialed registry used to log
// in, and the login wrote the credential to whatever store the machine's helm
// config named; where that named a helper binary the process could not run,
// every OCI operation failed. Nothing is written back now, so the build
// succeeds whatever the machine is configured with.
func TestCredentialedOCIClientNeedsNoCredentialHelper(t *testing.T) {
	ResetOCIClientCache()
	defer ResetOCIClientCache()

	// A config naming a helper that does not exist. Before the change this is
	// what the client would have tried to store through.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(dir+"/registry", 0o700))
	require.NoError(t, os.WriteFile(dir+"/registry/config.json",
		[]byte(`{"auths":{"reg.example.com":{}},"credsStore":"nonexistent-helper"}`), 0o600))
	t.Setenv("HELM_CONFIG_HOME", dir)

	client, err := NewOCIClientWithPlainHTTP("reg.example.com", "AWS", "token", false)
	require.NoError(t, err, "building a credentialed client must not depend on a credential helper")
	assert.NotNil(t, client)
}
