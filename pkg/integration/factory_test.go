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

package integration

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/stretchr/testify/require"
)

func TestParseIntegrationTemplate(t *testing.T) {
	r := require.New(t)
	content, err := ioutil.ReadFile("testdata/helm-repo.cue")
	r.Equal(err, nil)
	var inf = &kubeIntegrationFactory{}
	template, err := inf.ParseTemplate("default", content)
	r.Equal(err, nil)
	r.NotEqual(template, nil)
	r.Equal(template.Name, "default")
	r.NotEqual(template.Schema, nil)
	r.Equal(len(template.Schema.Properties), 4)
}

var _ = Describe("test integration factory", func() {

	var fac Factory
	BeforeEach(func() {
		fac = NewIntegrationFactory(k8sClient)
	})

	It("apply the nacos server template", func() {
		data, err := os.ReadFile("./testdata/nacos-server.cue")
		Expect(err).Should(BeNil())
		t, err := fac.ParseTemplate("", data)
		Expect(err).Should(BeNil())
		Expect(fac.ApplyTemplate(context.TODO(), "default", t)).Should(BeNil())
	})
	It("apply a integration to the nacos server", func() {

		By("create a nacos server integration")
		nacos, err := fac.ParseIntegration(context.TODO(), "nacos-server", "default", "nacos", "default", map[string]interface{}{
			"servers": []map[string]interface{}{{
				"ipAddr": "127.0.0.1",
				"port":   8849,
			}},
		})
		Expect(err).Should(BeNil())
		Expect(len(nacos.Secret.Data[SaveInputPropertiesKey]) > 0).Should(BeTrue())
		Expect(fac.ApplyIntegration(context.Background(), nacos, "default")).Should(BeNil())

		config, err := fac.ReadIntegration(context.TODO(), "default", "nacos")
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

		Expect(fac.ApplyTemplate(context.TODO(), "default", t)).Should(BeNil())

		db, err := fac.ParseIntegration(context.TODO(), "nacos", "default", "db-config", "default", map[string]interface{}{
			"dataId":  "dbconfig",
			"appName": "db",
			"content": map[string]interface{}{
				"mysqlHost": "127.0.0.1:3306",
				"mysqlPort": 3306,
				"username":  "test",
				"password":  "string",
			},
		})
		Expect(err).Should(BeNil())
		Expect(db.Template.ExpandedWriter).ShouldNot(BeNil())
		Expect(db.ExpandedWriterData).ShouldNot(BeNil())
		Expect(len(db.ExpandedWriterData.Nacos.Content) > 0).Should(BeTrue())
		Expect(db.ExpandedWriterData.Nacos.Metadata.DataID).Should(Equal("dbconfig"))
		Expect(err).Should(BeNil())
		Expect(fac.ApplyIntegration(context.Background(), db, "default")).Should(BeNil())
	})
})
