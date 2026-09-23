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

package config

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	pkgtypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	nacosmock "github.com/oam-dev/kubevela/test/mock/nacos"
)

func TestParseConfigTemplate(t *testing.T) {
	r := require.New(t)
	content, err := os.ReadFile("testdata/helm-repo.cue")
	r.Equal(err, nil)
	var inf = &kubeConfigFactory{}
	template, err := inf.ParseTemplate(context.Background(), "default", content)
	r.Equal(err, nil)
	r.NotEqual(template, nil)
	r.Equal(template.Name, "default")
	r.NotEqual(template.Schema, nil)
	r.Equal(len(template.Schema.Properties), 4)
}

func TestIsConfigCRDOwned(t *testing.T) {
	r := require.New(t)

	legacySecret := &v1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name: "legacy", Namespace: "default",
		Labels: map[string]string{types.LabelConfigCatalog: types.VelaCoreConfig},
	}}
	r.False(isConfigCRDOwned(legacySecret), "a hand-written legacy Secret has no owner reference")

	crdOwnedSecret := &v1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name: "crd-backed", Namespace: "default",
		Labels: map[string]string{types.LabelConfigCatalog: types.VelaCoreConfig},
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: configv1alpha1.SchemeGroupVersion.String(),
			Kind:       configv1alpha1.ConfigKind,
			Name:       "crd-backed",
			Controller: ptrBool(true),
		}},
	}}
	r.True(isConfigCRDOwned(crdOwnedSecret), "the Config CRD reconciler's materialized output Secret must be recognized")

	otherOwnedSecret := &v1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name: "other-owned", Namespace: "default",
		Labels: map[string]string{types.LabelConfigCatalog: types.VelaCoreConfig},
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: "v1",
			Kind:       "ConfigMap",
			Name:       "unrelated",
			Controller: ptrBool(true),
		}},
	}}
	r.False(isConfigCRDOwned(otherOwnedSecret), "an unrelated owner reference must not be mistaken for a Config CR")
}

func ptrBool(b bool) *bool { return &b }

const configTemplateCRDCueScript = `
metadata: { name: "from-crd" }
template: {
	parameter: {
		key: string
	}
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		stringData: {
			key: parameter.key
		}
	}
}
`

func TestConfigTemplateCRDToTemplate(t *testing.T) {
	r := require.New(t)

	t.Run("without status schema, schema is parsed from the template", func(t *testing.T) {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "from-crd", Namespace: "default"},
			Spec: configv1alpha1.ConfigTemplateSpec{
				Template: configTemplateCRDCueScript,
				Alias:    "alias",
				Scope:    configv1alpha1.ConfigTemplateScopeNamespace,
			},
		}
		template, err := configTemplateCRDToTemplate(context.Background(), ct)
		r.NoError(err)
		r.NotNil(template)
		r.Equal("from-crd", template.Name)
		r.Equal("alias", template.Alias)
		r.NotNil(template.Schema)
		r.Contains(template.Schema.Properties, "key")
	})

	t.Run("with a status schema, it is decoded directly", func(t *testing.T) {
		schema := &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeObject}}
		raw, err := json.Marshal(schema)
		r.NoError(err)
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "from-crd", Namespace: "default"},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: configTemplateCRDCueScript},
			Status: configv1alpha1.ConfigTemplateStatus{
				Schema: &runtime.RawExtension{Raw: raw},
			},
		}
		template, err := configTemplateCRDToTemplate(context.Background(), ct)
		r.NoError(err)
		r.NotNil(template.Schema)
		r.True(template.Schema.Type.Includes(openapi3.TypeObject))
	})

	t.Run("with an invalid status schema, it fails to parse", func(t *testing.T) {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "from-crd", Namespace: "default"},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: configTemplateCRDCueScript},
			Status: configv1alpha1.ConfigTemplateStatus{
				Schema: &runtime.RawExtension{Raw: []byte("not-json")},
			},
		}
		_, err := configTemplateCRDToTemplate(context.Background(), ct)
		r.Error(err)
		r.Contains(err.Error(), "fail to parse the schema")
	})

}

