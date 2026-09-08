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

package controllers_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/require"

	pkgmodule "github.com/oam-dev/kubevela/pkg/module"
	"github.com/oam-dev/kubevela/pkg/registry/component"
)

const moduleRegistryFixtureRelPath = "test/e2e-test/testdata/module/s3"

// TestPublishRoundTripECR runs a publish/pull round trip against a real ECR
// repository prefix when MODULE_ECR_REGISTRY is set, for example
// 123456789012.dkr.ecr.us-west-2.amazonaws.com/modules, or against any
// plain-HTTP registry via an "http://" prefix (e.g. a port-forwarded
// registry:2). Credentials come from the docker credential chain, so
// `aws ecr get-login-password | docker login` or docker-credential-ecr-login
// must be in place for ECR. The repository <prefix>/s3 must already
// exist. It skips cleanly when the environment variable is unset, so the
// default e2e run (no real ECR access) never touches a real registry; CI does
// not set it, so this is a manual/opt-in verification test, not a gate.
//
// It uses the realistic s3 fixture relocated from pkg/module testdata to the
// E2E fixture tree.
func TestPublishRoundTripECR(t *testing.T) {
	target := os.Getenv("MODULE_ECR_REGISTRY")
	if target == "" {
		t.Skip("set MODULE_ECR_REGISTRY to publish against a real ECR registry")
	}
	reg := component.Registry{Name: "ecr", Helm: &component.HelmSource{URL: target}}

	// Strip whichever scheme the target carries (oci://, https://, http://,
	// or none) to get the bare repoPrefix name.NewTag expects, and pass
	// name.Insecure only for an explicit http:// target so the manifest read
	// matches the scheme the push itself used; a real ECR target keeps
	// verifying HTTPS.
	repoPrefix := target
	var refOpts []name.Option
	switch {
	case strings.HasPrefix(target, "http://"):
		repoPrefix = strings.TrimPrefix(target, "http://")
		refOpts = append(refOpts, name.Insecure)
	case strings.HasPrefix(target, "https://"):
		repoPrefix = strings.TrimPrefix(target, "https://")
	case strings.HasPrefix(target, "oci://"):
		repoPrefix = strings.TrimPrefix(target, "oci://")
	}

	ctx := context.Background()
	fixtureDir := filepath.Join(modulePublishRepoRoot(), moduleRegistryFixtureRelPath)

	source, err := pkgmodule.ParseModuleDir(fixtureDir)
	require.NoError(t, err)

	art, err := pkgmodule.PackageModule(fixtureDir, "")
	require.NoError(t, err)
	require.NoError(t, component.PushOCIChart(ctx, reg, source.Name, art.Tag, art.Archive))

	ref, err := name.NewTag(repoPrefix+"/"+source.Name+":"+art.Tag, refOpts...)
	require.NoError(t, err)
	desc, err := remote.Get(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain), remote.WithContext(ctx))
	require.NoError(t, err)
	manifest, err := desc.Image()
	require.NoError(t, err)
	mf, err := manifest.Manifest()
	require.NoError(t, err)
	require.Equal(t, source.Name, mf.Annotations[pkgmodule.AnnotationModule])
	require.Equal(t, "v1", mf.Annotations[pkgmodule.AnnotationLines])
	require.Equal(t, "v1", mf.Annotations[pkgmodule.AnnotationEnabledLines])

	files, err := component.PullOCIChartFiles(ctx, reg, source.Name, art.Tag)
	require.NoError(t, err)
	pulled := fstest.MapFS{}
	for _, f := range files {
		pulled[f.Name] = &fstest.MapFile{Data: f.Data}
	}
	fetched, err := pkgmodule.ParseModule(pulled)
	require.NoError(t, err)
	require.Equal(t, source, fetched)
}
