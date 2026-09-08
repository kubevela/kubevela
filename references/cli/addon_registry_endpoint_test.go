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
	"testing"

	"github.com/stretchr/testify/assert"

	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
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
