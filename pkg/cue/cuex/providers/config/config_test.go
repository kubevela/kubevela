/*
Copyright 2023 The KubeVela Authors.

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

package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImageRegistry(t *testing.T) {
	ctx := context.Background()
	testCases := []struct {
		name             string
		validationParams *ImageRegistryParams
		expectResult     bool
	}{
		{
			name: "Should authenticate with correct credential",
			validationParams: &ImageRegistryParams{
				Params: ImageRegistryVars{
					Registry: "dockerhub.qingcloud.com",
					Auth: struct {
						Username string `json:"username"`
						Password string `json:"password"`
						Email    string `json:"email"`
					}{
						Username: "guest", Password: "guest",
					},
					Insecure: false,
					UseHTTP:  false,
				},
			},
			expectResult: true,
		},
		{
			name: "Shouldn't authenticate with incorrect credentials",
			validationParams: &ImageRegistryParams{
				Params: ImageRegistryVars{
					Registry: "index.docker.io",
					Auth: struct {
						Username string `json:"username"`
						Password string `json:"password"`
						Email    string `json:"email"`
					}{
						Username: "foo", Password: "bar",
					},
					Insecure: false,
					UseHTTP:  false,
				},
			},
			expectResult: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_v, err := ImageRegistry(ctx, testCase.validationParams)
			require.NoError(t, err)
			require.Equal(t, testCase.expectResult, _v.Returns.Result)
		})
	}
}

func TestHelmRepository(t *testing.T) {
	ctx := context.Background()
	testCases := []struct {
		name             string
		validationParams *HelmRepositoryParams
		expectResult     bool
	}{
		{
			name: "Should authenticate with correct credential",
			validationParams: &HelmRepositoryParams{
				Params: HelmRepositoryVars{
					URL: "https://charts.kubevela.net/core",
				},
			},
			expectResult: true,
		},
		{
			name: "Shouldn't authenticate with incorrect helm repo URL",
			validationParams: &HelmRepositoryParams{
				Params: HelmRepositoryVars{
					URL: "https://www.baidu.com",
				},
			},
			expectResult: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_v, err := HelmRepository(ctx, testCase.validationParams)
			require.NoError(t, err)
			require.Equal(t, testCase.expectResult, _v.Returns.Result)
		})
	}
}
