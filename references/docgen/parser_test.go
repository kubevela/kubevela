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

package docgen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var RefTestDir = filepath.Join(TestDir, "ref")

func TestCreateRefTestDir(t *testing.T) {
	if _, err := os.Stat(RefTestDir); err != nil && os.IsNotExist(err) {
		err := os.MkdirAll(RefTestDir, 0750)
		assert.NoError(t, err)
	}
}

func TestPrepareParameterTable(t *testing.T) {
	ref := MarkdownReference{}
	ref.I18N = &En
	tableName := "hello"
	parameterList := []ReferenceParameter{
		{
			PrintableType: "string",
		},
	}
	parameterName := "cpu"
	parameterList[0].Name = parameterName
	parameterList[0].Required = true
	refContent := ref.getParameterString(tableName, parameterList, types.CUECategory)
	assert.Contains(t, refContent, parameterName)
	assert.Contains(t, refContent, "cpu")
}

func TestDeleteRefTestDir(t *testing.T) {
	if _, err := os.Stat(RefTestDir); err == nil {
		err := os.RemoveAll(RefTestDir)
		assert.NoError(t, err)
	}
}

func TestWalkParameterSchema(t *testing.T) {
	testcases := []struct {
		data       string
		ExpectRefs map[string]map[string]ReferenceParameter
	}{
		{
			data: `{
    "properties": {
        "cmd": {
            "description": "Commands to run in the container",
            "items": {
                "type": "string"
            },
            "title": "cmd",
            "type": "array"
        },
        "image": {
            "description": "Which image would you like to use for your service",
            "title": "image",
            "type": "string"
        }
    },
    "required": [
        "image"
    ],
    "type": "object"
}`,
			ExpectRefs: map[string]map[string]ReferenceParameter{
				"# Specification": {
					"cmd": ReferenceParameter{
						Parameter: types.Parameter{
							Name:     "cmd",
							Usage:    "Commands to run in the container",
							JSONType: "array",
						},
						PrintableType: "array",
					},
					"image": ReferenceParameter{
						Parameter: types.Parameter{
							Name:     "image",
							Required: true,
							Usage:    "Which image would you like to use for your service",
							JSONType: "string",
						},
						PrintableType: "string",
					},
				},
			},
		},
		{
			data: `{
    "properties": {
        "obj": {
            "properties": {
                "f0": {
                    "default": "v0",
                    "type": "string"
                },
                "f1": {
                    "default": "v1",
                    "type": "string"
                },
                "f2": {
                    "default": "v2",
                    "type": "string"
                }
            },
            "type": "object"
        },
    },
    "type": "object"
}`,
			ExpectRefs: map[string]map[string]ReferenceParameter{
				"# Specification": {
					"obj": ReferenceParameter{
						Parameter: types.Parameter{
							Name:     "obj",
							JSONType: "object",
						},
						PrintableType: "[obj](#obj)",
					},
				},
				"## obj": {
					"f0": ReferenceParameter{
						Parameter: types.Parameter{
							Name:     "f0",
							Default:  "v0",
							JSONType: "string",
						},
						PrintableType: "string",
					},
					"f1": ReferenceParameter{
						Parameter: types.Parameter{
							Name:     "f1",
							Default:  "v1",
							JSONType: "string",
						},
						PrintableType: "string",
					},
					"f2": ReferenceParameter{
						Parameter: types.Parameter{
							Name:     "f2",
							Default:  "v2",
							JSONType: "string",
						},
						PrintableType: "string",
					},
				},
			},
		},
		{
			data: `{
    "properties": {
        "obj": {
            "properties": {
                "f0": {
                    "default": "v0",
                    "type": "string"
                },
                "f1": {
                    "default": "v1",
                    "type": "object",
                    "properties": {
                        "g0": {
                            "default": "v2",
                            "type": "string"
                        }
                    }
                }
            },
            "type": "object"
        }
    },
    "type": "object"
}`,
			ExpectRefs: map[string]map[string]ReferenceParameter{
				"# Specification": {
					"obj": ReferenceParameter{
						Parameter: types.Parameter{
							Name:     "obj",
							JSONType: "object",
						},
						PrintableType: "[obj](#obj)",
					},
				},
				"## obj": {
					"f0": ReferenceParameter{
						Parameter: types.Parameter{
							Name:     "f0",
							Default:  "v0",
							JSONType: "string",
						},
						PrintableType: "string",
					},
					"f1": ReferenceParameter{
						Parameter: types.Parameter{
							Name:     "f1",
							Default:  "v1",
							JSONType: "object",
						},
						PrintableType: "[f1](#f1)",
					},
				},
				"### f1": {
					"g0": ReferenceParameter{
						Parameter: types.Parameter{
							Name:     "g0",
							Default:  "v2",
							JSONType: "string",
						},
						PrintableType: "string",
					},
				},
			},
		},
	}
	for _, cases := range testcases {
		commonRefs = make([]CommonReference, 0)
		parameterJSON := fmt.Sprintf(BaseOpenAPIV3Template, cases.data)
		swagger, err := openapi3.NewLoader().LoadFromData(json.RawMessage(parameterJSON))
		assert.Equal(t, nil, err)
		parameters := swagger.Components.Schemas["parameter"].Value
		WalkParameterSchema(parameters, Specification, 0)
		refs := make(map[string]map[string]ReferenceParameter)
		for _, items := range commonRefs {
			refs[items.Name] = make(map[string]ReferenceParameter)
			for _, item := range items.Parameters {
				refs[items.Name][item.Name] = item
			}
		}
		assert.Equal(t, true, reflect.DeepEqual(cases.ExpectRefs, refs))
	}
}

