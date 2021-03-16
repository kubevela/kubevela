package definition

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/build"
	"gotest.tools/assert"

	"github.com/oam-dev/kubevela/pkg/dsl/model"
)

func TestPackage(t *testing.T) {
	var openAPISchema = `
{
    "definitions":{
        "Bucket":{
            "properties":{
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
	addImportsFor(bi)
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
	acl:                 "public-read-write" | "public-read" | *"private"
	type:                "alicloud_oss_bucket"
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
