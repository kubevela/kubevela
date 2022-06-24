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
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("Test Worker CR sync to datastore", func() {
	BeforeEach(func() {
	})

	It("Test app sync test-app1", func() {

		By("Preparing database")
		dbNamespace := "sync-db-ns1-test"
		appNS1 := "sync-worker-test-ns1"
		appNS2 := "sync-worker-test-ns2"
		ds, err := NewDatastore(datastore.Config{Type: "kubeapi", Database: "sync-test-db1"})
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
		ns.Name = appNS2
		ns.ResourceVersion = ""
		err = k8sClient.Create(context.TODO(), &ns)
		Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		By("Start syncing")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		appSync := &ApplicationSync{
			KubeClient: k8sClient,
			KubeConfig: cfg,
			Store:      ds,
			Queue:      workqueue.New(),
		}
		go appSync.Start(ctx, make(chan error))

		By("create test app1 and check the syncing results")
		app1 := &v1beta1.Application{}
		Expect(common2.ReadYamlToObject("testdata/test-app1.yaml", app1)).Should(BeNil())
		app1.Namespace = appNS1
		Expect(k8sClient.Create(context.TODO(), app1)).Should(BeNil())

		Eventually(func() error {
			appm := model.Application{Name: app1.Name}
			return ds.Get(ctx, &appm)
		}, 10*time.Second, 100*time.Millisecond).Should(BeNil())

		comp1 := model.ApplicationComponent{AppPrimaryKey: app1.Name, Name: "nginx"}
		Expect(ds.Get(ctx, &comp1)).Should(BeNil())
		Expect(comp1.Properties).Should(BeEquivalentTo(&model.JSONStruct{"image": "nginx"}))

		comp2 := model.ApplicationComponent{AppPrimaryKey: app1.Name, Name: "nginx2"}
		Expect(ds.Get(ctx, &comp2)).Should(BeNil())
		Expect(comp2.Properties).Should(BeEquivalentTo(&model.JSONStruct{"image": "nginx2"}))

		env := model.Env{Project: appNS1, Name: model.AutoGenEnvNamePrefix + appNS1}
		Expect(ds.Get(ctx, &env)).Should(BeNil())
		Expect(len(env.Targets)).Should(Equal(2))

		appPlc1 := model.ApplicationPolicy{AppPrimaryKey: app1.Name, Name: "topology-beijing-demo"}
		Expect(ds.Get(ctx, &appPlc1)).Should(BeNil())
		appPlc2 := model.ApplicationPolicy{AppPrimaryKey: app1.Name, Name: "topology-local"}
		Expect(ds.Get(ctx, &appPlc2)).Should(BeNil())
		appwf1 := model.Workflow{AppPrimaryKey: app1.Name, Name: model.AutoGenWorkflowNamePrefix + app1.Name}
		Expect(ds.Get(ctx, &appwf1)).Should(BeNil())

		By("create test app2 and check the syncing results")
		app2 := &v1beta1.Application{}
		Expect(common2.ReadYamlToObject("testdata/test-app2.yaml", app2)).Should(BeNil())
		app2.Namespace = appNS2
		Expect(k8sClient.Create(context.TODO(), app2)).Should(BeNil())

		Eventually(func() error {
			appm := model.Application{Name: formatAppComposedName(app2.Name, app2.Namespace)}
			return ds.Get(ctx, &appm)
		}, 10*time.Second, 100*time.Millisecond).Should(BeNil())

		By("delete test app1 and check the syncing results")
		Expect(k8sClient.Delete(context.TODO(), app1)).Should(BeNil())
		Eventually(func() error {
			appm := model.Application{Name: app1.Name}
			return ds.Get(ctx, &appm)
		}, 10*time.Second, 100*time.Millisecond).Should(BeEquivalentTo(datastore.ErrRecordNotExist))

		By("update test app2 and check the syncing results")
		newapp2 := &v1beta1.Application{}
		Expect(common2.ReadYamlToObject("testdata/test-app3.yaml", newapp2)).Should(BeNil())
		app2.Spec = newapp2.Spec
		Expect(k8sClient.Update(context.TODO(), app2)).Should(BeNil())

		Eventually(func() error {
			appm := model.ApplicationComponent{AppPrimaryKey: formatAppComposedName(app2.Name, app2.Namespace), Name: "nginx2"}
			return ds.Get(ctx, &appm)
		}, 10*time.Second, 100*time.Millisecond).Should(BeNil())

	})

})
