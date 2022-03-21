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
	"sync"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test Cache", func() {
	BeforeEach(func() {
	})

	It("Test cache update and delete", func() {

		By("Preparing database")
		dbNamespace := "cache-db-ns1-test"

		ds, err := NewDatastore(datastore.Config{Type: "kubeapi", Database: dbNamespace})
		Expect(ds).ToNot(BeNil())
		Expect(err).Should(BeNil())
		var ns = corev1.Namespace{}
		ns.Name = dbNamespace
		err = k8sClient.Create(context.TODO(), &ns)
		Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		cr2ux := CR2UX{ds: ds, cli: k8sClient, cache: sync.Map{}}

		ctx := context.Background()
		Expect(ds.Add(ctx, &model.Application{Name: "app1"})).Should(BeNil())
		Expect(ds.Add(ctx, &model.Application{Name: "app2", Labels: map[string]string{
			model.LabelSyncGeneration: "1",
			model.LabelSyncNamespace:  "app2-ns",
		}})).Should(BeNil())

		Expect(ds.Add(ctx, &model.Application{Name: "app3", Labels: map[string]string{
			model.LabelSyncGeneration: "1",
			model.LabelSyncNamespace:  "app3-ns",
			model.LabelSourceOfTruth:  model.FromUX,
		}})).Should(BeNil())

		Expect(cr2ux.initCache(ctx)).Should(BeNil())
		app1 := &v1beta1.Application{}
		app1.Name = "app1"
		app1.Namespace = "app1-ns"
		app1.Generation = 1
		Expect(cr2ux.shouldSync(ctx, app1, false)).Should(BeEquivalentTo(true))

		app2 := &v1beta1.Application{}
		app2.Name = "app2"
		app2.Namespace = "app2-ns"
		app2.Generation = 1

		Expect(cr2ux.shouldSync(ctx, app2, false)).Should(BeEquivalentTo(false))

		app3 := &v1beta1.Application{}
		app3.Name = "app3"
		app3.Namespace = "app3-ns"
		app3.Generation = 3

		Expect(cr2ux.shouldSync(ctx, app3, false)).Should(BeEquivalentTo(false))

		cr2ux.syncCache(formatAppComposedName(app1.Name, app1.Namespace), 1, 0)
		Expect(cr2ux.shouldSync(ctx, app1, false)).Should(BeEquivalentTo(false))
		Expect(cr2ux.shouldSync(ctx, app1, true)).Should(BeEquivalentTo(true))
	})

})
