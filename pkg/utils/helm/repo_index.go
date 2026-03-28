/*
Copyright 2023 The KubeVela Authors.

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

package helm

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"helm.sh/helm/v3/pkg/getter"
	helmrepo "helm.sh/helm/v3/pkg/repo"
	gossh "golang.org/x/crypto/ssh"
	"sigs.k8s.io/yaml"
)

// IndexYaml is the index.yaml of helm repo
const IndexYaml = "index.yaml"

// LoadRepoIndex load helm repo index
func LoadRepoIndex(_ context.Context, u string, cred *RepoCredential) (*helmrepo.IndexFile, error) {

	if !strings.HasSuffix(u, "/") {
		u = fmt.Sprintf("%s/%s", u, IndexYaml)
	} else {
		u = fmt.Sprintf("%s%s", u, IndexYaml)
	}

	resp, err := loadData(u, cred)
	if err != nil {
		return nil, err
	}

	indexFile, err := loadIndex(resp.Bytes())
	if err != nil {
		return nil, err
	}

	return indexFile, nil
}

func loadData(u string, cred *RepoCredential) (*bytes.Buffer, error) {
	// Handle SSH-based URLs via git clone
	if IsSSHURL(u) {
		return loadDataViaSSH(u, cred)
	}

	parsedURL, err := url.Parse(u)
	if err != nil {
		return nil, err
	}
	var resp *bytes.Buffer

	skipTLS := true
	if cred.InsecureSkipTLSVerify != nil && !*cred.InsecureSkipTLSVerify {
		skipTLS = false
	}

	indexURL := parsedURL.String()
	// TODO add user-agent
	g, _ := getter.NewHTTPGetter()
	resp, err = g.Get(indexURL,
		getter.WithTimeout(5*time.Minute),
		getter.WithURL(u),
		getter.WithInsecureSkipVerifyTLS(skipTLS),
		getter.WithTLSClientConfig(cred.CertFile, cred.KeyFile, cred.CAFile),
		getter.WithBasicAuth(cred.Username, cred.Password),
	)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// loadDataViaSSH clones a git repository using SSH authentication and reads index.yaml
func loadDataViaSSH(u string, cred *RepoCredential) (*bytes.Buffer, error) {
	// Strip the trailing /index.yaml if present (it was appended by LoadRepoIndex)
	repoURL := strings.TrimSuffix(u, "/"+IndexYaml)
	repoURL = strings.TrimSuffix(repoURL, IndexYaml)

	// Normalize git+ssh:// to ssh://
	if strings.HasPrefix(repoURL, "git+ssh://") {
		repoURL = strings.Replace(repoURL, "git+ssh://", "ssh://", 1)
	}

	cloneOpts := &git.CloneOptions{
		URL:   repoURL,
		Depth: 1,
	}

	if len(cred.SSHPrivateKey) > 0 {
		publicKeys, err := ssh.NewPublicKeys("git", []byte(cred.SSHPrivateKey), "")
		if err != nil {
			return nil, fmt.Errorf("failed to create SSH public keys from private key: %w", err)
		}
		if len(cred.KnownHosts) > 0 {
			hostKeyCallback, err := parseKnownHosts(cred.KnownHosts)
			if err != nil {
				return nil, fmt.Errorf("failed to parse known_hosts: %w", err)
			}
			publicKeys.HostKeyCallback = hostKeyCallback
		} else {
			// nolint:gosec
			publicKeys.HostKeyCallback = gossh.InsecureIgnoreHostKey()
		}
		cloneOpts.Auth = publicKeys
	}

	tmpDir, err := os.MkdirTemp("", "helm-ssh-repo-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	_, err = git.PlainClone(tmpDir, false, cloneOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to clone helm repository via SSH from %s: %w", repoURL, err)
	}

	indexPath := filepath.Join(tmpDir, IndexYaml)
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s from cloned repository: %w", IndexYaml, err)
	}

	return bytes.NewBuffer(data), nil
}

// parseKnownHosts parses known_hosts content and returns an ssh.HostKeyCallback
func parseKnownHosts(knownHostsData string) (gossh.HostKeyCallback, error) {
	tmpFile, err := os.CreateTemp("", "known_hosts-")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(knownHostsData); err != nil {
		tmpFile.Close()
		return nil, err
	}
	tmpFile.Close()

	callback, err := ssh.NewKnownHostsCallback(tmpFile.Name())
	if err != nil {
		return nil, err
	}
	return callback, nil
}

// loadIndex loads an index file and does minimal validity checking.
//
// This will fail if API Version is not set (ErrNoAPIVersion) or if the unmarshal fails.
func loadIndex(data []byte) (*helmrepo.IndexFile, error) {
	i := &helmrepo.IndexFile{}
	if err := yaml.Unmarshal(data, i); err != nil {
		return i, err
	}
	i.SortEntries()
	if i.APIVersion == "" {
		return i, helmrepo.ErrNoAPIVersion
	}
	return i, nil
}
