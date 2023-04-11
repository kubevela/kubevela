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

package registries

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-containerregistry/pkg/authn"
)

func buildImageRegistry(registry, username, password string, insecure bool, useHTTP bool) *ImageRegistry {
	imageRegistry := &ImageRegistry{
		Registry: registry,
		Auth:     Auth{Username: username, Password: password},
		Insecure: insecure,
		UseHTTP:  useHTTP,
	}

	return imageRegistry
}

func TestSecretAuthenticator(t *testing.T) {
	imageRegistry := buildImageRegistry("dockerhub.qingcloud.com", "guest", "guest", false, false)

	secretAuthenticator, err := NewSecretAuthenticator(imageRegistry)
	if err != nil {
		t.Fatal(err)
	}

	auth, err := secretAuthenticator.Authorization()
	if err != nil {
		t.Fatal(err)
	}

	expected := &authn.AuthConfig{
		Username: "guest",
		Password: "guest",
		Auth:     "Z3Vlc3Q6Z3Vlc3Q=",
	}

	if diff := cmp.Diff(auth, expected); len(diff) != 0 {
		t.Errorf("%T, got+ expected-, %s", expected, diff)
	}
}

func TestAuthn(t *testing.T) {
	testCases := []struct {
		name          string
		imageRegistry *ImageRegistry
		auth          bool
		expectErr     bool
	}{
		{
			name:          "Should authenticate with correct credential",
			imageRegistry: buildImageRegistry("dockerhub.qingcloud.com", "guest", "guest", false, false),
			auth:          true,
			expectErr:     false,
		},
		{
			name:          "Shouldn't authenticate with incorrect credentials",
			imageRegistry: buildImageRegistry("index.docker.io", "foo", "bar", false, false),
			auth:          false,
			expectErr:     true,
		},
		{
			name:          "Shouldn't authenticate with no credentials",
			imageRegistry: nil,
			auth:          false,
			expectErr:     true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			secretAuthenticator, err := NewSecretAuthenticator(testCase.imageRegistry)
			if err != nil {
				t.Errorf("error creating secretAuthenticator, %v", err)
			}

			ok, err := secretAuthenticator.Auth(context.Background())
			if testCase.auth != ok {
				t.Errorf("expected auth result: %v, but got %v", testCase.auth, ok)
			}

			if testCase.expectErr && err == nil {
				t.Errorf("expected error, but got nil")
			}

			if !testCase.expectErr && err != nil {
				t.Errorf("authentication error, %v", err)
			}
		})
	}
}
