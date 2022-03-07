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

package plugins

import (
	"context"
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
)

var RefTestDir = filepath.Join(TestDir, "ref")

func TestCreateRefTestDir(t *testing.T) {
	if _, err := os.Stat(RefTestDir); err != nil && os.IsNotExist(err) {
		err := os.MkdirAll(RefTestDir, 0750)
		assert.NoError(t, err)
	}
}

func TestCreateMarkdown(t *testing.T) {
	ctx := context.Background()
	ref := &MarkdownReference{}

	refZh := &MarkdownReference{}
	refZh.I18N = Zh

	workloadName := "workload1"
	traitName := "trait1"
	scopeName := "scope1"
	workloadName2 := "workload2"

	workloadCueTemplate := `
parameter: {
	// +usage=Which image would you like to use for your service
	// +short=i
	image: string
}
`
	traitCueTemplate := `
parameter: {
	replicas: int
}
`

	configuration := `
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
`

	cases := map[string]struct {
		reason       string
		ref          *MarkdownReference
		capabilities []types.Capability
		want         error
	}{
		"WorkloadTypeAndTraitCapability": {
			reason: "valid capabilities",
			ref:    ref,
			capabilities: []types.Capability{
				{
					Name:        workloadName,
					Type:        types.TypeWorkload,
					CueTemplate: workloadCueTemplate,
					Category:    types.CUECategory,
				},
				{
					Name:        traitName,
					Type:        types.TypeTrait,
					CueTemplate: traitCueTemplate,
					Category:    types.CUECategory,
				},
				{
					Name:                   workloadName2,
					TerraformConfiguration: configuration,
					Type:                   types.TypeWorkload,
					Category:               types.TerraformCategory,
				},
			},
			want: nil,
		},
		"ScopeTypeCapability": {
			reason: "invalid capabilities",
			ref:    ref,
			capabilities: []types.Capability{
				{
					Name: scopeName,
					Type: types.TypeScope,
				},
			},
			want: fmt.Errorf("the type of the capability is not right"),
		},
		"TerraformCapabilityInChinese": {
			reason: "terraform capability",
			ref:    refZh,
			capabilities: []types.Capability{
				{
					Name:                   workloadName2,
					TerraformConfiguration: configuration,
					Type:                   types.TypeWorkload,
					Category:               types.TerraformCategory,
				},
			},
			want: nil,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := tc.ref.CreateMarkdown(ctx, tc.capabilities, RefTestDir, ReferenceSourcePath, nil)
			if diff := cmp.Diff(tc.want, got, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nCreateMakrdown(...): -want error, +got error:\n%s", tc.reason, diff)
			}
		})
	}

}

