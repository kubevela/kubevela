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

	"github.com/oam-dev/kubevela/pkg/utils/common"

	"github.com/stretchr/testify/assert"
)

func TestVersionRegistry(t *testing.T) {
	go func() {
		http.HandleFunc("/", versionedHandler)
		http.HandleFunc("/authReg", basicAuthVersionedHandler)
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
