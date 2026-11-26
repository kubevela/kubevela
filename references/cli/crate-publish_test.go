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
	pkgcrate "github.com/oam-dev/kubevela/pkg/crate"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

// publishFixtureDir writes the smallest crate a publish can package into a
// fresh temp directory and returns its path. These tests only need a tree that
// parses and carries a known name and version, so building it here keeps them
// independent of any checked-in fixture -- the crate testdata now lives under
// test/e2e-test, for the tests that talk to a real registry.
func publishFixtureDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"_crate.cue":      "crate:  \"s3\"\nversion: \"1.0.0\"\n",
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

// crateRegistryClient returns a fake client holding the crate registry
// ConfigMap with the given entries, in the format the store unmarshals:
// a map keyed by registry name.
func crateRegistryClient(t *testing.T, entries map[string]pkgaddon.Registry) client.Client {
	t.Helper()
	data, err := json.Marshal(entries)
	require.NoError(t, err)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: pkgcrate.CrateRegistryConfigMap, Namespace: types.DefaultKubeVelaNS},
		Data:       map[string]string{"registries": string(data)},
	}
	return fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(cm).Build()
}

func TestCratePublishFailsBeforePush(t *testing.T) {
	// invalidTreeDir contains an _crate.cue that parses but fails
	// validation (a non-semver version), so the case below fails through the
	// parser's validation gate rather than through a missing directory.
	invalidTreeDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(invalidTreeDir, "_crate.cue"),
		[]byte("crate:  \"bad\"\nversion: \"nope\"\n"),
		0o600,
	))

	cases := []struct {
		name    string
		options func(t *testing.T, rec *recordedPush) *cratePublishOptions
		wantErr string
	}{
		{
			name: "invalid crate tree",
			options: func(_ *testing.T, rec *recordedPush) *cratePublishOptions {
				return &cratePublishOptions{dir: invalidTreeDir, ociRef: "oci://registry.example.com/crates", push: rec.push}
			},
			wantErr: "not a valid semver",
		},
		{
			// A git registry is refused while the target is being resolved, so
			// publish never reaches the push.
			name: "git registry target is refused",
			options: func(t *testing.T, rec *recordedPush) *cratePublishOptions {
				return &cratePublishOptions{dir: publishFixtureDir(t), registry: "catalog", push: rec.push}
			},
			wantErr: "git registries are not supported",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recordedPush{}
			cli := crateRegistryClient(t, map[string]pkgaddon.Registry{
				"catalog": {Name: "catalog", Git: &pkgaddon.GitAddonSource{URL: "https://github.com/org/repo", Path: "crate"}},
			})
			err := tc.options(t, rec).run(context.Background(), cli, &bytes.Buffer{})
			require.ErrorContains(t, err, tc.wantErr)
			require.Zero(t, rec.calls)
		})
	}
}

func TestCratePublishRequiresClusterForNamedRegistry(t *testing.T) {
	o := &cratePublishOptions{dir: publishFixtureDir(t), registry: "ecr"}
	err := o.run(context.Background(), nil, &bytes.Buffer{})
	require.ErrorContains(t, err, "cluster")
}

func TestCratePublishAlreadyPublishedWithoutForce(t *testing.T) {
	rec := &recordedPush{}
	o := &cratePublishOptions{
		dir:    publishFixtureDir(t),
		ociRef: "oci://registry.example.com/crates",
		push:   rec.push,
		tagExists: func(_ context.Context, _ pkgaddon.Registry, _, _ string) (bool, error) {
			return true, nil
		},
	}

	err := o.run(context.Background(), nil, &bytes.Buffer{})
	require.ErrorContains(t, err, "registry.example.com/crates/s3:1.0.0")
	require.ErrorContains(t, err, "_crate.cue")
	require.Zero(t, rec.calls)
}

// errRepositoryNotFoundTest is what ECR actually returns for a push to a
// repository that does not yet exist: the blob upload fails with a bare 404.
var errRepositoryNotFoundTest = errors.New("failed to push chart registry.example.com/crates/s3:1.0.0: " +
	"unexpected status from POST request to https://registry.example.com/v2/crates/s3/blobs/uploads/: 404 Not Found")

// errTagImmutableTest mimics ECR's rejection of a tag move on a repository
// with IMMUTABLE tag mutability, so IsOCITagImmutable recognizes it.
var errTagImmutableTest = errors.New("ImageTagAlreadyExistsException: tag 1.0.0 is immutable")

func TestCratePublishRepositoryNotFoundError(t *testing.T) {
	o := &cratePublishOptions{
		dir:    publishFixtureDir(t),
		ociRef: "oci://registry.example.com/crates",
		push: func(context.Context, pkgaddon.Registry, string, string, []byte) error {
			return errRepositoryNotFoundTest
		},
		tagExists: func(_ context.Context, _ pkgaddon.Registry, _, _ string) (bool, error) {
			return false, nil
		},
	}

	err := o.run(context.Background(), nil, &bytes.Buffer{})
	require.ErrorContains(t, err, "registry.example.com/crates/s3:1.0.0")
	require.ErrorContains(t, err, "does not exist")
	require.ErrorContains(t, err, "create it")
	require.True(t, errors.Is(err, errRepositoryNotFoundTest), "wrapped error must survive as the %%w chain")
}

