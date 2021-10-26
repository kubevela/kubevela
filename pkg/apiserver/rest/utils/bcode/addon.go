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

package bcode

import (
	"github.com/google/go-github/v32/github"
)

const createGithubTokenLink = "https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/creating-a-personal-access-token"

var (
	// ErrAddonNotExist addon registry not exist
	ErrAddonNotExist = NewBcode(404, 50001, "addon not exist")

	// ErrAddonRegistryExist addon registry already exist
	ErrAddonRegistryExist = NewBcode(400, 50002, "addon registry already exists")

	// ErrAddonRegistryInvalid addon registry is exist
	ErrAddonRegistryInvalid = NewBcode(400, 50003, "addon registry invalid")

	// ErrAddonRegistryRateLimit addon registry is rate limited by Github
	ErrAddonRegistryRateLimit = NewBcode(400, 50004, "Github rate limit, please add a token: "+createGithubTokenLink)

	// ErrAddonRender fail to render addon application
	ErrAddonRender = NewBcode(500, 50010, "addon render fail")

	// ErrAddonApply fail to apply application to cluster
	ErrAddonApply = NewBcode(500, 50011, "fail to apply addon application")

	// ErrReadGit fail to get addon application
	ErrReadGit = NewBcode(500, 50012, "fail to read git repo")

	// ErrGetAddonApplication fail to get addon application
	ErrGetAddonApplication = NewBcode(500, 50013, "fail to get addon application")
)

func IsGithubRateLimit(err error) bool {
	_, ok := err.(*github.RateLimitError)
	return ok
}

func WrapGithubRateLimitErr(err error) error {
	if IsGithubRateLimit(err) {
		return ErrAddonRegistryRateLimit
	}
	return err
}
