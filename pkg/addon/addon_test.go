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
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-github/v32/github"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	addonutil "github.com/oam-dev/kubevela/pkg/utils/addon"
	version2 "github.com/oam-dev/kubevela/version"
)

var paths = []string{
	"example/metadata.yaml",
	"example/readme.md",
	"example/template.cue",
	"example/definitions/helm.yaml",
	"example/resources/configmap.cue",
	"example/parameter.cue",
	"example/resources/service/source-controller.yaml",

	"example-legacy/metadata.yaml",
	"example-legacy/readme.md",
	"example-legacy/template.yaml",
	"example-legacy/definitions/helm.yaml",
	"example-legacy/resources/configmap.cue",
	"example-legacy/resources/parameter.cue",
	"example-legacy/resources/service/source-controller.yaml",

	"terraform/metadata.yaml",
	"terraform-alibaba/metadata.yaml",

	"test-error-addon/metadata.yaml",
	"test-error-addon/resources/parameter.cue",

	"test-disable-addon/metadata.yaml",
	"test-disable-addon/definitions/compDef.yaml",
	"test-disable-addon/definitions/traitDef.cue",
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

var helmHandler http.HandlerFunc = func(writer http.ResponseWriter, request *http.Request) {
	switch {
	case strings.Contains(request.URL.Path, "index.yaml"):
		files, err := os.ReadFile("./testdata/multiversion-helm-repo/index.yaml")
		if err != nil {
			_, _ = writer.Write([]byte(err.Error()))
		}
		writer.Write(files)
	case strings.Contains(request.URL.Path, "fluxcd-1.0.0.tgz"):
		files, err := os.ReadFile("./testdata/multiversion-helm-repo/fluxcd-1.0.0.tgz")
		if err != nil {
			_, _ = writer.Write([]byte(err.Error()))
		}
		writer.Write(files)
	case strings.Contains(request.URL.Path, "fluxcd-2.0.0.tgz"):
		files, err := os.ReadFile("./testdata/multiversion-helm-repo/fluxcd-2.0.0.tgz")
		if err != nil {
			_, _ = writer.Write([]byte(err.Error()))
		}
		writer.Write(files)
	}
}

var ctx = context.Background()

func testReaderFunc(t *testing.T, reader AsyncReader) {
	registryMeta, err := reader.ListAddonMeta()
	assert.NoError(t, err)

	testAddonName := "example"
	var testAddonMeta SourceMeta
	for _, m := range registryMeta {
		if m.Name == testAddonName {
			testAddonMeta = m
			break
		}
	}
	assert.NoError(t, err)
	uiData, err := GetUIDataFromReader(reader, &testAddonMeta, UIMetaOptions)
	assert.NoError(t, err)
	assert.Equal(t, uiData.Name, testAddonName)
	assert.True(t, uiData.Parameters != "")
	assert.Equal(t, len(uiData.APISchema.Properties), 1)
	assert.Equal(t, uiData.APISchema.Properties["example"].Value.Description, "the example field")
	assert.True(t, len(uiData.Definitions) > 0)

	testAddonName = "example-legacy"
	for _, m := range registryMeta {
		if m.Name == testAddonName {
			testAddonMeta = m
			break
		}
	}
	assert.NoError(t, err)
	uiData, err = GetUIDataFromReader(reader, &testAddonMeta, UIMetaOptions)
	assert.NoError(t, err)
	assert.Equal(t, uiData.Name, testAddonName)
	assert.True(t, uiData.Parameters != "")
	assert.True(t, len(uiData.Definitions) > 0)

	// test get ui data
	rName := "KubeVela"
	uiDataList, err := ListAddonUIDataFromReader(reader, registryMeta, rName, UIMetaOptions)
	fmt.Println(err.Error())
	assert.True(t, strings.Contains(err.Error(), "preference mark not allowed at this position"))
	assert.Equal(t, 5, len(uiDataList))
	assert.Equal(t, uiDataList[0].RegistryName, rName)

	// test get install package
	installPkg, err := GetInstallPackageFromReader(reader, &testAddonMeta, uiData)
	assert.NoError(t, err)
	assert.NotNil(t, installPkg, "should get install package")
	assert.Equal(t, len(installPkg.CUETemplates), 1)
}

func TestGetAddonData(t *testing.T) {
	server := httptest.NewServer(ossHandler)
	defer server.Close()

	reader, err := NewAsyncReader(server.URL, "", "", "", "", ossType)
	assert.NoError(t, err)
	testReaderFunc(t, reader)
}

func TestRenderApp(t *testing.T) {
	addon := baseAddon
	app, _, err := RenderApp(ctx, &addon, nil, map[string]interface{}{})
	assert.NoError(t, err, "render app fail")
	// definition should not be rendered
	assert.Equal(t, len(app.Spec.Components), 1)
}

func TestRenderAppWithNeedNamespace(t *testing.T) {
	addon := baseAddon
	addon.NeedNamespace = append(addon.NeedNamespace, types.DefaultKubeVelaNS, "test-ns2")
	app, _, err := RenderApp(ctx, &addon, nil, map[string]interface{}{})
	assert.NoError(t, err, "render app fail")
	assert.Equal(t, len(app.Spec.Components), 2)
	for _, c := range app.Spec.Components {
		assert.NotEqual(t, types.DefaultKubeVelaNS+"-namespace", c.Name, "namespace vela-system should not be rendered")
	}
}

func TestRenderDeploy2RuntimeAddon(t *testing.T) {
	addonDeployToRuntime := baseAddon
	addonDeployToRuntime.Meta.DeployTo = &DeployTo{
		DisableControlPlane: false,
		RuntimeCluster:      true,
	}
	defs, err := RenderDefinitions(&addonDeployToRuntime, nil)
	assert.NoError(t, err)
	assert.Equal(t, len(defs), 1)
	def := defs[0]
	assert.Equal(t, def.GetAPIVersion(), "core.oam.dev/v1beta1")
	assert.Equal(t, def.GetKind(), "TraitDefinition")

	app, _, err := RenderApp(ctx, &addonDeployToRuntime, nil, map[string]interface{}{})
	assert.NoError(t, err)
	policies := app.Spec.Policies
	assert.True(t, len(policies) == 1)
	assert.Equal(t, policies[0].Type, v1alpha1.TopologyPolicyType)
}

func TestRenderDefinitions(t *testing.T) {
	addonDeployToRuntime := baseAddon
	addonDeployToRuntime.Meta.DeployTo = &DeployTo{
		DisableControlPlane: false,
		RuntimeCluster:      false,
	}
	defs, err := RenderDefinitions(&addonDeployToRuntime, nil)
	assert.NoError(t, err)
	assert.Equal(t, len(defs), 1)
	def := defs[0]
	assert.Equal(t, def.GetAPIVersion(), "core.oam.dev/v1beta1")
	assert.Equal(t, def.GetKind(), "TraitDefinition")

	app, _, err := RenderApp(ctx, &addonDeployToRuntime, nil, map[string]interface{}{})
	assert.NoError(t, err)
	// addon which app work on no-runtime-cluster mode workflow is nil
	assert.Nil(t, app.Spec.Workflow)
}

func TestRenderViews(t *testing.T) {
	addonDeployToRuntime := viewAddon
	addonDeployToRuntime.Meta.DeployTo = &DeployTo{
		DisableControlPlane: false,
		RuntimeCluster:      false,
	}
	views, err := RenderViews(&addonDeployToRuntime)
	assert.NoError(t, err)
	assert.Equal(t, len(views), 2)

	view := views[0]
	assert.Equal(t, view.GetKind(), "ConfigMap")
	assert.Equal(t, view.GetAPIVersion(), "v1")
	assert.Equal(t, view.GetNamespace(), types.DefaultKubeVelaNS)
	assert.Equal(t, view.GetName(), "cloud-resource-view")

	view = views[1]
	assert.Equal(t, view.GetKind(), "ConfigMap")
	assert.Equal(t, view.GetAPIVersion(), "v1")
	assert.Equal(t, view.GetNamespace(), types.DefaultKubeVelaNS)
	assert.Equal(t, view.GetName(), "pod-view")

}

func TestRenderK8sObjects(t *testing.T) {
	addonMultiYaml := multiYamlAddon
	addonMultiYaml.Meta.DeployTo = &DeployTo{
		DisableControlPlane: false,
		RuntimeCluster:      false,
	}

	app, _, err := RenderApp(ctx, &addonMultiYaml, nil, map[string]interface{}{})
	assert.NoError(t, err)
	assert.Equal(t, len(app.Spec.Components), 1)
	comp := app.Spec.Components[0]
	assert.Equal(t, comp.Type, "k8s-objects")
}

func TestGetClusters(t *testing.T) {
	// string array test
	args := map[string]interface{}{
		types.ClustersArg: []string{
			"cluster1", "cluster2",
		},
	}
	clusters := getClusters(args)
	assert.Equal(t, clusters, []string{
		"cluster1", "cluster2",
	})
	// interface array test
	args1 := map[string]interface{}{
		types.ClustersArg: []interface{}{
			"cluster3", "cluster4",
		},
	}
	clusters1 := getClusters(args1)
	assert.Equal(t, clusters1, []string{
		"cluster3", "cluster4",
	})
	// no cluster arg test
	args2 := map[string]interface{}{
		"anyargkey": "anyargvalue",
	}
	clusters2 := getClusters(args2)
	assert.Nil(t, clusters2)
	// other type test
	args3 := map[string]interface{}{
		types.ClustersArg: "cluster5",
	}
	clusters3 := getClusters(args3)
	assert.Nil(t, clusters3)
}

func TestGetAddonStatus(t *testing.T) {
	getFunc := test.MockGetFn(func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
		switch key.Name {
		case "addon-disabled", "disabled":
			return kerrors.NewNotFound(schema.GroupResource{Group: "apiVersion: core.oam.dev/v1beta1", Resource: "app"}, key.Name)
		case "addon-suspend":
			o := obj.(*v1beta1.Application)
			app := &v1beta1.Application{}
			app.Status.Workflow = &common.WorkflowStatus{
				Suspend: true,
			}
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
		case "addon-secret-enabled":
			o := obj.(*corev1.Secret)
			secret := &corev1.Secret{}
			secret.Data = map[string][]byte{
				"some-key": []byte("some-value"),
			}
			*o = *secret
		case "addon-secret-disabling", "addon-secret-enabling":
			o := obj.(*corev1.Secret)
			secret := &corev1.Secret{}
			secret.Data = map[string][]byte{}
			*o = *secret
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
		name               string
		expectStatus       string
		expectedParameters map[string]interface{}
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

func TestGetAddonVersionMeetSystemRequirement(t *testing.T) {
	server := httptest.NewServer(helmHandler)
	defer server.Close()
	i := &Installer{
		r: &Registry{
			Helm: &HelmSource{
				URL: server.URL,
			},
		},
	}
	version := i.getAddonVersionMeetSystemRequirement("fluxcd-no-requirements")
	assert.Equal(t, version, "1.0.0")
	version = i.getAddonVersionMeetSystemRequirement("not-exist")
	assert.Equal(t, version, "")
}

var baseAddon = InstallPackage{
	Meta: Meta{
		Name:          "test-render-cue-definition-addon",
		NeedNamespace: []string{"test-ns"},
		DeployTo:      &DeployTo{RuntimeCluster: true},
	},
	CUEDefinitions: []ElementFile{
		{
			Data: testCueDef,
			Name: "test-def",
		},
	},
}

var multiYamlAddon = InstallPackage{
	Meta: Meta{
		Name: "test-render-multi-yaml-addon",
	},
	YAMLTemplates: []ElementFile{
		{
			Data: testYamlObject1,
			Name: "test-object-1",
		},
		{
			Data: testYamlObject2,
			Name: "test-object-2",
		},
	},
}

var viewAddon = InstallPackage{
	Meta: Meta{
		Name: "test-render-view-addon",
	},
	YAMLViews: []ElementFile{
		{
			Data: testYAMLView,
			Name: "cloud-resource-view",
		},
	},
	CUEViews: []ElementFile{
		{
			Data: testCUEView,
			Name: "pod-view",
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

var testYamlObject1 = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  labels:
    app: nginx
spec:
  replicas: 3
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
        ports:
        - containerPort: 80
`
var testYamlObject2 = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment-2
  labels:
    app: nginx
spec:
  replicas: 3
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
        ports:
        - containerPort: 80
`

var testYAMLView = `
apiVersion: "v1"
kind: "ConfigMap"
metadata:
  name: "cloud-resource-view"
  namespace: "vela-system"
data:
  template: |
    import (
    "vela/ql"
    )
    
    parameter: {
      appName: string
        appNs:   string
    }
    resources: ql.#ListResourcesInApp & {
      app: {
        name:      parameter.appName
          namespace: parameter.appNs
          filter: {
            "apiVersion": "terraform.core.oam.dev/v1beta1"
              "kind":       "Configuration"
          }
          withStatus: true
      }
    }
    status: {
      if resources.err == _|_ {
        "cloud-resources": [ for i, resource in resources.list {
          resource.object
        }]
      }
      if resources.err != _|_ {
        error: resources.err
      }
    }


`
var testCUEView = `
import (
	"vela/ql"
)

parameter: {
	name:      string
	namespace: string
	cluster:   *"" | string
}
pod: ql.#Read & {
	value: {
		apiVersion: "v1"
		kind:       "Pod"
		metadata: {
			name:      parameter.name
			namespace: parameter.namespace
		}
	}
	cluster: parameter.cluster
}
eventList: ql.#SearchEvents & {
	value: {
		apiVersion: "v1"
		kind:       "Pod"
		metadata:   pod.value.metadata
	}
	cluster: parameter.cluster
}
podMetrics: ql.#Read & {
	cluster: parameter.cluster
	value: {
		apiVersion: "metrics.k8s.io/v1beta1"
		kind:       "PodMetrics"
		metadata: {
			name:      parameter.name
			namespace: parameter.namespace
		}
	}
}
status: {
	if pod.err == _|_ {
		containers: [ for container in pod.value.spec.containers {
			name:  container.name
			image: container.image
			resources: {
				if container.resources.limits != _|_ {
					limits: container.resources.limits
				}
				if container.resources.requests != _|_ {
					requests: container.resources.requests
				}
				if podMetrics.err == _|_ {
					usage: {for containerUsage in podMetrics.value.containers {
						if containerUsage.name == container.name {
							cpu:    containerUsage.usage.cpu
							memory: containerUsage.usage.memory
						}
					}}
				}
			}
			if pod.value.status.containerStatuses != _|_ {
				status: {for containerStatus in pod.value.status.containerStatuses if containerStatus.name == container.name {
					state:        containerStatus.state
					restartCount: containerStatus.restartCount
				}}
			}
		}]
		if eventList.err == _|_ {
			events: eventList.list
		}
	}
	if pod.err != _|_ {
		error: pod.err
	}
}

`

func TestGetPatternFromItem(t *testing.T) {
	ossR, err := NewAsyncReader("http://ep.beijing", "some-bucket", "", "some-sub-path", "", ossType)
	assert.NoError(t, err)
	gitR, err := NewAsyncReader("https://github.com/oam-dev/catalog", "", "", "addons", "", gitType)
	assert.NoError(t, err)
	gitItemName := "parameter.cue"
	gitItemType := FileType
	gitItemPath := "addons/terraform/resources/parameter.cue"

	viewOSSR := localReader{
		dir:  "./testdata/test-view",
		name: "test-view",
	}
	viewPath := filepath.Join("./testdata/test-view/views/pod-view.cue", "pod-view.cue")

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
		{
			caseName: "views case",
			item: OSSItem{
				tp:   FileType,
				path: viewPath,
				name: "pod-view.cue",
			},
			root:        "test-view",
			meetPattern: "views",
			r:           viewOSSR,
		},
	}
	for _, tc := range testCases {
		res := GetPatternFromItem(tc.item, tc.r, tc.root)
		assert.Equal(t, res, tc.meetPattern, tc.caseName)
	}
}

func TestGitLabReaderNotPanic(t *testing.T) {
	_, err := NewAsyncReader("https://gitlab.com/test/catalog", "", "", "addons", "", gitType)
	assert.EqualError(t, err, "git type repository only support github for now")
}

func TestCheckSemVer(t *testing.T) {
	testCases := []struct {
		actual   string
		require  string
		nilError bool
		res      bool
	}{
		{
			actual:  "v1.2.1",
			require: "<=v1.2.1",
			res:     true,
		},
		{
			actual:  "v1.2.1",
			require: ">v1.2.1",
			res:     false,
		},
		{
			actual:  "v1.2.1",
			require: "<=v1.2.3",
			res:     true,
		},
		{
			actual:  "v1.2",
			require: "<=v1.2.3",
			res:     true,
		},
		{
			actual:  "v1.2.1",
			require: ">v1.2.3",
			res:     false,
		},
		{
			actual:  "v1.2.1",
			require: "=v1.2.1",
			res:     true,
		},
		{
			actual:  "1.2.1",
			require: "=v1.2.1",
			res:     true,
		},
		{
			actual:  "1.2.1",
			require: "",
			res:     true,
		},
		{
			actual:  "v1.2.2",
			require: "<=v1.2.3, >=v1.2.1",
			res:     true,
		},
		{
			actual:  "v1.2.0",
			require: "v1.2.0, <=v1.2.3",
			res:     true,
		},
		{
			actual:  "1.2.2",
			require: "v1.2.2",
			res:     true,
		},
		{
			actual:  "1.2.02",
			require: "v1.2.2",
			res:     true,
		},
		{
			actual:  "1.3.0-beta.1",
			require: ">=v1.3.0-alpha.1",
			res:     true,
		},
		{
			actual:  "1.3.0-alpha.2",
			require: ">=v1.3.0-alpha.1",
			res:     true,
		},
		{
			actual:  "1.2.3",
			require: ">=v1.3.0-alpha.1",
			res:     false,
		},
		{
			actual:  "v1.4.0-alpha.3",
			require: ">=v1.3.0-beta.2",
			res:     true,
		},
		{
			actual:  "v1.4.0-beta.1",
			require: ">=v1.3.0",
			res:     true,
		},
		{
			actual:  "v1.4.0",
			require: ">=v1.3.0-beta.2",
			res:     true,
		},
		{
			actual:  "1.2.4-beta.2",
			require: ">=v1.2.4-beta.3",
			res:     false,
		},
		{
			actual:  "1.5.0-beta.2",
			require: ">=1.5.0",
			res:     false,
		},
		{
			actual:  "1.5.0-alpha.2",
			require: ">=1.5.0",
			res:     false,
		},
		{
			actual:  "1.5.0-rc.2",
			require: ">=1.5.0-beta.1",
			res:     true,
		},
		{
			actual:  "1.5.0-rc.2",
			require: ">=1.5.0-rc.1",
			res:     true,
		},
		{
			actual:  "1.5.0-rc.1",
			require: ">=1.5.0-alpha.1",
			res:     true,
		},
		{
			actual:  "1.5.0-rc.2",
			require: ">=1.5.0",
			res:     false,
		},
		{
			actual:  "1.5.0-rc.2",
			require: ">=1.4.0",
			res:     true,
		},
	}
	for _, testCase := range testCases {
		result, err := checkSemVer(testCase.actual, testCase.require)
		assert.NoError(t, err)
		assert.Equal(t, result, testCase.res)
	}
}

func TestCheckAddonVersionMeetRequired(t *testing.T) {
	k8sClient := &test.MockClient{
		MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
			return nil
		}),
		MockList: test.NewMockListFn(nil, func(obj client.ObjectList) error {
			robj := obj.(*appsv1.DeploymentList)
			list := &appsv1.DeploymentList{
				Items: []appsv1.Deployment{
					{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								oam.LabelControllerName: oam.ApplicationControllerName,
							},
						},
						Spec: appsv1.DeploymentSpec{
							Template: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Image: "vela-core:v1.2.5",
										},
									},
								},
							},
						},
					},
				},
			}
			list.DeepCopyInto(robj)
			return nil
		}),
	}
	ctx := context.Background()
	assert.NoError(t, checkAddonVersionMeetRequired(ctx, &SystemRequirements{VelaVersion: ">=1.2.4"}, k8sClient, nil))

	version2.VelaVersion = "v1.2.3"
	if err := checkAddonVersionMeetRequired(ctx, &SystemRequirements{VelaVersion: ">=1.2.4"}, k8sClient, nil); err == nil {
		assert.Error(t, fmt.Errorf("should meet error"))
	}

	version2.VelaVersion = "v1.2.4"
	assert.NoError(t, checkAddonVersionMeetRequired(ctx, &SystemRequirements{VelaVersion: ">=1.2.4"}, k8sClient, nil))
}

