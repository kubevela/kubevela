/*
 Copyright 2026. The KubeVela Authors.

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

package controllers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// The OpenAPI encoder in cuelang rejects relational operators applied to strings,
// so a ComponentDefinition carrying `string & !=""` used to fail schema generation
// outright, leaving Synced=False and no schema ConfigMap. The definition itself is
// valid and renders fine, so generation now falls back to dropping only the
// constraints the encoder cannot represent.
var _ = Describe("ComponentDefinition OpenAPI schema generation", func() {
	ctx := context.Background()

	var namespace string
	var ns corev1.Namespace

	BeforeEach(func() {
		namespace = randomNamespaceName("def-schema-e2e")
		ns = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		Eventually(func() error {
			return k8sClient.Create(ctx, &ns)
		}, time.Second*3, time.Microsecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})

	AfterEach(func() {
		By("Clean up resources after a test")
		Expect(k8sClient.DeleteAllOf(ctx, &v1beta1.ComponentDefinition{}, client.InNamespace(namespace))).Should(Succeed())
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(Succeed())
	})

	// applyAndGetSchema creates a ComponentDefinition from a CUE template, waits for
	// the controller to report Synced=True and publish a ConfigMapRef, then returns
	// the decoded `properties` block of the stored schema.
	applyAndGetSchema := func(name, template string) map[string]any {
		cd := &v1beta1.ComponentDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: v1beta1.ComponentDefinitionSpec{
				Workload: common.WorkloadTypeDescriptor{
					Definition: common.WorkloadGVK{APIVersion: "v1", Kind: "ConfigMap"},
				},
				Schematic: &common.Schematic{
					CUE: &common.CUE{Template: template},
				},
			},
		}
		Expect(k8sClient.Create(ctx, cd)).Should(Succeed())

		By("Wait for the definition to report Synced=True")
		got := new(v1beta1.ComponentDefinition)
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, got)).Should(Succeed())
			synced := got.Status.GetCondition(condition.TypeSynced)
			g.Expect(string(synced.Status)).Should(Equal(string(corev1.ConditionTrue)),
				fmt.Sprintf("Synced condition was %q: %s", synced.Status, synced.Message))
		}, 30*time.Second, time.Second).Should(Succeed())

		By("Wait for the schema ConfigMap to be published")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, got)).Should(Succeed())
			g.Expect(got.Status.ConfigMapRef).ShouldNot(BeEmpty())
		}, 30*time.Second, time.Second).Should(Succeed())

		Expect(got.Status.ConfigMapRef).Should(Equal(fmt.Sprintf("component-%s%s", types.CapabilityConfigMapNamePrefix, name)))

		cm := new(corev1.ConfigMap)
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: got.Status.ConfigMapRef, Namespace: namespace}, cm)
		}, 30*time.Second, time.Second).Should(Succeed())

		raw := cm.Data[types.OpenapiV3JSONSchema]
		Expect(raw).ShouldNot(BeEmpty())

		var doc struct {
			Properties map[string]any `json:"properties"`
		}
		Expect(json.Unmarshal([]byte(raw), &doc)).Should(Succeed())
		Expect(doc.Properties).ShouldNot(BeEmpty())
		return doc.Properties
	}

	outputBlock := `
output: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: name: context.name
	data: k: "v"
}
`

	It("keeps a definition Synced and publishes a schema when a string constraint cannot be encoded", func() {
		props := applyAndGetSchema("schema-string-constraint", outputBlock+`
parameter: {
	// +usage=Name of the bucket
	bucketName: string & !=""
	// +usage=Target region
	region: *"us-west-2" | string
	// +usage=Storage class
	class?: "STANDARD" | "GLACIER"
	// +usage=Replica count
	replicas: int & >0 & <10
}
`)

		By("The unencodable constraint is dropped but the field keeps its type and docs")
		bucket := props["bucketName"].(map[string]any)
		Expect(bucket["type"]).Should(Equal("string"))
		Expect(bucket["description"]).Should(Equal("Name of the bucket"))
		Expect(bucket).ShouldNot(HaveKey("minLength"))

		By("Fields the encoder can represent are untouched")
		Expect(props["region"].(map[string]any)["default"]).Should(Equal("us-west-2"))
		Expect(props["class"].(map[string]any)["enum"]).Should(ConsistOf("STANDARD", "GLACIER"))
		replicas := props["replicas"].(map[string]any)
		Expect(replicas["type"]).Should(Equal("integer"))
		Expect(replicas["minimum"]).Should(BeEquivalentTo(0))
		Expect(replicas["maximum"]).Should(BeEquivalentTo(10))
	})

	It("preserves the binary format of a bytes field whose constraint is dropped", func() {
		props := applyAndGetSchema("schema-bytes-constraint", outputBlock+`
parameter: {
	// +usage=Binary blob
	blob: bytes & !=''
	// +usage=Control blob
	plain: bytes
}
`)

		blob := props["blob"].(map[string]any)
		plain := props["plain"].(map[string]any)
		Expect(blob["format"]).Should(Equal("binary"), "a bytes field must not be retyped as a plain string")
		Expect(blob["format"]).Should(Equal(plain["format"]))
	})

	It("does not let a parameter named string capture the substituted type", func() {
		props := applyAndGetSchema("schema-shadowed-ident", outputBlock+`
parameter: {
	// +usage=Field literally named string
	string: "hello"
	// +usage=Must stay an unconstrained string
	name: !=""
}
`)

		Expect(props["name"].(map[string]any)).ShouldNot(HaveKey("enum"))
		Expect(props["name"].(map[string]any)["type"]).Should(Equal("string"))
		Expect(props["string"].(map[string]any)["enum"]).Should(ConsistOf("hello"))
	})

	It("leaves maps, open structs and referenced definitions intact alongside a dropped constraint", func() {
		props := applyAndGetSchema("schema-mixed-shapes", `
#Port: {
	port:  int
	name?: string
}
`+outputBlock+`
parameter: {
	// +usage=Bucket name
	bucketName: string & !=""
	// +usage=Arbitrary labels
	labels: [string]: string
	// +usage=Open struct
	extra: {known: string, ...}
	// +usage=Referenced type
	p: #Port
}
`)

		By("The map keeps its value type")
		labels := props["labels"].(map[string]any)
		Expect(labels["additionalProperties"]).Should(Equal(map[string]any{"type": "string"}))

		By("The open struct keeps its ellipsis")
		extra := props["extra"].(map[string]any)
		Expect(extra).Should(HaveKey("additionalProperties"))
		Expect(extra["properties"].(map[string]any)).Should(HaveKey("known"))

		By("The referenced definition is still expanded")
		Expect(props["p"].(map[string]any)["properties"].(map[string]any)).Should(And(HaveKey("port"), HaveKey("name")))

		By("Only the offending field is degraded")
		Expect(props["bucketName"].(map[string]any)["type"]).Should(Equal("string"))
	})

	It("emits an unchanged schema for a definition the encoder fully supports", func() {
		props := applyAndGetSchema("schema-fully-encodable", outputBlock+`
parameter: {
	// +usage=Matched by a regular expression
	rx: =~"^a.*"
	// +usage=Excluded by a regular expression
	nrx: string & !~"^x"
	// +usage=Bounded number
	n: int & >0 & <10
	// +usage=One of a fixed set
	e: "A" | "B"
}
`)

		Expect(props["rx"].(map[string]any)["pattern"]).Should(Equal("^a.*"))
		Expect(props["nrx"].(map[string]any)).Should(HaveKey("not"))
		Expect(props["n"].(map[string]any)["maximum"]).Should(BeEquivalentTo(10))
		Expect(props["e"].(map[string]any)["enum"]).Should(ConsistOf("A", "B"))
	})
})
