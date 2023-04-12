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

// DockerConfig represents the config file used by the docker CLI.
// This config that represents the credentials that should be used
// when pulling images from specific image repositories.
type DockerConfig map[string]DockerConfigEntry

// DockerConfigEntry wraps a docker config as an entry
type DockerConfigEntry struct {
	Username string
	Password string
	Email    string
	Auth     string
}

// ImageRegistry the request body for validating image registry
type ImageRegistry struct {
	Registry string `json:"registry"`
	Auth     Auth   `json:"auth"`
	Insecure bool   `json:"insecure"`
	UseHTTP  bool   `json:"useHTTP"`
}

// Auth the auth of image registry
type Auth struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
}