var testUnmarshalToContent1 = `
{
  "type": "file",
  "encoding": "",
  "size": 651,
  "name": "metadata.yaml",
  "path": "example/metadata.yaml",
  "content": "name: example\r\nversion: 1.0.0\r\ndescription: Extended workload to do continuous and progressive delivery\r\nicon: https://raw.githubusercontent.com/fluxcd/flux/master/docs/_files/weave-flux.png\r\nurl: https://fluxcd.io\r\n\r\ntags:\r\n  - extended_workload\r\n  - gitops\r\n  - only_example\r\n\r\ndeployTo:\r\n  control_plane: true\r\n  runtime_cluster: false\r\n\r\ndependencies: []\r\n#- name: addon_name\r\n\r\n# set invisible means this won't be list and will be enabled when depended on\r\n# for example, terraform-alibaba depends on terraform which is invisible,\r\n# when terraform-alibaba is enabled, terraform will be enabled automatically\r\n# default: false\r\ninvisible: false\r\n"
}`
var testUnmarshalToContent2 = `
[
  {
    "type": "dir",
    "name": "example",
    "path": "example"
  },
  {
    "type": "dir",
    "name": "local",
    "path": "local"
  },
  {
    "type": "dir",
    "name": "terraform",
    "path": "terraform"
  },
  {
    "type": "dir",
    "name": "terraform-alibaba",
    "path": "terraform-alibaba"
  },
  {
    "type": "dir",
    "name": "test-error-addon",
    "path": "test-error-addon"
  }
]`
var testUnmarshalToContent3 = `
[
  {
    "type": "dir",
    "name": "example",
  },
  {
    "type": "dir",
    "name": "local",
    "path": "local"
  }
]`
var testUnmarshalToContent4 = ``