func TestGenerateTerraformCapabilityProperties(t *testing.T) {
	ref := &ConsoleReference{}
	type args struct {
		cap types.Capability
	}

	type want struct {
		tableName1 string
		tableName2 string
		errMsg     string
	}
	testcases := map[string]struct {
		args args
		want want
	}{
		"normal": {
			args: args{
				cap: types.Capability{
					TerraformConfiguration: `
resource "alicloud_oss_bucket" "bucket-acl" {
  bucket = var.bucket
  acl = var.acl
}

output "BUCKET_NAME" {
  value = "${alicloud_oss_bucket.bucket-acl.bucket}.${alicloud_oss_bucket.bucket-acl.extranet_endpoint}"
}

variable "bucket" {
  description = "OSS bucket name"
  default = "vela-website"
  type = string
}

variable "acl" {
  description = "OSS bucket ACL, supported 'private', 'public-read', 'public-read-write'"
  default = "private"
  type = string
}
`,
				},
			},
			want: want{
				errMsg:     "",
				tableName1: "",
				tableName2: "#### writeConnectionSecretToRef",
			},
		},
		"configuration is in git remote": {
			args: args{
				cap: types.Capability{
					Name:                   "ecs",
					TerraformConfiguration: "https://github.com/wonderflow/terraform-alicloud-ecs-instance.git",
					ConfigurationType:      "remote",
				},
			},
			want: want{
				errMsg:     "",
				tableName1: "",
				tableName2: "#### writeConnectionSecretToRef",
			},
		},
		"configuration is not valid": {
			args: args{
				cap: types.Capability{
					TerraformConfiguration: `abc`,
				},
			},
			want: want{
				errMsg: "failed to generate capability properties: :1,1-4: Argument or block definition required; An " +
					"argument or block definition is required here. To set an argument, use the equals sign \"=\" to " +
					"introduce the argument value.",
			},
		},
	}
	for name, tc := range testcases {
		consoleRef, err := ref.GenerateTerraformCapabilityProperties(tc.args.cap)
		var errMsg string
		if err != nil {
			errMsg = err.Error()
			if diff := cmp.Diff(tc.want.errMsg, errMsg, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nGenerateTerraformCapabilityProperties(...): -want error, +got error:\n%s\n", name, diff)
			}
		} else {
			if diff := cmp.Diff(len(consoleRef), 2); diff != "" {
				t.Errorf("\n%s\nGenerateTerraformCapabilityProperties(...): -want, +got:\n%s\n", name, diff)
			}
			if diff := cmp.Diff(tc.want.tableName1, consoleRef[0].TableName); diff != "" {
				t.Errorf("\n%s\nGenerateTerraformCapabilityProperties(...): -want, +got:\n%s\n", name, diff)
			}
			if diff := cmp.Diff(tc.want.tableName2, consoleRef[1].TableName); diff != "" {
				t.Errorf("\n%s\nGexnerateTerraformCapabilityProperties(...): -want, +got:\n%s\n", name, diff)
			}
		}
	}
}

