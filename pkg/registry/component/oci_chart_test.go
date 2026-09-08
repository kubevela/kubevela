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

package component

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOCIChartRef(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string
	}{
		{name: "scheme and prefix", url: "oci://registry.example.com/modules", want: "registry.example.com/modules/s3:1.0.0"},
		{name: "no prefix", url: "oci://registry.example.com", want: "registry.example.com/s3:1.0.0"},
		{name: "bare ecr host", url: "123456789012.dkr.ecr.us-west-2.amazonaws.com/modules", want: "123456789012.dkr.ecr.us-west-2.amazonaws.com/modules/s3:1.0.0"},
		{name: "http scheme", url: "http://127.0.0.1:5000/modules", want: "127.0.0.1:5000/modules/s3:1.0.0"},
		{name: "trailing slash", url: "oci://registry.example.com/modules/", want: "registry.example.com/modules/s3:1.0.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ref, err := OCIChartRef(Registry{Name: "r", Helm: &HelmSource{URL: tc.url}}, "s3", "1.0.0")
			require.NoError(t, err)
			require.Equal(t, tc.want, ref)
		})
	}
}

func TestOCIChartRefRejectsNonOCIRegistry(t *testing.T) {
	_, err := OCIChartRef(Registry{Name: "cat", Git: &GitAddonSource{URL: "https://github.com/org/repo"}}, "s3", "1.0.0")
	require.ErrorContains(t, err, "not an OCI registry")
}

func TestPushOCIChartRejectsNonOCIRegistry(t *testing.T) {
	err := PushOCIChart(context.Background(), Registry{Name: "cat", Git: &GitAddonSource{URL: "https://github.com/org/repo"}}, "s3", "1.0.0", []byte("x"))
	require.ErrorContains(t, err, "not an OCI registry")
}

func TestOCIChartTagExists(t *testing.T) {
	reg := Registry{Name: "r", Helm: &HelmSource{URL: "oci://registry.example.com/modules"}}
	cases := []struct {
		name    string
		tags    []string
		tagsErr error
		tag     string
		want    bool
		wantErr string
	}{
		{name: "tag present", tags: []string{"1.1.0", "1.0.0"}, tag: "1.0.0", want: true},
		{name: "tag absent", tags: []string{"1.1.0"}, tag: "1.0.0", want: false},
		{name: "empty repository", tags: nil, tag: "1.0.0", want: false},
		{name: "repository missing", tagsErr: errors.New("unexpected status: 404 Not Found: NAME_UNKNOWN"), tag: "1.0.0", want: false},
		{name: "listing failed", tagsErr: errors.New("unauthorized"), tag: "1.0.0", wantErr: "unauthorized"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			restore := ociTagListerForTest(func(_, _, _, _ string, _ bool) ([]string, error) {
				return tc.tags, tc.tagsErr
			})
			defer restore()

			got, err := OCIChartTagExists(context.Background(), reg, "s3", tc.tag)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestIsOCIRepositoryNotFound(t *testing.T) {
	require.True(t, IsOCIRepositoryNotFound(errors.New("unexpected status: 404 Not Found: NAME_UNKNOWN")))
	require.True(t, IsOCIRepositoryNotFound(errors.New("RepositoryNotFoundException: The repository with name 'modules/s3' does not exist")))
	require.False(t, IsOCIRepositoryNotFound(errors.New("unauthorized: authentication required")))
}

func TestIsOCITagImmutable(t *testing.T) {
	require.True(t, IsOCITagImmutable(errors.New("ImageTagAlreadyExistsException: Tag 1.0.0 is immutable")))
	require.False(t, IsOCITagImmutable(errors.New("unauthorized: authentication required")))
}
