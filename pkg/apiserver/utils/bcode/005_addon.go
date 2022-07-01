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
	"github.com/pkg/errors"

	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
)

var (
	// ErrAddonNotExist addon registry not exist
	ErrAddonNotExist = NewBcode(404, 50001, "addon not exist")

	// ErrAddonRegistryExist addon registry already exist
	ErrAddonRegistryExist = NewBcode(400, 50002, "addon registry already exists")

	// ErrAddonRegistryInvalid addon registry is exist
	ErrAddonRegistryInvalid = NewBcode(400, 50003, "addon registry invalid")

	// ErrAddonRegistryRateLimit addon registry is rate limited by Github
	ErrAddonRegistryRateLimit = NewBcode(400, 50004, "Exceed Github rate limit")

	// ErrAddonRegistryNotExist addon registry doesn't exist
	ErrAddonRegistryNotExist = NewBcode(400, 50006, "addon registry doesn't exist")

	// ErrAddonRender fail to render addon application
	ErrAddonRender = NewBcode(500, 50010, "addon render fail")

	// ErrAddonApply fail to apply application to cluster
	ErrAddonApply = NewBcode(500, 50011, "fail to apply addon resources")

	// ErrReadGit fail to get addon application
	ErrReadGit = NewBcode(500, 50012, "fail to read git repo")

	// ErrGetAddonApplication fail to get addon application
	ErrGetAddonApplication = NewBcode(500, 50013, "fail to get addon application")

	// ErrAddonIsEnabled means addon has been enabled
	ErrAddonIsEnabled = NewBcode(500, 50014, "addon has been enabled")

	// ErrAddonSecretApply means fail to apply addon argument secret
	ErrAddonSecretApply = NewBcode(500, 50015, "fail to apply addon argument secret")

	// ErrAddonSecretGet means fail to get addon argument secret
	ErrAddonSecretGet = NewBcode(500, 50016, "fail to get addon argument secret")

	// ErrAddonDependencyNotSatisfy means addon's dependencies is not enabled
	ErrAddonDependencyNotSatisfy = NewBcode(500, 50017, "addon's dependencies is not enabled")

	// ErrAddonSystemVersionMismatch means addon's version required mismatch
	ErrAddonSystemVersionMismatch = NewBcode(400, 50018, "addon's system version requirement mismatch")

	// ErrAddonInvalidVersion means add version is invalid
	ErrAddonInvalidVersion = NewBcode(400, 50019, "")

	// ErrCloudShellAddonNotEnabled means the cloudshell CRD is not installed
	ErrCloudShellAddonNotEnabled = NewBcode(200, 50020, "Please enable the CloudShell addon first")

	// ErrCloudShellNotInit means the cloudshell CR not created
	ErrCloudShellNotInit = NewBcode(400, 50021, "Closing the console window and retry")
)

// isGithubRateLimit check if error is github rate limit
func isGithubRateLimit(err error) bool {
	return errors.Is(err, pkgaddon.ErrRateLimit)
}

// WrapGithubRateLimitErr wraps error if it is github rate limit
func WrapGithubRateLimitErr(err error) error {
	if isGithubRateLimit(err) {
		return ErrAddonRegistryRateLimit
	}
	return err
}

// NewBcodeWrapErr new bcode error
func NewBcodeWrapErr(httpCode, businessCode int32, err error, message string) error {
	return NewBcode(httpCode, businessCode, errors.Wrap(err, message).Error())
}
