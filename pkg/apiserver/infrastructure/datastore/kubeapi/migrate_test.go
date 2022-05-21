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

package kubeapi

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/clients"
)

var _ = Describe("Test Migrate", func() {

	It("Test migrate labels", func() {
		clients.SetKubeClient(k8sClient)

		nsName := "test-migrate"
		ds := &kubeapi{kubeClient: k8sClient, namespace: nsName}
		ns := &v1.Namespace{}
		ns.Name = nsName
		Expect(k8sClient.Create(context.Background(), ns)).Should(BeNil())
		entity := &model.Application{Name: "my-app"}
		cm := ds.generateConfigMap(entity)
		name := fmt.Sprintf("veladatabase-%s-%s", entity.TableName(), entity.PrimaryKey())
		cm.Name = strings.ReplaceAll(name, "_", "-")
		cm.Namespace = nsName
		Expect(ds.kubeClient.Create(context.Background(), cm)).Should(BeNil())

		migrate(nsName)
		cmList := v1.ConfigMapList{}
		Expect(k8sClient.List(context.Background(), &cmList, client.InNamespace(nsName))).Should(BeNil())
		Expect(len(cmList.Items)).Should(BeEquivalentTo(2))

		es, err := ds.List(context.Background(), &model.Application{}, nil)
		Expect(err).Should(BeNil())
		Expect(len(es)).Should(BeEquivalentTo(1))
	})

})
