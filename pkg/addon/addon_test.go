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

package addon

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-github/v32/github"
	v1alpha12 "github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

var paths = []string{
	"example/metadata.yaml",
	"example/readme.md",
	"example/template.yaml",
	"example/definitions/helm.yaml",
	"example/resources/configmap.cue",
	"example/resources/parameter.cue",
	"example/resources/service/source-controller.yaml",

	"terraform/metadata.yaml",
	"terraform-alibaba/metadata.yaml",

	"test-error-addon/metadata.yaml",
	"test-error-addon/resources/parameter.cue",
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
				// Size 100 is for mock a file
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

var ctx = context.Background()

func TestGetAddon(t *testing.T) {
	server := httptest.NewServer(ossHandler)
	defer server.Close()

	_, err := NewAsyncReader(server.URL, "", "", "", ossType)
	assert.NoError(t, err)

	//registryMeta, err := reader.ListAddonMeta(".")
	//assert.NoError(t, err)
	//
	//testAddonName := "example"
	//var testAddonMeta SourceMeta
	//for _, m := range registryMeta {
	//	if m.Name == testAddonName {
	//		testAddonMeta = m
	//		break
	//	}
	//}
	//assert.NoError(t, err)
	//addon, err := GetUIDataFromReader(reader, &testAddonMeta, UIMetaOptions)
	//assert.NoError(t, err)
	//assert.Equal(t, addon.Name, testAddonName)
	//assert.True(t, addon.Parameters != "")
	//assert.True(t, len(addon.Definitions) > 0)
	//
	//addons, err := GetAddonUIMetaFromReader(reader, registryMeta, UIMetaOptions)
	//assert.True(t, strings.Contains(err.Error(), "#parameter.example: preference mark not allowed at this position"))
	//assert.Equal(t, len(addons), 3)
}

func TestRender(t *testing.T) {
	testcases := []struct {
		envs   []ObservabilityEnvironment
		tmpl   string
		expect string
		err    error
	}{
		{
			envs: []ObservabilityEnvironment{
				{
					Cluster: "c1",
					Domain:  "a.com",
				},
				{
					Cluster: "c2",
					Domain:  "b.com",
				},
			},
			tmpl: ObservabilityEnvBindingEnvTmpl,
			expect: `
        
          
          - name: c1
            placement:
              clusterSelector:
                name: c1
            patch:
              components:
                - name: grafana
                  type: helm
                  traits:
                    - type: pure-ingress
                      properties:
                        domain: a.com
          
          - name: c2
            placement:
              clusterSelector:
                name: c2
            patch:
              components:
                - name: grafana
                  type: helm
                  traits:
                    - type: pure-ingress
                      properties:
                        domain: b.com
          
        `,

			err: nil,
		},
		{
			envs: []ObservabilityEnvironment{
				{
					Cluster: "c1",
					Domain:  "a.com",
				},
				{
					Cluster: "c2",
					Domain:  "b.com",
				},
			},
			tmpl: ObservabilityWorkflow4EnvBindingTmpl,
			expect: `

  
  - name: c1
    type: deploy2env
    properties:
      policy: domain
      env: c1
      parallel: true
  
  - name: c2
    type: deploy2env
    properties:
      policy: domain
      env: c2
      parallel: true
  
`,

			err: nil,
		},
	}
	for _, tc := range testcases {
		t.Run("", func(t *testing.T) {
			rendered, err := render(tc.envs, tc.tmpl)
			assert.Equal(t, tc.err, err)
			assert.Equal(t, tc.expect, rendered)
		})
	}
}

func TestRenderApp(t *testing.T) {
	addon := baseAddon
	app, err := RenderApp(ctx, &addon, nil, nil, map[string]interface{}{})
	assert.NoError(t, err, "render app fail")
	assert.Equal(t, len(app.Spec.Components), 2)
}

func TestRenderDeploy2RuntimeAddon(t *testing.T) {
	addonDeployToRuntime := baseAddon
	addonDeployToRuntime.Meta.DeployTo = &DeployTo{
		ControlPlane:   true,
		RuntimeCluster: true,
	}
	defs, err := RenderDefinitions(&addonDeployToRuntime, nil)
	assert.NoError(t, err)
	assert.Equal(t, len(defs), 1)
	def := defs[0]
	assert.Equal(t, def.GetAPIVersion(), "core.oam.dev/v1beta1")
	assert.Equal(t, def.GetKind(), "TraitDefinition")

	app, err := RenderApp(ctx, &addonDeployToRuntime, nil, nil, map[string]interface{}{})
	assert.NoError(t, err)
	steps := app.Spec.Workflow.Steps
	assert.True(t, len(steps) >= 2)
	assert.Equal(t, steps[len(steps)-2].Type, "apply-application")
	assert.Equal(t, steps[len(steps)-1].Type, "deploy2runtime")
}

func TestGetAddonStatus(t *testing.T) {
	getFunc := test.MockGetFn(func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
		switch key.Name {
		case "addon-disabled", "disabled":
			return errors.NewNotFound(schema.GroupResource{Group: "apiVersion: core.oam.dev/v1beta1", Resource: "app"}, key.Name)
		case "addon-suspend":
			o := obj.(*v1beta1.Application)
			app := &v1beta1.Application{}
			app.Status.Workflow = &common.WorkflowStatus{Suspend: true}
			*o = *app
		case "addon-enabled":
			o := obj.(*v1beta1.Application)
			app := &v1beta1.Application{}
			app.Status.Phase = common.ApplicationRunning
			*o = *app
		case "addon-disabling":
			o := obj.(*v1beta1.Application)
			app := &v1beta1.Application{}
			app.Status.Phase = common.ApplicationDeleting
			*o = *app
		default:
			o := obj.(*v1beta1.Application)
			app := &v1beta1.Application{}
			app.Status.Phase = common.ApplicationRendering
			*o = *app
		}
		return nil
	})

	cli := test.MockClient{
		MockGet: getFunc,
	}

	cases := []struct {
		name         string
		expectStatus string
	}{
		{
			name: "disabled", expectStatus: "disabled",
		},
		{
			name: "suspend", expectStatus: "suspend",
		},
		{
			name: "enabled", expectStatus: "enabled",
		},
		{
			name: "disabling", expectStatus: "disabling",
		},
		{
			name: "enabling", expectStatus: "enabling",
		},
	}

	for _, s := range cases {
		addonStatus, err := GetAddonStatus(context.Background(), &cli, s.name)
		assert.NoError(t, err)
		assert.Equal(t, addonStatus.AddonPhase, s.expectStatus)
	}
}

