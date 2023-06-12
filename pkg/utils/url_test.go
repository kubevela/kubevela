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
	"fmt"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEndpoint(t *testing.T) {
	testCases := []struct {
		Input    string
		Output   string
		HasError bool
	}{{
		Input:  "127.0.0.1",
		Output: "https://127.0.0.1:443",
	}, {
		Input:  "http://127.0.0.1",
		Output: "http://127.0.0.1:80",
	}, {
		Input:  "127.0.0.1:6443",
		Output: "https://127.0.0.1:6443",
	}, {
		Input:  "127.0.0.1:80",
		Output: "http://127.0.0.1:80",
	}, {
		Input:  "localhost",
		Output: "https://localhost:443",
	}, {
		Input:  "https://worker-control-plane:6443",
		Output: "https://worker-control-plane:6443",
	}, {
		Input:    "invalid url",
		HasError: true,
	}}
	r := require.New(t)
	for _, testCase := range testCases {
		output, err := ParseAPIServerEndpoint(testCase.Input)
		if testCase.HasError {
			r.Error(err)
			continue
		}
		r.NoError(err)
		r.Equal(testCase.Output, output)
	}
}

func TestIsValidURL(t *testing.T) {
	type args struct {
		strURL string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "empty url should valid error",
			args: args{
				strURL: "",
			},
			want: false,
		},
		{
			name: "invalid url format should valid error",
			args: args{
				strURL: "invalid url",
			},
			want: false,
		},
		{
			name: "invalid scheme should valid error",
			args: args{
				strURL: "http://",
			},
			want: false,
		},
		{
			name: "invalid host should valid error",
			args: args{
				strURL: "http:// :8080",
			},
			want: false,
		},
		{
			name: "normal url should valid",
			args: args{
				strURL: "http://localhost:8080",
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, IsValidURL(tt.args.strURL), "IsValidURL(%v)", tt.args.strURL)
		})
	}
}

func TestJoinURL(t *testing.T) {
	testcase := []struct {
		baseURL     string
		subPath     string
		expectedUrl string
		err         error
	}{
		{
			baseURL:     "https://www.kubevela.com",
			subPath:     "index.yaml",
			expectedUrl: "https://www.kubevela.com/index.yaml",
			err:         nil,
		},
		{
			baseURL:     "http://www.kubevela.com",
			subPath:     "index.yaml",
			expectedUrl: "http://www.kubevela.com/index.yaml",
			err:         nil,
		},
		{
			baseURL:     "0x7f:",
			subPath:     "index.yaml",
			expectedUrl: "",
			err:         &url.Error{Op: "parse", URL: "0x7f:", Err: fmt.Errorf("first path segment in URL cannot contain colon")},
		},
	}

	for _, tc := range testcase {
		url, err := JoinURL(tc.baseURL, tc.subPath)
		assert.Equal(t, tc.expectedUrl, url)
		assert.Equal(t, tc.err, err)
	}
}
