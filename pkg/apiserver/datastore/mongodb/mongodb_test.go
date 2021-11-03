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

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
)

var mongodbDriver datastore.DataStore
var _ = BeforeSuite(func(done Done) {
	rand.Seed(time.Now().UnixNano())
	By("bootstrapping mongodb test environment")
	var err error
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

	It("Test add funtion", func() {
		err := mongodbDriver.Add(context.TODO(), &model.ApplicationPlan{Name: "kubevela-app", Description: "default"})
		Expect(err).ToNot(HaveOccurred())
	})

	It("Test batch add funtion", func() {
		var datas = []datastore.Entity{
			&model.ApplicationPlan{Name: "kubevela-app-2", Description: "this is demo 2"},
			&model.ApplicationPlan{Namespace: "test-namespace", Name: "kubevela-app-3", Description: "this is demo 3"},
			&model.ApplicationPlan{Namespace: "test-namespace2", Name: "kubevela-app-4", Description: "this is demo 4"},
		}
		err := mongodbDriver.BatchAdd(context.TODO(), datas)
		Expect(err).ToNot(HaveOccurred())

		var datas2 = []datastore.Entity{
			&model.ApplicationPlan{Namespace: "test-namespace", Name: "can-delete", Description: "this is demo can-delete"},
			&model.ApplicationPlan{Name: "kubevela-app-2", Description: "this is demo 2"},
		}
		err = mongodbDriver.BatchAdd(context.TODO(), datas2)
		equal := cmp.Diff(strings.Contains(err.Error(), "save components occur error"), true)
		Expect(equal).To(BeEmpty())
	})

	It("Test get funtion", func() {
		app := &model.ApplicationPlan{Name: "kubevela-app"}
		err := mongodbDriver.Get(context.TODO(), app)
		Expect(err).Should(BeNil())
		diff := cmp.Diff(app.Description, "default")
		Expect(diff).Should(BeEmpty())
	})

	It("Test put funtion", func() {
		err := mongodbDriver.Put(context.TODO(), &model.ApplicationPlan{Name: "kubevela-app", Description: "this is demo"})
		Expect(err).ToNot(HaveOccurred())
	})
	It("Test list funtion", func() {
		var app model.ApplicationPlan
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

		app.Namespace = "test-namespace"
		list, err = mongodbDriver.List(context.TODO(), &app, nil)
		Expect(err).ShouldNot(HaveOccurred())
		diff = cmp.Diff(len(list), 1)
		Expect(diff).Should(BeEmpty())
	})

	It("Test count function", func() {
		var app model.ApplicationPlan
		count, err := mongodbDriver.Count(context.TODO(), &app)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).Should(Equal(int64(4)))

		app.Namespace = "test-namespace"
		count, err = mongodbDriver.Count(context.TODO(), &app)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(count).Should(Equal(int64(1)))
	})

	It("Test isExist funtion", func() {
		var app model.ApplicationPlan
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

	It("Test delete funtion", func() {
		var app model.ApplicationPlan
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
	})
})
