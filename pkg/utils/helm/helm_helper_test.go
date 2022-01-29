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

package helm

import (
	"os"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/utils/util"
)

var _ = Describe("Test helm helper", func() {

	It("Test LoadCharts ", func() {
		helper := NewHelper()
		chart, err := helper.LoadCharts("./testdata/autoscalertrait-0.1.0.tgz")
		Expect(err).Should(BeNil())
		Expect(chart).ShouldNot(BeNil())
		Expect(chart.Metadata).ShouldNot(BeNil())
		Expect(cmp.Diff(chart.Metadata.Version, "0.1.0")).Should(BeEmpty())
	})

	It("Test UpgradeChart", func() {
		helper := NewHelper()
		chart, err := helper.LoadCharts("./testdata/autoscalertrait-0.1.0.tgz")
		Expect(err).Should(BeNil())
		_, err = helper.UpgradeChart(chart, "autoscalertrait", "default", nil, UpgradeChartOptions{
			Config:  cfg,
			Detail:  false,
			Logging: util.IOStreams{Out: os.Stdout, ErrOut: os.Stderr},
			Wait:    false,
		})
		Expect(err).Should(BeNil())
	})

	It("Test UninstallRelease", func() {
		helper := NewHelper()
		err := helper.UninstallRelease("autoscalertrait", "default", cfg, false, util.IOStreams{Out: os.Stdout, ErrOut: os.Stderr})
		Expect(err).Should(BeNil())
	})

	It("Test ListVersions ", func() {
		helper := NewHelper()
		versions, err := helper.ListVersions("./testdata", "autoscalertrait")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(versions), 2)).Should(BeEmpty())
	})

})
