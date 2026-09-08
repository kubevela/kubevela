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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/types"
	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	pkgmodule "github.com/oam-dev/kubevela/pkg/module"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

// publishFixtureDir writes the smallest module a publish can package into a
// fresh temp directory and returns its path. These tests only need a tree that
// parses and carries a known name and version, so building it here keeps them
// independent of any checked-in fixture -- the module testdata now lives under
// test/e2e-test, for the tests that talk to a real registry.
func publishFixtureDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"_module.cue":     "module:  \"s3\"\nversion: \"1.0.0\"\n",
		"v1/_version.cue": "apiVersion: \"v1\"\n",
		"v1/definitions/bucket.yaml": `apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: bucket
  namespace: vela-system
spec:
  workload:
    definition:
      apiVersion: v1
      kind: ConfigMap
`,
	}
	for rel, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o750))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o600))
	}
	return dir
}

// recordedPush captures what run() would have pushed.
type recordedPush struct {
	calls   int
	reg     pkgaddon.Registry
	name    string
	version string
	archive []byte
}

func (r *recordedPush) push(_ context.Context, reg pkgaddon.Registry, name, version string, archive []byte) error {
	r.calls++
	r.reg, r.name, r.version, r.archive = reg, name, version, archive
	return nil
}

// moduleRegistryClient returns a fake client holding the module registry
// ConfigMap with the given entries, in the format the store unmarshals:
// a map keyed by registry name.
func moduleRegistryClient(t *testing.T, entries map[string]pkgaddon.Registry) client.Client {
	t.Helper()
	data, err := json.Marshal(entries)
	require.NoError(t, err)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: pkgmodule.ModuleRegistryConfigMap, Namespace: types.DefaultKubeVelaNS},
		Data:       map[string]string{"registries": string(data)},
	}
	return fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(cm).Build()
}

func TestModulePublishFailsBeforePush(t *testing.T) {
	// invalidTreeDir contains an _module.cue that parses but fails
	// validation (a non-semver version), so the case below fails through the
	// parser's validation gate rather than through a missing directory.
	invalidTreeDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(invalidTreeDir, "_module.cue"),
		[]byte("module:  \"bad\"\nversion: \"nope\"\n"),
		0o600,
	))

	cases := []struct {
		name    string
		options func(t *testing.T, rec *recordedPush) *modulePublishOptions
		wantErr string
	}{
		{
			name: "invalid module tree",
			options: func(_ *testing.T, rec *recordedPush) *modulePublishOptions {
				return &modulePublishOptions{dir: invalidTreeDir, ociRef: "oci://registry.example.com/modules", push: rec.push}
			},
			wantErr: "not a valid semver",
		},
		{
			name: "git registry target",
			options: func(t *testing.T, rec *recordedPush) *modulePublishOptions {
				return &modulePublishOptions{dir: publishFixtureDir(t), registry: "catalog", push: rec.push}
			},
			wantErr: "supports OCI/ECR only",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recordedPush{}
			cli := moduleRegistryClient(t, map[string]pkgaddon.Registry{
				"catalog": {Name: "catalog", Git: &pkgaddon.GitAddonSource{URL: "https://github.com/org/repo", Path: "module"}},
			})
			err := tc.options(t, rec).run(context.Background(), cli, &bytes.Buffer{})
			require.ErrorContains(t, err, tc.wantErr)
			require.Zero(t, rec.calls)
		})
	}
}

func TestModulePublishRequiresClusterForNamedRegistry(t *testing.T) {
	o := &modulePublishOptions{dir: publishFixtureDir(t), registry: "ecr"}
	err := o.run(context.Background(), nil, &bytes.Buffer{})
	require.ErrorContains(t, err, "cluster")
}

func TestModulePublishAlreadyPublishedWithoutForce(t *testing.T) {
	rec := &recordedPush{}
	o := &modulePublishOptions{
		dir:    publishFixtureDir(t),
		ociRef: "oci://registry.example.com/modules",
		push:   rec.push,
		tagExists: func(_ context.Context, _ pkgaddon.Registry, _, _ string) (bool, error) {
			return true, nil
		},
	}

	err := o.run(context.Background(), nil, &bytes.Buffer{})
	require.ErrorContains(t, err, "registry.example.com/modules/s3:1.0.0")
	require.ErrorContains(t, err, "_module.cue")
	require.Zero(t, rec.calls)
}

// errRepositoryNotFoundTest mimics ECR's rejection of a push to a repository
// that does not yet exist, so IsOCIRepositoryNotFound recognizes it.
var errRepositoryNotFoundTest = errors.New("RepositoryNotFoundException: the repository with name 'modules/s3' does not exist")