var _ = Describe("test config factory", func() {

	var fac Factory
	BeforeEach(func() {
		fac = NewConfigFactory(k8sClient)
	})

	It("apply the nacos server template", func() {
		data, err := os.ReadFile("./testdata/nacos-server.cue")
		Expect(err).Should(BeNil())
		t, err := fac.ParseTemplate(context.Background(), "", data)
		Expect(err).Should(BeNil())
		Expect(fac.CreateOrUpdateConfigTemplate(context.TODO(), "default", t)).Should(BeNil())
	})
	It("apply a config to the nacos server", func() {

		By("create a nacos server config")
		nacos, err := fac.ParseConfig(context.TODO(), NamespacedName{Name: "nacos-server", Namespace: "default"}, Metadata{NamespacedName: NamespacedName{Name: "nacos", Namespace: "default"}, Properties: map[string]interface{}{
			"servers": []map[string]interface{}{{
				"ipAddr": "127.0.0.1",
				"port":   8849,
			}},
		}})
		Expect(err).Should(BeNil())
		Expect(len(nacos.Secret.Data[SaveInputPropertiesKey]) > 0).Should(BeTrue())
		Expect(fac.CreateOrUpdateConfig(context.Background(), nacos, "default")).Should(BeNil())

		config, err := fac.ReadConfig(context.TODO(), "default", "nacos")
		Expect(err).Should(BeNil())
		servers, ok := config["servers"].([]interface{})
		Expect(ok).Should(BeTrue())
		Expect(len(servers)).Should(Equal(1))

		By("apply a template that with the nacos writer")
		data, err := os.ReadFile("./testdata/mysql-db-nacos.cue")
		Expect(err).Should(BeNil())
		t, err := fac.ParseTemplate(context.Background(), "", data)
		Expect(err).Should(BeNil())
		Expect(t.ExpandedWriter.Nacos).ShouldNot(BeNil())
		Expect(t.ExpandedWriter.Nacos.Endpoint.Name).Should(Equal("nacos"))

		Expect(fac.CreateOrUpdateConfigTemplate(context.TODO(), "default", t)).Should(BeNil())

		db, err := fac.ParseConfig(context.TODO(), NamespacedName{Name: "nacos", Namespace: "default"}, Metadata{NamespacedName: NamespacedName{Name: "db-config", Namespace: "default"}, Properties: map[string]interface{}{
			"dataId":  "dbconfig",
			"appName": "db",
			"content": map[string]interface{}{
				"mysqlHost": "127.0.0.1:3306",
				"mysqlPort": 3306,
				"username":  "test",
				"password":  "string",
			},
		}})
		Expect(err).Should(BeNil())
		Expect(db.Template.ExpandedWriter).ShouldNot(BeNil())
		Expect(db.ExpandedWriterData).ShouldNot(BeNil())
		Expect(len(db.ExpandedWriterData.Nacos.Content) > 0).Should(BeTrue())
		Expect(db.ExpandedWriterData.Nacos.Metadata.DataID).Should(Equal("dbconfig"))

		Expect(len(db.OutputObjects)).Should(Equal(1))

		nacosClient := nacosmock.NewMockIConfigClient(ctl)
		db.ExpandedWriterData.Nacos.Client = nacosClient
		nacosClient.EXPECT().PublishConfig(gomock.Any()).Return(true, nil)

		Expect(err).Should(BeNil())
		Expect(fac.CreateOrUpdateConfig(context.Background(), db, "default")).Should(BeNil())

	})

	It("list all templates", func() {
		templates, err := fac.ListTemplates(context.TODO(), "", "")
		Expect(err).Should(BeNil())
		Expect(len(templates)).Should(Equal(2))
	})

	It("list all configs", func() {
		configs, err := fac.ListConfigs(context.TODO(), "", "", "", true)
		Expect(err).Should(BeNil())
		Expect(len(configs)).Should(Equal(2))
	})

	It("distribute a config", func() {
		err := fac.CreateOrUpdateDistribution(context.TODO(), "default", "distribute-db-config", &CreateDistributionSpec{
			Configs: []*NamespacedName{
				{Name: "db-config", Namespace: "default"},
			},
			Targets: []*ClusterTarget{
				{ClusterName: "local", Namespace: "test"},
			},
		})
		Expect(err).Should(BeNil())
	})

	It("get the config", func() {
		config, err := fac.GetConfig(context.TODO(), "default", "db-config", true)
		Expect(err).Should(BeNil())
		Expect(len(config.ObjectReferences)).ShouldNot(BeNil())
		Expect(config.ObjectReferences[0].Kind).Should(Equal("ConfigMap"))
		Expect(len(config.Targets)).Should(Equal(1))
	})

	It("check if the config exist", func() {
		exist, err := fac.IsExist(context.TODO(), "default", "db-config")
		Expect(err).Should(BeNil())
		Expect(exist).Should(BeTrue())
	})

	It("list the distributions", func() {
		distributions, err := fac.ListDistributions(context.TODO(), "default")
		Expect(err).Should(BeNil())
		Expect(len(distributions)).Should(Equal(1))
	})

	It("delete the distribution", func() {
		err := fac.DeleteDistribution(context.TODO(), "default", "distribute-db-config")
		Expect(err).Should(BeNil())
	})

	It("delete the config", func() {
		err := fac.DeleteConfig(context.TODO(), "default", "db-config")
		Expect(err).Should(BeNil())
	})

	It("delete the config template", func() {
		err := fac.DeleteTemplate(context.TODO(), "default", "nacos")
		Expect(err).Should(BeNil())
	})

	It("should fail to parse template with invalid CUE syntax", func() {
		_, err := fac.ParseTemplate(context.Background(), "invalid-cue", []byte("metadata: { name: }"))
		Expect(err).To(HaveOccurred())
	})

	It("should fail to parse template missing template block", func() {
		_, err := fac.ParseTemplate(context.Background(), "missing-template", []byte(`metadata: { name: "t" }`))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("template"))
	})

	It("should load a template from a ConfigTemplate CRD before falling back to the ConfigMap convention", func() {
		ct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "crd-template", Namespace: "default"},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: configTemplateCRDCueScript},
		}
		Expect(k8sClient.Create(context.TODO(), ct)).Should(BeNil())

		template, err := fac.LoadTemplate(context.TODO(), "crd-template", "default")
		Expect(err).Should(BeNil())
		Expect(template.Name).Should(Equal("crd-template"))
	})

	It("should fail to parse config when template not found", func() {
		_, err := fac.ParseConfig(context.TODO(), NamespacedName{Name: "non-existent-template", Namespace: "default"}, Metadata{})
		Expect(err).To(Equal(ErrTemplateNotFound))
	})

	It("should parse a template-less config", func() {
		config, err := fac.ParseConfig(context.TODO(), NamespacedName{}, Metadata{
			NamespacedName: NamespacedName{Name: "template-less-config", Namespace: "default"},
			Properties:     map[string]interface{}{"key": "value"},
		})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(config.Name).To(Equal("template-less-config"))
		Expect(config.Secret.Labels[types.LabelConfigType]).To(Equal(""))
	})

	It("should fail to update config when changing the template", func() {
		nacos, err := fac.ParseConfig(context.TODO(), NamespacedName{Name: "nacos-server", Namespace: "default"}, Metadata{NamespacedName: NamespacedName{Name: "config-to-change", Namespace: "default"}, Properties: map[string]interface{}{
			"servers": []map[string]interface{}{{
				"ipAddr": "127.0.0.1",
				"port":   8849,
			}},
		}})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(fac.CreateOrUpdateConfig(context.Background(), nacos, "default")).ShouldNot(HaveOccurred())

		nacos.Template.Name = "another-template"
		err = fac.CreateOrUpdateConfig(context.Background(), nacos, "default")
		Expect(err).To(Equal(ErrChangeTemplate))
	})

	It("should return error when getting a sensitive config", func() {
		sensitiveTpl, err := fac.ParseTemplate(context.Background(), "", []byte(`
metadata: { name: "sensitive-tpl", sensitive: true }
template: { parameter: { key: string } }
`))
		Expect(err).ShouldNot(HaveOccurred())
		Expect(fac.CreateOrUpdateConfigTemplate(context.TODO(), "default", sensitiveTpl)).ShouldNot(HaveOccurred())

		sensitiveConfig, err := fac.ParseConfig(context.TODO(), NamespacedName{Name: "sensitive-tpl", Namespace: "default"}, Metadata{
			NamespacedName: NamespacedName{Name: "sensitive-config", Namespace: "default"},
			Properties:     map[string]interface{}{"key": "secret-value"},
		})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(fac.CreateOrUpdateConfig(context.Background(), sensitiveConfig, "default")).ShouldNot(HaveOccurred())

		_, err = fac.GetConfig(context.TODO(), "default", "sensitive-config", false)
		Expect(err).To(Equal(ErrSensitiveConfig))
	})

	It("should fail to delete a secret that is not a KubeVela config", func() {
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "not-a-config", Namespace: "default"},
			Data:       map[string][]byte{"key": []byte("value")},
		}
		Expect(k8sClient.Create(context.TODO(), secret)).ShouldNot(HaveOccurred())

		err := fac.DeleteConfig(context.TODO(), "default", "not-a-config")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("is not a config"))
	})

	It("should fail to convert configmap to template if labels are missing", func() {
		cm := v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "no-labels"},
		}
		_, err := convertConfigMap2Template(cm)
		Expect(err).To(HaveOccurred())
	})

	It("should fail to convert secret to config if labels are missing", func() {
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "no-labels"},
		}
		_, err := convertSecret2Config(secret)
		Expect(err).To(HaveOccurred())
	})
})

