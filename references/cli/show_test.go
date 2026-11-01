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

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/apis/types"
)

const BaseDir = "testdata"

var TestDir = filepath.Join(BaseDir, "show")

func TestCreateTestDir(t *testing.T) {
	if _, err := os.Stat(TestDir); err != nil && os.IsNotExist(err) {
		err := os.MkdirAll(TestDir, 0750)
		assert.NoError(t, err)
	}
}

func TestGenerateSideBar(t *testing.T) {
	workloadName := "component1"
	traitName := "trait1"

	cases := map[string]struct {
		reason       string
		capabilities []types.Capability
		want         error
	}{
		"ComponentTypeAndTraitCapability": {
			reason: "valid capabilities",
			capabilities: []types.Capability{
				{
					Name: workloadName,
					Type: types.TypeComponentDefinition,
				},
				{
					Name: traitName,
					Type: types.TypeTrait,
				},
			},
			want: nil,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// Own temp dir so the test does not depend on TestCreateTestDir running first.
			dir := t.TempDir()
			got := generateSideBar(tc.capabilities, dir)
			if diff := cmp.Diff(tc.want, got, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ngenerateSideBar(...): -want error, +got error:\n%s", tc.reason, diff)
			}
			data, err := os.ReadFile(filepath.Clean(filepath.Join(dir, SideBar)))
			assert.NoError(t, err)
			for _, c := range tc.capabilities {
				assert.Contains(t, string(data), c.Name)
			}
		})
	}
}

func TestGenerateNavBar(t *testing.T) {
	assert.NoError(t, generateNavBar(TestDir))
	_, err := os.Stat(filepath.Clean(filepath.Join(TestDir, NavBar)))
	assert.NoError(t, err)
}

func TestGenerateIndexHTML(t *testing.T) {
	assert.NoError(t, generateIndexHTML(TestDir))
	_, err := os.Stat(filepath.Clean(filepath.Join(TestDir, IndexHTML)))
	assert.NoError(t, err)
}

func TestGenerateCustomCSS(t *testing.T) {
	assert.NoError(t, generateCustomCSS(TestDir))
	_, err := os.Stat(filepath.Clean(filepath.Join(TestDir, CSS)))
	assert.NoError(t, err)
}

func TestGenerateREADME(t *testing.T) {
	workloadName := "component1"
	traitName := "trait1"

	cases := map[string]struct {
		reason       string
		capabilities []types.Capability
		want         error
	}{
		"ComponentTypeAndTraitCapability": {
			reason: "valid capabilities",
			capabilities: []types.Capability{
				{
					Name: workloadName,
					Type: types.TypeComponentDefinition,
				},
				{
					Name: traitName,
					Type: types.TypeTrait,
				},
			},
			want: nil,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// Own temp dir so the test does not depend on TestCreateTestDir running first.
			dir := t.TempDir()
			got := generateREADME(tc.capabilities, dir)
			if diff := cmp.Diff(tc.want, got, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ngenerateREADME(...): -want error, +got error:\n%s", tc.reason, diff)
			}
			data, err := os.ReadFile(filepath.Clean(filepath.Join(dir, README)))
			assert.NoError(t, err)
			for _, c := range tc.capabilities {
				switch c.Type {
				case types.TypeComponentDefinition:
					assert.Contains(t, string(data), fmt.Sprintf("  - [%s](%s/%s.md)\n", c.Name, types.TypeComponentDefinition, c.Name))
				case types.TypeTrait:
					assert.Contains(t, string(data), fmt.Sprintf("  - [%s](%s/%s.md)\n", c.Name, types.TypeTrait, c.Name))
				}
			}
		})
	}
}

func TestGetWorkloadAndTraits(t *testing.T) {
	type want struct {
		workloads []string
		traits    []string
		policies  []string
	}

	var (
		workloadName = "component1"
		traitName    = "trait1"
		policyName   = "policy1"
	)

	cases := map[string]struct {
		reason       string
		capabilities []types.Capability
		want         want
	}{
		"ComponentTypeAndTraitCapability": {
			reason: "valid capabilities",
			capabilities: []types.Capability{
				{
					Name: workloadName,
					Type: types.TypeComponentDefinition,
				},
				{
					Name: traitName,
					Type: types.TypeTrait,
				},
			},
			want: want{
				workloads: []string{workloadName},
				traits:    []string{traitName},
			},
		},
		"PolicyTypeCapability": {
			capabilities: []types.Capability{
				{
					Name: policyName,
					Type: types.TypePolicy,
				},
			},
			want: want{
				policies: []string{policyName},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			gotWorkloads, gotTraits, _, gotPolicies := getDefinitions(tc.capabilities)
			assert.Equal(t, tc.want, want{workloads: gotWorkloads, traits: gotTraits, policies: gotPolicies})
		})
	}
}

func TestDeleteTestDir(t *testing.T) {
	if _, err := os.Stat(BaseDir); err == nil {
		err := os.RemoveAll(BaseDir)
		assert.NoError(t, err)
	}
}

func TestParseShowRevision(t *testing.T) {
	cases := map[string]struct {
		in      string
		rev     int64
		version string
		wantErr bool
	}{
		"empty":              {in: ""},
		"revision number":    {in: "2", rev: 2},
		"v revision number":  {in: "v2", rev: 2},
		"semver":             {in: "1.2.3", version: "1.2.3"},
		"v semver":           {in: "v1.2.3", version: "1.2.3"},
		"semver prerelease":  {in: "1.2.3-beta.1", version: "1.2.3-beta.1"},
		"not a version":      {in: "abc", wantErr: true},
		"bare v":             {in: "v", wantErr: true},
		"wildcard not exact": {in: "1.x", wantErr: true},
		"double v semver":    {in: "vv1.2.3", wantErr: true},
		"double v revision":  {in: "vv2", wantErr: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rev, version, err := parseShowRevision(tc.in)
			if tc.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "revision number (2, v2)")
				assert.Contains(t, err.Error(), "semantic version (1.2.3, v1.2.3)")
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.rev, rev)
			assert.Equal(t, tc.version, version)
		})
	}
}
