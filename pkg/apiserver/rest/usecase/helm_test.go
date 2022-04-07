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

package usecase

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/oam/util"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/stretchr/testify/assert"
)

func TestFlattenKeyFunc(t *testing.T) {
	srcMap := map[string]interface{}{}
	err := json.Unmarshal([]byte(src), &srcMap)
	assert.NoError(t, err)

	dstMap := map[string]interface{}{}
	err = json.Unmarshal([]byte(dst), &dstMap)
	assert.NoError(t, err)

	res := map[string]interface{}{}
	flattenKey("", srcMap, res)
	assert.Equal(t, dstMap, res)
}

var _ = Describe("Test helm repo list", func() {
	ctx := context.Background()
	var pSec, gSec, aSec v1.Secret

	BeforeEach(func() {
		pSec = v1.Secret{}
		gSec = v1.Secret{}
		aSec = v1.Secret{}
		Expect(k8sClient.Create(ctx, &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vela-system"}})).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		Expect(yaml.Unmarshal([]byte(projectSecret), &pSec)).Should(BeNil())
		Expect(yaml.Unmarshal([]byte(globalSecret), &gSec)).Should(BeNil())
		Expect(yaml.Unmarshal([]byte(authSecret), &aSec)).Should(BeNil())
		Expect(k8sClient.Create(ctx, &pSec)).Should(BeNil())
		Expect(k8sClient.Create(ctx, &gSec)).Should(BeNil())
		Expect(k8sClient.Create(ctx, &aSec)).Should(BeNil())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, &gSec)).Should(BeNil())
		Expect(k8sClient.Delete(ctx, &pSec)).Should(BeNil())
		Expect(k8sClient.Delete(ctx, &aSec)).Should(BeNil())
	})

	It("Test list with project ", func() {
		u := NewHelmUsecase()
		list, err := u.ListChartRepo(ctx, "my-project")
		Expect(err).Should(BeNil())
		Expect(len(list.ChartRepoResponse)).Should(BeEquivalentTo(2))
		found := 0
		for _, response := range list.ChartRepoResponse {
			if response.SecretName == "project-helm-repo" {
				Expect(response.URL).Should(BeEquivalentTo("https://kedacore.github.io/charts"))
				found++
			}
			if response.SecretName == "global-helm-repo" {
				Expect(response.URL).Should(BeEquivalentTo("https://charts.bitnami.com/bitnami"))
				found++
			}
		}
		Expect(found).Should(BeEquivalentTo(2))
	})

	It("Test list func with not exist project", func() {
		u := NewHelmUsecase()
		list, err := u.ListChartRepo(ctx, "not-exist-project")
		Expect(err).Should(BeNil())
		Expect(len(list.ChartRepoResponse)).Should(BeEquivalentTo(1))
		Expect(list.ChartRepoResponse[0].URL).Should(BeEquivalentTo("https://charts.bitnami.com/bitnami"))
		Expect(list.ChartRepoResponse[0].SecretName).Should(BeEquivalentTo("global-helm-repo"))
	})

	It("Test list func without project", func() {
		u := NewHelmUsecase()
		list, err := u.ListChartRepo(ctx, "")
		Expect(err).Should(BeNil())
		Expect(len(list.ChartRepoResponse)).Should(BeEquivalentTo(1))
		Expect(list.ChartRepoResponse[0].URL).Should(BeEquivalentTo("https://charts.bitnami.com/bitnami"))
		Expect(list.ChartRepoResponse[0].SecretName).Should(BeEquivalentTo("global-helm-repo"))
	})

	It("Test auth info secret func", func() {
		opts, err := setAuthInfo(context.Background(), k8sClient, "auth-secret")
		Expect(err).Should(BeNil())
		Expect(opts.Username).Should(BeEquivalentTo("admin"))
		Expect(opts.Password).Should(BeEquivalentTo("admin"))
	})

	It("Test auth info secret func", func() {
		_, err := setAuthInfo(context.Background(), k8sClient, "auth-secret-1")
		Expect(err).ShouldNot(BeNil())
	})
})

var _ = Describe("test helm usecasae", func() {
	ctx := context.Background()
	var repoSec v1.Secret

	BeforeEach(func() {
		Expect(k8sClient.Create(ctx, &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vela-system"}})).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))

		repoSec = v1.Secret{}
		Expect(yaml.Unmarshal([]byte(repoSecret), &repoSec)).Should(BeNil())
		Expect(k8sClient.Create(ctx, &repoSec)).Should(BeNil())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, &repoSec)).Should(BeNil())
	})

	It("helm associated usecase interface test", func() {
		var mockServer *httptest.Server

		handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			u, p, ok := request.BasicAuth()
			if !ok || u != "admin" || p != "admin" {
				writer.WriteHeader(401)
				return
			}
			switch {
			case request.URL.Path == "/index.yaml":
				index, err := ioutil.ReadFile("./testdata/helm/index.yaml")
				indexFile := string(index)
				indexFile = strings.ReplaceAll(indexFile, "server-url", mockServer.URL)
				if err != nil {
					writer.Write([]byte(err.Error()))
					return
				}
				writer.Write([]byte(indexFile))

				return
			case strings.Contains(request.URL.Path, "mysql-8.8.23.tgz"):
				pkg, err := ioutil.ReadFile("./testdata/helm/mysql-8.8.23.tgz")
				if err != nil {
					writer.Write([]byte(err.Error()))
					return
				}
				writer.Write(pkg)
				return
			default:
				writer.Write([]byte("404 page not found"))
			}
		})

		mockServer = httptest.NewServer(handler)

		defer mockServer.Close()

		u := NewHelmUsecase()
		charts, err := u.ListChartNames(ctx, mockServer.URL, "repo-secret", false)
		Expect(err).Should(BeNil())
		Expect(len(charts)).Should(BeEquivalentTo(1))
		Expect(charts[0]).Should(BeEquivalentTo("mysql"))

		versions, err := u.ListChartVersions(ctx, mockServer.URL, "mysql", "repo-secret", false)
		Expect(err).Should(BeNil())
		Expect(len(versions)).Should(BeEquivalentTo(1))
		Expect(versions[0].Version).Should(BeEquivalentTo("8.8.23"))

		values, err := u.GetChartValues(ctx, mockServer.URL, "mysql", "8.8.23", "repo-secret", false)
		Expect(err).Should(BeNil())
		Expect(values).ShouldNot(BeNil())
		Expect(len(values)).ShouldNot(BeEquivalentTo(0))
	})
})