// A caller that needs its own labels on a template - the SourceDefinition
// controller stamps the owning definition so a sweep can find it - must not have
// to reach past the API into whichever object the factory happens to write.
//
// Reaching through was the status quo: `tmpl.ConfigMap.Labels[...] = x`, guarded
// by a nil check. configTemplateCRDToTemplate returns a Template with no
// ConfigMap at all, so that guard silently dropped the labels the moment a
// ConfigTemplate CR was involved.
func TestParseTemplateCarriesCallerLabels(t *testing.T) {
	r := require.New(t)
	k8sClient := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	f := NewConfigFactory(k8sClient)

	tmpl, err := f.ParseTemplate(context.Background(), "labelled", []byte(`
metadata: {
	name: "labelled"
	scope: "system"
}
template: {
	parameter: {name: string}
	output: {}
}
`))
	r.NoError(err)

	tmpl.Labels = map[string]string{
		"sourcedefinition.oam.dev/name":      "configmap-local",
		"sourcedefinition.oam.dev/namespace": "vela-system",
	}
	r.NoError(f.CreateOrUpdateConfigTemplate(context.Background(), "vela-system", tmpl))

	var cm v1.ConfigMap
	r.NoError(k8sClient.Get(context.Background(),
		pkgtypes.NamespacedName{Namespace: "vela-system", Name: TemplateConfigMapNamePrefix + "labelled"}, &cm))

	r.Equal("configmap-local", cm.Labels["sourcedefinition.oam.dev/name"],
		"a caller label must reach the written object")
	r.Equal("vela-system", cm.Labels["sourcedefinition.oam.dev/namespace"])
	// The factory's own labels are not displaced by the caller's.
	r.Equal(types.VelaCoreConfig, cm.Labels[types.LabelConfigCatalog])
	r.Equal("system", cm.Labels[types.LabelConfigScope])
}

