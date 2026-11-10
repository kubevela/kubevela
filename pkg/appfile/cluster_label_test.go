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

package appfile

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/oam"
)

// An Application's own app.oam.dev/cluster label says where that Application
// was dispatched. It is not copied onto the objects it renders: dispatch only
// sets the cluster on an object that has none, so an inherited value would
// override the Application's own topology.
var _ = Describe("Common labels", func() {
	It("does not copy the Application's cluster label onto its objects", func() {
		af := &Appfile{
			Name:            "module-store",
			Namespace:       "vela-system",
			AppRevisionName: "module-store-v1",
			AppLabels: map[string]string{
				oam.LabelAppCluster:   "local",
				"addons.oam.dev/name": "placement-platform",
			},
		}

		labels := af.generateAndFilterCommonLabels("store-aux")

		Expect(labels).NotTo(HaveKey(oam.LabelAppCluster))
		Expect(labels).To(HaveKeyWithValue("addons.oam.dev/name", "placement-platform"), "other Application labels are still copied")
		Expect(labels).To(HaveKeyWithValue(oam.LabelAppName, "module-store"))
	})
})
