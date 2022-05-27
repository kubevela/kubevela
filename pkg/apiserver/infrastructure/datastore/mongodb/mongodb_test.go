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

package mongodb

import (
	"context"
	"math/rand"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
)

var mongodbDriver datastore.DataStore
var _ = BeforeSuite(func(done Done) {
	rand.Seed(time.Now().UnixNano())
	By("bootstrapping mongodb test environment")
	clientOpts := options.Client().ApplyURI("mongodb://localhost:27017")
	client, err := mongo.Connect(context.TODO(), clientOpts)
	Expect(err).ToNot(HaveOccurred())
	client.Database("kubevela").Drop(context.TODO())

	mongodbDriver, err = New(context.TODO(), datastore.Config{
		URL:      "mongodb://localhost:27017",
		Database: "kubevela",
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(mongodbDriver).ToNot(BeNil())

	mongodbDriver, err = New(context.TODO(), datastore.Config{
		URL:      "localhost:27017",
		Database: "kubevela",
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(mongodbDriver).ToNot(BeNil())
	By("create mongodb driver success")
	close(done)
}, 120)

var _ = Describe("Test mongodb datastore driver", func() {

	It("Test add function", func() {
		err := mongodbDriver.Add(context.TODO(), &model.Application{Name: "kubevela-app", Description: "default"})
		Expect(err).ToNot(HaveOccurred())
	})

	It("Test batch add function", func() {
		var datas = []datastore.Entity{
			&model.Application{Name: "kubevela-app-2", Description: "this is demo 2"},
			&model.Application{Name: "kubevela-app-3", Description: "this is demo 3"},
			&model.Application{Name: "kubevela-app-4", Project: "test-project", Description: "this is demo 4"},
			&model.Workflow{Name: "kubevela-app-workflow", AppPrimaryKey: "kubevela-app-2", Description: "this is workflow"},
			&model.ApplicationTrigger{Name: "kubevela-app-trigger", AppPrimaryKey: "kubevela-app-2", Token: "token-test", Description: "this is demo 4"},
		}
		err := mongodbDriver.BatchAdd(context.TODO(), datas)
		Expect(err).ToNot(HaveOccurred())

		var datas2 = []datastore.Entity{
			&model.Application{Name: "can-delete", Description: "this is demo can-delete"},
			&model.Application{Name: "kubevela-app-2", Description: "this is demo 2"},
		}
		err = mongodbDriver.BatchAdd(context.TODO(), datas2)
		equal := cmp.Diff(strings.Contains(err.Error(), "save entities occur error"), true)
		Expect(equal).To(BeEmpty())
	})

	It("Test get function", func() {
		app := &model.Application{Name: "kubevela-app"}
		err := mongodbDriver.Get(context.TODO(), app)
		Expect(err).Should(BeNil())
		diff := cmp.Diff(app.Description, "default")
		Expect(diff).Should(BeEmpty())

		workflow := &model.Workflow{Name: "kubevela-app-workflow", AppPrimaryKey: "kubevela-app-2"}
		err = mongodbDriver.Get(context.TODO(), workflow)
		Expect(err).Should(BeNil())
		diff = cmp.Diff(workflow.Description, "this is workflow")
		Expect(diff).Should(BeEmpty())
	})

	It("Test put function", func() {
		err := mongodbDriver.Put(context.TODO(), &model.Application{Name: "kubevela-app", Description: "this is demo"})
		Expect(err).ToNot(HaveOccurred())
	})
	It("Test list function", func() {
		var app model.Application
		list, err := mongodbDriver.List(context.TODO(), &app, &datastore.ListOptions{Page: -1})
		Expect(err).ShouldNot(HaveOccurred())
		diff := cmp.Diff(len(list), 4)
		Expect(diff).Should(BeEmpty())

		list, err = mongodbDriver.List(context.TODO(), &app, &datastore.ListOptions{Page: 2, PageSize: 2})
		Expect(err).ShouldNot(HaveOccurred())
		diff = cmp.Diff(len(list), 2)
		Expect(diff).Should(BeEmpty())

		list, err = mongodbDriver.List(context.TODO(), &app, &datastore.ListOptions{Page: 1, PageSize: 2})
		Expect(err).ShouldNot(HaveOccurred())
		diff = cmp.Diff(len(list), 2)
		Expect(diff).Should(BeEmpty())

		list, err = mongodbDriver.List(context.TODO(), &app, nil)
		Expect(err).ShouldNot(HaveOccurred())
		diff = cmp.Diff(len(list), 4)
		Expect(diff).Should(BeEmpty())

		var workflow = model.Workflow{
			AppPrimaryKey: "kubevela-app-2",
		}
		list, err = mongodbDriver.List(context.TODO(), &workflow, nil)
		Expect(err).ShouldNot(HaveOccurred())
		diff = cmp.Diff(len(list), 1)
		Expect(diff).Should(BeEmpty())

		list, err = mongodbDriver.List(context.TODO(), &app, &datastore.ListOptions{FilterOptions: datastore.FilterOptions{In: []datastore.InQueryOption{
			{
				Key:    "name",
				Values: []string{"kubevela-app-3", "kubevela-app-2"},
			},
		}}})
		Expect(err).ShouldNot(HaveOccurred())
		diff = cmp.Diff(len(list), 2)
		Expect(diff).Should(BeEmpty())

		list, err = mongodbDriver.List(context.TODO(), &app, &datastore.ListOptions{FilterOptions: datastore.FilterOptions{IsNotExist: []datastore.IsNotExistQueryOption{
			{
				Key: "project",
			},
		}}})
		Expect(err).ShouldNot(HaveOccurred())
		diff = cmp.Diff(len(list), 3)
		Expect(diff).Should(BeEmpty())
	})

	It("Test list clusters with sort and fuzzy query", func() {
		clusters, err := mongodbDriver.List(context.TODO(), &model.Cluster{}, nil)
		Expect(err).Should(Succeed())
		for _, cluster := range clusters {
			Expect(mongodbDriver.Delete(context.TODO(), cluster)).Should(Succeed())
		}
		for _, name := range []string{"first", "second", "third"} {
			Expect(mongodbDriver.Add(context.TODO(), &model.Cluster{Name: name})).Should(Succeed())
			time.Sleep(time.Millisecond * 100)
		}
		entities, err := mongodbDriver.List(context.TODO(), &model.Cluster{}, &datastore.ListOptions{
			SortBy: []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderAscending}}})
		Expect(err).Should(Succeed())
		Expect(len(entities)).Should(Equal(3))
		for i, name := range []string{"first", "second", "third"} {
			Expect(entities[i].(*model.Cluster).Name).Should(Equal(name))
		}
		entities, err = mongodbDriver.List(context.TODO(), &model.Cluster{}, &datastore.ListOptions{
			SortBy:   []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}},
			Page:     1,
			PageSize: 2,
		})
		Expect(err).Should(Succeed())
		Expect(len(entities)).Should(Equal(2))
		for i, name := range []string{"third", "second"} {
			Expect(entities[i].(*model.Cluster).Name).Should(Equal(name))
		}
		entities, err = mongodbDriver.List(context.TODO(), &model.Cluster{}, &datastore.ListOptions{
			SortBy:   []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}},
			Page:     2,
			PageSize: 2,
		})
		Expect(err).Should(Succeed())
		Expect(len(entities)).Should(Equal(1))
		for i, name := range []string{"first"} {
			Expect(entities[i].(*model.Cluster).Name).Should(Equal(name))
		}
		entities, err = mongodbDriver.List(context.TODO(), &model.Cluster{}, &datastore.ListOptions{
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
		count, err := mongodbDriver.Count(context.TODO(), &app, nil)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).Should(Equal(int64(4)))

		count, err = mongodbDriver.Count(context.TODO(), &model.Cluster{}, &datastore.FilterOptions{
			Queries: []datastore.FuzzyQueryOption{{Key: "name", Query: "ir"}},
		})
		Expect(err).Should(Succeed())
		Expect(count).Should(Equal(int64(2)))

		count, err = mongodbDriver.Count(context.TODO(), &app, &datastore.FilterOptions{In: []datastore.InQueryOption{
			{
				Key:    "name",
				Values: []string{"kubevela-app-3", "kubevela-app-2"},
			},
		}})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).Should(Equal(int64(2)))

		count, err = mongodbDriver.Count(context.TODO(), &app, &datastore.FilterOptions{IsNotExist: []datastore.IsNotExistQueryOption{
			{
				Key: "project",
			},
		}})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).Should(Equal(int64(3)))

		app.Name = "kubevela-app-3"
		count, err = mongodbDriver.Count(context.TODO(), &app, &datastore.FilterOptions{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).Should(Equal(int64(1)))
	})

	It("Test isExist function", func() {
		var app model.Application
		app.Name = "kubevela-app-3"
		exist, err := mongodbDriver.IsExist(context.TODO(), &app)
		Expect(err).ShouldNot(HaveOccurred())
		diff := cmp.Diff(exist, true)
		Expect(diff).Should(BeEmpty())

		app.Name = "kubevela-app-5"
		notexist, err := mongodbDriver.IsExist(context.TODO(), &app)
		Expect(err).ShouldNot(HaveOccurred())
		diff = cmp.Diff(notexist, false)
		Expect(diff).Should(BeEmpty())
	})

	It("Test delete function", func() {
		var app model.Application
		app.Name = "kubevela-app"
		err := mongodbDriver.Delete(context.TODO(), &app)
		Expect(err).ShouldNot(HaveOccurred())

		app.Name = "kubevela-app-2"
		err = mongodbDriver.Delete(context.TODO(), &app)
		Expect(err).ShouldNot(HaveOccurred())

		app.Name = "kubevela-app-3"
		err = mongodbDriver.Delete(context.TODO(), &app)
		Expect(err).ShouldNot(HaveOccurred())

		app.Name = "kubevela-app-4"
		err = mongodbDriver.Delete(context.TODO(), &app)
		Expect(err).ShouldNot(HaveOccurred())

		app.Name = "kubevela-app-4"
		err = mongodbDriver.Delete(context.TODO(), &app)
		equal := cmp.Equal(err, datastore.ErrRecordNotExist, cmpopts.EquateErrors())
		Expect(equal).Should(BeTrue())

		workflow := model.Workflow{Name: "kubevela-app-workflow", AppPrimaryKey: "kubevela-app-2", Description: "this is workflow"}
		err = mongodbDriver.Delete(context.TODO(), &workflow)
		Expect(err).ShouldNot(HaveOccurred())

		trigger := model.ApplicationTrigger{Name: "kubevela-app-trigger", AppPrimaryKey: "kubevela-app-2", Token: "token-test", Description: "this is demo 4"}
		err = mongodbDriver.Delete(context.TODO(), &trigger)
		Expect(err).ShouldNot(HaveOccurred())
	})
})