func TestPrepareTerraformOutputs(t *testing.T) {
	type args struct {
		tableName     string
		parameterList []ReferenceParameter
	}

	param := ReferenceParameter{}
	param.Name = "ID"
	param.Usage = "Identity of the cloud resource"

	testcases := []struct {
		args   args
		expect string
	}{
		{
			args: args{
				tableName:     "",
				parameterList: nil,
			},
			expect: "",
		},
		{
			args: args{
				tableName:     "abc",
				parameterList: []ReferenceParameter{param},
			},
			expect: "\n\nabc\n\n Name | Description \n ------------ | ------------- \n ID | Identity of the cloud resource\n",
		},
	}
	ref := &MarkdownReference{}
	for _, tc := range testcases {
		t.Run("", func(t *testing.T) {
			content := ref.prepareTerraformOutputs(tc.args.tableName, tc.args.parameterList)
			if content != tc.expect {
				t.Errorf("prepareTerraformOutputs(...): -want, +got:\n%s\n", cmp.Diff(tc.expect, content))
			}
		})
	}
}

func TestMakeReadableTitle(t *testing.T) {
	type args struct {
		ref   *MarkdownReference
		title string
	}

	ref := &MarkdownReference{}

	refZh := &MarkdownReference{}
	refZh.I18N = &Zh

	testcases := []struct {
		args args
		want string
	}{
		{
			args: args{
				title: "abc",
				ref:   ref,
			},
			want: "Abc",
		},
		{
			args: args{
				title: "abc-def",
				ref:   ref,
			},
			want: "Abc-Def",
		},
		{
			args: args{
				title: "alibaba-def-ghi",
				ref:   ref,
			},
			want: "Alibaba Cloud DEF-GHI",
		},
		{
			args: args{
				title: "alibaba-def-ghi",
				ref:   refZh,
			},
			want: "阿里云 DEF-GHI",
		},
		{
			args: args{
				title: "aws-jk",
				ref:   refZh,
			},
			want: "AWS JK",
		},
		{
			args: args{
				title: "azure-jk",
				ref:   refZh,
			},
			want: "Azure JK",
		},
	}
	for _, tc := range testcases {
		t.Run("", func(t *testing.T) {
			title := tc.args.ref.makeReadableTitle(tc.args.title)
			if title != tc.want {
				t.Errorf("makeReadableTitle(...): -want, +got:\n%s\n", cmp.Diff(tc.want, title))
			}
		})
	}
}

