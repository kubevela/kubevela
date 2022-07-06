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

package collect

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/onsi/gomega/format"

	"gotest.tools/assert"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test calculate cronJob", func() {
	var (
		ds          datastore.DataStore
		testProject string
		i           InfoCalculateCronJob
		ctx         = context.Background()
	)

	mockDataInDs := func() {
		app1 := model.Application{BaseModel: model.BaseModel{CreateTime: time.Now()}, Name: "app1", Project: testProject}
		app2 := model.Application{BaseModel: model.BaseModel{CreateTime: time.Now()}, Name: "app2", Project: testProject}
		trait1 := model.ApplicationTrait{Type: "rollout"}
		trait2 := model.ApplicationTrait{Type: "expose"}
		trait3 := model.ApplicationTrait{Type: "rollout"}
		trait4 := model.ApplicationTrait{Type: "patch"}
		trait5 := model.ApplicationTrait{Type: "patch"}
		trait6 := model.ApplicationTrait{Type: "rollout"}
		appComp1 := model.ApplicationComponent{AppPrimaryKey: app1.PrimaryKey(), Name: "comp1", Type: "helm", Traits: []model.ApplicationTrait{trait1, trait4}}
		appComp2 := model.ApplicationComponent{AppPrimaryKey: app2.PrimaryKey(), Name: "comp2", Type: "webservice", Traits: []model.ApplicationTrait{trait3}}
		appComp3 := model.ApplicationComponent{AppPrimaryKey: app2.PrimaryKey(), Name: "comp3", Type: "webservice", Traits: []model.ApplicationTrait{trait2, trait5, trait6}}
		Expect(ds.Add(ctx, &app1)).Should(SatisfyAny(BeNil(), DataExistMatcher{}))
		Expect(ds.Add(ctx, &app2)).Should(SatisfyAny(BeNil(), DataExistMatcher{}))
		Expect(ds.Add(ctx, &appComp1)).Should(SatisfyAny(BeNil(), DataExistMatcher{}))
		Expect(ds.Add(ctx, &appComp2)).Should(SatisfyAny(BeNil(), DataExistMatcher{}))
		Expect(ds.Add(ctx, &appComp3)).Should(SatisfyAny(BeNil(), DataExistMatcher{}))
		Expect(k8sClient.Create(ctx, &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vela-system"}})).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		Expect(k8sClient.Create(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Namespace: "vela-system", Name: "addon-fluxcd", Labels: map[string]string{oam.LabelAddonName: "fluxcd"}}, Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{},
		}})).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		Expect(k8sClient.Create(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Namespace: "vela-system", Name: "addon-rollout", Labels: map[string]string{oam.LabelAddonName: "rollout"}}, Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{},
		}})).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
	}

	BeforeEach(func() {
		var err error
		ds, err = NewDatastore(datastore.Config{Type: "kubeapi", Database: "target-test-kubevela"})
		Expect(ds).ShouldNot(BeNil())
		Expect(err).Should(BeNil())

		testProject = "test-cronjob-project"
		mockDataInDs()
		i = InfoCalculateCronJob{
			Store: ds,
		}
		systemInfo := model.SystemInfo{InstallID: "test-id", EnableCollection: true}
		Expect(ds.Add(ctx, &systemInfo)).Should(SatisfyAny(BeNil(), DataExistMatcher{}))
	})

	It("Test calculate app Info", func() {
		appNum, topKCom, topKTrait, _, _, err := i.calculateAppInfo(ctx)
		Expect(err).Should(BeNil())
		Expect(appNum).Should(BeEquivalentTo(2))
		Expect(topKCom).Should(BeEquivalentTo([]string{"webservice", "helm"}))
		Expect(topKTrait).Should(BeEquivalentTo([]string{"rollout", "patch", "expose"}))
	})

	It("Test calculate addon Info", func() {
		enabledAddon, err := i.calculateAddonInfo(ctx)
		Expect(err).Should(BeNil())
		Expect(enabledAddon).Should(BeEquivalentTo(map[string]string{
			"fluxcd":  "enabling",
			"rollout": "enabling",
		}))
	})

	It("Test calculate cluster Info", func() {
		clusterNum, err := i.calculateClusterInfo(ctx)
		Expect(err).Should(BeNil())
		Expect(clusterNum).Should(BeEquivalentTo(1))
	})

	It("Test calculateAndUpdate func", func() {
		systemInfo := model.SystemInfo{}
		es, err := ds.List(ctx, &systemInfo, &datastore.ListOptions{})
		Expect(err).Should(BeNil())
		Expect(len(es)).Should(BeEquivalentTo(1))
		info, ok := es[0].(*model.SystemInfo)
		Expect(ok).Should(BeTrue())
		Expect(info.InstallID).Should(BeEquivalentTo("test-id"))

		Expect(i.calculateAndUpdate(ctx, *info)).Should(BeNil())

		systemInfo = model.SystemInfo{}
		es, err = ds.List(ctx, &systemInfo, &datastore.ListOptions{})
		Expect(err).Should(BeNil())
		Expect(len(es)).Should(BeEquivalentTo(1))
		info, ok = es[0].(*model.SystemInfo)
		Expect(ok).Should(BeTrue())
		Expect(info.InstallID).Should(BeEquivalentTo("test-id"))
		Expect(info.StatisticInfo.AppCount).Should(BeEquivalentTo("<10"))
		Expect(info.StatisticInfo.ClusterCount).Should(BeEquivalentTo("<3"))
		Expect(info.StatisticInfo.TopKCompDef).Should(BeEquivalentTo([]string{"webservice", "helm"}))
		Expect(info.StatisticInfo.TopKTraitDef).Should(BeEquivalentTo([]string{"rollout", "patch", "expose"}))
		Expect(info.StatisticInfo.EnabledAddon).Should(BeEquivalentTo(map[string]string{
			"fluxcd":  "enabling",
			"rollout": "enabling",
		}))
	})

	It("Test run func", func() {
		app3 := model.Application{BaseModel: model.BaseModel{CreateTime: time.Now()}, Name: "app3", Project: testProject}
		Expect(ds.Add(ctx, &app3)).Should(BeNil())

		systemInfo := model.SystemInfo{InstallID: "test-id", EnableCollection: false}
		Expect(ds.Put(ctx, &systemInfo)).Should(BeNil())
		Expect(i.run()).Should(BeNil())
	})
})

