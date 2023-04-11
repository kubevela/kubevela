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
	"strings"
	"time"

	"helm.sh/helm/v3/pkg/getter"
	helmrepo "helm.sh/helm/v3/pkg/repo"
	"sigs.k8s.io/yaml"
)

// IndexYaml is the index.yaml of helm repo
const IndexYaml = "index.yaml"

// LoadRepoIndex load helm repo index
func LoadRepoIndex(ctx context.Context, u string, cred *RepoCredential) (*helmrepo.IndexFile, error) {

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
