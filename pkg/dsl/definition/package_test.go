package definition

import (
	"fmt"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/pkg/dsl/model"
)

func TestPackage(t *testing.T) {
	var openAPISchema = `
{	
	"paths": {
		"paths...": {
			"post":{
				"x-kubernetes-group-version-kind": {
                    "group": "test.io",
                    "kind": "Bucket",
                    "version": "v1"
                }
			}
		}
	},
    "definitions":{
        "Bucket":{
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

	pkg := newPackage("oss")
	assert.NilError(t, pkg.addOpenAPI(openAPISchema))
	pkg.mount()
	bi := build.NewContext().NewInstance("", nil)
	AddVelaInternalPackagesFor(bi)
	bi.AddFile("-", `
import "oss"
output: oss.#Bucket
`)
	var r cue.Runtime
	inst, err := r.Build(bi)
	assert.NilError(t, err)
	base, err := model.NewBase(inst.Value())
	assert.NilError(t, err)

	exceptObj := `output: close({
	kind:                "Bucket"
	apiVersion:          "test.io/v1"
	type:                "alicloud_oss_bucket"
	acl:                 "public-read-write" | "public-read" | *"private"
	dataRedundancyType?: "ZRS" | *"LRS"
	dataSourceRef?:      close({
		dsPath: string
	})
	importRef?: close({
		importKey: string
	})
	output: close({
		{[!~"^(bucketName|extranetEndpoint|intranetEndpoint|masterUserId)$"]: {
											outRef: string
		} | {
			// Example: demoVpc.vpcId
			valueRef: string
		}}
		bucketName: close({
			outRef: "self.name"
		})
		extranetEndpoint: close({
			outRef: "self.state.extranetEndpoint"
		})
		intranetEndpoint: close({
			outRef: "self.state.intranetEndpoint"
		})
		masterUserId: close({
			outRef: "self.state.masterUserId"
		})
	})
	profile: close({
		baasRepo: string | close({
			// Example: demoVpc.vpcId
			valueRef: string
		})
		cloudProduct: "AliCloudOSS"
		endpoint?:    string | close({
			// Example: demoVpc.vpcId
			valueRef: string
		})
		envType?: "testing" | "product" | close({
			// Example: demoVpc.vpcId
			valueRef: string
		})
		provider: "alicloud"
		region:   string | close({
			// Example: demoVpc.vpcId
			valueRef: string
		})
		serviceAccount?: string | close({
			// Example: demoVpc.vpcId
			valueRef: string
		})
	})
	storageClass?: "IA" | "Archive" | "ColdArchive" | *"Standard"
})
`
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
	assert.Error(t, retInst.Lookup("case1").Err(), "case1: field \"additionalProperty\" not allowed in closed struct")
	assert.Error(t, retInst.Lookup("case2", "metadata").Err(), "case2.metadata: field \"additionalProperty\" not allowed in closed struct")
}

func TestMount(t *testing.T) {
	velaBuiltinPkgs = nil
	testPkg := newPackage("foo")
	testPkg.mount()
	assert.Equal(t, len(velaBuiltinPkgs), 1)
	testPkg.mount()
	assert.Equal(t, len(velaBuiltinPkgs), 1)
	assert.Equal(t, velaBuiltinPkgs[0], testPkg.Instance)
}

func TestGetGVK(t *testing.T) {
	srcTmpl := `
{
	"x-kubernetes-group-version-kind": {
		"group": "test.io",
		"kind": "Foo",
		"version": "v1"
	}
}
`
	var r cue.Runtime
	inst, err := r.Compile("-", srcTmpl)
	assert.NilError(t, err)
	gvk, err := getGVK(inst.Value().Lookup("x-kubernetes-group-version-kind"))
	assert.NilError(t, err)
	assert.Equal(t, gvk, metav1.GroupVersionKind{
		Group:   "test.io",
		Version: "v1",
		Kind:    "Foo",
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
			result: "[#Endpoint]",
		},
		{
			input:  []string{"definitions", "io.k8s.apiextensions-apiserver.pkg.apis.apiextensions.v1.JSONSchemaProps"},
			pos:    token.NoPos.Add(1),
			result: "[_]",
		},
		{
			input:  []string{"definitions", "io.k8s.apiextensions-apiserver.pkg.apis.apiextensions.v1.JSONSchemaProps"},
			pos:    token.NoPos,
			result: "[#JSONSchemaProps]",
		},
		{
			input:  []string{"definitions"},
			pos:    token.NoPos,
			errMsg: "openAPIMapping format invalid",
		},
	}

	for _, tCase := range testCases {
		labels, err := openAPIMapping(tCase.pos, tCase.input)
		if tCase.errMsg != "" {
			assert.Error(t, err, tCase.errMsg)
			continue
		}
		assert.NilError(t, err)
		assert.Equal(t, len(labels), 1)
		assert.Equal(t, tCase.result, fmt.Sprint(labels))
	}
}
