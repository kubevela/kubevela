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
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestVersionRegistry(t *testing.T) {
	go func() {
		http.HandleFunc("/", versionedHandler)
		err := http.ListenAndServe(fmt.Sprintf(":%d", 18083), nil)
		if err != nil {
			log.Fatal("Setup server error:", err)
		}
	}()
	// wait server setup
	time.Sleep(3 * time.Second)
	r := BuildVersionedRegistry("helm-repo", "http://127.0.0.1:18083")
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