var baseAddon = InstallPackage{
	Meta: Meta{
		Name:          "test-render-cue-definition-addon",
		NeedNamespace: []string{"test-ns"},
	},
	CUEDefinitions: []ElementFile{
		{
			Data: testCueDef,
			Name: "test-def",
		},
	},
}

var testCueDef = `annotations: {
	type: "trait"
	annotations: {}
	labels: {
		"ui-hidden": "true"
	}
	description: "Add annotations on K8s pod for your workload which follows the pod spec in path 'spec.template'."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["*"]
	}
}
template: {
	patch: {
		metadata: {
			annotations: {
				for k, v in parameter {
					"\(k)": v
				}
			}
		}
		spec: template: metadata: annotations: {
			for k, v in parameter {
				"\(k)": v
			}
		}
	}
	parameter: [string]: string
}
`

func TestRenderApp4Observability(t *testing.T) {
	k8sClient := fake.NewClientBuilder().Build()
	testcases := []struct {
		addon       InstallPackage
		args        map[string]interface{}
		application string
		err         error
	}{
		{
			addon: InstallPackage{
				Meta: Meta{
					Name: "observability",
				},
			},
			args:        map[string]interface{}{},
			application: "",
			err:         ErrorNoDomain,
		},
		{
			addon: InstallPackage{
				Meta: Meta{
					Name: "observability",
				},
			},
			args: map[string]interface{}{
				"domain": "a.com",
			},
			application: `{"kind":"Application","apiVersion":"core.oam.dev/v1beta1","metadata":{"name":"addon-observability","namespace":"vela-system","creationTimestamp":null,"labels":{"addons.oam.dev/name":"observability"}},"spec":{"components":[],"policies":[{"name":"domain","type":"env-binding","properties":{"envs":null}}],"workflow":{"steps":[{"name":"deploy-control-plane","type":"apply-application-in-parallel"}]}},"status":{}}`,
		},
	}
	for _, tc := range testcases {
		t.Run("", func(t *testing.T) {
			app, err := RenderApp(ctx, &tc.addon, nil, k8sClient, tc.args)
			assert.Equal(t, tc.err, err)
			if app != nil {
				data, err := json.Marshal(app)
				assert.NoError(t, err)
				assert.Equal(t, tc.application, string(data))
			}
		})
	}
}

