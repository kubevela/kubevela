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
	"context"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	v1 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"

	types2 "github.com/oam-dev/kubevela/apis/types"
	util2 "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

var _ = Describe("Test helm helper", func() {

	It("Test LoadCharts ", func() {
		helper := NewHelper()
		chart, err := helper.LoadCharts("./testdata/autoscalertrait-0.1.0.tgz", nil)
		Expect(err).Should(BeNil())
		Expect(chart).ShouldNot(BeNil())
		Expect(chart.Metadata).ShouldNot(BeNil())
		Expect(cmp.Diff(chart.Metadata.Version, "0.1.0")).Should(BeEmpty())
	})

	It("Test UpgradeChart", func() {
		helper := NewHelper()
		chart, err := helper.LoadCharts("./testdata/autoscalertrait-0.1.0.tgz", nil)
		Expect(err).Should(BeNil())
		release, err := helper.UpgradeChart(chart, "autoscalertrait", "default", nil, UpgradeChartOptions{
			Config:  cfg,
			Detail:  false,
			Logging: util.IOStreams{Out: os.Stdout, ErrOut: os.Stderr},
			Wait:    false,
		})
		crds := GetCRDFromChart(release.Chart)
		Expect(cmp.Diff(len(crds), 1)).Should(BeEmpty())
		Expect(err).Should(BeNil())
	})

	It("Test UninstallRelease", func() {
		helper := NewHelper()
		err := helper.UninstallRelease("autoscalertrait", "default", cfg, false, util.IOStreams{Out: os.Stdout, ErrOut: os.Stderr})
		Expect(err).Should(BeNil())
	})

	It("Test ListVersions ", func() {
		helper := NewHelper()
		versions, err := helper.ListVersions("./testdata", "autoscalertrait", true, nil)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(versions), 2)).Should(BeEmpty())
	})

	It("Test getValues from chart", func() {
		helper := NewHelper()
		values, err := helper.GetValuesFromChart("./testdata", "autoscalertrait", "0.2.0", true, nil)
		Expect(err).Should(BeNil())
		Expect(values).ShouldNot(BeEmpty())
	})
})

var _ = Describe("Test helm associated func", func() {
	ctx := context.Background()
	var aSec v1.Secret

	BeforeEach(func() {
		Expect(k8sClient.Create(ctx, &v1.Namespace{ObjectMeta: v12.ObjectMeta{Name: "vela-system"}})).Should(SatisfyAny(BeNil(), util2.AlreadyExistMatcher{}))
		aSec = v1.Secret{}
		Expect(yaml.Unmarshal([]byte(authSecret), &aSec)).Should(BeNil())
		Expect(k8sClient.Create(ctx, &aSec)).Should(SatisfyAny(BeNil(), util2.AlreadyExistMatcher{}))
	})

	It("Test auth info secret func", func() {
		opts, err := SetBasicAuthInfo(context.Background(), k8sClient, types.NamespacedName{Namespace: types2.DefaultKubeVelaNS, Name: "auth-secret"})
		Expect(err).Should(BeNil())
		Expect(opts.Username).Should(BeEquivalentTo("admin"))
		Expect(opts.Password).Should(BeEquivalentTo("admin"))
	})

	It("Test auth info secret func", func() {
		_, err := SetBasicAuthInfo(context.Background(), k8sClient, types.NamespacedName{Namespace: types2.DefaultKubeVelaNS, Name: "auth-secret-1"})
		Expect(err).ShouldNot(BeNil())
	})
})

var (
	authSecret = `
apiVersion: v1
kind: Secret
metadata:
  name: auth-secret
  namespace: vela-system
  labels:
    config.oam.dev/type: config-helm-repository
    config.oam.dev/project: my-project-1
stringData:
  url: https://kedacore.github.io/charts
  username: admin
  password: admin
type: Opaque
`
)
