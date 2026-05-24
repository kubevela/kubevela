/*
Copyright 2026 The KubeVela Authors.

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

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("types", func() {

	Describe("RenderParams structure", func() {
		It("should hold all fields correctly", func() {
			params := &RenderParams{
				Chart: ChartSourceParams{
					Source:  "nginx",
					RepoURL: "https://charts.bitnami.com/bitnami",
					Version: "1.0.0",
				},
				Release: &ReleaseParams{
					Name:      "my-release",
					Namespace: "my-namespace",
				},
				Values: map[string]interface{}{
					"replicaCount": 2,
				},
				Context: &ContextParams{
					AppName:      "my-app",
					AppNamespace: "my-app-ns",
					Name:         "nginx-component",
					Namespace:    "my-namespace",
				},
			}

			Expect(params.Chart.Source).To(Equal("nginx"))
			Expect(params.Release.Name).To(Equal("my-release"))
			Expect(params.Values.(map[string]interface{})["replicaCount"]).To(Equal(2))
			Expect(params.Context.AppName).To(Equal("my-app"))
			Expect(params.Context.Name).To(Equal("nginx-component"))
		})
	})

	Describe("return types", func() {
		It("RenderReturns should hold resources and notes", func() {
			ret := RenderReturns{
				Resources: []map[string]interface{}{
					{"kind": "Deployment", "metadata": map[string]interface{}{"name": "test"}},
				},
				Notes: "install notes",
			}
			Expect(ret.Resources).To(HaveLen(1))
			Expect(ret.Notes).To(Equal("install notes"))
		})
	})

})
