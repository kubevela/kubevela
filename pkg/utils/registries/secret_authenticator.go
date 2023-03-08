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
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	v1 "k8s.io/api/core/v1"
)

// SecretAuthenticator provides helper functions for secret authenticator operations
type SecretAuthenticator interface {
	Options() []Option

	Auth() (bool, error)

	Authorization() (*authn.AuthConfig, error)
}

type secretAuthenticator struct {
	auths    DockerConfig
	insecure bool // force using insecure when talk to the remote registry, even registry address starts with https
}

// NewSecretAuthenticator creates a secret authenticator
func NewSecretAuthenticator(secret *v1.Secret) (SecretAuthenticator, error) {

	if secret == nil {
		return &secretAuthenticator{}, nil
	}

	sa := &secretAuthenticator{
		insecure: false,
	}

	if secret.Type != v1.SecretTypeDockerConfigJson {
		return nil, fmt.Errorf("expected secret type: %s, got: %s", v1.SecretTypeDockerConfigJson, secret.Type)
	}

	// force insecure if secret has Data insecure-skip-verify
	if val, ok := secret.Data["insecure-skip-verify"]; ok && string(val) == "true" {
		sa.insecure = true
	}

	configJSON, ok := secret.Data[v1.DockerConfigJsonKey]
	if !ok {
		return nil, fmt.Errorf("expected key %s in data, found none", v1.DockerConfigJsonKey)
	}

	dockerConfigJSON := DockerConfigJSON{}
	if err := json.Unmarshal(configJSON, &dockerConfigJSON); err != nil {
		return nil, err
	}

	if len(dockerConfigJSON.Auths) == 0 {
		return nil, fmt.Errorf("not found valid auth in secret, %v", dockerConfigJSON)
	}

	sa.auths = map[string]DockerConfigEntry{}
	for key, value := range dockerConfigJSON.Auths {
		if val, ok := secret.Data["protocol-use-http"]; ok && string(val) == "true" {
			sa.auths[fmt.Sprintf("http://%s", key)] = value
		} else {
			sa.auths[fmt.Sprintf("https://%s", key)] = value
		}
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

func (s *secretAuthenticator) Auth() (bool, error) {
	for k := range s.auths {
		return s.AuthRegistry(k)
	}
	return false, fmt.Errorf("no registry found in secret")
}

func (s *secretAuthenticator) AuthRegistry(reg string) (bool, error) {
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

	ctx := context.TODO()
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
