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

package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gosuri/uitable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
)

// Only the Helm branch of `vela addon registry add` verified anything before
// storing, so a git registry with an endpoint the reader cannot use was accepted
// and only failed later, on `vela addon list`. See kubevela#7364.
func TestValidateReadableEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name     string
		registry pkgaddon.Registry
		wantErr  error
	}{
		{
			name:     "git registry with a git:// endpoint is refused",
			registry: pkgaddon.Registry{Name: "repro-git", Git: &pkgaddon.GitAddonSource{URL: "git://127.0.0.1:9418/poc.git", Path: "addons"}},
			wantErr:  pkgaddon.ErrUnsupportedGitEndpoint,
		},
		{
			name:     "git registry with an ssh:// endpoint is refused",
			registry: pkgaddon.Registry{Name: "repro-ssh", Git: &pkgaddon.GitAddonSource{URL: "ssh://git@github.com/kubevela/catalog", Path: "addons"}},
			wantErr:  pkgaddon.ErrUnsupportedGitEndpoint,
		},
		{
			name:     "gitee registry with a git:// endpoint is refused",
			registry: pkgaddon.Registry{Name: "repro-gitee", Gitee: &pkgaddon.GiteeAddonSource{URL: "git://127.0.0.1:9418/poc.git", Path: "addons"}},
			wantErr:  pkgaddon.ErrUnsupportedGiteeEndpoint,
		},
		{
			name:     "git registry with an oss:// endpoint is refused",
			registry: pkgaddon.Registry{Name: "repro-oss", Git: &pkgaddon.GitAddonSource{URL: "oss://oss-cn-hangzhou.aliyuncs.com/kubevela-addons", Path: "addons"}},
			wantErr:  pkgaddon.ErrUnsupportedGitEndpoint,
		},
		{
			name:     "a github endpoint is still accepted",
			registry: pkgaddon.Registry{Name: "catalog", Git: &pkgaddon.GitAddonSource{URL: "https://github.com/kubevela/catalog", Path: "addons"}},
		},
		{
			name:     "a gitee endpoint is still accepted",
			registry: pkgaddon.Registry{Name: "catalog", Gitee: &pkgaddon.GiteeAddonSource{URL: "https://gitee.com/kubevela/catalog", Path: "addons"}},
		},
		{
			// Building an OSS or gitlab reader dials out, so those are left to
			// the existing paths rather than checked here.
			name:     "an OSS registry is not checked here",
			registry: pkgaddon.Registry{Name: "oss", OSS: &pkgaddon.OSSAddonSource{Endpoint: "oss-cn-hangzhou.aliyuncs.com", Bucket: "kubevela-addons"}},
		},
		{
			name:     "a helm registry is not checked here",
			registry: pkgaddon.Registry{Name: "helm", Helm: &pkgaddon.HelmSource{URL: "http://localhost:9090"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateReadableEndpoint(tc.registry)
			if tc.wantErr == nil {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, tc.wantErr)
		})
	}
}

// An endpoint utils.Parse rejects outright (another HTTP host) is named in the
// error, so an operator can tell which endpoint to fix.
func TestValidateReadableEndpointNamesAParseFailure(t *testing.T) {
	const endpoint = "https://gitlab.example.com/kubevela/catalog"
	err := validateReadableEndpoint(pkgaddon.Registry{Name: "other-host", Git: &pkgaddon.GitAddonSource{URL: endpoint, Path: "addons"}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), endpoint)
}

// listAddons skips a registry it cannot read so that one bad registry does not
// hide the addons of the others. A git registry stored before add-time
// validation existed (or by hand in the ConfigMap) used to panic in readRepo
// instead, taking every other registry down with it. See kubevela#7364.
func TestListAddonsSkipsAnUnreadableGitRegistry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/index.yaml") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`apiVersion: v1
entries:
  fluxcd:
    - name: fluxcd
      description: Extended workload to do continuous and progressive delivery
      urls:
        - fluxcd-1.0.0.tgz
      version: 1.0.0
generated: "0001-01-01T00:00:00Z"
`))
	}))
	defer server.Close()

	ctx := context.Background()
	clt := fake.NewClientBuilder().WithScheme(common2.Scheme).Build()
	ds := pkgaddon.NewRegistryDataStore(clt)
	// Written through the data store, not `vela addon registry add`, so the
	// endpoint is stored unvalidated. Registries are listed by name, so the
	// broken one is read first.
	broken := pkgaddon.Registry{Name: "a-broken-git", Git: &pkgaddon.GitAddonSource{URL: "git://127.0.0.1:9418/poc.git", Path: "addons"}}
	healthy := pkgaddon.Registry{Name: "b-healthy-helm", Helm: &pkgaddon.HelmSource{URL: server.URL}}
	require.NoError(t, ds.AddRegistry(ctx, broken))
	require.NoError(t, ds.AddRegistry(ctx, healthy))

	t.Run("every registry is listed, the unreadable one is skipped", func(t *testing.T) {
		var table *uitable.Table
		var err error
		require.NotPanics(t, func() { table, err = listAddons(ctx, clt, "") })
		require.NoError(t, err)

		var registries []string
		for _, row := range table.Rows[1:] {
			assert.Equal(t, "fluxcd", row.Cells[0].Data)
			registries = append(registries, fmt.Sprint(row.Cells[1].Data))
		}
		assert.Equal(t, []string{healthy.Name}, registries)
	})

	t.Run("asking for the unreadable registry by name returns its error", func(t *testing.T) {
		var err error
		require.NotPanics(t, func() { _, err = listAddons(ctx, clt, broken.Name) })
		assert.ErrorIs(t, err, pkgaddon.ErrUnsupportedGitEndpoint)
	})
}
