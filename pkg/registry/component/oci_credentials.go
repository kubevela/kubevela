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
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// The helm registry client authenticates from a credentials file in Docker's
// config format. Left unset it defaults to the user's own helm config, and the
// oras client behind it treats that file as the place to write credentials to
// as well: registry.Client.Login ends by storing the credential in whatever
// store that file names.
//
// That made authentication depend on machine state we do not own. A file
// carrying "credsStore": "osxkeychain" sends the write to a credential helper
// binary, and the store is used without checking that the binary exists, so
// the same registry and the same Secret succeeded from a developer's shell and
// failed in the controller, which has a different PATH. Under a read-only root
// filesystem the write fails outright, taking every OCI fetch with it.
//
// So we write the credential we already hold into a file of our own and hand
// that to the client. Because the file carries an auths entry, the Docker
// config machinery never reaches its "detect the platform credential helper"
// path, and nothing is written back: Login is not called at all, since oras
// resolves credentials for pull and push by reading these files.

// ociCredentialsDir is the per-process directory holding those files. It is
// created on first use, so a process that never touches an authenticated
// registry creates nothing.
var (
	ociCredentialsDirOnce sync.Once
	ociCredentialsDirPath string
	ociCredentialsDirErr  error
)

func ociCredentialsDir() (string, error) {
	ociCredentialsDirOnce.Do(func() {
		ociCredentialsDirPath, ociCredentialsDirErr = os.MkdirTemp("", "vela-oci-creds-")
	})
	return ociCredentialsDirPath, ociCredentialsDirErr
}

// writeOCICredentialsFile writes a Docker-format credentials file holding one
// host's credential and returns its path. The caller removes it when the
// client it belongs to leaves the cache.
func writeOCICredentialsFile(cacheKey, host, username, password string) (string, error) {
	dir, err := ociCredentialsDir()
	if err != nil {
		return "", fmt.Errorf("create the OCI credentials directory: %w", err)
	}

	entry := map[string]string{}
	if username == "" {
		// Matches how oras treats a credential with no username: the secret is
		// an identity token rather than a password. Encoding it as an auth
		// string instead would be rejected, since decoding one requires a
		// non-empty user.
		entry["identitytoken"] = password
	} else {
		entry["auth"] = base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	}

	doc := map[string]interface{}{
		"auths": map[string]interface{}{ociAuthHostname(host): entry},
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("encode the OCI credentials file: %w", err)
	}

	sum := sha256.Sum256([]byte(cacheKey))
	path := filepath.Join(dir, fmt.Sprintf("%x.json", sum[:16]))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("write the OCI credentials file: %w", err)
	}
	return path, nil
}

// removeOCICredentialsFile deletes a credentials file, ignoring a path that is
// already gone. A failure here strands a small file in a temp directory and is
// not worth failing a registry operation over.
func removeOCICredentialsFile(path string) {
	if path == "" {
		return
	}
	_ = os.Remove(path)
}

// ociAuthHostname is the key a credential is stored under, mirroring the
// hostname resolution oras applies before looking one up. Docker Hub is
// addressed by several names and canonicalised to the v1 index URL; every
// other registry is keyed by its own host. Diverging here would write a
// credential the lookup cannot find, which reads as an anonymous pull.
func ociAuthHostname(host string) string {
	switch host {
	case "index.docker.io", "docker.io", "registry-1.docker.io":
		return "https://index.docker.io/v1/"
	}
	return host
}
