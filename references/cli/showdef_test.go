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
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

var _ = Describe("Test show definition cli", func() {

	When("test vela show notification", func() {

		It("should notification", func() {

			buffer := bytes.NewBuffer(nil)
			ioStreams := util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer}
			ctx := context.Background()
			c := common.Args{}
			c.SetConfig(cfg)
			c.SetClient(k8sClient)

			Expect(ShowReferenceMarkdown(ctx, c, ioStreams, "../../vela-templates/definitions/internal/workflowstep/notification.cue", "", "", "", "", 0)).Should(BeNil())

		})
	})

	When("test vela show git-file --format md", func() {

		It("should render the source parameters", func() {

			ioStreams := util.IOStreams{In: os.Stdin, Out: bytes.NewBuffer(nil), ErrOut: bytes.NewBuffer(nil)}
			ctx := context.Background()
			c := common.Args{}
			c.SetConfig(cfg)
			c.SetClient(k8sClient)

			data, err := os.ReadFile("../../charts/vela-core/definitions/git-file.yaml")
			Expect(err).Should(BeNil())
			var def v1beta1.SourceDefinition
			Expect(yaml.Unmarshal(data, &def)).Should(BeNil())
			def.Namespace = types.DefaultKubeVelaNS
			Expect(k8sClient.Create(ctx, &def)).Should(SatisfyAny(BeNil(), oamutil.AlreadyExistMatcher{}))
			DeferCleanup(func() {
				Expect(k8sClient.Delete(context.Background(), &def)).Should(SatisfyAny(Succeed(), oamutil.NotFoundMatcher{}))
			})

			outputPath := GinkgoT().TempDir()
			Expect(ShowReferenceMarkdown(ctx, c, ioStreams, "git-file", outputPath, "", "", types.DefaultKubeVelaNS, 0)).Should(BeNil())
			doc, err := os.ReadFile(filepath.Join(outputPath, "git-file.md"))
			Expect(err).Should(BeNil())
			Expect(string(doc)).Should(ContainSubstring(" registry | Name of a registry configured in this cluster. | string | true |  |  \n"))
		})
	})
})
