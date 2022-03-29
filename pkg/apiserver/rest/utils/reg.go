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

package utils

import (
	"fmt"

	"github.com/heroku/docker-registry-client/registry"
)

// Registry is the metadata for an image registry.
type Registry struct {
	Registry string `json:"registry"`
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
}

// Image is the metadata for an image.
type Image struct {
	Name string `json:"name"`
	Tag  string `json:"tag"`
}

// GetPrivateImages returns a list of private images.
func (r Registry) GetPrivateImages() ([]Image, error) {
	url := r.Registry
	username := r.Username
	password := r.Password
	hub, err := registry.New(url, username, password)
	if err != nil {
		return nil, err
	}
	repositories, err := hub.Repositories()
	if err != nil {
		return nil, err
	}
	fmt.Println(repositories)
	return nil, nil
}
