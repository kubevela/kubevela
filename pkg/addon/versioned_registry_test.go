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
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/oam-dev/kubevela/pkg/utils/helm"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/repo"

	"github.com/oam-dev/kubevela/pkg/utils/common"

	"github.com/stretchr/testify/assert"
)

func TestVersionRegistry(t *testing.T) {
	go func() {
		http.HandleFunc("/", versionedHandler)
		http.HandleFunc("/authReg", basicAuthVersionedHandler)
		http.HandleFunc("/multi/", multiVersionHandler)
		err := http.ListenAndServe(fmt.Sprintf(":%d", 18083), nil)
		if err != nil {
			log.Fatal("Setup server error:", err)
		}
	}()

	// wait server setup
	time.Sleep(3 * time.Second)
	r := BuildVersionedRegistry("helm-repo", "http://127.0.0.1:18083", nil)
	addons, err := r.ListAddon()
	assert.NoError(t, err)
	assert.Equal(t, len(addons), 1)
	assert.Equal(t, addons[0].Name, "fluxcd")
	assert.Equal(t, len(addons[0].AvailableVersions), 1)

	addonUIData, err := r.GetAddonUIData(context.Background(), "fluxcd", "1.0.0")
	assert.NoError(t, err)
	assert.NotEmpty(t, addonUIData.Definitions)
	assert.NotEmpty(t, addonUIData.Icon)

	addonsInstallPackage, err := r.GetAddonInstallPackage(context.Background(), "fluxcd", "1.0.0")
	assert.NoError(t, err)
	assert.NotEmpty(t, addonsInstallPackage)
	assert.NotEmpty(t, addonsInstallPackage.YAMLTemplates)
	assert.NotEmpty(t, addonsInstallPackage.DefSchemas)

	addonWholePackage, err := r.GetDetailedAddon(context.Background(), "fluxcd", "1.0.0")
	assert.NoError(t, err)
	assert.NotEmpty(t, addonWholePackage)
	assert.NotEmpty(t, addonWholePackage.YAMLTemplates)
	assert.NotEmpty(t, addonWholePackage.DefSchemas)
	assert.NotEmpty(t, addonWholePackage.RegistryName)

	ar := BuildVersionedRegistry("auth-helm-repo", "http://127.0.0.1:18083/authReg", &common.HTTPOption{Username: "hello", Password: "hello"})
	addons, err = ar.ListAddon()
	assert.NoError(t, err)
	assert.Equal(t, len(addons), 1)
	assert.Equal(t, addons[0].Name, "fluxcd")
	assert.Equal(t, len(addons[0].AvailableVersions), 1)

	addonUIData, err = ar.GetAddonUIData(context.Background(), "fluxcd", "1.0.0")
	assert.NoError(t, err)
	assert.NotEmpty(t, addonUIData.Definitions)
	assert.NotEmpty(t, addonUIData.Icon)

	addonsInstallPackage, err = ar.GetAddonInstallPackage(context.Background(), "fluxcd", "1.0.0")
	assert.NoError(t, err)
	assert.NotEmpty(t, addonsInstallPackage)
	assert.NotEmpty(t, addonsInstallPackage.YAMLTemplates)
	assert.NotEmpty(t, addonsInstallPackage.DefSchemas)

	addonWholePackage, err = ar.GetDetailedAddon(context.Background(), "fluxcd", "1.0.0")
	assert.NoError(t, err)
	assert.NotEmpty(t, addonWholePackage)
	assert.NotEmpty(t, addonWholePackage.YAMLTemplates)
	assert.NotEmpty(t, addonWholePackage.DefSchemas)
	assert.NotEmpty(t, addonWholePackage.RegistryName)

	testListUIData(t)
	mr := BuildVersionedRegistry("multiversion-helm-repo", "http://127.0.0.1:18083/multi", nil)
	addons, err = mr.ListAddon()
	assert.NoError(t, err)
	assert.Equal(t, len(addons), 1)
	assert.Equal(t, addons[0].Name, "fluxcd")
	assert.Equal(t, len(addons[0].AvailableVersions), 2)

	addonUIData, err = mr.GetAddonUIData(context.Background(), "fluxcd", "2.0.0")
	assert.NoError(t, err)
	assert.NotEmpty(t, addonUIData.Definitions)
	assert.NotEmpty(t, addonUIData.Icon)
	assert.Equal(t, addonUIData.Version, "2.0.0")

	addonsInstallPackage, err = mr.GetAddonInstallPackage(context.Background(), "fluxcd", "1.0.0")
	assert.NoError(t, err)
	assert.NotEmpty(t, addonsInstallPackage)
	assert.NotEmpty(t, addonsInstallPackage.YAMLTemplates)
	assert.NotEmpty(t, addonsInstallPackage.DefSchemas)
	assert.NotEmpty(t, addonsInstallPackage.SystemRequirements.VelaVersion, "1.3.0")
	assert.NotEmpty(t, addonsInstallPackage.SystemRequirements.KubernetesVersion, "1.10.0")

	addonWholePackage, err = mr.GetDetailedAddon(context.Background(), "fluxcd", "1.0.0")
	assert.NoError(t, err)
	assert.NotEmpty(t, addonWholePackage)
	assert.NotEmpty(t, addonWholePackage.YAMLTemplates)
	assert.NotEmpty(t, addonWholePackage.DefSchemas)
	assert.NotEmpty(t, addonWholePackage.RegistryName)
	assert.Equal(t, addonWholePackage.RegistryName, "multiversion-helm-repo")

	version, err := mr.GetAddonAvailableVersion("fluxcd")
	assert.NoError(t, err)
	assert.Equal(t, len(version), 2)
	assert.Equal(t, addonWholePackage.SystemRequirements.VelaVersion, ">=1.3.0")
	assert.Equal(t, addonWholePackage.SystemRequirements.KubernetesVersion, ">=1.10.0")

}

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
		files, err := ioutil.ReadFile("./testdata/helm-repo/index.yaml")
		if err != nil {
			_, _ = writer.Write([]byte(err.Error()))
		}
		writer.Write(files)
	case strings.Contains(request.URL.Path, "fluxcd-1.0.0.tgz"):
		files, err := ioutil.ReadFile("./testdata/helm-repo/fluxcd-1.0.0.tgz")
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
		files, err := ioutil.ReadFile("./testdata/basicauth-helm-repo/index.yaml")
		if err != nil {
			_, _ = writer.Write([]byte(err.Error()))
		}
		writer.Write(files)
	case strings.Contains(request.URL.Path, "fluxcd-1.0.0.tgz"):
		files, err := ioutil.ReadFile("./testdata/basicauth-helm-repo/fluxcd-1.0.0.tgz")
		if err != nil {
			_, _ = writer.Write([]byte(err.Error()))
		}
		writer.Write(files)
	}
}

