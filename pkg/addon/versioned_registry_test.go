/*
Copyright 2021 The KubeVela Authors.

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
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/repo"

	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/pkg/utils/helm"
)

func TestChooseAddonVersion(t *testing.T) {
	versions := []*repo.ChartVersion{
		{
			Metadata: &chart.Metadata{
				Version: "v1.4.0-beta1",
			},
		},
		{
			Metadata: &chart.Metadata{
				Version: "v1.3.6",
			},
		},
		{
			Metadata: &chart.Metadata{
				Version: "v1.2.0",
			},
		},
	}
	targetVersion, availableVersion := chooseVersion("v1.2.0", versions)
	assert.Equal(t, availableVersion, []string{"v1.4.0-beta1", "v1.3.6", "v1.2.0"})
	assert.Equal(t, targetVersion.Version, "v1.2.0")

	targetVersion, availableVersion = chooseVersion("", versions)
	assert.Equal(t, availableVersion, []string{"v1.4.0-beta1", "v1.3.6", "v1.2.0"})
	assert.Equal(t, targetVersion.Version, "v1.3.6")
}

var versionedHandler http.HandlerFunc = func(writer http.ResponseWriter, request *http.Request) {
	switch {
	case strings.Contains(request.URL.Path, "index.yaml"):
		files, err := os.ReadFile("./testdata/helm-repo/index.yaml")
		if err != nil {
			_, _ = writer.Write([]byte(err.Error()))
		}
		writer.Write(files)
	case strings.Contains(request.URL.Path, "fluxcd-1.0.0.tgz"):
		files, err := os.ReadFile("./testdata/helm-repo/fluxcd-1.0.0.tgz")
		if err != nil {
			_, _ = writer.Write([]byte(err.Error()))
		}
		writer.Write(files)
	}
}

var basicAuthVersionedHandler http.HandlerFunc = func(writer http.ResponseWriter, request *http.Request) {
	authHeader := request.Header.Get("Authorization")
	if len(authHeader) != 0 {
		auth := strings.SplitN(authHeader, " ", 2)
		bs, err := base64.StdEncoding.DecodeString(auth[1])
		pairs := strings.SplitN(string(bs), ":", 2)
		// mock auth, just for test
		if pairs[0] != pairs[1] {
			_, _ = writer.Write([]byte(err.Error()))
		}

	}
	switch {
	case strings.Contains(request.URL.Path, "index.yaml"):
		files, err := os.ReadFile("./testdata/basicauth-helm-repo/index.yaml")
		if err != nil {
			_, _ = writer.Write([]byte(err.Error()))
		}
		writer.Write(files)
	case strings.Contains(request.URL.Path, "fluxcd-1.0.0.tgz"):
		files, err := os.ReadFile("./testdata/basicauth-helm-repo/fluxcd-1.0.0.tgz")
		if err != nil {
			_, _ = writer.Write([]byte(err.Error()))
		}
		writer.Write(files)
	}
}

var multiVersionHandler http.HandlerFunc = func(writer http.ResponseWriter, request *http.Request) {
	switch {
	case strings.Contains(request.URL.Path, "index.yaml"):
		files, err := os.ReadFile("./testdata/multiversion-helm-repo/index.yaml")
		if err != nil {
			_, _ = writer.Write([]byte(err.Error()))
		}
		writer.Write(files)
	case strings.Contains(request.URL.Path, "fluxcd-1.0.0.tgz"):
		files, err := os.ReadFile("./testdata/multiversion-helm-repo/fluxcd-1.0.0.tgz")
		if err != nil {
			_, _ = writer.Write([]byte(err.Error()))
		}
		writer.Write(files)
	case strings.Contains(request.URL.Path, "fluxcd-2.0.0.tgz"):
		files, err := os.ReadFile("./testdata/multiversion-helm-repo/fluxcd-2.0.0.tgz")
		if err != nil {
			_, _ = writer.Write([]byte(err.Error()))
		}
		writer.Write(files)
	}
}

func TestLoadSystemRequirements(t *testing.T) {
	req := LoadSystemRequirements(map[string]string{velaSystemRequirement: ">=1.3.0", kubernetesSystemRequirement: ">=1.10.0"})
	assert.Equal(t, req.VelaVersion, ">=1.3.0")
	assert.Equal(t, req.KubernetesVersion, ">=1.10.0")

	req = LoadSystemRequirements(nil)
	assert.Empty(t, req)

	req = LoadSystemRequirements(map[string]string{kubernetesSystemRequirement: ">=1.10.0"})
	assert.Equal(t, req.KubernetesVersion, ">=1.10.0")

	req = LoadSystemRequirements(map[string]string{velaSystemRequirement: ">=1.4.0"})
	assert.Equal(t, req.VelaVersion, ">=1.4.0")
}

func TestLoadAddonVersions(t *testing.T) {
	server := httptest.NewServer(multiVersionHandler)
	defer server.Close()
	mr := &versionedRegistry{
		name: "multiversion-helm-repo",
		url:  server.URL,
		h:    helm.NewHelperWithCache(),
		Opts: nil,
	}
	versions, err := mr.loadAddonVersions("not-exist")
	assert.Error(t, err)
	assert.Equal(t, err, ErrNotExist)
	assert.Equal(t, len(versions), 0)

	mr = &versionedRegistry{
		name: "multiversion-helm-repo",
		url:  server.URL,
		h:    helm.NewHelperWithCache(),
		Opts: nil,
	}
	versions, err = mr.loadAddonVersions("not-exist")
	assert.Error(t, err)
	assert.Equal(t, len(versions), 0)
}