func TestParseLocalFile(t *testing.T) {
	testcases := []struct {
		localFilePath string
		want          types.Capability
	}{
		{
			localFilePath: "testdata/terraform-alibaba-ask.yaml",
			want: types.Capability{
				Name:                   "alibaba-ask",
				Description:            "Terraform configuration for Alibaba Cloud Serverless Kubernetes (ASK)",
				TerraformConfiguration: "https://github.com/kubevela-contrib/terraform-modules.git",
				ConfigurationType:      "remote",
				Path:                   "alibaba/cs/serverless-kubernetes",
			},
		},
		{
			localFilePath: "testdata/terraform-aws-elb.yaml",
			want: types.Capability{
				Name:                   "aws-elb",
				Description:            "Terraform module which creates ELB resources on AWS",
				TerraformConfiguration: "https://github.com/terraform-aws-modules/terraform-aws-elb.git",
				ConfigurationType:      "remote",
				Path:                   "",
			},
		},
		{
			localFilePath: "testdata/terraform-azure-database-mariadb.yaml",
			want: types.Capability{
				Name:                   "azure-database-mariadb",
				Description:            "Terraform configuration for Azure Database Mariadb",
				TerraformConfiguration: "",
				ConfigurationType:      "",
				Path:                   "",
			},
		},
		{
			localFilePath: "testdata/terraform-baidu-vpc.yaml",
			want: types.Capability{
				Name:                   "baidu-vpc",
				Description:            "Baidu Cloud VPC",
				TerraformConfiguration: "",
				ConfigurationType:      "",
				Path:                   "",
			},
		},
		{
			localFilePath: "testdata/terraform-gcp-gcs.yaml",
			want: types.Capability{
				Name:                   "gcp-gcs",
				Description:            "GCP Gcs",
				TerraformConfiguration: "https://github.com/woernfl/terraform-gcp-gcs.git",
				ConfigurationType:      "remote",
				Path:                   "",
			},
		},
		{
			localFilePath: "testdata/terraform-tencent-subnet.yaml",
			want: types.Capability{
				Name:                   "tencent-subnet",
				Description:            "Tencent Cloud Subnet",
				TerraformConfiguration: "",
				ConfigurationType:      "",
				Path:                   "",
			},
		},
	}
	for _, tc := range testcases {
		t.Run("", func(t *testing.T) {
			lc, err := ParseLocalFile(tc.localFilePath, common.Args{})
			if err != nil {
				t.Errorf("ParseLocalFile(...): -want: %v, got error: %s\n", tc.want, err)
			}
			if !reflect.DeepEqual(*lc, tc.want) {
				if !reflect.DeepEqual(lc.Name, tc.want.Name) {
					t.Errorf("Name not equal, - got: %v - want: %v", lc.Name, tc.want.Name)
				}
				if !reflect.DeepEqual(lc.Description, tc.want.Description) {
					t.Errorf("Description not equal, - got: %v - want: %v", lc.Description, tc.want.Description)
				}
				if !reflect.DeepEqual(lc.TerraformConfiguration, tc.want.TerraformConfiguration) {
					if len(lc.TerraformConfiguration) == 0 {
						t.Errorf("Parse TerraformConfiguration failed")
					}
				}
				if !reflect.DeepEqual(lc.ConfigurationType, tc.want.ConfigurationType) {
					t.Errorf("ConfigurationType not equal, - got: %v - want: %v", lc.ConfigurationType, tc.want.ConfigurationType)
				}
				if !reflect.DeepEqual(lc.Path, tc.want.Path) {
					t.Errorf("Path not equal, - got: %v - want: %v", lc.Path, tc.want.Path)
				}
			}
		})
	}
}

func TestExtractParameter(t *testing.T) {
	testcases := map[string]struct {
		cueTemplate string
		contains    string
	}{
		"normal-case": {
			cueTemplate: `parameter: {
	str: string
	itr: int
	btr: bool
	ct: cts: string
}`,
			contains: `### normal-case

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 str |  | string | true |  
 itr |  | int | true |  
 btr |  | bool | true |  
 ct |  | [ct](#ct) | true |  


#### ct

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 cts |  | string | true |`,
		},
		"normal-map-string-string": {
			cueTemplate: `parameter: {
	envMappings: [string]: string
}`,
			contains: `### normal-map-string-string

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 envMappings |  | map[string]string | true |`,
		},
		"normal-map-case": {
			cueTemplate: `parameter: {
	// +usage=The mapping of environment variables to secret
	envMappings: [string]: #KeySecret
}
#KeySecret: {
	key?:   string
	secret: string
}`,
			contains: `### normal-map-case

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 envMappings | The mapping of environment variables to secret. | map[string]KeySecret(#keysecret) | true |  


#### KeySecret

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 key |  | string | false |  
 secret |  | string | true |`,
		},
		"or-case-with-type": {
			cueTemplate: `	parameter: {
	   		orValue:  #KeyConfig | #KeySecret
	   	}

	   	#KeySecret: {
	   		key:   "abc"
	   		secret: string
	   	}

	   	#KeyConfig: {
	   		key:   "abc"
	   		config: string
	   	}`,
			contains: `### or-case-with-type

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 orValue |  | [KeyConfig](#keyconfig) or [KeySecret](#keysecret) | true |  


#### KeyConfig

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 key |  | string | true |  
 config |  | string | true |  


#### KeySecret

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 key |  | string | true |  
 secret |  | string | true | `,
		},
		"or-type-with-const-str": {
			cueTemplate: `parameter: {
	  type: *"configMap" | "secret" | "emptyDir" | "ephemeral"
}`,
			contains: `### or-type-with-const-str

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 type |  | "configMap" or "secret" or "emptyDir" or "ephemeral" | false | configMap`,
		},
		"or-type-with-const-and-string": {
			cueTemplate: `parameter: {
	  type: *"configMap" | "secret" | "emptyDir" | string
}`,
			contains: `### or-type-with-const-and-string

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 type |  | string | false | configMap`,
		},
		"var-or-with-struct-var": {
			cueTemplate: `
	   	parameter: {
	   		orValue:  KeyConfig | KeySecret
	   	}

	   	KeySecret: {
	   		key:   "abc"
	   		secret: string
	   	}

	   	KeyConfig: {
	   		key:   "abc"
	   		config: string
	   	}`,
			contains: `### var-or-with-struct-var

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 orValue |  | [KeyConfig](#keyconfig) or [KeySecret](#keysecret) | true |  


#### KeyConfig

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 key |  | string | true |  
 config |  | string | true |  


#### KeySecret

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 key |  | string | true |  
 secret |  | string | true | `,
		},
	}

	ref := &MarkdownReference{}
	for key, ca := range testcases {
		cueValue, _ := common.GetCUEParameterValue(ca.cueTemplate, nil)
		out, _, err := ref.parseParameters("", cueValue, key, 0, false)
		assert.NoError(t, err, key)
		assert.Contains(t, out, ca.contains, key)
	}
}

