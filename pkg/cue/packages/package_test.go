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

// nolint: staticcheck,golint
package packages

import (
	"fmt"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
	"github.com/google/go-cmp/cmp"
	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/pkg/cue/model"
)

func TestPackage(t *testing.T) {
	var openAPISchema = `
{	
	"paths": {
		"paths...": {
			"post":{
				"x-kubernetes-group-version-kind": {
                    "group": "apps.test.io",
                    "kind": "Bucket",
                    "version": "v1"
                }
			}
		}
	},
    "definitions":{
        "io.test.apps.v1.Bucket":{
            "properties":{
				"apiVersion": {"type": "string"}
 				"kind": {"type": "string"}
                "acl":{
                    "default":"private",
                    "enum":[
                        "public-read-write",
                        "public-read",
                        "private"
                    ],
                    "type":"string"
                },
                "dataRedundancyType":{
                    "default":"LRS",
                    "enum":[
                        "LRS",
                        "ZRS"
                    ],
                    "type":"string"
                },
                "dataSourceRef":{
                    "properties":{
                        "dsPath":{
                            "type":"string"
                        }
                    },
                    "required":[
                        "dsPath"
                    ],
                    "type":"object"
                },
                "importRef":{
                    "properties":{
                        "importKey":{
                            "type":"string"
                        }
                    },
                    "required":[
                        "importKey"
                    ],
                    "type":"object"
                },
                "output":{
                    "additionalProperties":{
                        "oneOf":[
                            {
                                "properties":{
                                    "outRef":{
                                        "type":"string"
                                    }
                                },
                                "required":[
                                    "outRef"
                                ]
                            },
                            {
                                "properties":{
                                    "valueRef":{
                                        "description":"Example: demoVpc.vpcId",
                                        "type":"string"
                                    }
                                },
                                "required":[
                                    "valueRef"
                                ]
                            }
                        ],
                        "type":"object"
                    },
                    "properties":{
                        "bucketName":{
                            "properties":{
                                "outRef":{
                                    "enum":[
                                        "self.name"
                                    ],
                                    "type":"string"
                                }
                            },
                            "required":[
                                "outRef"
                            ],
                            "type":"object"
                        },
                        "extranetEndpoint":{
                            "properties":{
                                "outRef":{
                                    "enum":[
                                        "self.state.extranetEndpoint"
                                    ],
                                    "type":"string"
                                }
                            },
                            "required":[
                                "outRef"
                            ],
                            "type":"object"
                        },
                        "intranetEndpoint":{
                            "properties":{
                                "outRef":{
                                    "enum":[
                                        "self.state.intranetEndpoint"
                                    ],
                                    "type":"string"
                                }
                            },
                            "required":[
                                "outRef"
                            ],
                            "type":"object"
                        },
                        "masterUserId":{
                            "properties":{
                                "outRef":{
                                    "enum":[
                                        "self.state.masterUserId"
                                    ],
                                    "type":"string"
                                }
                            },
                            "required":[
                                "outRef"
                            ],
                            "type":"object"
                        }
                    },
                    "required":[
                        "bucketName",
                        "extranetEndpoint",
                        "intranetEndpoint",
                        "masterUserId"
                    ],
                    "type":"object"
                },
                "profile":{
                    "properties":{
                        "baasRepo":{
                            "oneOf":[
                                {
                                    "type":"string"
                                },
                                {
                                    "properties":{
                                        "valueRef":{
                                            "description":"Example: demoVpc.vpcId",
                                            "type":"string"
                                        }
                                    },
                                    "required":[
                                        "valueRef"
                                    ],
                                    "type":"object"
                                }
                            ]
                        },
                        "cloudProduct":{
                            "enum":[
                                "AliCloudOSS"
                            ],
                            "type":"string"
                        },
                        "endpoint":{
                            "oneOf":[
                                {
                                    "type":"string"
                                },
                                {
                                    "properties":{
                                        "valueRef":{
                                            "description":"Example: demoVpc.vpcId",
                                            "type":"string"
                                        }
                                    },
                                    "required":[
                                        "valueRef"
                                    ],
                                    "type":"object"
                                }
                            ]
                        },
                        "envType":{
                            "oneOf":[
                                {
                                    "enum":[
                                        "testing",
                                        "product"
                                    ]
                                },
                                {
                                    "properties":{
                                        "valueRef":{
                                            "description":"Example: demoVpc.vpcId",
                                            "type":"string"
                                        }
                                    },
                                    "required":[
                                        "valueRef"
                                    ],
                                    "type":"object"
                                }
                            ]
                        },
                        "provider":{
                            "enum":[
                                "alicloud"
                            ],
                            "type":"string"
                        },
                        "region":{
                            "oneOf":[
                                {
                                    "type":"string"
                                },
                                {
                                    "properties":{
                                        "valueRef":{
                                            "description":"Example: demoVpc.vpcId",
                                            "type":"string"
                                        }
                                    },
                                    "required":[
                                        "valueRef"
                                    ],
                                    "type":"object"
                                }
                            ]
                        },
                        "serviceAccount":{
                            "oneOf":[
                                {
                                    "type":"string"
                                },
                                {
                                    "properties":{
                                        "valueRef":{
                                            "description":"Example: demoVpc.vpcId",
                                            "type":"string"
                                        }
                                    },
                                    "required":[
                                        "valueRef"
                                    ],
                                    "type":"object"
                                }
                            ]
                        }
                    },
                    "required":[
                        "cloudProduct",
                        "provider",
                        "baasRepo",
                        "region"
                    ],
                    "type":"object"
                },
                "storageClass":{
                    "default":"Standard",
                    "enum":[
                        "Standard",
                        "IA",
                        "Archive",
                        "ColdArchive"
                    ],
                    "type":"string"
                },
                "type":{
                    "enum":[
                        "alicloud_oss_bucket"
                    ],
                    "type":"string"
                }
            },
            "required":[
                "type",
                "output",
                "profile",
                "acl"
            ],
            "type":"object"
        }
    }
}
`
	mypd := &PackageDiscover{pkgKinds: make(map[string][]VersionKind)}
	mypd.addKubeCUEPackagesFromCluster(openAPISchema)
	expectPkgKinds := map[string][]VersionKind{
		"test.io/apps/v1": []VersionKind{{
			DefinitionName: "#Bucket",
			APIVersion:     "apps.test.io/v1",
			Kind:           "Bucket",
		}},
		"kube/apps.test.io/v1": []VersionKind{{
			DefinitionName: "#Bucket",
			APIVersion:     "apps.test.io/v1",
			Kind:           "Bucket",
		}},
	}
	assert.Equal(t, cmp.Diff(mypd.ListPackageKinds(), expectPkgKinds), "")

	exceptObj := `output: {
	apiVersion:          "apps.test.io/v1"
	kind:                "Bucket"
	acl:                 *"private" | "public-read" | "public-read-write"
	dataRedundancyType?: "LRS" | "ZRS" | *"LRS"
	dataSourceRef?: {
		dsPath: string
	}
	importRef?: {
		importKey: string
	}
	output: {
		bucketName: {
			outRef: "self.name"
		}
		extranetEndpoint: {
			outRef: "self.state.extranetEndpoint"
		}
		intranetEndpoint: {
			outRef: "self.state.intranetEndpoint"
		}
		masterUserId: {
			outRef: "self.state.masterUserId"
		}
	}
	profile: {
		baasRepo: string | {
			// Example: demoVpc.vpcId
			valueRef: string
		}
		cloudProduct: "AliCloudOSS"
		endpoint?:    string | {
			// Example: demoVpc.vpcId
			valueRef: string
		}
		envType?: "testing" | "product" | {
			// Example: demoVpc.vpcId
			valueRef: string
		}
		provider: "alicloud"
		region:   string | {
			// Example: demoVpc.vpcId
			valueRef: string
		}
		serviceAccount?: string | {
			// Example: demoVpc.vpcId
			valueRef: string
		}
	}
	storageClass?: "Standard" | "IA" | "Archive" | "ColdArchive" | *"Standard"
	type:          "alicloud_oss_bucket"
}
`
	bi := build.NewContext().NewInstance("", nil)
	bi.AddFile("-", `
import "test.io/apps/v1"
output: v1.#Bucket
`)
	inst, err := mypd.ImportPackagesAndBuildInstance(bi)
	assert.NilError(t, err)
	base, err := model.NewBase(inst.Value())
	assert.NilError(t, err)
	assert.Equal(t, base.String(), exceptObj)

	bi = build.NewContext().NewInstance("", nil)
	bi.AddFile("-", `
import "kube/apps.test.io/v1"
output: v1.#Bucket
`)
	inst, err = mypd.ImportPackagesAndBuildInstance(bi)
	assert.NilError(t, err)
	base, err = model.NewBase(inst.Value())
	assert.NilError(t, err)
	assert.Equal(t, base.String(), exceptObj)
}

