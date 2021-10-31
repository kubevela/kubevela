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

package cloudprovider

const (
	// ProviderAliyun cloud provider aliyun
	ProviderAliyun = "aliyun"
)

// CloudCluster describes the interface that cloud provider should return
type CloudCluster struct {
	Provider     string            `json:"provider"`
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	Zone         string            `json:"zone"`
	Labels       map[string]string `json:"labels"`
	Status       string            `json:"status"`
	APIServerURL string            `json:"apiServerURL"`
	DashBoardURL string            `json:"dashboardURL"`
}
