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

package image

import (
	"fmt"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// Meta is the struct for image metadata
type Meta struct {
	Registry   string
	Repository string
	Name       string
	Tag        string
}

// DockerHubImageTagResponse is the struct for docker hub image tag response
type DockerHubImageTagResponse struct {
	Count   int `json:"count"`
	Results []Result
}

// Result is the struct for docker hub image tag result
type Result struct {
	Name string `json:"name"`
}

// IsExisted checks whether a public or private image exists
func IsExisted(username, password, image string) (bool, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return false, err
	}

	var img v1.Image

	if username != "" || password != "" {
		basic := &authn.Basic{
			Username: username,
			Password: password,
		}

		option := remote.WithAuth(basic)
		img, err = remote.Image(ref, option)
		if err != nil {
			return false, err
		}
	} else {
		img, err = remote.Image(ref)
		if err != nil {
			return false, err
		}
	}

	_, err = img.Digest()
	if err != nil {
		return false, err
	}

	m, err := img.Manifest()
	if err != nil {
		return false, err
	}
	fmt.Println(m)

	return true, nil
}

// RegistryMeta is the struct for registry metadata
type RegistryMeta struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