func TestProcessFile(t *testing.T) {
	srcTmpl := `
#Definition: {
	kind?: string
	apiVersion?: string
	metadata: {
		name: string
		...
	}
	...
}
`
	file, err := parser.ParseFile("-", srcTmpl)
	assert.NilError(t, err)
	testPkg := newPackage("foo")
	testPkg.processOpenAPIFile(file)

	var r cue.Runtime
	inst, err := r.CompileFile(file)
	assert.NilError(t, err)
	testCasesInst, err := r.Compile("-", `
	#Definition: {}
	case1: #Definition & {additionalProperty: "test"}

	case2: #Definition & {
		metadata: {
			additionalProperty: "test"
		}
}
`)
	assert.NilError(t, err)
	retInst, err := inst.Fill(testCasesInst.Value())
	assert.NilError(t, err)
	assert.Error(t, retInst.Lookup("case1").Err(), "case1: field not allowed: additionalProperty")
	assert.Error(t, retInst.Lookup("case2", "metadata").Err(), "case2.metadata: field not allowed: additionalProperty")
}

func TestMount(t *testing.T) {
	mypd := &PackageDiscover{pkgKinds: make(map[string][]VersionKind)}
	testPkg := newPackage("foo")
	mypd.mount(testPkg, []VersionKind{})
	assert.Equal(t, len(mypd.velaBuiltinPackages), 1)
	mypd.mount(testPkg, []VersionKind{})
	assert.Equal(t, len(mypd.velaBuiltinPackages), 1)
	assert.Equal(t, mypd.velaBuiltinPackages[0], testPkg.Instance)
}

