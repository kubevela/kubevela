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

package auth

import (
	"k8s.io/apiserver/pkg/authentication/user"

	"github.com/oam-dev/kubevela/apis/types"
)

const (
	// DefaultAuthenticateGroupPattern default value of groups patterns for authentication
	DefaultAuthenticateGroupPattern = types.KubeVelaName + ":*"
)

var (
	// AuthenticationWithUser flag for enable the authentication of User in requests
	AuthenticationWithUser = false
	// AuthenticationDefaultUser the default user to use while no User is set in application
	AuthenticationDefaultUser = user.Anonymous
	// AuthenticationGroupPattern pattern for the authentication of Group in requests
	AuthenticationGroupPattern = DefaultAuthenticateGroupPattern
)
