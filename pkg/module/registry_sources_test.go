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

package module

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/registry/component"
)

// failingGetStore fails GetRegistry with something other than not-found, as
// an RBAC denial or an unreachable apiserver would.
type failingGetStore struct {
	component.RegistryDataStore
	err error
}

func (f failingGetStore) GetRegistry(context.Context, string) (component.Registry, error) {
	return component.Registry{}, f.err
}

func TestResolveRegistryRefusesCredentialsOverPlainHTTP(t *testing.T) {
	for name, src := range map[string]*component.HelmSource{
		"username":   {URL: "http://127.0.0.1:30500/modules", Username: "AWS"},
		"token":      {URL: "http://127.0.0.1:30500/modules", Token: "t"},
		"secret ref": {URL: "HTTP://127.0.0.1:30500/modules", TokenSecretRef: "module-registry-local"},
	} {
		t.Run(name, func(t *testing.T) {
			store := moduleStoreWith(t, component.Registry{Name: "local", Helm: src})
			_, err := ResolveRegistry(context.Background(), store, "local")
			require.Error(t, err)
			assert.Contains(t, err.Error(), `module registry "local" uses http:// and carries credentials`)
		})
	}

	anonymous := moduleStoreWith(t, component.Registry{Name: "local", Helm: &component.HelmSource{URL: "http://127.0.0.1:30500/modules"}})
	reg, err := ResolveRegistry(context.Background(), anonymous, "local")
	require.NoError(t, err, "a plain-HTTP test registry without credentials is fine")
	assert.Equal(t, "local", reg.Name)
}

func TestResolveRegistryReportsStoreFailures(t *testing.T) {
	boom := errors.New("configmaps is forbidden")

	_, err := ResolveRegistry(context.Background(), failingGetStore{err: boom}, "catalog")
	assert.ErrorIs(t, err, boom, "a read failure is not a not-found")
	assert.NotErrorIs(t, err, ErrRegistryNotFound)

	_, err = ResolveRegistry(context.Background(), failingListStore{err: boom}, "")
	assert.ErrorIs(t, err, boom, "choosing a default needs the list, and its failure is reported as such")
}

func TestSourceTypeName(t *testing.T) {
	assert.Equal(t, "git", SourceTypeName(gitRegistry("gh")))
	assert.Equal(t, "gitee", SourceTypeName(component.Registry{Gitee: &component.GiteeAddonSource{URL: "https://gitee.com/o/r"}}))
	assert.Equal(t, "oci", SourceTypeName(ociRegistry("ecr")))
	assert.Equal(t, "oci", SourceTypeName(component.Registry{Helm: &component.HelmSource{URL: "http://127.0.0.1:30500/modules"}}), "a plain-HTTP registry is still an OCI one")
	assert.Equal(t, "helm", SourceTypeName(helmRegistry("museum")))
	assert.Equal(t, "oss", SourceTypeName(component.Registry{OSS: &component.OSSAddonSource{Endpoint: "oss.example.com", Bucket: "b"}}))
	assert.Equal(t, "unknown", SourceTypeName(component.Registry{Name: "blank"}))
}

func TestNotFoundErrorIsTheResolverWording(t *testing.T) {
	store := moduleStoreWith(t, ociRegistry("catalog"), ociRegistry("team"))
	err := NotFoundError(context.Background(), store, "ghost")
	assert.ErrorIs(t, err, ErrRegistryNotFound)
	assert.Equal(t, `module registry "ghost" not found; configured registries: catalog, team (`+ErrRegistryNotFound.Error()+")", err.Error(),
		"delete reports an unknown name exactly as ResolveRegistry does")
}
