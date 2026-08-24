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
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/repo"
)

// fakeVersionedRegistry is a test double for VersionedRegistry.
type fakeVersionedRegistry struct {
	uiDataMap map[string]*UIData
	err       error
}

func (f *fakeVersionedRegistry) ListAddon() ([]*UIData, error) { return nil, nil }
func (f *fakeVersionedRegistry) GetAddonUIData(_ context.Context, name, version string) (*UIData, error) {
	if f.err != nil {
		return nil, f.err
	}
	if d, ok := f.uiDataMap[name+"-"+version]; ok {
		return d, nil
	}
	return nil, errors.New("not found")
}
func (f *fakeVersionedRegistry) GetAddonInstallPackage(context.Context, string, string) (*InstallPackage, error) {
	return nil, nil
}
func (f *fakeVersionedRegistry) GetDetailedAddon(context.Context, string, string) (*WholeAddonPackage, error) {
	return nil, nil
}
func (f *fakeVersionedRegistry) GetAddonAvailableVersion(string) ([]*repo.ChartVersion, error) {
	return nil, nil
}

func TestCacheVersionedUIData(t *testing.T) {
	t.Run("caches addon and carries available versions", func(t *testing.T) {
		c := NewCache(nil)
		fakeReg := &fakeVersionedRegistry{
			uiDataMap: map[string]*UIData{
				"fluxcd-1.0.0": {Meta: Meta{Name: "fluxcd", Version: "1.0.0"}},
			},
		}
		listing := []*UIData{
			{Meta: Meta{Name: "fluxcd", Version: "1.0.0"}, AvailableVersions: []string{"1.0.0", "0.9.0"}},
		}

		c.cacheVersionedUIData("test-reg", fakeReg, listing)

		cached := c.versionedUIData["test-reg"]["fluxcd-1.0.0"]
		require.NotNil(t, cached)
		assert.Equal(t, "fluxcd", cached.Name)
		assert.Equal(t, []string{"1.0.0", "0.9.0"}, cached.AvailableVersions)

		latest := c.versionedUIData["test-reg"]["fluxcd-latest"]
		require.NotNil(t, latest)
		assert.Equal(t, "fluxcd", latest.Name)
	})

	t.Run("skips addon on fetch error", func(t *testing.T) {
		c := NewCache(nil)
		fakeReg := &fakeVersionedRegistry{err: errors.New("network error")}
		listing := []*UIData{{Meta: Meta{Name: "fluxcd", Version: "1.0.0"}}}

		c.cacheVersionedUIData("test-reg", fakeReg, listing)
		assert.Empty(t, c.versionedUIData["test-reg"])
	})

	t.Run("skips addon with empty name from chart", func(t *testing.T) {
		c := NewCache(nil)
		fakeReg := &fakeVersionedRegistry{
			uiDataMap: map[string]*UIData{
				"badchart-1.0.0": {Meta: Meta{Name: "", Version: "1.0.0"}},
			},
		}
		listing := []*UIData{{Meta: Meta{Name: "badchart", Version: "1.0.0"}}}

		c.cacheVersionedUIData("test-reg", fakeReg, listing)
		assert.Empty(t, c.versionedUIData["test-reg"])
	})

	t.Run("deletes stale entries no longer in listing", func(t *testing.T) {
		c := NewCache(nil)
		// Pre-populate with an addon that won't appear in the new listing
		c.putVersionedUIData2Cache("test-reg", "old-addon", "1.0.0", &UIData{Meta: Meta{Name: "old-addon", Version: "1.0.0"}})
		c.putVersionedUIData2Cache("test-reg", "old-addon", "latest", &UIData{Meta: Meta{Name: "old-addon", Version: "1.0.0"}})
		require.NotNil(t, c.versionedUIData["test-reg"]["old-addon-1.0.0"])

		fakeReg := &fakeVersionedRegistry{
			uiDataMap: map[string]*UIData{
				"fluxcd-1.0.0": {Meta: Meta{Name: "fluxcd", Version: "1.0.0"}},
			},
		}
		listing := []*UIData{{Meta: Meta{Name: "fluxcd", Version: "1.0.0"}}}

		c.cacheVersionedUIData("test-reg", fakeReg, listing)

		assert.NotNil(t, c.versionedUIData["test-reg"]["fluxcd-1.0.0"], "new addon must be cached")
		assert.Nil(t, c.versionedUIData["test-reg"]["old-addon-1.0.0"], "stale addon must be deleted")
	})

	t.Run("preserves available versions when fetch already has them", func(t *testing.T) {
		c := NewCache(nil)
		fakeReg := &fakeVersionedRegistry{
			uiDataMap: map[string]*UIData{
				"fluxcd-1.0.0": {Meta: Meta{Name: "fluxcd", Version: "1.0.0"}, AvailableVersions: []string{"1.0.0"}},
			},
		}
		listing := []*UIData{
			{Meta: Meta{Name: "fluxcd", Version: "1.0.0"}, AvailableVersions: []string{"1.0.0", "0.9.0"}},
		}

		c.cacheVersionedUIData("test-reg", fakeReg, listing)

		cached := c.versionedUIData["test-reg"]["fluxcd-1.0.0"]
		require.NotNil(t, cached)
		// fetch already had versions, so the listing's list should NOT override
		assert.Equal(t, []string{"1.0.0"}, cached.AvailableVersions)
	})
}
