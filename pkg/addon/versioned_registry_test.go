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
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/repo"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/utils/common"
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
	avs := []string{"v1.4.0-beta1", "v1.3.6", "v1.2.0"}
	for _, tc := range []struct {
		name             string
		specifiedVersion string
		wantVersion      string
		wantAVersions    []string
	}{
		{
			name:             "choose specified",
			specifiedVersion: "v1.2.0",
			wantVersion:      "v1.2.0",
			wantAVersions:    avs,
		},
		{
			name:             "choose specified, ignore v prefix",
			specifiedVersion: "1.2.0",
			wantVersion:      "v1.2.0",
			wantAVersions:    avs,
		},
		{
			name:             "not specifying version, choose non-prerelease && highest version",
			specifiedVersion: "",
			wantVersion:      "v1.3.6",
			wantAVersions:    avs,
		},
	} {
		targetVersion, availableVersion := chooseVersion(tc.specifiedVersion, versions)
		assert.Equal(t, availableVersion, tc.wantAVersions)
		assert.Equal(t, targetVersion.Version, tc.wantVersion)
	}
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

func TestToVersionedRegistry(t *testing.T) {
	registry := Registry{
		Name: "helm-based-registry",
		Helm: &HelmSource{
			URL:      "http://repo.example",
			Username: "example-user",
			Password: "example-password",
		},
	}

	// Test case 1: convert a helm-based registry
	actual, err := ToVersionedRegistry(registry)

	assert.NoError(t, err)
	expected := &versionedRegistry{
		name: registry.Name,
		url:  registry.Helm.URL,
		h:    helm.NewHelperWithCache(),
		Opts: &common.HTTPOption{
			Username: registry.Helm.Username,
			Password: registry.Helm.Password,
		},
	}
	assert.Equal(t, expected, actual)

	// Test case 2: when converting a git-based registry, return error
	registry = Registry{
		Name: "git-based-registry",
		Git: &GitAddonSource{
			URL: "http://repo.example",
		},
	}
	actual, err = ToVersionedRegistry(registry)
	assert.EqualError(t, err, "registry 'git-based-registry' is not a versioned registry")
	assert.Nil(t, actual)
}

func TestResolveAddonListFromIndex(t *testing.T) {
	r := &versionedRegistry{name: "test-repo"}
	indexFile := &repo.IndexFile{
		Entries: map[string]repo.ChartVersions{
			"addon-good": {
				{Metadata: &chart.Metadata{Name: "addon-good", Version: "1.0.0", Description: "old desc", Icon: "old_icon", Keywords: []string{"tag1"}}},
				{Metadata: &chart.Metadata{Name: "addon-good", Version: "1.2.0", Description: "latest desc", Icon: "latest_icon", Keywords: []string{"tag2"}}},
				{Metadata: &chart.Metadata{Name: "addon-good", Version: "1.1.0", Description: "middle desc", Icon: "middle_icon", Keywords: []string{"tag3"}}},
			},
			"addon-empty": {},
			"addon-single": {
				{Metadata: &chart.Metadata{Name: "addon-single", Version: "0.1.0", Description: "single desc"}},
			},
		},
	}

	result := r.resolveAddonListFromIndex(r.name, indexFile)

	assert.Equal(t, 2, len(result))

	var addonGood, addonSingle *UIData
	for _, addon := range result {
		if addon.Name == "addon-good" {
			addonGood = addon
		}
		if addon.Name == "addon-single" {
			addonSingle = addon
		}
	}

	require.NotNil(t, addonGood)
	assert.Equal(t, "addon-good", addonGood.Name)
	assert.Equal(t, "test-repo", addonGood.RegistryName)
	assert.Equal(t, "1.2.0", addonGood.Version)
	assert.Equal(t, "latest desc", addonGood.Description)
	assert.Equal(t, "latest_icon", addonGood.Icon)
	assert.Equal(t, []string{"tag2"}, addonGood.Tags)
	assert.Equal(t, []string{"1.2.0", "1.1.0", "1.0.0"}, addonGood.AvailableVersions)

	require.NotNil(t, addonSingle)
	assert.Equal(t, "addon-single", addonSingle.Name)
	assert.Equal(t, "0.1.0", addonSingle.Version)
	assert.Equal(t, []string{"0.1.0"}, addonSingle.AvailableVersions)
}

// setupAddonTestServer creates a mock HTTP server for testing addon loading.
// It can simulate success, 404 errors, or serving corrupt data based on the handlerType.
func setupAddonTestServer(t *testing.T, handlerType string) string {
	var server *httptest.Server
	// This handler rewrites URLs in the index file to point to the server it's running on.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "index.yaml") {
			content, err := os.ReadFile("./testdata/multiversion-helm-repo/index.yaml")
			assert.NoError(t, err)
			newContent := strings.ReplaceAll(string(content), "http://127.0.0.1:18083/multi", server.URL)
			_, err = w.Write([]byte(newContent))
			assert.NoError(t, err)
			return
		}

		// After serving the index, the next request depends on the handler type.
		switch handlerType {
		case "success":
			multiVersionHandler(w, r)
		case "notfound":
			w.WriteHeader(http.StatusNotFound)
		case "corrupt":
			_, err := w.Write([]byte("this is not a valid tgz file"))
			assert.NoError(t, err)
		default:
			t.Errorf("unknown handler type: %s", handlerType)
		}
	})
	server = httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server.URL
}

func TestLoadAddon(t *testing.T) {
	testCases := []struct {
		name           string
		handlerType    string
		addonName      string
		addonVersion   string
		expectErr      bool
		expectedErrStr string
		checkFunc      func(t *testing.T, pkg *WholeAddonPackage)
	}{
		{
			name:         "Success case",
			handlerType:  "success",
			addonName:    "fluxcd",
			addonVersion: "1.0.0",
			expectErr:    false,
			checkFunc: func(t *testing.T, pkg *WholeAddonPackage) {
				assert.NotNil(t, pkg)
				assert.Equal(t, "fluxcd", pkg.Name)
				assert.Equal(t, "1.0.0", pkg.Version)
				assert.NotEmpty(t, pkg.YAMLTemplates)
				assert.Equal(t, []string{"2.0.0", "1.0.0"}, pkg.AvailableVersions)
			},
		},
		{
			name:           "Version not found",
			handlerType:    "success",
			addonName:      "fluxcd",
			addonVersion:   "3.0.0",
			expectErr:      true,
			expectedErrStr: "specified version 3.0.0 for addon fluxcd not exist",
		},
		{
			name:           "Chart download fails",
			handlerType:    "notfound",
			addonName:      "fluxcd",
			addonVersion:   "1.0.0",
			expectErr:      true,
			expectedErrStr: ErrFetch.Error(),
		},
		{
			name:           "Corrupt chart file",
			handlerType:    "corrupt",
			addonName:      "fluxcd",
			addonVersion:   "1.0.0",
			expectErr:      true,
			expectedErrStr: ErrFetch.Error(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			serverURL := setupAddonTestServer(t, tc.handlerType)
			reg := &versionedRegistry{
				name: "test-registry",
				url:  serverURL,
				h:    helm.NewHelperWithCache(),
				Opts: nil,
			}

			pkg, err := reg.loadAddon(context.Background(), tc.addonName, tc.addonVersion)

			if tc.expectErr {
				assert.Error(t, err)
				if tc.expectedErrStr != "" {
					assert.Contains(t, err.Error(), tc.expectedErrStr)
				}
			} else {
				assert.NoError(t, err)
			}

			if tc.checkFunc != nil {
				tc.checkFunc(t, pkg)
			}
		})
	}
}
