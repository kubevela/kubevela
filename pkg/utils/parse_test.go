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

package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestByteCountIEC(t *testing.T) {
	testCases := map[string]struct {
		Input  int64
		Output string
	}{
		"1 B": {
			Input:  int64(1),
			Output: "1 B",
		},
		"1.1 KiB": {
			Input:  int64(1124),
			Output: "1.1 KiB",
		},
		"1.2 MiB": {
			Input:  int64(1258291),
			Output: "1.2 MiB",
		},
		"3.3 GiB": {
			Input:  int64(3543348020),
			Output: "3.3 GiB",
		},
	}
	r := require.New(t)
	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			r.Equal(tt.Output, ByteCountIEC(tt.Input))
		})
	}
}

func TestParse(t *testing.T) {
	testCases := []struct {
		name        string
		addr        string
		wantType    string
		wantContent *Content
		wantErr     bool
	}{
		{
			name:        "github url with branch",
			addr:        "https://github.com/kubevela/catalog/tree/master/addons/fluxcd",
			wantType:    TypeGithub,
			wantContent: &Content{GithubContent: GithubContent{Owner: "kubevela", Repo: "catalog", Path: "addons/fluxcd", Ref: "master"}},
		},
		{
			name:        "github url without branch",
			addr:        "https://github.com/kubevela/catalog/addons/fluxcd",
			wantType:    TypeGithub,
			wantContent: &Content{GithubContent: GithubContent{Owner: "kubevela", Repo: "catalog", Path: "addons/fluxcd", Ref: ""}},
		},
		{
			name:        "github api url with single segment path",
			addr:        "https://api.github.com/repos/kubevela/catalog/contents/my-addon?ref=master",
			wantType:    TypeGithub,
			wantContent: &Content{GithubContent: GithubContent{Owner: "kubevela", Repo: "catalog", Path: "my-addon", Ref: "master"}},
		},
		{
			name:        "gitee url with branch",
			addr:        "https://gitee.com/kubevela/catalog/tree/master/addons/fluxcd",
			wantType:    TypeGitee,
			wantContent: &Content{GiteeContent: GiteeContent{Owner: "kubevela", Repo: "catalog", Path: "addons/fluxcd", Ref: "master"}},
		},
		{
			name:        "gitee url without branch",
			addr:        "https://gitee.com/kubevela/catalog/addons/fluxcd",
			wantType:    TypeGitee,
			wantContent: &Content{GiteeContent: GiteeContent{Owner: "kubevela", Repo: "catalog", Path: "addons/fluxcd", Ref: ""}},
		},
		{
			name:        "oss url",
			addr:        "oss://kubevela-contrib/registry",
			wantType:    TypeOss,
			wantContent: &Content{OssContent: OssContent{EndPoint: "kubevela-contrib", Bucket: "/registry"}},
		},
		{
			name:        "local url",
			addr:        "file:///Users/somebody/addons",
			wantType:    TypeLocal,
			wantContent: &Content{LocalContent: LocalContent{AbsDir: "/Users/somebody/addons"}},
		},
		{
			name:    "invalid github url",
			addr:    "https://github.com/kubevela",
			wantErr: true,
		},
		{
			name:    "unsupported git url",
			addr:    "https://bitbucket.org/foo/bar",
			wantErr: true,
		},
		{
			name:    "malformed url",
			addr:    "://abc",
			wantErr: true,
		},
		{
			name:     "unknown type",
			addr:     "myscheme://foo/bar",
			wantType: TypeUnknown,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotType, gotContent, err := Parse(tc.addr)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantType, gotType)
			require.Equal(t, tc.wantContent, gotContent)
		})
	}
}

func TestParseGitlab(t *testing.T) {
	testCases := []struct {
		name        string
		addr        string
		repo        string
		wantType    string
		wantContent *Content
		wantErr     bool
	}{
		{
			name:        "gitlab url without branch",
			addr:        "https://gitlab.com/kubevela/catalog",
			repo:        "catalog",
			wantType:    TypeGitlab,
			wantContent: &Content{GitlabContent: GitlabContent{Host: "https://gitlab.com", Owner: "kubevela", Repo: "catalog", Ref: ""}},
		},
		{
			name:        "gitlab url with branch",
			addr:        "https://gitlab.com/kubevela/catalog/tree/master",
			repo:        "catalog",
			wantType:    TypeGitlab,
			wantContent: &Content{GitlabContent: GitlabContent{Host: "https://gitlab.com", Owner: "kubevela", Repo: "catalog", Ref: "master"}},
		},
		{
			name:    "invalid gitlab url repo not match",
			addr:    "https://gitlab.com/kubevela/catalog",
			repo:    "wrong-repo",
			wantErr: true,
		},
		{
			name:    "malformed gitlab url",
			addr:    "://abc",
			repo:    "repo",
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotType, gotContent, err := ParseGitlab(tc.addr, tc.repo)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantType, gotType)
			require.Equal(t, tc.wantContent, gotContent)
		})
	}
}