func TestGetDGVK(t *testing.T) {
	srcTmpl := `
{
	"x-kubernetes-group-version-kind": {
		"group": "apps.test.io",
		"kind": "Foo",
		"version": "v1"
	}
}
`
	var r cue.Runtime
	inst, err := r.Compile("-", srcTmpl)
	assert.NilError(t, err)
	gvk, err := getDGVK(inst.Value().Lookup("x-kubernetes-group-version-kind"))
	assert.NilError(t, err)
	assert.Equal(t, gvk, domainGroupVersionKind{
		Domain:     "test.io",
		Group:      "apps",
		Version:    "v1",
		Kind:       "Foo",
		APIVersion: "apps.test.io/v1",
	})

	srcTmpl = `
{
	"x-kubernetes-group-version-kind": {
		"group": "test.io",
		"kind": "Foo",
		"version": "v1"
	}
}
`
	inst, err = r.Compile("-", srcTmpl)
	assert.NilError(t, err)
	gvk, err = getDGVK(inst.Value().Lookup("x-kubernetes-group-version-kind"))
	assert.NilError(t, err)
	assert.Equal(t, gvk, domainGroupVersionKind{
		Group:      "test.io",
		Version:    "v1",
		Kind:       "Foo",
		APIVersion: "test.io/v1",
	})
}

func TestOpenAPIMapping(t *testing.T) {
	testCases := []struct {
		input  []string
		pos    token.Pos
		result string
		errMsg string
	}{
		{
			input:  []string{"definitions", "io.k8s.api.discovery.v1beta1.Endpoint"},
			pos:    token.NoPos,
			result: "[io_k8s_api_discovery_v1beta1_Endpoint]",
		},
		{
			input:  []string{"definitions", "io.k8s.apiextensions-apiserver.pkg.apis.apiextensions.v1.JSONSchemaProps"},
			pos:    token.NoPos.Add(1),
			result: "[_]",
		},
		{
			input:  []string{"definitions", "io.k8s.apiextensions-apiserver.pkg.apis.apiextensions.v1.JSONSchemaProps"},
			pos:    token.NoPos,
			result: "[io_k8s_apiextensions_apiserver_pkg_apis_apiextensions_v1_JSONSchemaProps]",
		},
		{
			input:  []string{"definitions"},
			pos:    token.NoPos,
			errMsg: "openAPIMapping format invalid",
		},
	}

	emptyMapper := make(map[string]domainGroupVersionKind)
	for _, tCase := range testCases {
		labels, err := openAPIMapping(emptyMapper)(tCase.pos, tCase.input)
		if tCase.errMsg != "" {
			assert.Error(t, err, tCase.errMsg)
			continue
		}
		assert.NilError(t, err)
		assert.Equal(t, len(labels), 1)
		assert.Equal(t, tCase.result, fmt.Sprint(labels))
	}
}

func TestGeneratePkgName(t *testing.T) {
	testCases := []struct {
		dgvk        domainGroupVersionKind
		sdPkgName   string
		openPkgName string
	}{
		{
			dgvk: domainGroupVersionKind{
				Domain:  "k8s.io",
				Group:   "networking",
				Version: "v1",
				Kind:    "Ingress",
			},
			sdPkgName:   "k8s.io/networking/v1",
			openPkgName: "kube/networking.k8s.io",
		},
		{
			dgvk: domainGroupVersionKind{
				Group:   "example.com",
				Version: "v1",
				Kind:    "Sls",
			},
			sdPkgName:   "example.com/v1",
			openPkgName: "kube/example.com/v1",
		},
	}

	for _, tCase := range testCases {
		assert.Equal(t, genStandardPkgName(tCase.dgvk), tCase.sdPkgName)
	}
}

func TestReverseString(t *testing.T) {
	testCases := []struct {
		gvr           metav1.GroupVersionKind
		reverseString string
	}{
		{
			gvr: metav1.GroupVersionKind{
				Group:   "networking.k8s.io",
				Version: "v1",
				Kind:    "NetworkPolicy",
			},
			reverseString: "io_k8s_api_networking_v1_NetworkPolicy",
		},
		{
			gvr: metav1.GroupVersionKind{
				Group:   "example.com",
				Version: "v1",
				Kind:    "Sls",
			},
			reverseString: "com_example_v1_Sls",
		},
		{
			gvr: metav1.GroupVersionKind{
				Version: "v1",
				Kind:    "Pod",
			},
			reverseString: "io_k8s_api_core_v1_Pod",
		},
	}

	for _, tCase := range testCases {
		assert.Equal(t, convert2DGVK(tCase.gvr).reverseString(), tCase.reverseString)
	}
}
