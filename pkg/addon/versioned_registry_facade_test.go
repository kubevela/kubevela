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

package addon

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/repo"
)

// fakeBackend is a chartBackend whose every answer is supplied by the test, so
// the shared facade can be exercised without either transport.
type fakeBackend struct {
	listFn       func(ctx context.Context) ([]*UIData, error)
	versionsFn   func(ctx context.Context, addonName string) ([]*repo.ChartVersion, error)
	resolveFn    func(ctx context.Context, addonName, version string) (*resolvedChart, error)
	supportsReqs bool
}

func (f *fakeBackend) listUIData(ctx context.Context) ([]*UIData, error) {
	return f.listFn(ctx)
}

func (f *fakeBackend) versions(ctx context.Context, addonName string) ([]*repo.ChartVersion, error) {
	return f.versionsFn(ctx, addonName)
}

func (f *fakeBackend) resolve(ctx context.Context, addonName, version string) (*resolvedChart, error) {
	return f.resolveFn(ctx, addonName, version)
}

func (f *fakeBackend) supportsVersionRequirements() bool { return f.supportsReqs }

// classifyingBackend also implements errorClassifier, the way the OCI backend
// does, so the facade's opt-in error wrapping can be tested against a backend
// that wants it and one that does not.
type classifyingBackend struct {
	fakeBackend
}

func (c *classifyingBackend) classify(err error) error {
	if err == nil || errors.Is(err, ErrNotExist) || errors.Is(err, ErrFetch) {
		return err
	}
	return errors.Wrapf(ErrFetch, "classified: %v", err)
}

func testChartFiles(t *testing.T) []*loader.BufferedFile {
	t.Helper()
	archive, err := os.ReadFile("./testdata/helm-repo/fluxcd-1.0.0.tgz")
	require.NoError(t, err)
	files, err := loader.LoadArchiveFiles(bytes.NewReader(archive))
	require.NoError(t, err)
	return files
}

func TestHelmRegistrySystemRequirementsOverride(t *testing.T) {
	files := testChartFiles(t)

	resolveWith := func(t *testing.T, resolved *resolvedChart) *WholeAddonPackage {
		t.Helper()
		r := &helmRegistry{
			name: "my-registry",
			backend: &fakeBackend{
				resolveFn: func(_ context.Context, _, _ string) (*resolvedChart, error) {
					return resolved, nil
				},
			},
		}
		pkg, err := r.GetDetailedAddon(context.Background(), "fluxcd", "")
		require.NoError(t, err)
		return pkg
	}

	// A backend with no opinion leaves whatever the addon package itself declared.
	fromPackage := resolveWith(t, &resolvedChart{files: files}).Meta.SystemRequirements

	t.Run("no opinion keeps the package value", func(t *testing.T) {
		pkg := resolveWith(t, &resolvedChart{files: files, requirementsSet: false,
			requirements: &SystemRequirements{VelaVersion: ">=1.5.0"}})
		assert.Equal(t, fromPackage, pkg.Meta.SystemRequirements,
			"requirements must be ignored unless the backend marks them as set")
	})

	t.Run("an explicit value overrides", func(t *testing.T) {
		want := &SystemRequirements{VelaVersion: ">=1.5.0", KubernetesVersion: ">=1.20.0"}
		pkg := resolveWith(t, &resolvedChart{files: files, requirementsSet: true, requirements: want})
		assert.Equal(t, want, pkg.Meta.SystemRequirements)
	})

	t.Run("an explicit nil overrides", func(t *testing.T) {
		// The HTTP backend reads requirements from index annotations and must be
		// able to say "the index declares none", which is distinct from silence.
		pkg := resolveWith(t, &resolvedChart{files: files, requirementsSet: true, requirements: nil})
		assert.Nil(t, pkg.Meta.SystemRequirements)
	})
}