func TestUnmarshalToContent(t *testing.T) {
	_, _, err1 := unmarshalToContent([]byte(testUnmarshalToContent1))
	assert.NoError(t, err1)
	_, _, err2 := unmarshalToContent([]byte(testUnmarshalToContent2))
	assert.NoError(t, err2)
	_, _, err3 := unmarshalToContent([]byte(testUnmarshalToContent3))
	assert.Error(t, err3, "unmarshalling failed for both file and directory content: invalid character '}' looking for beginnin")
	_, _, err4 := unmarshalToContent([]byte(testUnmarshalToContent4))
	assert.Error(t, err4, "unmarshalling failed for both file and directory content: unexpected end of JSON input and unexpecte")
}

// Test readResFile, only accept .cue and .yaml/.yml
func TestReadResFile(t *testing.T) {

	// setup test data
	testAddonName := "example"
	testAddonDir := fmt.Sprintf("./testdata/%s", testAddonName)
	reader := localReader{dir: testAddonDir, name: testAddonName}
	metas, err := reader.ListAddonMeta()
	testAddonMeta := metas[testAddonName]
	assert.NoError(t, err)

	// run test
	var addon = &InstallPackage{}
	ptItems := ClassifyItemByPattern(&testAddonMeta, reader)
	items := ptItems[ResourcesDirName]
	for _, it := range items {
		err := readResFile(addon, reader, reader.RelativePath(it))
		assert.NoError(t, err)
	}

	// verify
	assert.True(t, len(addon.YAMLTemplates) == 1)
}

