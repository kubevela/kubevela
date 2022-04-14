/*
 Copyright 2022 The KubeVela Authors.

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

package sync

import (
	"context"
	"fmt"
	"sync"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("Test CR convert to ux", func() {
	BeforeEach(func() {
	})

	It("Test get app with occupied app", func() {

		By("Preparing database")
		dbNamespace := "get-app-db-ns1-test"

		apName1 := "example"
		appNS1 := "get-app-test-ns1"
		appNS2 := "get-app-test-ns2"
		ds, err := NewDatastore(datastore.Config{Type: "kubeapi", Database: dbNamespace})
		Expect(ds).ToNot(BeNil())
		Expect(err).Should(BeNil())
		var ns = corev1.Namespace{}
		ns.Name = dbNamespace
		err = k8sClient.Create(context.TODO(), &ns)
		Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		ns.Name = appNS1
		ns.ResourceVersion = ""
		err = k8sClient.Create(context.TODO(), &ns)
		Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		By("no app created, test the name")

		cr2ux := CR2UX{ds: ds, cli: k8sClient, cache: sync.Map{}}
		gotApp, gotAppName, err := cr2ux.getApp(context.Background(), apName1, appNS1)
		Expect(gotAppName).Should(BeEquivalentTo(apName1))
		Expect(gotApp).Should(BeNil())
		Expect(err).ShouldNot(BeNil())

		By("create test app2 and check the syncing results")
		app2 := &v1beta1.Application{}
		Expect(common2.ReadYamlToObject("testdata/test-app2.yaml", app2)).Should(BeNil())
		app2.Namespace = appNS2
		Expect(cr2ux.AddOrUpdate(context.Background(), app2)).Should(BeNil())
		comp1 := model.ApplicationComponent{AppPrimaryKey: apName1, Name: "blog"}
		Expect(ds.Get(context.Background(), &comp1)).Should(BeNil())
		Expect(comp1.Properties).Should(BeEquivalentTo(&model.JSONStruct{"image": "wordpress"}))

		By("app not created, but the name is occupied by the same name app from other namespace")
		gotApp, gotAppName, err = cr2ux.getApp(context.Background(), apName1, appNS1)
		Expect(gotAppName).Should(BeEquivalentTo(formatAppComposedName(apName1, appNS1)))
		Expect(gotApp).Should(BeNil())
		Expect(err).ShouldNot(BeNil())

		By("app get the created app")
		gotApp, gotAppName, err = cr2ux.getApp(context.Background(), apName1, appNS2)
		Expect(gotAppName).Should(BeEquivalentTo(apName1))
		Expect(gotApp.Labels[model.LabelSourceOfTruth]).Should(BeEquivalentTo(model.FromCR))
		Expect(err).Should(BeNil())
		Expect(gotApp.IsSynced()).Should(BeEquivalentTo(true))

	})
	It("Test app updated and delete app", func() {
		ctx := context.Background()
		By("Preparing database")
		dbNamespace := "update-app-db-ns1-test"

		apName1 := "example"
		appNS1 := "update-app-test-ns1"
		ds, err := NewDatastore(datastore.Config{Type: "kubeapi", Database: dbNamespace})
		Expect(ds).ToNot(BeNil())
		Expect(err).Should(BeNil())
		var ns = corev1.Namespace{}
		ns.Name = dbNamespace
		err = k8sClient.Create(context.TODO(), &ns)
		Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		ns.Name = appNS1
		ns.ResourceVersion = ""
		err = k8sClient.Create(context.TODO(), &ns)
		Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		cr2ux := CR2UX{ds: ds, cli: k8sClient, cache: sync.Map{}}

		By("create test app1 and check the syncing results")
		app1 := &v1beta1.Application{}
		Expect(common2.ReadYamlToObject("testdata/test-app1.yaml", app1)).Should(BeNil())
		app1.Namespace = appNS1
		Expect(cr2ux.AddOrUpdate(context.Background(), app1)).Should(BeNil())
		comp1 := model.ApplicationComponent{AppPrimaryKey: apName1, Name: "nginx"}
		Expect(ds.Get(context.Background(), &comp1)).Should(BeNil())
		Expect(comp1.Properties).Should(BeEquivalentTo(&model.JSONStruct{"image": "nginx"}))

		comp2 := model.ApplicationComponent{AppPrimaryKey: app1.Name, Name: "nginx2"}
		Expect(ds.Get(ctx, &comp2)).Should(BeNil())
		Expect(comp2.Properties).Should(BeEquivalentTo(&model.JSONStruct{"image": "nginx2"}))

		appPlc1 := model.ApplicationPolicy{AppPrimaryKey: app1.Name, Name: "topology-beijing-demo"}
		Expect(ds.Get(ctx, &appPlc1)).Should(BeNil())
		Expect(appPlc1.Properties).Should(BeEquivalentTo(&model.JSONStruct{"namespace": "demo", "clusterLabelSelector": map[string]interface{}{"region": "beijing"}}))

		appPlc2 := model.ApplicationPolicy{AppPrimaryKey: app1.Name, Name: "topology-local"}
		Expect(ds.Get(ctx, &appPlc2)).Should(BeNil())
		Expect(appPlc2.Properties).Should(BeEquivalentTo(&model.JSONStruct{"targets": []interface{}{"local/demo", "local/ackone-demo"}}))

		appwf1 := model.Workflow{AppPrimaryKey: app1.Name, Name: model.AutoGenWorkflowNamePrefix + app1.Name}
		Expect(ds.Get(ctx, &appwf1)).Should(BeNil())
		Expect(len(appwf1.Steps)).Should(BeEquivalentTo(1))

		app2 := &v1beta1.Application{}
		Expect(common2.ReadYamlToObject("testdata/test-app2.yaml", app2)).Should(BeNil())
		app1.Namespace = appNS1
		app1.Generation = 2
		app1.Spec = app2.Spec
		Expect(cr2ux.AddOrUpdate(context.Background(), app1)).Should(BeNil())
		comp3 := model.ApplicationComponent{AppPrimaryKey: apName1, Name: "blog"}
		Expect(ds.Get(context.Background(), &comp3)).Should(BeNil())
		Expect(comp3.Properties).Should(BeEquivalentTo(&model.JSONStruct{"image": "wordpress"}))

		Expect(ds.Get(ctx, &comp1)).Should(BeEquivalentTo(datastore.ErrRecordNotExist))
		Expect(ds.Get(ctx, &comp2)).Should(BeEquivalentTo(datastore.ErrRecordNotExist))
		Expect(ds.Get(ctx, &appPlc1)).Should(BeEquivalentTo(datastore.ErrRecordNotExist), fmt.Sprintf("plc name %s, creator %s", appPlc1.Name, appPlc1.Creator))
		Expect(ds.Get(ctx, &appPlc2)).Should(BeEquivalentTo(datastore.ErrRecordNotExist), fmt.Sprintf("plc name %s, creator %s", appPlc2.Name, appPlc2.Creator))
		appwf2 := &model.Workflow{AppPrimaryKey: apName1, Name: appwf1.Name}
		Expect(ds.Get(ctx, appwf2)).Should(BeNil())

		Expect(len(appwf2.Steps)).Should(BeEquivalentTo(0))

		Expect(cr2ux.DeleteApp(ctx, app1)).Should(BeNil())
		Expect(ds.Get(context.Background(), &comp3)).Should(BeEquivalentTo(datastore.ErrRecordNotExist))
		Expect(ds.Get(context.Background(), &model.Application{Name: apName1})).Should(BeEquivalentTo(datastore.ErrRecordNotExist))
	})

})
