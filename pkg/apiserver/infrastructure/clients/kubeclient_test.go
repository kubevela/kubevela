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

package clients

import (
	"context"
	"sync"

	"github.com/oam-dev/kubevela/pkg/apiserver/config"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Test kube client", func() {

	PIt("Test client-side throttling", func() {
		defer GinkgoRecover()
		err := SetKubeConfig(config.Config{
			KubeQPS:   50,
			KubeBurst: 100,
		})
		Expect(err).Should(BeNil())
		cli, err := GetKubeClient()
		Expect(err).ToNot(HaveOccurred())

		var group sync.WaitGroup
		for i := 0; i < 500; i++ {
			group.Add(1)
			go func() {
				defer group.Done()
				var pods corev1.PodList
				err := cli.List(context.TODO(), &pods, &client.ListOptions{Namespace: "vela-system"})
				Expect(err).Should(BeNil())
			}()
		}
		group.Wait()
	})
})
