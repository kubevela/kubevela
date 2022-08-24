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

package cli

import (
	"bytes"
	"context"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

var _ = Describe("Test def apply cli", func() {

	When("test vela def apply", func() {

		It("should not have err and applied all def files", func() {

			buffer := bytes.NewBuffer(nil)
			ioStreams := util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer}

			ctx := context.Background()
			c := common.Args{}
			c.SetConfig(cfg)
			c.SetClient(k8sClient)
			err := defApplyAll(ctx, c, ioStreams, types.DefaultKubeVelaNS, "./test-data/defapply", false)
			Expect(err).Should(BeNil())

			By("check component definition in YAML exist")
			cml := v1beta1.ComponentDefinition{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "testdefyaml", Namespace: types.DefaultKubeVelaNS}, &cml)).Should(BeNil())

			By("check trait definition in CUE exist")
			traitd := v1beta1.TraitDefinition{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "testdefcue", Namespace: types.DefaultKubeVelaNS}, &traitd)).Should(BeNil())

		})
	})
})
