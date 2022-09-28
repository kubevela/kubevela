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
package cli

import (
	"bytes"
	"context"
	"os"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	util2 "github.com/oam-dev/kubevela/pkg/oam/util"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

var _ = Describe("Test port-forward cli", func() {

	When("test port-forward cli", func() {

		It("should not have err", func() {
			app := v1beta1.Application{}
			Expect(yaml.Unmarshal([]byte(appWithNginx), &app)).Should(BeNil())
			Expect(k8sClient.Create(context.Background(), &app)).Should(SatisfyAny(BeNil(), util2.AlreadyExistMatcher{}))
			arg := common2.Args{}
			arg.SetClient(k8sClient)
			buffer := bytes.NewBuffer(nil)
			ioStreams := util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer}
			cmd := NewPortForwardCommand(arg, "nginx", ioStreams)
			Expect(cmd.Execute()).Should(BeNil())
			buf, ok := ioStreams.Out.(*bytes.Buffer)
			Expect(ok).Should(BeTrue())
			Expect(strings.Contains(buf.String(), "error")).Should(BeFalse())
		})
	})
})

const (
	appWithNginx = `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: nginx
  namespace: default
spec:
  components:
  - name: nginx
    type: webservice
    properties:
      image: nginx
      ports:
      - expose: true
        port: 80
        protocol: TCP
    traits:
    - properties:
        replicas: 1
      type: scaler
`
)
