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
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/require"

	nacosmock "github.com/oam-dev/kubevela/test/mock/nacos"
)

func TestParseConfigTemplate(t *testing.T) {
	r := require.New(t)
	content, err := os.ReadFile("testdata/helm-repo.cue")
	r.Equal(err, nil)
	var inf = &kubeConfigFactory{}
	template, err := inf.ParseTemplate("default", content)
	r.Equal(err, nil)
	r.NotEqual(template, nil)
	r.Equal(template.Name, "default")
	r.NotEqual(template.Schema, nil)
	r.Equal(len(template.Schema.Properties), 4)
}

var _ = Describe("test config factory", func() {

	var fac Factory
	BeforeEach(func() {
		fac = NewConfigFactory(k8sClient)
	})

	It("apply the nacos server template", func() {
		data, err := os.ReadFile("./testdata/nacos-server.cue")
		Expect(err).Should(BeNil())
		t, err := fac.ParseTemplate("", data)
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
		t, err := fac.ParseTemplate("", data)
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
})