// Same contract as Template.Labels, on the config side. The source cache stamps
// identity and lifetime metadata so a context-free sweep can reason about an
// entry; it should not have to reach into Config.Secret to do it.
//
// Annotations as well as labels: the cache's TTL, last-sync and template-hash
// markers are annotations, and they are what the freshness logic reads.
func TestCreateOrUpdateConfigCarriesCallerMetadata(t *testing.T) {
	r := require.New(t)
	k8sClient := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	f := NewConfigFactory(k8sClient)

	cfg := &Config{
		Metadata: Metadata{NamespacedName: NamespacedName{Name: "cache-entry", Namespace: "vela-system"}},
		Template: Template{NamespacedName: NamespacedName{Name: "source-atlas-abc123"}},
		Secret: &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cache-entry", Namespace: "vela-system",
				Labels: map[string]string{types.LabelConfigType: "source-atlas-abc123"},
			},
			Data: map[string][]byte{SaveInputPropertiesKey: []byte(`{"host":"example.com"}`)},
		},
		Labels: map[string]string{
			"sourcedefinition.oam.dev/name":        "atlas",
			"sourcedefinition.oam.dev/ctx.cluster": "local",
		},
		Annotations: map[string]string{
			types.AnnotationConfigTTL:                "5m0s",
			"sourcedefinition.oam.dev/template-hash": "abc123",
		},
	}
	r.NoError(f.CreateOrUpdateConfig(context.Background(), cfg, "vela-system"))

	var got v1.Secret
	r.NoError(k8sClient.Get(context.Background(),
		pkgtypes.NamespacedName{Namespace: "vela-system", Name: "cache-entry"}, &got))

	r.Equal("atlas", got.Labels["sourcedefinition.oam.dev/name"])
	r.Equal("local", got.Labels["sourcedefinition.oam.dev/ctx.cluster"])
	r.Equal("5m0s", got.Annotations[types.AnnotationConfigTTL])
	r.Equal("abc123", got.Annotations["sourcedefinition.oam.dev/template-hash"])
	// The factory's own type label is not displaced.
	r.Equal("source-atlas-abc123", got.Labels[types.LabelConfigType])
	// And the data still lands in the Secret.
	r.Equal(`{"host":"example.com"}`, string(got.Data[SaveInputPropertiesKey]))
}