func TestHelmRegistryErrorClassification(t *testing.T) {
	boom := errors.New("boom")

	t.Run("a backend without a classifier surfaces the error unchanged", func(t *testing.T) {
		r := &helmRegistry{name: "http", backend: &fakeBackend{
			resolveFn: func(_ context.Context, _, _ string) (*resolvedChart, error) { return nil, boom },
		}}
		_, err := r.GetDetailedAddon(context.Background(), "fluxcd", "")
		require.Error(t, err)
		assert.False(t, errors.Is(err, ErrFetch), "the HTTP path must keep its unwrapped errors")
		assert.Equal(t, boom, err)
	})

	t.Run("a classifying backend wraps the error", func(t *testing.T) {
		r := &helmRegistry{name: "oci", backend: &classifyingBackend{fakeBackend{
			resolveFn: func(_ context.Context, _, _ string) (*resolvedChart, error) { return nil, boom },
		}}}
		_, err := r.GetDetailedAddon(context.Background(), "fluxcd", "")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrFetch), "installDependency relies on ErrFetch to try the next registry")
	})

	t.Run("a classifying backend leaves ErrNotExist alone", func(t *testing.T) {
		r := &helmRegistry{name: "oci", backend: &classifyingBackend{fakeBackend{
			resolveFn: func(_ context.Context, _, _ string) (*resolvedChart, error) { return nil, ErrNotExist },
		}}}
		_, err := r.GetDetailedAddon(context.Background(), "fluxcd", "")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrNotExist))
		assert.False(t, errors.Is(err, ErrFetch))
	})

	t.Run("classification also covers a malformed package", func(t *testing.T) {
		// The OCI path wraps everything its loadAddon touches, decoding included,
		// so a corrupt archive stays skippable rather than aborting dependency
		// resolution. metadata.yaml must be present but unparseable, otherwise
		// loadAddonPackage tolerates the input and the test proves nothing.
		r := &helmRegistry{name: "oci", backend: &classifyingBackend{fakeBackend{
			resolveFn: func(_ context.Context, _, _ string) (*resolvedChart, error) {
				return &resolvedChart{files: []*loader.BufferedFile{
					{Name: "fluxcd/metadata.yaml", Data: []byte("\tname: [unclosed")},
				}}, nil
			},
		}}}
		_, err := r.GetDetailedAddon(context.Background(), "fluxcd", "")
		require.Error(t, err, "a malformed package must not decode successfully")
		assert.True(t, errors.Is(err, ErrFetch),
			"installDependency relies on ErrFetch to fall through to the next registry")
	})
}

func TestCompatibleVersionFromSkipsUnawareBackend(t *testing.T) {
	// An OCI registry's versions are synthesized from tags and carry no
	// annotations, so LoadSystemRequirements reads nil for each of them and a nil
	// requirement passes every check. The guard is what stops the newest tag being
	// reported as compatible regardless of what it actually requires.
	newRegistry := func(supportsReqs bool) (VersionedRegistry, *bool) {
		called := false
		return &helmRegistry{name: "reg", backend: &fakeBackend{
			supportsReqs: supportsReqs,
			versionsFn: func(_ context.Context, _ string) ([]*repo.ChartVersion, error) {
				called = true
				return []*repo.ChartVersion{{Metadata: &chart.Metadata{Name: "fluxcd", Version: "9.9.9"}}}, nil
			},
		}}, &called
	}
	installer := &Installer{ctx: context.Background()}

	t.Run("an unaware backend yields no suggestion", func(t *testing.T) {
		reg, called := newRegistry(false)
		assert.Equal(t, "", installer.compatibleVersionFrom(reg, "fluxcd"))
		assert.False(t, *called, "an unaware backend must not even be asked for versions")
	})

	t.Run("an aware backend is consulted", func(t *testing.T) {
		reg, called := newRegistry(true)
		assert.Equal(t, "9.9.9", installer.compatibleVersionFrom(reg, "fluxcd"))
		assert.True(t, *called)
	})
}

// TestMisconfiguredRegistryIsSkippable pins the fix for a regression this
// refactor introduced. BuildVersionedRegistry could never fail, so a bad
// registry record only broke that registry. NewVersionedRegistry validates
// credentials and can fail, and listAvailableAddons/installDependency use
// isSkippableRegistryError to decide whether to try the next registry. An
// unclassified error there aborts the whole loop, so a single hand-edited
// ConfigMap entry would break `vela addon enable` for every addon.
func TestMisconfiguredRegistryIsSkippable(t *testing.T) {
	cases := map[string]Registry{
		"token on an http registry": {
			Name: "bad-http",
			Helm: &HelmSource{URL: "https://charts.example.com", Token: "tok"},
		},
		"password on an oci registry": {
			Name: "bad-oci",
			Helm: &HelmSource{URL: "oci://reg.example.com/addon", Password: "pw"},
		},
		"insecureSkipTLS on an oci registry": {
			Name: "bad-tls",
			Helm: &HelmSource{URL: "oci://reg.example.com/addon", InsecureSkipTLS: true},
		},
	}

	for name, reg := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ToVersionedRegistry(reg)
			require.Error(t, err)
			assert.True(t, isSkippableRegistryError(err),
				"a misconfigured registry must not abort resolution across every other registry")
		})
	}
}

// TestHelmRegistryListAddonStampsRegistryName guards the stamping the backends
// deliberately do not do: resolveAddonListFromIndex leaves RegistryName unset
// because the facade owns it, so without this nothing asserts it gets set.
func TestHelmRegistryListAddonStampsRegistryName(t *testing.T) {
	r := &helmRegistry{
		name: "my-registry",
		backend: &fakeBackend{
			listFn: func(_ context.Context) ([]*UIData, error) {
				return []*UIData{
					{Meta: Meta{Name: "fluxcd"}},
					{Meta: Meta{Name: "velaux"}, RegistryName: "stale"},
				}, nil
			},
		},
	}

	addons, err := r.ListAddon()
	require.NoError(t, err)
	require.Len(t, addons, 2)
	for _, addon := range addons {
		assert.Equal(t, "my-registry", addon.RegistryName)
	}
}