func TestExtractParameterFromFiles(t *testing.T) {
	ref := &MarkdownReference{}
	testcases := map[string]struct {
		path     string
		contains string
	}{
		"env": {
			path: "testdata/parameter/env.cue",
			contains: `### env

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
  |  | [PatchParams](#patchparams) or [type-option-2](#type-option-2) | false |  


#### PatchParams

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 containerName | Specify the name of the target container, if not set, use the component name. | string | false | empty 
 replace | Specify if replacing the whole environment settings for the container. | bool | false | false 
 env | Specify the  environment variables to merge, if key already existing, override its value. | map[string]string | true |  
 unset | Specify which existing environment variables to unset. | []string | true |  


#### type-option-2

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 containers | Specify the environment variables for multiple containers. | [[]containers](#containers) | true |  


##### containers

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 containerName | Specify the name of the target container, if not set, use the component name. | string | false | empty 
 replace | Specify if replacing the whole environment settings for the container. | bool | false | false 
 env | Specify the  environment variables to merge, if key already existing, override its value. | map[string]string | true |  
 unset | Specify which existing environment variables to unset. | []string | true |`,
		},
		"command": {
			path: "testdata/parameter/command.cue",
			contains: `### command

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
  |  | [PatchParams](#patchparams) or [type-option-2](#type-option-2) | false |  


#### PatchParams

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 containerName | Specify the name of the target container, if not set, use the component name. | string | false | empty 
 command | Specify the command to use in the target container, if not set, it will not be changed. | null | true |  
 args | Specify the args to use in the target container, if set, it will override existing args. | null | true |  
 addArgs | Specify the args to add in the target container, existing args will be kept, cannot be used with args. | null | true |  
 delArgs | Specify the existing args to delete in the target container, cannot be used with args. | null | true |  


#### type-option-2

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 containers | Specify the commands for multiple containers. | [[]containers](#containers) | true |  


##### containers

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 containerName | Specify the name of the target container, if not set, use the component name. | string | false | empty 
 command | Specify the command to use in the target container, if not set, it will not be changed. | null | true |  
 args | Specify the args to use in the target container, if set, it will override existing args. | null | true |  
 addArgs | Specify the args to add in the target container, existing args will be kept, cannot be used with args. | null | true |  
 delArgs | Specify the existing args to delete in the target container, cannot be used with args. | null | true |`,
		},
		"condition": {
			path: "testdata/parameter/condition.cue",
			contains: `### condition

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 volumes |  | [[]volumes](#volumes) | true |  


#### volumes

 Name | Description | Type | Required | Default 
 ---- | ----------- | ---- | -------- | ------- 
 name |  | string | true |  
 defaultMode | only works when type equals configmap. | int | false | 420 
 type |  | "configMap" or "secret" or "emptyDir" or "ephemeral" | false | configMap`,
		},
	}
	for key, ca := range testcases {
		content, err := os.ReadFile(ca.path)
		assert.NoError(t, err, ca.path)
		cueValue, _ := common.GetCUEParameterValue(string(content), nil)
		out, _, err := ref.parseParameters("", cueValue, key, 0, false)
		assert.NoError(t, err, key)
		assert.Contains(t, out, ca.contains, key)
	}
}