// The other half of the contract pinned in pkg/sources by
// TestCacheDataKeyMatchesTheConfigAPI. The source cache reads its data from the
// Secret under this key; ParseConfig writes it here, and the Config CRD's
// controller writes it under the same key when it materialises an entry.
//
// An import cycle stops the two constants being compared directly, so each side
// pins the literal. A rename on either would not fail to compile: the cache
// would read an absent key and report every entry as a miss.
func TestSaveInputPropertiesKeyMatchesTheSourceCache(t *testing.T) {
	require.Equal(t, "input-properties", SaveInputPropertiesKey)
}

// Writing a template as a ConfigTemplate CR, with the caller's labels on it.
//
// The CLI grew its own applyConfigTemplateCRD for this, which sets Spec and
// nothing else - so a caller needing to find its templates later, as the source
// cache sweep does, has nowhere to record ownership.
func TestCreateOrUpdateConfigTemplateCRWritesTheCR(t *testing.T) {
	r := require.New(t)
	k8sClient := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	f := NewConfigFactory(k8sClient)

	tmpl, err := f.ParseTemplate(context.Background(), "atlas-schema", []byte(`
metadata: {
	name: "atlas-schema"
	alias: "atlas"
	scope: "system"
	description: "generated"
}
template: {
	parameter: {host: string}
	output: {}
}
`))
	r.NoError(err)
	tmpl.Labels = map[string]string{"sourcedefinition.oam.dev/name": "atlas"}

	r.NoError(f.CreateOrUpdateConfigTemplateCR(context.Background(), "vela-system", tmpl))

	var ct configv1alpha1.ConfigTemplate
	r.NoError(k8sClient.Get(context.Background(),
		pkgtypes.NamespacedName{Namespace: "vela-system", Name: "atlas-schema"}, &ct))

	r.Equal("atlas", ct.Labels["sourcedefinition.oam.dev/name"], "caller labels reach the CR")
	r.Contains(ct.Spec.Template, "parameter:", "the CUE is carried verbatim")
	r.Equal(configv1alpha1.ConfigTemplateScopeSystem, ct.Spec.Scope)
	r.Equal("atlas", ct.Spec.Alias)
	// ParseTemplate does not carry description off the CUE metadata, so there is
	// none to write. The CLI's own CR writer has the same gap.
	r.Empty(ct.Spec.Description)

	// Idempotent: a second write updates rather than failing.
	tmpl.Labels["sourcedefinition.oam.dev/namespace"] = "vela-system"
	r.NoError(f.CreateOrUpdateConfigTemplateCR(context.Background(), "vela-system", tmpl))
	r.NoError(k8sClient.Get(context.Background(),
		pkgtypes.NamespacedName{Namespace: "vela-system", Name: "atlas-schema"}, &ct))
	r.Equal("vela-system", ct.Labels["sourcedefinition.oam.dev/namespace"])
}