// errTagImmutableTest mimics ECR's rejection of a tag move on a repository
// with IMMUTABLE tag mutability, so IsOCITagImmutable recognizes it.
var errTagImmutableTest = errors.New("ImageTagAlreadyExistsException: tag 1.0.0 is immutable")

func TestModulePublishRepositoryNotFoundError(t *testing.T) {
	o := &modulePublishOptions{
		dir:    publishFixtureDir(t),
		ociRef: "oci://registry.example.com/modules",
		push: func(context.Context, pkgaddon.Registry, string, string, []byte) error {
			return errRepositoryNotFoundTest
		},
		tagExists: func(_ context.Context, _ pkgaddon.Registry, _, _ string) (bool, error) {
			return false, nil
		},
	}

	err := o.run(context.Background(), nil, &bytes.Buffer{})
	require.ErrorContains(t, err, "registry.example.com/modules/s3:1.0.0")
	require.ErrorContains(t, err, "does not exist")
	require.ErrorContains(t, err, "create it")
	require.True(t, errors.Is(err, errRepositoryNotFoundTest), "wrapped error must survive as the %%w chain")
}

func TestModulePublishTagImmutableError(t *testing.T) {
	o := &modulePublishOptions{
		dir:    publishFixtureDir(t),
		ociRef: "oci://registry.example.com/modules",
		push: func(context.Context, pkgaddon.Registry, string, string, []byte) error {
			return errTagImmutableTest
		},
		tagExists: func(_ context.Context, _ pkgaddon.Registry, _, _ string) (bool, error) {
			return false, nil
		},
	}

	err := o.run(context.Background(), nil, &bytes.Buffer{})
	require.ErrorContains(t, err, "registry.example.com/modules/s3:1.0.0")
	require.ErrorContains(t, err, "_module.cue")
	require.True(t, errors.Is(err, errTagImmutableTest), "wrapped error must survive as the %%w chain")
}

func TestModulePublishOverridesResolvedRegistryCredentials(t *testing.T) {
	rec := &recordedPush{}
	cli := moduleRegistryClient(t, map[string]pkgaddon.Registry{
		"ecr": {Name: "ecr", Helm: &pkgaddon.HelmSource{
			URL:      "oci://123456789012.dkr.ecr.us-west-2.amazonaws.com/modules",
			Username: "entry-user",
			Token:    "entry-token",
		}},
	})
	o := &modulePublishOptions{
		dir:      publishFixtureDir(t),
		registry: "ecr",
		username: "flag-user",
		password: "flag-password",
		push:     rec.push,
		tagExists: func(_ context.Context, _ pkgaddon.Registry, _, _ string) (bool, error) {
			return false, nil
		},
	}

	require.NoError(t, o.run(context.Background(), cli, &bytes.Buffer{}))
	require.Equal(t, "flag-user", rec.reg.Helm.Username)
	require.Equal(t, "flag-password", rec.reg.Helm.Token)
}

// TestModulePublishCommandDryRunReflectsFlags exercises RunE's flag-to-struct
// wiring through the real cobra command, rather than modulePublishOptions
// built directly. It uses a positional OCI reference with --dry-run so the
// path needs no cluster and pushes nothing, and asserts the printed target
// reflects --version, the one flag observable in dry-run output.
func TestModulePublishCommandDryRunReflectsFlags(t *testing.T) {
	cmd := NewModulePublishCommand(common.Args{}, cmdutil.NewDefaultIOStreams())
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{publishFixtureDir(t), "oci://registry.example.com/modules", "--dry-run", "--version", "9.9.9"})

	require.NoError(t, cmd.Execute())
	require.Contains(t, out.String(), "registry.example.com/modules/s3:9.9.9")
}

// TestModulePublishCommandRejectsRegistryFlagWithPositionalRef exercises the
// mutual-exclusion rejection through RunE rather than a direct
// modulePublishOptions construction, proving --registry is read from the
// real flag under its own name and reaches the options struct. This path
// needs no cluster and no network: the rejection in run happens before
// resolveTarget, so it is reachable even though --registry alone would
// otherwise require a client.
func TestModulePublishCommandRejectsRegistryFlagWithPositionalRef(t *testing.T) {
	cmd := NewModulePublishCommand(common.Args{}, cmdutil.NewDefaultIOStreams())
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{publishFixtureDir(t), "oci://registry.example.com/modules", "--registry", "ecr"})

	err := cmd.Execute()
	require.ErrorContains(t, err, "cannot be combined")
}