func TestCratePublishTagImmutableError(t *testing.T) {
	o := &cratePublishOptions{
		dir:    publishFixtureDir(t),
		ociRef: "oci://registry.example.com/crates",
		push: func(context.Context, pkgaddon.Registry, string, string, []byte) error {
			return errTagImmutableTest
		},
		tagExists: func(_ context.Context, _ pkgaddon.Registry, _, _ string) (bool, error) {
			return false, nil
		},
	}

	err := o.run(context.Background(), nil, &bytes.Buffer{})
	require.ErrorContains(t, err, "registry.example.com/crates/s3:1.0.0")
	require.ErrorContains(t, err, "_crate.cue")
	require.True(t, errors.Is(err, errTagImmutableTest), "wrapped error must survive as the %%w chain")
}

func TestCratePublishOverridesResolvedRegistryCredentials(t *testing.T) {
	rec := &recordedPush{}
	cli := crateRegistryClient(t, map[string]pkgaddon.Registry{
		"ecr": {Name: "ecr", Helm: &pkgaddon.HelmSource{
			URL:      "oci://123456789012.dkr.ecr.us-west-2.amazonaws.com/crates",
			Username: "entry-user",
			Token:    "entry-token",
		}},
	})
	o := &cratePublishOptions{
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

// TestCratePublishCommandDryRunReflectsFlags exercises RunE's flag-to-struct
// wiring through the real cobra command, rather than cratePublishOptions
// built directly. It uses a positional OCI reference with --dry-run so the
// path needs no cluster and pushes nothing, and asserts the printed target
// reflects --version, the one flag observable in dry-run output.
func TestCratePublishCommandDryRunReflectsFlags(t *testing.T) {
	cmd := NewCratePublishCommand(common.Args{}, cmdutil.NewDefaultIOStreams())
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{publishFixtureDir(t), "oci://registry.example.com/crates", "--dry-run", "--version", "9.9.9"})

	require.NoError(t, cmd.Execute())
	require.Contains(t, out.String(), "registry.example.com/crates/s3:9.9.9")
}

func TestCratePublishPrintsPlacementWarnings(t *testing.T) {
	dir := publishFixtureDir(t)
	full := filepath.Join(dir, "auxiliary", "spoke", "skip.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o750))
	require.NoError(t, os.WriteFile(full, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: skip\n"), 0o600))

	cmd := NewCratePublishCommand(common.Args{}, cmdutil.NewDefaultIOStreams())
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{dir, "oci://registry.example.com/crates", "--dry-run"})

	require.NoError(t, cmd.Execute())
	require.Contains(t, out.String(), "Warning: auxiliary/spoke is not a recognized folder")
}

func TestCratePublishDryRunRejectsBadPlacement(t *testing.T) {
	cases := []struct {
		name  string
		cue   string
		wants []string
	}{
		{
			name:  "empty",
			cue:   "crate: \"s3\"\nversion: \"1.0.0\"\nplacement: \"\"\n",
			wants: []string{"_crate.cue", "empty"},
		},
		{
			name:  "invalid",
			cue:   "crate: \"s3\"\nversion: \"1.0.0\"\nplacement: \"spoke\"\n",
			wants: []string{"_crate.cue", "spoke"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := publishFixtureDir(t)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "_crate.cue"), []byte(c.cue), 0o600))

			cmd := NewCratePublishCommand(common.Args{}, cmdutil.NewDefaultIOStreams())
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetArgs([]string{dir, "oci://registry.example.com/crates", "--dry-run"})

			err := cmd.Execute()
			require.Error(t, err)
			for _, want := range c.wants {
				require.ErrorContains(t, err, want)
			}
		})
	}
}

// TestCratePublishCommandRejectsRegistryFlagWithPositionalRef exercises the
// mutual-exclusion rejection through RunE rather than a direct
// cratePublishOptions construction, proving --registry is read from the
// real flag under its own name and reaches the options struct. This path
// needs no cluster and no network: the rejection in run happens before
// resolveTarget, so it is reachable even though --registry alone would
// otherwise require a client.
func TestCratePublishCommandRejectsRegistryFlagWithPositionalRef(t *testing.T) {
	cmd := NewCratePublishCommand(common.Args{}, cmdutil.NewDefaultIOStreams())
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{publishFixtureDir(t), "oci://registry.example.com/crates", "--registry", "ecr"})

	err := cmd.Execute()
	require.ErrorContains(t, err, "cannot be combined")
}
