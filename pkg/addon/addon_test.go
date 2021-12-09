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
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"

	"gotest.tools/assert"

	"github.com/oam-dev/kubevela/apis/types"
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

func TestGetAddon(t *testing.T) {
	server := httptest.NewServer(ossHandler)
	defer server.Close()

	reader, err := NewAsyncReader(server.URL, "", "", ossType)

	assert.NilError(t, err)

	testAddonName := "example"
	assert.NilError(t, err)
	addon, err := GetSingleAddonFromReader(reader, testAddonName, EnableLevelOptions)
	assert.NilError(t, err)
	assert.Equal(t, addon.Name, testAddonName)
	assert.Assert(t, addon.Parameters != "")
	assert.Assert(t, len(addon.Definitions) > 0)

	addons, err := GetAddonsFromReader(reader, EnableLevelOptions)
	assert.Assert(t, strings.Contains(err.Error(), "#parameter.example: preference mark not allowed at this position"))
	assert.Equal(t, len(addons), 3)

	// test listing from OSS will act like listing from directory
	_, items, err := reader.Read("terraform")
	assert.NilError(t, err)
	assert.Equal(t, len(items), 1, "should list items only from terraform/ without terraform-alibaba/")
	assert.Equal(t, items[0].GetPath(), "terraform/metadata.yaml")
}

func TestRenderApp(t *testing.T) {

	addon := baseAddon
	app, err := RenderApp(&addon, nil, map[string]interface{}{})
	assert.NilError(t, err, "render app fail")
	assert.Equal(t, len(app.Spec.Components), 1)
}

func TestRenderDefinitions(t *testing.T) {
	addonDeployToRuntime := baseAddon
	addonDeployToRuntime.AddonMeta.DeployTo = &types.AddonDeployTo{
		ControlPlane:   true,
		RuntimeCluster: true,
	}
	defs, err := RenderDefinitions(&addonDeployToRuntime, nil)
	assert.NilError(t, err)
	assert.Equal(t, len(defs), 1)
	def := defs[0]
	assert.Equal(t, def.GetAPIVersion(), "core.oam.dev/v1beta1")
	assert.Equal(t, def.GetKind(), "TraitDefinition")
}

var baseAddon = types.Addon{
	AddonMeta: types.AddonMeta{
		Name: "test-render-cue-definition-addon",
	},
	CUEDefinitions: []types.AddonElementFile{
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