// Test readDefFile only accept .cue and .yaml/.yml
func TestReadDefFile(t *testing.T) {

	// setup test data
	testAddonName := "example"
	testAddonDir := fmt.Sprintf("./testdata/%s", testAddonName)
	reader := localReader{dir: testAddonDir, name: testAddonName}
	metas, err := reader.ListAddonMeta()
	testAddonMeta := metas[testAddonName]
	assert.NoError(t, err)

	// run test
	var uiData = &UIData{}
	ptItems := ClassifyItemByPattern(&testAddonMeta, reader)
	items := ptItems[DefinitionsDirName]

	for _, it := range items {
		err := readDefFile(uiData, reader, reader.RelativePath(it))
		if err != nil {
			assert.Error(t, fmt.Errorf("Something wrong."))
		}
	}

	// verify
	assert.True(t, len(uiData.Definitions) == 1)
}

// Test readDefFile only accept .cue
func TestReadViewFile(t *testing.T) {

	// setup test data
	testAddonName := "test-view"
	testAddonDir := fmt.Sprintf("./testdata/%s", testAddonName)
	reader := localReader{dir: testAddonDir, name: testAddonName}
	metas, err := reader.ListAddonMeta()
	testAddonMeta := metas[testAddonName]
	assert.NoError(t, err)

	// run test
	var addon = &InstallPackage{}
	ptItems := ClassifyItemByPattern(&testAddonMeta, reader)
	items := ptItems[ViewDirName]

	for _, it := range items {
		err := readViewFile(addon, reader, reader.RelativePath(it))
		if err != nil {
			assert.NoError(t, err)
		}
	}
	notExistErr := readViewFile(addon, reader, "not-exist.cue")
	assert.Error(t, notExistErr)

	// verify
	assert.True(t, len(addon.CUEViews) == 1)
	assert.True(t, len(addon.YAMLViews) == 1)
}