func TestPrepareParameterTable(t *testing.T) {
	ref := MarkdownReference{}
	tableName := "hello"
	parameterList := []ReferenceParameter{
		{
			PrintableType: "string",
		},
	}
	parameterName := "cpu"
	parameterList[0].Name = parameterName
	parameterList[0].Required = true
	refContent := ref.prepareParameter(tableName, parameterList, types.CUECategory)
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
				"# Properties": {
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
				"# Properties": {
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
				"# Properties": {
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
		swagger, err := openapi3.NewSwaggerLoader().LoadSwaggerFromData(json.RawMessage(parameterJSON))
		assert.Equal(t, nil, err)
		parameters := swagger.Components.Schemas["parameter"].Value
		WalkParameterSchema(parameters, "Properties", 0)
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
				tableName1: "### Properties",
				tableName2: "#### writeConnectionSecretToRef",
			},
		},
		"configuration is in git remote": {
			args: args{
				cap: types.Capability{
					TerraformConfiguration: "https://github.com/zzxwill/terraform-alibaba-eip.git",
					ConfigurationType:      "remote",
				},
			},
			want: want{
				errMsg:     "",
				tableName1: "### Properties",
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
	refZh.I18N = Zh

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
				Description:            "Terraform module which creates ELB resources on",
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
				TerraformConfiguration: "|\n        # Configure the Microsoft Azure Provider\n        provider \"azurerm\" {\n          features {}\n        }\n\n        resource \"azurerm_resource_group\" \"example\" {\n          name = var.resource_group\n          location = var.location\n        }\n\n        resource \"azurerm_mariadb_server\" \"example\" {\n          name = var.server_name\n          location = var.location\n          resource_group_name = azurerm_resource_group.example.name\n\n          sku_name = \"B_Gen5_2\"\n\n          storage_mb = 51200\n          backup_retention_days = 7\n          geo_redundant_backup_enabled = false\n\n          administrator_login = var.username\n          administrator_login_password = var.password\n          version = \"10.2\"\n          ssl_enforcement_enabled = true\n        }\n\n        resource \"azurerm_mariadb_database\" \"example\" {\n          name = var.db_name\n          resource_group_name = azurerm_resource_group.example.name\n          server_name = azurerm_mariadb_server.example.name\n          charset = \"utf8\"\n          collation = \"utf8_general_ci\"\n        }\n\n        variable \"server_name\" {\n          type = string\n          description = \"mariadb server name\"\n          default = \"mariadb-svr-sample\"\n        }\n\n        variable \"db_name\" {\n          default = \"backend\"\n          type = string\n          description = \"Database instance name\"\n        }\n\n        variable \"username\" {\n          default = \"acctestun\"\n          type = string\n          description = \"Database instance username\"\n        }\n\n        variable \"password\" {\n          default = \"H@Sh1CoR3!faked\"\n          type = string\n          description = \"Database instance password\"\n        }\n\n        variable \"location\" {\n          description = \"Azure location\"\n          type = string\n          default = \"West Europe\"\n        }\n\n        variable \"resource_group\" {\n          description = \"Resource group\"\n          type = string\n          default = \"kubevela-group\"\n        }\n\n        output \"SERVER_NAME\" {\n          value = var.server_name\n          description = \"mariadb server name\"\n        }\n\n        output \"DB_NAME\" {\n          value = var.db_name\n          description = \"Database instance name\"\n        }\n        output \"DB_USER\" {\n          value = var.username\n          description = \"Database instance username\"\n        }\n        output \"DB_PASSWORD\" {\n          sensitive = true\n          value = var.password\n          description = \"Database instance password\"\n        }\n        output \"DB_PORT\" {\n          value = \"3306\"\n          description = \"Database instance port\"\n        }\n        output \"DB_HOST\" {\n          value = azurerm_mariadb_server.example.fqdn\n          description = \"Database instance host\"\n        }",
				ConfigurationType:      "",
				Path:                   "",
			},
		},
		{
			localFilePath: "testdata/terraform-baidu-vpc.yaml",
			want: types.Capability{
				Name:                   "baidu-vpc",
				Description:            "Baidu Cloud VPC",
				TerraformConfiguration: "|-\n        terraform {\n          required_providers {\n            baiducloud = {\n              source = \"baidubce/baiducloud\"\n              version = \"1.12.0\"\n            }\n          }\n        }\n\n        resource \"baiducloud_vpc\" \"default\" {\n          name        = var.name\n          description = var.description\n          cidr        = var.cidr\n        }\n\n        variable \"name\" {\n          default = \"terraform-vpc\"\n          description = \"The name of the VPC\"\n          type = string\n        }\n\n        variable \"description\" {\n          description = \"The description of the VPC\"\n          default = \"this is created by terraform\"\n          type = string\n        }\n\n        variable \"cidr\" {\n          description = \"The CIDR of the VPC\"\n          default = \"192.168.0.0/24\"\n          type = string\n        }\n\n        output \"vpcs\" {\n          value = baiducloud_vpc.default.id\n        }",
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
				TerraformConfiguration: "|\n        terraform {\n          required_providers {\n            tencentcloud = {\n              source = \"tencentcloudstack/tencentcloud\"\n            }\n          }\n        }\n\n        variable \"availability_zone\" {\n          description = \"Availability Zone\"\n          default = \"ap-beijing-1\"\n          type = string\n        }\n\n        resource \"tencentcloud_vpc\" \"foo\" {\n          name       = \"guagua-ci-temp-test\"\n          cidr_block = \"10.0.0.0/16\"\n        }\n\n        resource \"tencentcloud_subnet\" \"subnet\" {\n          availability_zone = var.availability_zone\n          name              = var.name\n          vpc_id            = tencentcloud_vpc.foo.id\n          cidr_block        = var.cidr_block\n          is_multicast      = var.is_multicast\n        }\n\n        variable \"name\" {\n          description = \"Subnet name\"\n          default = \"guagua-ci-temp-test\"\n          type = string\n        }\n\n        variable \"cidr_block\" {\n          description = \"Subnet CIDR block\"\n          default = \"10.0.20.0/28\"\n          type = string\n        }\n\n        variable \"is_multicast\" {\n          description = \"Subnet is multicast\"\n          default = false\n          type = bool\n        }\n\n        output \"SUBNET_ID\" {\n          description = \"Subnet ID\"\n          value = tencentcloud_subnet.subnet.id\n        }",
				ConfigurationType:      "",
				Path:                   "",
			},
		},
	}
	for _, tc := range testcases {
		t.Run("", func(t *testing.T) {
			lc, err := ParseLocalFile(tc.localFilePath)
			if err != nil {
				t.Errorf("ParseLocalFile(...): -want: %v, got error: %s\n", tc.want, err)
			}
			if reflect.DeepEqual(*lc, tc.want) {
				t.Errorf("ParseLocalFile(...): -want: %v, got: %v\n", tc.want, *lc)
			}
		})
	}
}
