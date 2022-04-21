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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/valyala/fasthttp"

	"github.com/heroku/docker-registry-client/registry"
)

const (
	dockerHub            = "hub.docker.com"
	dockerIO             = "docker.io"
	dockerHubDefaultRepo = "library"
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
	meta, err := retrieveImageMeta(image)
	if err != nil {
		return false, err
	}

	if username != "" || password != "" {
		hub, err := registry.New("https://"+meta.Registry, username, password)
		if err != nil {
			return false, err
		}
		digest, err := hub.ManifestDigest(meta.Repository+"/"+meta.Name, meta.Tag)
		if err != nil {
			return false, err
		}
		if digest == "" {
			return false, fmt.Errorf("image %s not found as its degest is empty", image)
		}
		return true, nil
	}

	switch meta.Registry {
	case dockerHub:
		api := fmt.Sprintf("https://%s/v2/repositories/%s/%s/tags?page_size=10000", meta.Registry, meta.Repository, meta.Name)
		statusCode, body, err := fasthttp.Get(nil, api) //nolint:gosec
		if err != nil {
			return false, err
		}
		if statusCode == 200 {
			var r DockerHubImageTagResponse
			var tagExisted bool
			if err := json.Unmarshal(body, &r); err == nil {
				for _, result := range r.Results {
					if result.Name == meta.Tag {
						tagExisted = true
						break
					}
				}
			}
			if tagExisted {
				return true, nil
			}
			return false, fmt.Errorf("image %s not found as its tag %s is not existed", meta.Name, meta.Tag)
		}
		return false, nil
	default:
		return false, fmt.Errorf("image doesn't exist as its registry %s is not supported yet", meta.Registry)
	}
}

func retrieveImageMeta(image string) (*Meta, error) {
	var (
		reg  string
		repo string
		name string
		tag  string
	)
	if image == "" {
		return nil, fmt.Errorf("image is empty")
	}
	meta := strings.Split(image, ":")
	if len(meta) == 1 {
		tag = "latest"
	} else {
		tag = meta[1]
	}

	tmp := strings.Split(meta[0], "/")
	switch len(tmp) {
	case 1:
		reg = dockerHub
		repo = dockerHubDefaultRepo
		name = tmp[0]
	case 2:
		if tmp[0] == dockerIO {
			repo = dockerHubDefaultRepo
		} else {
			repo = tmp[0]
		}
		reg = dockerHub
		name = tmp[1]
	case 3:
		if tmp[0] == dockerIO {
			reg = dockerHub
		} else {
			reg = tmp[0]
		}
		repo = tmp[1]
		name = tmp[2]
	}
	return &Meta{Registry: reg, Repository: repo, Name: name, Tag: tag}, nil
}

// RegistryMeta is the struct for registry metadata
type RegistryMeta struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