func TestRenderCUETemplate(t *testing.T) {
	fileDate, err := os.ReadFile("./testdata/example/resources/configmap.cue")
	assert.NoError(t, err)
	addon := &InstallPackage{
		Meta: Meta{
			Version: "1.0.1",
		},
		Parameters: "{\"example\": \"\"}",
	}
	component, err := renderCompAccordingCUETemplate(ElementFile{Data: string(fileDate), Name: "configmap.cue"}, addon, map[string]interface{}{
		"example": "render",
	})
	assert.NoError(t, err)
	assert.True(t, component.Type == "raw")
	var config = make(map[string]interface{})
	err = json.Unmarshal(component.Properties.Raw, &config)
	assert.NoError(t, err)
	assert.True(t, component.Type == "raw")
	assert.True(t, config["metadata"].(map[string]interface{})["labels"].(map[string]interface{})["version"] == "1.0.1")
}

func TestCheckEnableAddonErrorWhenMissMatch(t *testing.T) {
	version2.VelaVersion = "v1.3.0"
	i := InstallPackage{Meta: Meta{SystemRequirements: &SystemRequirements{VelaVersion: ">=1.4.0"}}}
	installer := &Installer{}
	_, err := installer.enableAddon(&i)
	assert.Equal(t, errors.As(err, &VersionUnMatchError{}), true)
}