// TestRenderApp4ObservabilityWithEnvBinding tests the case of RenderApp for Addon Observability with some Kubernetes data
func TestRenderApp4ObservabilityWithK8sData(t *testing.T) {
	k8sClient := fake.NewClientBuilder().Build()
	ctx := context.Background()
	secret1 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-secret",
			Labels: map[string]string{
				v1alpha12.LabelKeyClusterCredentialType: string(v1alpha12.CredentialTypeX509Certificate),
			},
		},
		Data: map[string][]byte{
			"test-key": []byte("test-value"),
		},
	}
	err := k8sClient.Create(ctx, secret1)
	assert.NoError(t, err)

	testcases := []struct {
		addon       InstallPackage
		args        map[string]interface{}
		application string
		err         error
	}{
		{
			addon: InstallPackage{
				Meta: Meta{
					Name: "observability",
				},
			},
			args: map[string]interface{}{
				"domain": "a.com",
			},
			application: `{"kind":"Application","apiVersion":"core.oam.dev/v1beta1","metadata":{"name":"addon-observability","namespace":"vela-system","creationTimestamp":null,"labels":{"addons.oam.dev/name":"observability"}},"spec":{"components":[],"policies":[{"name":"domain","type":"env-binding","properties":{"envs":[{"name":"test-secret","patch":{"components":[{"name":"grafana","traits":[{"properties":{"domain":"test-secret.a.com"},"type":"pure-ingress"}],"type":"helm"}]},"placement":{"clusterSelector":{"name":"test-secret"}}}]}}],"workflow":{"steps":[{"name":"deploy-control-plane","type":"apply-application-in-parallel"},{"name":"test-secret","type":"deploy2env","properties":{"env":"test-secret","parallel":true,"policy":"domain"}}]}},"status":{}}`,
		},
	}
	for _, tc := range testcases {
		t.Run("", func(t *testing.T) {
			app, err := RenderApp(ctx, &tc.addon, nil, k8sClient, tc.args)
			assert.Equal(t, tc.err, err)
			if app != nil {
				data, err := json.Marshal(app)
				assert.NoError(t, err)
				assert.Equal(t, tc.application, string(data))
			}
		})
	}
}

func TestGetPatternFromItem(t *testing.T) {
	ossR, err := NewAsyncReader("http://ep.beijing", "some-bucket", "some-sub-path", "", ossType)
	assert.NoError(t, err)
	gitR, err := NewAsyncReader("https://github.com/oam-dev/catalog", "", "addons", "", gitType)
	assert.NoError(t, err)
	gitItemName := "parameter.cue"
	gitItemType := FileType
	gitItemPath := "addons/terraform/resources/parameter.cue"
	testCases := []struct {
		caseName    string
		item        Item
		root        string
		meetPattern string
		r           AsyncReader
	}{
		{
			caseName: "OSS case",
			item: OSSItem{
				tp:   FileType,
				path: "terraform/resources/parameter.cue",
				name: "parameter.cue",
			},
			root:        "terraform",
			meetPattern: "resources/parameter.cue",
			r:           ossR,
		},
		{
			caseName:    "git case",
			item:        &github.RepositoryContent{Name: &gitItemName, Type: &gitItemType, Path: &gitItemPath},
			root:        "terraform",
			meetPattern: "resources/parameter.cue",
			r:           gitR,
		},
	}
	for _, tc := range testCases {
		res := GetPatternFromItem(tc.item, tc.r, tc.root)
		assert.Equal(t, res, tc.meetPattern, tc.caseName)
	}
}