var multiVersionHandler http.HandlerFunc = func(writer http.ResponseWriter, request *http.Request) {
	switch {
	case strings.Contains(request.URL.Path, "index.yaml"):
		files, err := ioutil.ReadFile("./testdata/multiversion-helm-repo/index.yaml")
		if err != nil {
			_, _ = writer.Write([]byte(err.Error()))
		}
		writer.Write(files)
	case strings.Contains(request.URL.Path, "fluxcd-1.0.0.tgz"):
		files, err := ioutil.ReadFile("./testdata/multiversion-helm-repo/fluxcd-1.0.0.tgz")
		if err != nil {
			_, _ = writer.Write([]byte(err.Error()))
		}
		writer.Write(files)
	case strings.Contains(request.URL.Path, "fluxcd-2.0.0.tgz"):
		files, err := ioutil.ReadFile("./testdata/multiversion-helm-repo/fluxcd-2.0.0.tgz")
		if err != nil {
			_, _ = writer.Write([]byte(err.Error()))
		}
		writer.Write(files)
	}
}

func TestLoadSystemRequirements(t *testing.T) {
	req := LoadSystemRequirements("vela>=1.3.0; kubernetes>=1.10.0")
	assert.Equal(t, req.VelaVersion, ">=1.3.0")
	assert.Equal(t, req.KubernetesVersion, ">=1.10.0")

	req = LoadSystemRequirements("")
	assert.Empty(t, req)

	req = LoadSystemRequirements("&&&%%")
	assert.Empty(t, req)

	req = LoadSystemRequirements("vela>=; kubernetes>=1.10.0")
	assert.Empty(t, req)
}

func TestLoadAddonVersions(t *testing.T) {
	go func() {
		http.HandleFunc("/multi/", multiVersionHandler)
		err := http.ListenAndServe(fmt.Sprintf(":%d", 18083), nil)
		if err != nil {
			log.Fatal("Setup server error:", err)
		}
	}()
	mr := &versionedRegistry{
		name: "multiversion-helm-repo",
		url:  "http://127.0.0.1:18083/multi",
		h:    helm.NewHelperWithCache(),
		Opts: nil,
	}
	versions, err := mr.loadAddonVersions("not-exist")
	assert.Error(t, err)
	assert.Equal(t, err, ErrNotExist)
	assert.Equal(t, len(versions), 0)

	mr = &versionedRegistry{
		name: "multiversion-helm-repo",
		url:  "http://127.0.0.1:18083/fail",
		h:    helm.NewHelperWithCache(),
		Opts: nil,
	}
	versions, err = mr.loadAddonVersions("not-exist")
	assert.Error(t, err)
	assert.Equal(t, len(versions), 0)
}