func TestPackageAddon(t *testing.T) {
	pwd, _ := os.Getwd()

	validAddonDict := "./testdata/example-legacy"
	archiver, err := PackageAddon(validAddonDict)
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(pwd, "example-legacy-1.0.1.tgz"), archiver)
	// Remove generated package after tests
	defer func() {
		_ = os.RemoveAll(filepath.Join(pwd, "example-legacy-1.0.1.tgz"))
	}()

	invalidAddonDict := "./testdata"
	archiver, err = PackageAddon(invalidAddonDict)
	assert.NotNil(t, err)
	assert.Equal(t, "", archiver)

	invalidAddonMetadata := "./testdata/invalid-metadata"
	archiver, err = PackageAddon(invalidAddonMetadata)
	assert.NotNil(t, err)
	assert.Equal(t, "", archiver)
}

func TestGenerateAnnotation(t *testing.T) {
	meta := Meta{SystemRequirements: &SystemRequirements{
		VelaVersion:       ">1.4.0",
		KubernetesVersion: ">1.20.0",
	}}
	res := generateAnnotation(&meta)
	assert.Equal(t, res[velaSystemRequirement], ">1.4.0")
	assert.Equal(t, res[kubernetesSystemRequirement], ">1.20.0")

	meta = Meta{}
	meta.SystemRequirements = &SystemRequirements{KubernetesVersion: ">=1.20.1"}
	res = generateAnnotation(&meta)
	assert.Equal(t, res[velaSystemRequirement], "")
	assert.Equal(t, res[kubernetesSystemRequirement], ">=1.20.1")
}

