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

package helm

// RepoCredential is the helm repo credential
type RepoCredential struct {
	// chart repository username
	Username string `json:"username,omitempty"`
	// chart repository password
	Password string `json:"password,omitempty"`
	// identify HTTPS client using this SSL certificate file
	CertFile string `json:"certFile,omitempty"`
	// identify HTTPS client using this SSL key file
	KeyFile string `json:"keyFile,omitempty"`
	// verify certificates of HTTPS-enabled servers using this CA bundle
	CAFile string `json:"caFile,omitempty"`
	// skip tls certificate checks for the repository, default is ture
	InsecureSkipTLSVerify *bool `json:"insecureSkipTLSVerify,omitempty"`
}

// Repository is the helm repository
type Repository struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
	CAFile   string `json:"caFile"`
}
