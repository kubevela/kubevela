/*
Copyright 2026 The KubeVela Authors.

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

package service

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// addonAppMap is an addon Application as the renderer holds it just before
// returning: unstructured, with the given components and policies.
func addonAppMap(components, policies string) map[string]interface{} {
	var app map[string]interface{}
	Expect(json.Unmarshal([]byte(`{"spec":{"components":`+components+`,"policies":`+policies+`}}`), &app)).To(Succeed())
	return app
}

func specOf(app map[string]interface{}) map[string]interface{} {
	return app["spec"].(map[string]interface{})
}

func asJSON(v interface{}) string {
	b, err := json.Marshal(v)
	Expect(err).NotTo(HaveOccurred())
	return string(b)
}

const pinComponents = `[
	{"name":"addon-resources","type":"k8s-objects"},
	{"name":"store","type":"crate","dependsOn":["addon-resources"]},
	{"name":"addon-secret","type":"k8s-objects"}
]`

var _ = Describe("pinCrateComponentsToHub", func() {
	It("keeps crate components on the hub and the rest on the addon's topology", func() {
		app := addonAppMap(pinComponents, `[
			{"name":"topo-runtime","type":"topology","properties":{"clusters":["spoke-a","spoke-b"]}},
			{"name":"patch-all","type":"override","properties":{"components":[{"type":"k8s-objects"}]}},
			{"name":"addon-component-state-keep","type":"apply-once","properties":{"enable":false}}
		]`)

		pinCrateComponentsToHub(app, []string{"store"})

		spec := specOf(app)
		Expect(asJSON(spec["policies"])).To(MatchJSON(`[
			{"name":"topo-runtime","type":"topology","properties":{"clusters":["spoke-a","spoke-b"]}},
			{"name":"patch-all","type":"override","properties":{"components":[{"type":"k8s-objects"}]}},
			{"name":"addon-component-state-keep","type":"apply-once","properties":{"enable":false}},
			{"name":"addon-components","type":"override","properties":{"selector":["addon-resources","addon-secret"]}},
			{"name":"addon-crates-hub","type":"topology","properties":{"clusters":["local"]}},
			{"name":"addon-crates","type":"override","properties":{"selector":["store"]}}
		]`), "the crate components get their own hub topology; everything else keeps the addon's")
		Expect(asJSON(spec["workflow"])).To(MatchJSON(`{"mode":{"steps":"StepByStep","subSteps":"DAG"},"steps":[
			{"name":"deploy-topo-runtime","type":"deploy","properties":{"policies":["patch-all","addon-components","topo-runtime"]}},
			{"name":"deploy-addon-crates","type":"deploy","properties":{"policies":["patch-all","addon-crates-hub","addon-crates"]}}
		]}`), "one step per addon topology, as the default workflow would, then the crates on the hub")
		Expect(asJSON(spec["components"])).To(MatchJSON(`[
			{"name":"addon-resources","type":"k8s-objects"},
			{"name":"store","type":"crate"},
			{"name":"addon-secret","type":"k8s-objects"}
		]`), "a deploy step only waits for components it deploys itself, so the step order replaces the crate's dependsOn")
	})

	It("writes one deploy step per addon topology", func() {
		app := addonAppMap(pinComponents, `[
			{"name":"east","type":"topology","properties":{"clusters":["spoke-a"]}},
			{"name":"west","type":"topology","properties":{"clusterLabelSelector":{"region":"west"}}}
		]`)

		pinCrateComponentsToHub(app, []string{"store"})

		Expect(asJSON(specOf(app)["workflow"])).To(MatchJSON(`{"mode":{"steps":"StepByStep","subSteps":"DAG"},"steps":[
			{"name":"deploy-east","type":"deploy","properties":{"policies":["addon-components","east"]}},
			{"name":"deploy-west","type":"deploy","properties":{"policies":["addon-components","west"]}},
			{"name":"deploy-addon-crates","type":"deploy","properties":{"policies":["addon-crates-hub","addon-crates"]}}
		]}`))
	})

	It("writes an empty selector when every component is a crate", func() {
		app := addonAppMap(`[
			{"name":"store","type":"crate"},
			{"name":"cache","type":"crate"}
		]`, `[{"name":"topo-runtime","type":"topology","properties":{"clusters":["spoke"]}}]`)

		pinCrateComponentsToHub(app, []string{"store", "cache"})

		Expect(asJSON(specOf(app)["policies"])).To(MatchJSON(`[
			{"name":"topo-runtime","type":"topology","properties":{"clusters":["spoke"]}},
			{"name":"addon-components","type":"override","properties":{"selector":[]}},
			{"name":"addon-crates-hub","type":"topology","properties":{"clusters":["local"]}},
			{"name":"addon-crates","type":"override","properties":{"selector":["store","cache"]}}
		]`), "a nil selector would select every component, so the crates-only case stays an empty list")
	})

	It("picks names the addon does not use yet", func() {
		app := addonAppMap(pinComponents, `[
			{"name":"addon-crates","type":"topology","properties":{"clusters":["spoke-a"]}},
			{"name":"addon-components","type":"override","properties":{"components":[{"type":"k8s-objects"}]}}
		]`)

		pinCrateComponentsToHub(app, []string{"store"})

		spec := specOf(app)
		Expect(asJSON(spec["policies"])).To(MatchJSON(`[
			{"name":"addon-crates","type":"topology","properties":{"clusters":["spoke-a"]}},
			{"name":"addon-components","type":"override","properties":{"components":[{"type":"k8s-objects"}]}},
			{"name":"addon-components-2","type":"override","properties":{"selector":["addon-resources","addon-secret"]}},
			{"name":"addon-crates-hub","type":"topology","properties":{"clusters":["local"]}},
			{"name":"addon-crates-2","type":"override","properties":{"selector":["store"]}}
		]`))
		Expect(asJSON(spec["workflow"])).To(MatchJSON(`{"mode":{"steps":"StepByStep","subSteps":"DAG"},"steps":[
			{"name":"deploy-addon-crates","type":"deploy","properties":{"policies":["addon-components","addon-components-2","addon-crates"]}},
			{"name":"deploy-addon-crates-2","type":"deploy","properties":{"policies":["addon-components","addon-crates-hub","addon-crates-2"]}}
		]}`))
	})

	DescribeTable("leaves other addons alone",
		func(components, policies, workflow string, crates []string) {
			app := addonAppMap(components, policies)
			if workflow != "" {
				var wf map[string]interface{}
				Expect(json.Unmarshal([]byte(workflow), &wf)).To(Succeed())
				specOf(app)["workflow"] = wf
			}
			before := asJSON(app)

			pinCrateComponentsToHub(app, crates)

			Expect(asJSON(app)).To(MatchJSON(before))
		},
		Entry("no topology, everything is already on the hub", pinComponents,
			`[{"name":"addon-component-state-keep","type":"apply-once","properties":{"enable":false}}]`, "", []string{"store"}),
		Entry("no crate component", `[{"name":"addon-resources","type":"k8s-objects"}]`,
			`[{"name":"topo-runtime","type":"topology","properties":{"clusters":["spoke-a"]}}]`, "", nil),
		Entry("the author wrote a workflow", pinComponents,
			`[{"name":"topo-runtime","type":"topology","properties":{"clusters":["spoke-a"]}}]`,
			`{"steps":[{"name":"mine","type":"deploy","properties":{"policies":["topo-runtime"]}}]}`, []string{"store"}),
	)
})
