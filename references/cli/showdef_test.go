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
})
