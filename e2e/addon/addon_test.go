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

package e2e

import (
	"context"
	"encoding/xml"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/e2e"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("Addon Test", func() {
	args := common.Args{Schema: common.Scheme}
	k8sClient, err := args.GetClient()
	Expect(err).Should(BeNil())
	ctx := context.Background()
	originCm := v1.ConfigMap{}
	cm := v1.ConfigMap{}

	BeforeEach(func() {
		server := ghttp.NewServer()
		pathExp, err := regexp.Compile(".+")
		Expect(err).Should(BeNil())
		server.RouteToHandler("GET", pathExp, ossHandler)
		registryCmStr := strings.ReplaceAll(velaRegistry, "REGISTRY_ADDR", server.Addr())

		Expect(yaml.Unmarshal([]byte(registryCmStr), &cm))

		err = k8sClient.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, &originCm)
		if err != nil {
			if apierrors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, &cm)).Should(BeNil())
			} else {
				Expect(err).Should(BeNil())
			}
		} else {
			cm.ResourceVersion = originCm.ResourceVersion
			Expect(k8sClient.Update(ctx, &cm)).Should(BeNil())
		}
	})

	AfterEach(func() {
		// after test we should write configmap back
		latestCm := v1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, &latestCm))
		originCm.ResourceVersion = latestCm.ResourceVersion
		Expect(k8sClient.Update(ctx, &originCm)).Should(BeNil())
	})

	Context("List addons", func() {
		It("List all addon", func() {
			output, err := e2e.Exec("vela addon list")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("test-addon"))
		})

		It("Enable addon test-addon", func() {
			output, err := e2e.Exec("vela addon enable test-addon")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("Successfully enable addon"))
		})

		It("Disable addon test-addon", func() {
			output, err := e2e.LongTimeExec("vela addon disable test-addon", 600*time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("Successfully disable addon"))
			Eventually(func(g Gomega) {
				g.Expect(apierrors.IsNotFound(k8sClient.Get(context.Background(), types.NamespacedName{Name: "addon-test-addon", Namespace: "vela-system"}, &v1beta1.Application{}))).Should(BeTrue())
			}, 60*time.Second).Should(Succeed())
		})

		It("Enable addon with input", func() {
			output, err := e2e.LongTimeExec("vela addon enable test-addon example=redis", 300*time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("Successfully enable addon"))
		})

		It("Disable addon test-addon", func() {
			output, err := e2e.LongTimeExec("vela addon disable test-addon", 600*time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("Successfully disable addon"))
			Eventually(func(g Gomega) {
				g.Expect(apierrors.IsNotFound(k8sClient.Get(context.Background(), types.NamespacedName{Name: "addon-test-addon", Namespace: "vela-system"}, &v1beta1.Application{}))).Should(BeTrue())
			}, 60*time.Second).Should(Succeed())
		})

	})
})

var velaRegistry = `
apiVersion: v1
data:
  registries: '{ "KubeVela":{ "name": "KubeVela", "oss": { "end_point": "http://REGISTRY_ADDR",
    "bucket": "", "path": "stable" } } }'
kind: ConfigMap
metadata:
  name: vela-addon-registry
  namespace: vela-system
`

// ListBucketResult describe a file list from OSS
type ListBucketResult struct {
	Files []File `xml:"Contents"`
	Count int    `xml:"KeyCount"`
}

// File is for oss xml parse
type File struct {
	Name string `xml:"Key"`
	Size int    `xml:"Size"`
}

var ossHandler http.HandlerFunc = func(rw http.ResponseWriter, req *http.Request) {
	queryPath := strings.TrimPrefix(req.URL.Path, "/")
	if strings.Contains(req.URL.RawQuery, "prefix") {
		prefix := req.URL.Query().Get("prefix")
		res := ListBucketResult{
			Files: []File{},
			Count: 0,
		}
		for _, p := range paths {
			if strings.HasPrefix(p, prefix) {
				res.Files = append(res.Files, File{Name: p, Size: 100})
				res.Count += 1
			}
		}
		data, err := xml.Marshal(res)
		if err != nil {
			rw.Write([]byte(err.Error()))
		}
		rw.Write(data)
	} else {
		found := false
		for _, p := range paths {
			if queryPath == p {
				file, err := os.ReadFile(path.Join("testdata", queryPath))
				if err != nil {
					rw.Write([]byte(err.Error()))
				}
				found = true
				rw.Write(file)
				break
			}
		}
		if !found {
			rw.Write([]byte("not found"))
		}
	}
}

var paths = []string{
	"test-addon/metadata.yaml",
	"test-addon/template.yaml",
}
