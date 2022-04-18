/*
Copyright 2022 The KubeVela Authors.

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

var (
	// ErrUnsupportedEmailModification is the error of unsupported email modification
	ErrUnsupportedEmailModification = NewBcode(400, 14001, "the user already has an email address and cannot modify it again")
	// ErrUserAlreadyDisabled is the error of user already disabled
	ErrUserAlreadyDisabled = NewBcode(400, 14002, "the user is already disabled")
	// ErrUserAlreadyEnabled is the error of user already enabled
	ErrUserAlreadyEnabled = NewBcode(400, 14003, "the user is already enabled")
	// ErrUserCannotModified is the error of user cannot modified
	ErrUserCannotModified = NewBcode(400, 14004, "the user cannot be modified in dex login mode")
	// ErrUserInvalidPassword is the error of user invalid password
	ErrUserInvalidPassword = NewBcode(400, 14005, "the password is invalid")
	// ErrDexConfigNotFound means the dex config is not configured
	ErrDexConfigNotFound = NewBcode(200, 14006, "the dex config is not found")
	// ErrUserInconsistentPassword is the error of user inconsistent password
	ErrUserInconsistentPassword = NewBcode(401, 14007, "the password is inconsistent with the user")
	// ErrUsernameNotExist is the error of username not exist
	ErrUsernameNotExist = NewBcode(401, 14008, "the username is not exist")
	// ErrDexNotFound is the error of dex not found
	ErrDexNotFound = NewBcode(200, 14009, "the dex is not found")
	// ErrEmptyAdminEmail is the error of empty admin email
	ErrEmptyAdminEmail = NewBcode(400, 14010, "the admin email is empty, please set the admin email before using sso login")
)
