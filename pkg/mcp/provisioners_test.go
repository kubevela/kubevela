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

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// testCRD builds a CRD whose storage version has the given spec schema.
func testCRD(group, plural, kind string, scope crdv1.ResourceScope, spec crdv1.JSONSchemaProps) *crdv1.CustomResourceDefinition {
	return &crdv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: plural + "." + group},
		Spec: crdv1.CustomResourceDefinitionSpec{
			Group: group,
			Names: crdv1.CustomResourceDefinitionNames{Plural: plural, Kind: kind},
			Scope: scope,
			Versions: []crdv1.CustomResourceDefinitionVersion{
				{Name: "v1alpha0", Served: true},
				{Name: "v1alpha1", Served: true, Storage: true, Schema: &crdv1.CustomResourceValidation{OpenAPIV3Schema: &crdv1.JSONSchemaProps{
					Type:       "object",
					Properties: map[string]crdv1.JSONSchemaProps{"spec": spec},
				}}},
			},
		},
	}
}

func ackTableCRD() *crdv1.CustomResourceDefinition {
	return testCRD("dynamodb.services.k8s.aws", "tables", "Table", crdv1.NamespaceScoped, crdv1.JSONSchemaProps{
		Type:     "object",
		Required: []string{"attributeDefinitions", "keySchema", "tableName"},
		Properties: map[string]crdv1.JSONSchemaProps{
			"tableName":   {Type: "string", Description: "The name of the table."},
			"billingMode": {Type: "string"},
			"keySchema": {Type: "array", Items: &crdv1.JSONSchemaPropsOrArray{Schema: &crdv1.JSONSchemaProps{Type: "object", Properties: map[string]crdv1.JSONSchemaProps{
				"attributeName": {Type: "string"}, "keyType": {Type: "string"},
			}}}},
			"attributeDefinitions": {Type: "array"},
		},
	})
}

func crossplaneTableCRD(group string, scope crdv1.ResourceScope) *crdv1.CustomResourceDefinition {
	return testCRD(group, "tables", "Table", scope, crdv1.JSONSchemaProps{
		Type:       "object",
		Required:   []string{"forProvider"},
		Properties: map[string]crdv1.JSONSchemaProps{"forProvider": {Type: "object"}},
	})
}

