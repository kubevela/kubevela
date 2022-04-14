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

var (
	// ErrUnsupportedLoginType is the error of unsupported login type
	ErrUnsupportedLoginType = NewBcode(401, 12001, "the login type is not supported")
	// ErrTokenExpired is the error of token expired
	ErrTokenExpired = NewBcode(401, 12002, "the token is expired")
	// ErrTokenNotValidYet is the error of token not valid yet
	ErrTokenNotValidYet = NewBcode(401, 12003, "the token is not valid yet")
	// ErrTokenInvalid is the error of token invalid
	ErrTokenInvalid = NewBcode(401, 12004, "the token is invalid")
	// ErrTokenMalformed is the error of token malformed
	ErrTokenMalformed = NewBcode(403, 12005, "the token is malformed")
	// ErrNotAuthorized is the error of not authorized
	ErrNotAuthorized = NewBcode(401, 12006, "the user is not authorized")
	// ErrNotAccessToken is the error of not access token
	ErrNotAccessToken = NewBcode(401, 12007, "the token is not an access token")
	// ErrInvalidLoginRequest is the error of invalid login request
	ErrInvalidLoginRequest = NewBcode(400, 12008, "the login request is invalid")
	// ErrInvalidDexConfig is the error of invalid dex config
	ErrInvalidDexConfig = NewBcode(400, 12009, "the dex config is invalid")
	// ErrRefreshTokenExpired is the error of refresh token expired
	ErrRefreshTokenExpired = NewBcode(400, 12010, "the refresh token is expired")
	// ErrNoDexConnector is the error of no dex connector
	ErrNoDexConnector = NewBcode(400, 12011, "there is no dex connector")
)