func TestMergeAddonInstallArgs(t *testing.T) {
	k8sClient := fake.NewClientBuilder().Build()
	ctx := context.Background()

	testcases := []struct {
		name        string
		legacyArgs  string
		args        map[string]interface{}
		mergedArgs  string
		application string
		err         error
	}{
		{
			name:       "addon1",
			legacyArgs: "{\"clusters\":[\"\"],\"imagePullSecrets\":[\"test-hub\"],\"repo\":\"hub.vela.com\",\"serviceType\":\"NodePort\"}",
			args: map[string]interface{}{
				"serviceType": "NodePort",
			},
			mergedArgs: "{\"clusters\":[\"\"],\"imagePullSecrets\":[\"test-hub\"],\"repo\":\"hub.vela.com\",\"serviceType\":\"NodePort\"}",
		},
		{
			name:       "addon2",
			legacyArgs: "{\"clusters\":[\"\"]}",
			args: map[string]interface{}{
				"repo":             "hub.vela.com",
				"serviceType":      "NodePort",
				"imagePullSecrets": []string{"test-hub"},
			},
			mergedArgs: "{\"clusters\":[\"\"],\"imagePullSecrets\":[\"test-hub\"],\"repo\":\"hub.vela.com\",\"serviceType\":\"NodePort\"}",
		},
		{
			name:       "addon3",
			legacyArgs: "{\"clusters\":[\"\"],\"imagePullSecrets\":[\"test-hub\"],\"repo\":\"hub.vela.com\",\"serviceType\":\"NodePort\"}",
			args: map[string]interface{}{
				"imagePullSecrets": []string{"test-hub-2"},
			},
			mergedArgs: "{\"clusters\":[\"\"],\"imagePullSecrets\":[\"test-hub-2\"],\"repo\":\"hub.vela.com\",\"serviceType\":\"NodePort\"}",
		},
		{
			// merge nested parameters
			name:       "addon4",
			legacyArgs: "{\"clusters\":[\"\"],\"p1\":{\"p11\":\"p11-v1\",\"p12\":\"p12-v1\"}}",
			args: map[string]interface{}{
				"p1": map[string]interface{}{
					"p12": "p12-v2",
					"p13": "p13-v1",
				},
			},
			mergedArgs: "{\"clusters\":[\"\"],\"p1\":{\"p11\":\"p11-v1\",\"p12\":\"p12-v2\",\"p13\":\"p13-v1\"}}",
		},
		{
			// there is not legacyArgs
			name:       "addon5",
			legacyArgs: "",
			args: map[string]interface{}{
				"p1": map[string]interface{}{
					"p12": "p12-v2",
					"p13": "p13-v1",
				},
			},
			mergedArgs: "{\"p1\":{\"p12\":\"p12-v2\",\"p13\":\"p13-v1\"}}",
		},
		{
			// there is not new args
			name:       "addon6",
			legacyArgs: "{\"clusters\":[\"\"],\"p1\":{\"p11\":\"p11-v1\",\"p12\":\"p12-v1\"}}",
			args:       nil,
			mergedArgs: "{\"clusters\":[\"\"],\"p1\":{\"p11\":\"p11-v1\",\"p12\":\"p12-v1\"}}",
		},
	}

	for _, tc := range testcases {
		t.Run("", func(t *testing.T) {
			if len(tc.legacyArgs) != 0 {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      addonutil.Addon2SecName(tc.name),
						Namespace: types.DefaultKubeVelaNS,
					},
					Data: map[string][]byte{
						AddonParameterDataKey: []byte(tc.legacyArgs),
					},
				}
				err := k8sClient.Create(ctx, secret)
				assert.NoError(t, err)
			}

			addonArgs, err := MergeAddonInstallArgs(ctx, k8sClient, tc.name, tc.args)
			assert.NoError(t, err)
			args, err := json.Marshal(addonArgs)
			assert.NoError(t, err)
			assert.Equal(t, tc.mergedArgs, string(args), tc.name)
		})
	}

}

func TestGenerateConflictError(t *testing.T) {
	confictAddon := map[string]string{
		"helm":      "definition: helm already exist and not belong to any addon \n",
		"kustomize": "definition: kustomize in this addon already exist in fluxcd \n",
	}
	err := produceDefConflictError(confictAddon)
	assert.Error(t, err)
	strings.Contains(err.Error(), "in this addon already exist in fluxcd")

	assert.NoError(t, produceDefConflictError(map[string]string{}))
}
