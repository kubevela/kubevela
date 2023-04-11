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
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
)

// SecretAuthenticator provides helper functions for secret authenticator operations
type SecretAuthenticator interface {
	Options() []Option

	Auth(ctx context.Context) (bool, error)

	Authorization() (*authn.AuthConfig, error)
}

type secretAuthenticator struct {
	auths    DockerConfig
	insecure bool // force using insecure when talk to the remote registry, even registry address starts with https
}

// NewSecretAuthenticator creates a secret authenticator
func NewSecretAuthenticator(imageRegistry *ImageRegistry) (SecretAuthenticator, error) {

	if imageRegistry == nil {
		return &secretAuthenticator{}, nil
	}

	sa := &secretAuthenticator{
		insecure: false,
	}

	// force insecure if imageRegistry has Insecure
	sa.insecure = imageRegistry.Insecure
	if imageRegistry.Insecure {
		sa.insecure = true
	}

	auth := fmt.Sprintf("%s:%s", imageRegistry.Auth.Username, imageRegistry.Auth.Password)

	entry := DockerConfigEntry{
		Username: imageRegistry.Auth.Username,
		Password: imageRegistry.Auth.Password,
		Email:    imageRegistry.Auth.Email,
		Auth:     base64.StdEncoding.EncodeToString([]byte(auth)),
	}

	sa.auths = map[string]DockerConfigEntry{}
	if imageRegistry.UseHTTP {
		sa.auths[fmt.Sprintf("http://%s", imageRegistry.Registry)] = entry
	} else {
		sa.auths[fmt.Sprintf("https://%s", imageRegistry.Registry)] = entry
	}
	return sa, nil
}

func (s *secretAuthenticator) Authorization() (*authn.AuthConfig, error) {
	for _, v := range s.auths {
		return &authn.AuthConfig{
			Username: v.Username,
			Password: v.Password,
			Auth:     v.Auth,
		}, nil
	}
	return &authn.AuthConfig{}, nil
}

func (s *secretAuthenticator) Auth(ctx context.Context) (bool, error) {
	for k := range s.auths {
		return s.AuthRegistry(ctx, k)
	}
	return false, fmt.Errorf("no registry found in image-registry")
}

func (s *secretAuthenticator) AuthRegistry(ctx context.Context, reg string) (bool, error) {
	url, err := url.Parse(reg) // in case reg is unformatted like http://docker.index.io
	if err != nil {
		return false, err
	}

	options := make([]name.Option, 0)
	if url.Scheme == "http" || s.insecure {
		options = append(options, name.Insecure)
	}

	registry, err := name.NewRegistry(url.Host, options...)
	if err != nil {
		return false, err
	}

	_, err = transport.NewWithContext(ctx, registry, s, http.DefaultTransport, []string{})
	if err != nil {
		return false, err
	}

	return true, nil
}

func (s *secretAuthenticator) Options() []Option {
	options := make([]Option, 0)

	options = append(options, WithAuth(s))
	if s.insecure {
		options = append(options, Insecure)
	}

	return options
}