var (
	src = `{
    "OAMSpecVer":"v0.2",
    "admissionWebhooks":{
        "autoGenWorkloadDefinition":true,
        "certManager":{
            "enabled":false,
            "revisionHistoryLimit":3
        },
        "certificate":{
            "mountPath":"/etc/k8s-webhook-certs"
        },
        "enabled":true,
        "failurePolicy":"Fail",
        "patch":{
            "affinity":{

            },
            "enabled":true,
            "image":{
                "pullPolicy":"IfNotPresent",
                "repository":"oamdev/kube-webhook-certgen",
                "tag":"v2.3"
            },
            "tolerations":[

            ]
        }
    },
    "affinity":{

    },
    "applyOnceOnly":"off",
    "concurrentReconciles":4,
    "dependCheckWait":"30s",
    "disableCaps":"all",
    "fullnameOverride":"",
    "healthCheck":{
        "port":11440
    },
    "image":{
        "pullPolicy":"Always",
        "repository":"oamdev/vela-core",
        "tag":"v1.2.4"
    },
    "imagePullSecrets":[

    ],
    "imageRegistry":"",
    "logDebug":false,
    "logFileMaxSize":1024,
    "logFilePath":"",
    "nameOverride":"",
    "nodeSelector":{

    },
    "podSecurityContext":{

    },
    "rbac":{
        "create":true
    },
    "replicaCount":1,
    "resources":{
        "limits":{
            "cpu":"500m",
            "memory":"1Gi"
        },
        "requests":{
            "cpu":"50m",
            "memory":"20Mi"
        }
    },
    "securityContext":{

    },
    "serviceAccount":{
        "annotations":{

        },
        "create":true,
        "name":null
    },
    "systemDefinitionNamespace":"oam-runtime-system",
    "test":{
        "app":{
            "repository":"oamdev/busybox",
            "tag":"v1"
        }
    },
    "tolerations":[

    ],
    "webhookService":{
        "port":11443,
        "type":"ClusterIP"
    }
}`
	dst = `{
    "OAMSpecVer": "v0.2",
    "admissionWebhooks.autoGenWorkloadDefinition": true,
    "admissionWebhooks.certManager.enabled": false,
    "admissionWebhooks.certManager.revisionHistoryLimit": 3,
    "admissionWebhooks.certificate.mountPath": "/etc/k8s-webhook-certs",
    "admissionWebhooks.enabled": true,
    "admissionWebhooks.failurePolicy": "Fail",
    "admissionWebhooks.patch.enabled": true,
    "admissionWebhooks.patch.image.pullPolicy": "IfNotPresent",
    "admissionWebhooks.patch.image.repository": "oamdev/kube-webhook-certgen",
    "admissionWebhooks.patch.image.tag": "v2.3",
    "applyOnceOnly": "off",
    "concurrentReconciles": 4,
    "dependCheckWait": "30s",
    "disableCaps": "all",
    "fullnameOverride": "",
    "healthCheck.port": 11440,
    "image.pullPolicy": "Always",
    "image.repository": "oamdev/vela-core",
    "image.tag": "v1.2.4",
    "imageRegistry": "",
    "logDebug": false,
    "logFileMaxSize": 1024,
    "logFilePath": "",
    "nameOverride": "",
    "rbac.create": true,
    "replicaCount": 1,
    "resources.limits.cpu": "500m",
    "resources.limits.memory": "1Gi",
    "resources.requests.cpu": "50m",
    "resources.requests.memory": "20Mi",
    "serviceAccount.create": true,
    "serviceAccount.name": null,
    "systemDefinitionNamespace": "oam-runtime-system",
    "test.app.repository": "oamdev/busybox",
    "test.app.tag": "v1",
    "webhookService.port": 11443,
    "webhookService.type": "ClusterIP"
}`
	globalSecret = `
apiVersion: v1
stringData:
  url: https://charts.bitnami.com/bitnami
kind: Secret
metadata:
  labels:
    config.oam.dev/type: config-helm-repository
  name: global-helm-repo
  namespace: vela-system
type: Opaque
`
	projectSecret = `
apiVersion: v1
kind: Secret
metadata:
  name: project-helm-repo
  namespace: vela-system
  labels:
    config.oam.dev/type: config-helm-repository
    config.oam.dev/project: my-project
stringData:
  url: https://kedacore.github.io/charts
type: Opaque
`
	authSecret = `
apiVersion: v1
kind: Secret
metadata:
  name: auth-secret
  namespace: vela-system
  labels:
    config.oam.dev/type: config-helm-repository
    config.oam.dev/project: my-project-1
stringData:
  url: https://kedacore.github.io/charts
  username: admin
  password: admin
type: Opaque
`
	repoSecret = `
apiVersion: v1
kind: Secret
metadata:
  name: repo-secret
  namespace: vela-system
  labels:
    config.oam.dev/type: config-helm-repository
    config.oam.dev/project: my-project-2
stringData:
  username: admin
  password: admin
type: Opaque
`
)
