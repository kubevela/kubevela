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
	"io/ioutil"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile/dryrun"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("Test dry run with policy", func() {
	It("Test dry run with normal policy", func() {
		webservice, err := ioutil.ReadFile("../../charts/vela-core/templates/defwithtemplate/webservice.yaml")
		Expect(err).Should(BeNil())
		webserviceYAML := strings.Replace(string(webservice), "{{ include \"systemDefinitionNamespace\" . }}", types.DefaultKubeVelaNS, 1)
		wwd := v1beta1.ComponentDefinition{}
		Expect(yaml.Unmarshal([]byte(webserviceYAML), &wwd)).Should(BeNil())
		Expect(k8sClient.Create(context.TODO(), &wwd)).Should(BeNil())

		plcd := v1beta1.PolicyDefinition{}
		Expect(yaml.Unmarshal([]byte(plcdef), &plcd)).Should(BeNil())
		plcd.Namespace = types.DefaultKubeVelaNS
		Expect(k8sClient.Create(context.TODO(), &plcd)).Should(BeNil())
		app := v1beta1.Application{}
		Expect(yaml.Unmarshal([]byte(plcapp), &app)).Should(BeNil())
		c := common2.Args{}
		c.SetConfig(cfg)
		c.SetClient(k8sClient)
		pd, err := c.GetPackageDiscover()
		Expect(err).Should(BeNil())
		dm, err := discoverymapper.New(cfg)
		Expect(err).Should(BeNil())

		dryRunOpt := dryrun.NewDryRunOption(k8sClient, cfg, dm, pd, nil)

		comps, plcs, err := dryRunOpt.ExecuteDryRun(context.TODO(), &app)
		Expect(err).Should(BeNil())
		speci := plcs[0].Object["spec"].(map[string]interface{})
		Expect(speci["service"].(string)).Should(BeEquivalentTo("unified"))
		buff := bytes.NewBufferString("")
		Expect(dryRunOpt.PrintDryRun(buff, app.Name, comps, plcs)).Should(BeNil())
		Expect(buff.String()).Should(ContainSubstring(`backends:
  - service: server-v1
    weight: 80
  - service: server-v2
    weight: 20
  service: unified`))
		Expect(buff.String()).Should(ContainSubstring("- image: oamdev/hello-world:v1\n        name: server-v1"))
		Expect(buff.String()).Should(ContainSubstring("- image: oamdev/hello-world:v2\n        name: server-v2"))
	})

})

var plcapp = `apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: my-test-2
spec:
  components:
    - name: server-v1
      type: webservice
      properties:
        image: oamdev/hello-world:v1
    - name: server-v2
      type: webservice
      properties:
        image: oamdev/hello-world:v2
  policies:
    - type: my-plc
      name: unified
      properties:
        weights:
          - service: server-v1
            weight: 80
          - service: server-v2
            weight: 20
`

var plcdef = `apiVersion: core.oam.dev/v1beta1
kind: PolicyDefinition
metadata:
  annotations:
    definition.oam.dev/description: My ingress route policy.
  name: my-plc
spec:
  schematic:
    cue:
      template: |
        #ServerWeight: {
        	service: string
        	weight:  int
        }
        parameter: weights: [...#ServerWeight]
        output: {
        	apiVersion: "split.smi-spec.io/v1alpha3"
        	kind:       "TrafficSplit"
        	metadata: name: context.name
        	spec: {
        		service:  context.name
        		backends: parameter.weights
        	}
        }`