func TestGenCountInfo(t *testing.T) {
	testcases := []struct {
		count int
		res   string
	}{
		{
			count: 3,
			res:   "<10",
		},
		{
			count: 14,
			res:   "<50",
		},
		{
			count: 80,
			res:   "<100",
		},
		{
			count: 350,
			res:   "<500",
		},
		{
			count: 1800,
			res:   "<2000",
		},
		{
			count: 4000,
			res:   "<5000",
		},
		{
			count: 9000,
			res:   "<10000",
		},
		{
			count: 30000,
			res:   ">=10000",
		},
	}
	for _, testcase := range testcases {
		assert.Equal(t, genCountInfo(testcase.count), testcase.res)
	}
}

func TestGenClusterCountInfo(t *testing.T) {
	testcases := []struct {
		count int
		res   string
	}{
		{
			count: 2,
			res:   "<3",
		},
		{
			count: 7,
			res:   "<10",
		},
		{
			count: 34,
			res:   "<50",
		},
		{
			count: 90,
			res:   "<100",
		},
		{
			count: 110,
			res:   "<125",
		},
		{
			count: 137,
			res:   "<150",
		},
		{
			count: 170,
			res:   "<200",
		},
		{
			count: 400,
			res:   "<500",
		},
		{
			count: 520,
			res:   ">=500",
		},
	}
	for _, testcase := range testcases {
		assert.Equal(t, genClusterCountInfo(testcase.count), testcase.res)
	}
}

func TestTopKFrequent(t *testing.T) {
	testCases := []struct {
		def map[string]int
		k   int
		res []string
	}{
		{
			def: map[string]int{
				"rollout": 4,
				"patch":   3,
				"expose":  6,
			},
			k:   3,
			res: []string{"expose", "rollout", "patch"},
		},
		{
			// just return top2
			def: map[string]int{
				"rollout": 4,
				"patch":   3,
				"expose":  6,
			},
			k:   2,
			res: []string{"expose", "rollout"},
		},
	}
	for _, testCase := range testCases {
		assert.DeepEqual(t, topKFrequent(testCase.def, testCase.k), testCase.res)
	}
}

type DataExistMatcher struct{}

// Match matches error.
func (matcher DataExistMatcher) Match(actual interface{}) (success bool, err error) {
	if actual == nil {
		return false, nil
	}
	actualError := actual.(error)
	return errors.Is(actualError, datastore.ErrRecordExist), nil
}

// FailureMessage builds an error message.
func (matcher DataExistMatcher) FailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "to be already exist")
}

// NegatedFailureMessage builds an error message.
func (matcher DataExistMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "not to be already exist")
}