var _ = Describe("cloud provisioners in get_platform_info", func() {
	var h *harness

	AfterEach(func() {
		if h != nil {
			h.close()
			h = nil
		}
	})

	platform := func(args map[string]any) GetPlatformInfoOutput {
		var out GetPlatformInfoOutput
		res := h.call("get_platform_info", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		return out
	}
	objs := func(extra ...client.Object) []client.Object {
		return append(platformFixtures(), extra...)
	}

	It("lists the AWS services each provisioner serves", func() {
		h = startHarness(Options{}, objs(
			ackTableCRD(),
			testCRD("s3.services.k8s.aws", "buckets", "Bucket", crdv1.NamespaceScoped, crdv1.JSONSchemaProps{Type: "object"}),
			crossplaneTableCRD("sqs.aws.m.upbound.io", crdv1.NamespaceScoped),
			testCRD("postgresql.cnpg.io", "clusters", "Cluster", crdv1.NamespaceScoped, crdv1.JSONSchemaProps{Type: "object"}),
		)...)
		out := platform(nil)
		Expect(out.CloudProvisioners).NotTo(BeNil())
		Expect(out.CloudProvisioners.ACK).To(Equal([]string{"dynamodb", "s3"}))
		Expect(out.CloudProvisioners.Crossplane).To(Equal([]string{"sqs"}))
		Expect(out.Provisioner).To(BeNil(), "no service was asked about")
	})

	It("chooses ACK when both ACK and Crossplane serve the service, and returns the kind's schema", func() {
		h = startHarness(Options{}, objs(ackTableCRD(), crossplaneTableCRD("dynamodb.aws.upbound.io", crdv1.ClusterScoped))...)
		p := platform(map[string]any{"service": "dynamodb", "kind": "table"}).Provisioner
		Expect(p).NotTo(BeNil())
		Expect(p.Provisioner).To(Equal(ProvisionerACK))
		Expect(p.Also).To(Equal([]string{ProvisionerCrossplane}))
		Expect(p.Kinds).To(Equal([]ProvisionedKind{{Kind: "Table", APIVersion: "dynamodb.services.k8s.aws/v1alpha1", Scope: "Namespaced"}}), "the storage version, not the first served one")
		Expect(p.Message).To(BeEmpty())
		Expect(p.Resource).NotTo(BeNil())
		Expect(p.Resource.Required).To(Equal([]string{"attributeDefinitions", "keySchema", "tableName"}))
		Expect(p.Resource.Spec["properties"]).To(HaveKey("keySchema"))
		Expect(p.Resource.Health).To(ContainSubstring("ACK.ResourceSynced"))
	})

	It("falls back to Crossplane, preferring the namespaced v2 group", func() {
		h = startHarness(Options{}, objs(
			crossplaneTableCRD("dynamodb.aws.upbound.io", crdv1.ClusterScoped),
			crossplaneTableCRD("dynamodb.aws.m.upbound.io", crdv1.NamespaceScoped),
		)...)
		p := platform(map[string]any{"service": "dynamodb", "kind": "Table"}).Provisioner
		Expect(p.Provisioner).To(Equal(ProvisionerCrossplane))
		Expect(p.Kinds).To(HaveLen(1))
		Expect(p.Resource.APIVersion).To(Equal("dynamodb.aws.m.upbound.io/v1alpha1"))
		Expect(p.Resource.Required).To(Equal([]string{"forProvider"}))
		Expect(p.Resource.Health).To(ContainSubstring("Ready and Synced"))
	})

	It("says that ACK or Crossplane is needed when neither serves the service", func() {
		h = startHarness(Options{}, objs(testCRD("s3.services.k8s.aws", "buckets", "Bucket", crdv1.NamespaceScoped, crdv1.JSONSchemaProps{Type: "object"}))...)
		p := platform(map[string]any{"service": "dynamodb"}).Provisioner
		Expect(p.Provisioner).To(BeEmpty())
		Expect(p.Kinds).To(BeEmpty())
		Expect(p.Message).To(ContainSubstring("Neither ACK nor Crossplane provides dynamodb on this cluster"))
		Expect(p.Message).To(ContainSubstring("dynamodb.services.k8s.aws"))
		Expect(p.Message).To(ContainSubstring("dynamodb.aws.m.upbound.io"))
		Expect(p.Message).To(ContainSubstring("oci://public.ecr.aws/aws-controllers-k8s/dynamodb-chart"))
		Expect(p.Message).To(ContainSubstring("provider-aws-dynamodb"))
	})

	It("names the installed kinds when the kind asked for is not one of them", func() {
		h = startHarness(Options{}, objs(ackTableCRD())...)
		p := platform(map[string]any{"service": "dynamodb", "kind": "Stream"}).Provisioner
		Expect(p.Provisioner).To(Equal(ProvisionerACK))
		Expect(p.Resource).To(BeNil())
		Expect(p.Message).To(ContainSubstring("has no Stream kind; the installed kinds are Table"))
	})

	It("reports a CRD list it cannot read as unknown, never as nothing installed", func() {
		forbid := &interceptor.Funcs{List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			if pl, ok := list.(*metav1.PartialObjectMetadataList); ok && strings.HasPrefix(pl.GroupVersionKind().Kind, "CustomResourceDefinition") {
				return apierrors.NewForbidden(schema.GroupResource{Group: "apiextensions.k8s.io", Resource: "customresourcedefinitions"}, "", errors.New("cannot list"))
			}
			return c.List(ctx, list, opts...)
		}}
		h = startHarnessIntercepted(Options{}, nil, forbid, objs(ackTableCRD())...)
		out := platform(map[string]any{"service": "dynamodb"})
		Expect(out.CloudProvisioners).To(BeNil())
		Expect(out.Provisioner.Message).To(ContainSubstring("whether ACK or Crossplane is installed is unknown"))
		Expect(out.Provisioner.Message).NotTo(ContainSubstring("Neither"))
		Expect(out.Meta.Warnings).To(ContainElement(ContainSubstring("could not be listed")))
	})

	It("drops descriptions from a schema too large to return whole", func() {
		big := ackTableCRD()
		spec := big.Spec.Versions[1].Schema.OpenAPIV3Schema.Properties["spec"]
		for i := 0; i < 400; i++ {
			spec.Properties["field"+strings.Repeat("x", i%7)+string(rune('a'+i%26))+strings.Repeat("y", i/26)] = crdv1.JSONSchemaProps{Type: "string", Description: strings.Repeat("long description ", 40)}
		}
		big.Spec.Versions[1].Schema.OpenAPIV3Schema.Properties["spec"] = spec
		h = startHarness(Options{}, objs(big)...)
		out := platform(map[string]any{"service": "dynamodb", "kind": "Table"})
		Expect(toJSON(out.Provisioner.Resource.Spec)).NotTo(ContainSubstring("long description"))
		Expect(out.Provisioner.Resource.Spec["properties"]).To(HaveKey("tableName"))
		Expect(out.Meta.Warnings).To(ContainElement(ContainSubstring("field descriptions were dropped")))
		Expect(out.Meta.Truncated).To(BeTrue(), "a cut schema marks the answer truncated")
	})

	It("keeps the top-level fallback within the size limit, required fields first", func() {
		props := map[string]any{}
		for i := 0; i < 50; i++ {
			props[fmt.Sprintf("f%02d", i)] = map[string]any{"type": "string"}
		}
		spec := map[string]any{"type": "object", "required": []any{"f49"}, "properties": props}
		out, dropped := topLevelOnly(spec, 400)
		b, _ := json.Marshal(out)
		Expect(len(b)).To(BeNumerically("<=", 400))
		Expect(dropped).To(BeNumerically(">", 0))
		Expect(out["properties"]).To(HaveKey("f49"), "required fields are kept")
	})

	It("does not claim a kind is missing when a CRD could not be read", func() {
		failGet := &interceptor.Funcs{Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*crdv1.CustomResourceDefinition); ok && key.Name == "tables.dynamodb.services.k8s.aws" {
				return apierrors.NewForbidden(schema.GroupResource{Resource: "customresourcedefinitions"}, key.Name, errors.New("denied"))
			}
			return c.Get(ctx, key, obj, opts...)
		}}
		backups := testCRD("dynamodb.services.k8s.aws", "backups", "Backup", crdv1.NamespaceScoped, crdv1.JSONSchemaProps{Type: "object"})
		h = startHarnessIntercepted(Options{}, nil, failGet, objs(ackTableCRD(), backups)...)
		p := platform(map[string]any{"service": "dynamodb", "kind": "Table"}).Provisioner
		Expect(p.Provisioner).To(Equal(ProvisionerACK))
		Expect(p.Message).To(ContainSubstring("whether it has a Table kind is unknown"))
		Expect(p.Message).NotTo(ContainSubstring("has no Table kind"))
	})

	It("rejects a kind without a service and a malformed service", func() {
		h = startHarness(Options{}, objs()...)
		res := h.call("get_platform_info", map[string]any{"kind": "Table"}, nil)
		Expect(res.IsError).To(BeTrue())
		Expect(errorText(res)).To(ContainSubstring("kind needs a service"))
		res = h.call("get_platform_info", map[string]any{"service": "DynamoDB Tables"}, nil)
		Expect(res.IsError).To(BeTrue())
		Expect(errorText(res)).To(ContainSubstring("not an AWS service name"))
	})
})
