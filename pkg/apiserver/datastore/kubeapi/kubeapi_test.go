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

package kubeapi

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var testScheme = runtime.NewScheme()

var _ = BeforeSuite(func(done Done) {
	rand.Seed(time.Now().UnixNano())
	By("bootstrapping test environment")

	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute * 3,
		ControlPlaneStopTimeout:  time.Minute,
		UseExistingCluster:       pointer.BoolPtr(false),
	}

	By("start kube test env")
	var err error
	cfg, err = testEnv.Start()
	Expect(err).ShouldNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = scheme.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())

	By("new kube client")
	cfg.Timeout = time.Minute * 2
	k8sClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	Expect(err).Should(BeNil())
	Expect(k8sClient).ToNot(BeNil())
	By("new kube client success")
	close(done)
}, 240)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

var _ = Describe("Test kubeapi datastore driver", func() {

	clients.SetKubeClient(k8sClient)
	kubeStore, err := New(context.TODO(), datastore.Config{Database: "test"})
	Expect(err).Should(BeNil())
	Expect(kubeStore).ToNot(BeNil())

	It("Test add function", func() {
		err := kubeStore.Add(context.TODO(), &model.Application{Name: "kubevela-app", Description: "default"})
		Expect(err).ToNot(HaveOccurred())
	})

	It("Test batch add function", func() {
		var datas = []datastore.Entity{
			&model.Application{Name: "kubevela-app-2", Description: "this is demo 2"},
			&model.Application{Name: "kubevela-app-3", Description: "this is demo 3"},
			&model.Application{Name: "kubevela-app-4", Description: "this is demo 4"},
		}
		err := kubeStore.BatchAdd(context.TODO(), datas)
		Expect(err).ToNot(HaveOccurred())

		var datas2 = []datastore.Entity{
			&model.Application{Name: "can-delete", Description: "this is demo can-delete"},
			&model.Application{Name: "kubevela-app-2", Description: "this is demo 2"},
		}
		err = kubeStore.BatchAdd(context.TODO(), datas2)
		equal := cmp.Diff(strings.Contains(err.Error(), "save components occur error"), true)
		Expect(equal).To(BeEmpty())
	})

	It("Test get function", func() {
		app := &model.Application{Name: "kubevela-app"}
		err := kubeStore.Get(context.TODO(), app)
		Expect(err).Should(BeNil())
		diff := cmp.Diff(app.Description, "default")
		Expect(diff).Should(BeEmpty())
	})

	It("Test put function", func() {
		err := kubeStore.Put(context.TODO(), &model.Application{Name: "kubevela-app", Description: "this is demo"})
		Expect(err).ToNot(HaveOccurred())
	})
	It("Test index", func() {
		var app = model.Application{
			Name: "test",
		}
		selector, err := labels.Parse(fmt.Sprintf("table=%s", app.TableName()))
		Expect(err).ToNot(HaveOccurred())
		Expect(cmp.Diff(app.Index()["name"], "test")).Should(BeEmpty())
		for k, v := range app.Index() {
			rq, err := labels.NewRequirement(k, selection.Equals, []string{v})
			Expect(err).ToNot(HaveOccurred())
			selector = selector.Add(*rq)
		}
		Expect(cmp.Diff(selector.String(), "name=test,table=vela_application")).Should(BeEmpty())
	})
	It("Test list function", func() {
		var app model.Application
		list, err := kubeStore.List(context.TODO(), &app, &datastore.ListOptions{Page: -1})
		Expect(err).ShouldNot(HaveOccurred())
		diff := cmp.Diff(len(list), 4)
		Expect(diff).Should(BeEmpty())

		list, err = kubeStore.List(context.TODO(), &app, &datastore.ListOptions{Page: 2, PageSize: 2})
		Expect(err).ShouldNot(HaveOccurred())
		diff = cmp.Diff(len(list), 2)
		Expect(diff).Should(BeEmpty())

		list, err = kubeStore.List(context.TODO(), &app, &datastore.ListOptions{Page: 1, PageSize: 2})
		Expect(err).ShouldNot(HaveOccurred())
		diff = cmp.Diff(len(list), 2)
		Expect(diff).Should(BeEmpty())

		list, err = kubeStore.List(context.TODO(), &app, nil)
		Expect(err).ShouldNot(HaveOccurred())
		diff = cmp.Diff(len(list), 4)
		Expect(diff).Should(BeEmpty())
	})

	It("Test list clusters with sort and fuzzy query", func() {
		clusters, err := kubeStore.List(context.TODO(), &model.Cluster{}, nil)
		Expect(err).Should(Succeed())
		for _, cluster := range clusters {
			Expect(kubeStore.Delete(context.TODO(), cluster)).Should(Succeed())
		}
		for _, name := range []string{"first", "second", "third"} {
			Expect(kubeStore.Add(context.TODO(), &model.Cluster{Name: name})).Should(Succeed())
			time.Sleep(time.Millisecond * 100)
		}
		entities, err := kubeStore.List(context.TODO(), &model.Cluster{}, &datastore.ListOptions{SortBy: []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderAscending}}})
		Expect(err).Should(Succeed())
		Expect(len(entities)).Should(Equal(3))
		for i, name := range []string{"first", "second", "third"} {
			Expect(entities[i].(*model.Cluster).Name).Should(Equal(name))
		}

		entities, err = kubeStore.List(context.TODO(), &model.Cluster{}, &datastore.ListOptions{
			SortBy:   []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}},
			Page:     1,
			PageSize: 2,
		})
		Expect(err).Should(Succeed())
		Expect(len(entities)).Should(Equal(2))
		for i, name := range []string{"third", "second"} {
			Expect(entities[i].(*model.Cluster).Name).Should(Equal(name))
		}

		entities, err = kubeStore.List(context.TODO(), &model.Cluster{}, &datastore.ListOptions{
			SortBy:   []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}},
			Page:     2,
			PageSize: 2,
		})
		Expect(err).Should(Succeed())
		Expect(len(entities)).Should(Equal(1))
		for i, name := range []string{"first"} {
			Expect(entities[i].(*model.Cluster).Name).Should(Equal(name))
		}
		entities, err = kubeStore.List(context.TODO(), &model.Cluster{}, &datastore.ListOptions{
			SortBy: []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}},
			FilterOptions: datastore.FilterOptions{
				Queries: []datastore.FuzzyQueryOption{{Key: "name", Query: "ir"}},
			},
		})
		Expect(err).Should(Succeed())
		Expect(len(entities)).Should(Equal(2))
		for i, name := range []string{"third", "first"} {
			Expect(entities[i].(*model.Cluster).Name).Should(Equal(name))
		}
	})

	It("Test count function", func() {
		var app model.Application
		count, err := kubeStore.Count(context.TODO(), &app, nil)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).Should(Equal(int64(4)))

		count, err = kubeStore.Count(context.TODO(), &model.Cluster{}, &datastore.FilterOptions{
			Queries: []datastore.FuzzyQueryOption{{Key: "name", Query: "ir"}},
		})
		Expect(err).Should(Succeed())
		Expect(count).Should(Equal(int64(2)))
	})

	It("Test isExist function", func() {
		var app model.Application
		app.Name = "kubevela-app-3"
		exist, err := kubeStore.IsExist(context.TODO(), &app)
		Expect(err).ShouldNot(HaveOccurred())
		diff := cmp.Diff(exist, true)
		Expect(diff).Should(BeEmpty())

		app.Name = "kubevela-app-5"
		notexist, err := kubeStore.IsExist(context.TODO(), &app)
		Expect(err).ShouldNot(HaveOccurred())
		diff = cmp.Diff(notexist, false)
		Expect(diff).Should(BeEmpty())
	})

	It("Test delete function", func() {
		var app model.Application
		app.Name = "kubevela-app"
		err := kubeStore.Delete(context.TODO(), &app)
		Expect(err).ShouldNot(HaveOccurred())

		app.Name = "kubevela-app-2"
		err = kubeStore.Delete(context.TODO(), &app)
		Expect(err).ShouldNot(HaveOccurred())

		app.Name = "kubevela-app-3"
		err = kubeStore.Delete(context.TODO(), &app)
		Expect(err).ShouldNot(HaveOccurred())

		app.Name = "kubevela-app-4"
		err = kubeStore.Delete(context.TODO(), &app)
		Expect(err).ShouldNot(HaveOccurred())

		app.Name = "kubevela-app-4"
		err = kubeStore.Delete(context.TODO(), &app)
		equal := cmp.Equal(err, datastore.ErrRecordNotExist, cmpopts.EquateErrors())
		Expect(equal).Should(BeTrue())
	})
})
