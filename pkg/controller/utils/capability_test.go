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

package utils

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"gotest.tools/assert"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/oam/util"

	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"golang.org/x/crypto/ssh/testdata"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const TestDir = "testdata/definition"

func TestGetOpenAPISchema(t *testing.T) {
	type want struct {
		data string
		err  error
	}
	cases := map[string]struct {
		reason string
		name   string
		data   string
		want   want
	}{
		"parameter in cue is a structure type,": {
			reason: "Prepare a normal parameter cue file",
			name:   "workload1",
			data: `
project: {
	name: string
}

	parameter: {
	min: int
}
`,
			want: want{data: "{\"properties\":{\"min\":{\"title\":\"min\",\"type\":\"integer\"}},\"required\":[\"min\"],\"type\":\"object\"}", err: nil},
		},
		"parameter in cue is a dict type,": {
			reason: "Prepare a normal parameter cue file",
			name:   "workload2",
			data: `
annotations: {
	for k, v in parameter {
		"\(k)": v
	}
}

parameter: [string]: string
`,
			want: want{data: `{"additionalProperties":{"type":"string"},"type":"object"}`, err: nil},
		},
		"parameter in cue is a string type,": {
			reason: "Prepare a normal parameter cue file",
			name:   "workload3",
			data: `
annotations: {
	"test":parameter
}

   parameter:string
`,
			want: want{data: "{\"type\":\"string\"}", err: nil},
		},
		"parameter in cue is a list type,": {
			reason: "Prepare a list parameter cue file",
			name:   "workload4",
			data: `
annotations: {
	"test":parameter
}

   parameter:[...string]
`,
			want: want{data: "{\"items\":{\"type\":\"string\"},\"type\":\"array\"}", err: nil},
		},
		"parameter in cue is an int type,": {
			reason: "Prepare an int parameter cue file",
			name:   "workload5",
			data: `
annotations: {
	"test":parameter
}

   parameter: int
`,
			want: want{data: "{\"type\":\"integer\"}", err: nil},
		},
		"parameter in cue is a float type,": {
			reason: "Prepare a float parameter cue file",
			name:   "workload6",
			data: `
annotations: {
	"test":parameter
}

   parameter: float
`,
			want: want{data: "{\"type\":\"number\"}", err: nil},
		},
		"parameter in cue is a bool type,": {
			reason: "Prepare a bool parameter cue file",
			name:   "workload7",
			data: `
annotations: {
	"test":parameter
}

   parameter: bool
`,
			want: want{data: "{\"type\":\"boolean\"}", err: nil},
		},
		"cue doesn't contain parameter section": {
			reason: "Prepare a cue file which doesn't contain `parameter` section",
			name:   "invalidWorkload",
			data: `
project: {
	name: string
}

noParameter: {
	min: int
}
`,
			want: want{data: "{\"type\":\"object\"}", err: nil},
		},
		"cue doesn't parse other sections except parameter": {
			reason: "Prepare a cue file which contains `context.appName` field",
			name:   "withContextField",
			data: `
patch: {
	spec: template: metadata: labels: {
		for k, v in parameter {
			"\(k)": v
		}
	}
	spec: template: metadata: annotations: {
		"route-name.oam.dev": #routeName
	}
}

#routeName: "\(context.appName)-\(context.name)"

parameter: [string]: string
`,
			want: want{data: `{"additionalProperties":{"type":"string"},"type":"object"}`, err: nil},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			schematic := &common.Schematic{
				CUE: &common.CUE{
					Template: tc.data,
				},
			}
			capability, _ := appfile.ConvertTemplateJSON2Object(tc.name, nil, schematic)
			schema, err := getOpenAPISchema(capability)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ngetOpenAPISchema(...): -want error, +got error:\n%s", tc.reason, diff)
			}
			if tc.want.err == nil {
				assert.Equal(t, string(schema), tc.want.data)
			}
		})
	}
}

func TestNewCapabilityComponentDef(t *testing.T) {
	terraform := &common.Terraform{
		Configuration: "test",
	}
	componentDefinition := &v1beta1.ComponentDefinition{
		Spec: v1beta1.ComponentDefinitionSpec{
			Schematic: &common.Schematic{
				Terraform: terraform,
			},
		},
	}
	def := NewCapabilityComponentDef(componentDefinition)
	assert.Equal(t, def.WorkloadType, util.TerraformDef)
	assert.Equal(t, def.Terraform, terraform)
}

func TestGetOpenAPISchemaFromTerraformComponentDefinition(t *testing.T) {
	type want struct {
		subStr string
		err    error
	}
	cases := map[string]struct {
		configuration string
		want          want
	}{
		"valid": {
			configuration: `
module "rds" {
  source = "terraform-alicloud-modules/rds/alicloud"
  engine = "MySQL"
  engine_version = "8.0"
  instance_type = "rds.mysql.c1.large"
  instance_storage = "20"
  instance_name = var.instance_name
  account_name = var.account_name
  password = var.password
}

output "DB_NAME" {
  value = module.rds.this_db_instance_name
}
output "DB_USER" {
  value = module.rds.this_db_database_account
}
output "DB_PORT" {
  value = module.rds.this_db_instance_port
}
output "DB_HOST" {
  value = module.rds.this_db_instance_connection_string
}
output "DB_PASSWORD" {
  value = module.rds.this_db_instance_port
}

variable "instance_name" {
  description = "RDS instance name"
  type = string
  default = "poc"
}

variable "account_name" {
  description = "RDS instance user account name"
  type = "string"
  default = "oam"
}

variable "password" {
  description = "RDS instance account password"
  type = "string"
  default = "xxx"
}

variable "intVar" {
  type = "number"
}`,
			want: want{
				subStr: `"required":["intVar"]`,
				err:    nil,
			},
		},
		"null type variable": {
			configuration: `
variable "name" {
  default = "abc"
}`,
			want: want{
				subStr: "abc",
				err:    nil,
			},
		},
		"null type variable, while default value is a slice": {
			configuration: `
variable "name" {
  default = [123]
}`,
			want: want{
				subStr: "123",
				err:    nil,
			},
		},
		"null type variable, while default value is a map": {
			configuration: `
variable "name" {
  default = {a = 1}
}`,
			want: want{
				subStr: "a",
				err:    nil,
			},
		},
		"null type variable, while default value is number": {
			configuration: `
variable "name" {
  default = 123
}`,
			want: want{
				subStr: "123",
				err:    nil,
			},
		},
		"complicated list variable": {
			configuration: `
variable "aaa" {
  type = list(object({
    type = string
    sourceArn = string
    config = string
  }))
  default = []
}`,
			want: want{
				subStr: "aaa",
				err:    nil,
			},
		},
		"complicated map variable": {
			configuration: `
variable "bbb" {
  type = map({
    type = string
    sourceArn = string
    config = string
  })
  default = []
}`,
			want: want{
				subStr: "bbb",
				err:    nil,
			},
		},
		"not supported complicated variable": {
			configuration: `
variable "bbb" {
  type = xxxxx(string)
}`,
			want: want{
				subStr: "",
				err:    fmt.Errorf("the type `%s` of variable %s is NOT supported", "xxxxx(string)", "bbb"),
			},
		},
		"any type, slice default": {
			configuration: `
variable "bbb" {
  type = any
  default = []
}`,
			want: want{
				subStr: "bbb",
				err:    nil,
			},
		},
		"any type, map default": {
			configuration: `
variable "bbb" {
  type = any
  default = {}
}`,
			want: want{
				subStr: "bbb",
				err:    nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			schema, err := GetOpenAPISchemaFromTerraformComponentDefinition(tc.configuration)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nGetOpenAPISchemaFromTerraformComponentDefinition(...): -want error, +got error:\n%s", name, diff)
			}
			if tc.want.err == nil {
				data := string(schema)
				assert.Equal(t, strings.Contains(data, tc.want.subStr), true)
			}
		})
	}
}

func TestGetGitSSHPublicKey(t *testing.T) {
	sshAuth := make(map[string][]byte)
	sshAuth[corev1.SSHAuthPrivateKey] = testdata.PEMBytes["rsa"]
	sshAuth[GitCredsKnownHosts] = []byte(`github.com ssh-rsa AAAAB3NzaC1yc2EAAAABIwAAAQEAq2A7hRGmdnm9tUDbO9IDSwBK6TbQa
	+PXYPCPy6rbTrTtw7PHkccKrpp0yVhp5HdEIcKr6pLlVDBfOLX9QUsyCOV0wzfjIJNlGEYsdlLJizHhbn2mUjvSAHQqZETYP81eFzLQNnPHt4EVVUh7
	VfDESU84KezmD5QlWpXLmvU31/yMf+Se8xhHTvKSCZIFImWwoG6mbUoWf9nzpIoaSjB+weqqUUmpaaasXVal72J+UX2B+2RPW3RcT0eOzQgqlJL3RKr
	TJvdsjE3JEAvGq3lGHSZXy28G3skua2SmVi/w4yCE6gbODqnTWlg7+wC604ydGXA8VJiS5ap43JXiUFFAaQ==`)

	pubKey, err := gitssh.NewPublicKeys("git", sshAuth[corev1.SSHAuthPrivateKey], "")
	assert.NilError(t, err)

	k8sClient := fake.NewClientBuilder().Build()
	ctx := context.Background()

	secret := corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "git-ssh-auth",
			Namespace: "default",
		},
		Data: sshAuth,
		Type: corev1.SecretTypeSSHAuth,
	}
	err = k8sClient.Create(ctx, &secret)
	assert.NilError(t, err)

	secret = corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "git-ssh-auth-no-ssh-privatekey",
			Namespace: "default",
		},
		Data: map[string][]byte{
			GitCredsKnownHosts: sshAuth[GitCredsKnownHosts],
		},
	}
	err = k8sClient.Create(ctx, &secret)
	assert.NilError(t, err)

	secret = corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "git-ssh-auth-no-known_hosts",
			Namespace: "default",
		},
		Data: map[string][]byte{
			corev1.SSHAuthPrivateKey: sshAuth[corev1.SSHAuthPrivateKey],
		},
		Type: corev1.SecretTypeSSHAuth,
	}
	err = k8sClient.Create(ctx, &secret)
	assert.NilError(t, err)

	type args struct {
		k8sClient                     client.Client
		GitCredentialsSecretReference *corev1.SecretReference
	}
	type want struct {
		publicKey *gitssh.PublicKeys
		err       string
	}
	cases := []struct {
		name string
		args args
		want want
	}{
		{
			name: "git credentials secret does not exist",
			args: args{
				k8sClient: k8sClient,
				GitCredentialsSecretReference: &corev1.SecretReference{
					Name:      "git-ssh-auth-secret-not-exist",
					Namespace: "default",
				},
			},
			want: want{
				publicKey: nil,
				err:       "failed to  get git credentials secret: secrets \"git-ssh-auth-secret-not-exist\" not found",
			},
		},
		{
			name: "ssh-privatekey not in git credentials secret",
			args: args{
				k8sClient: k8sClient,
				GitCredentialsSecretReference: &corev1.SecretReference{
					Name:      "git-ssh-auth-no-ssh-privatekey",
					Namespace: "default",
				},
			},
			want: want{
				publicKey: nil,
				err:       fmt.Sprintf("'%s' not in git credentials secret", corev1.SSHAuthPrivateKey),
			},
		},
		{
			name: "known_hosts not in git credentials secret",
			args: args{
				k8sClient: k8sClient,
				GitCredentialsSecretReference: &corev1.SecretReference{
					Name:      "git-ssh-auth-no-known_hosts",
					Namespace: "default",
				},
			},
			want: want{
				publicKey: nil,
				err:       fmt.Sprintf("'%s' not in git credentials secret", GitCredsKnownHosts),
			},
		},
		{
			name: "valid git credentials secret found",
			args: args{
				k8sClient: k8sClient,
				GitCredentialsSecretReference: &corev1.SecretReference{
					Name:      "git-ssh-auth",
					Namespace: "default",
				},
			},
			want: want{
				publicKey: pubKey,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			publicKey, err := GetGitSSHPublicKey(ctx, tc.args.k8sClient, tc.args.GitCredentialsSecretReference)

			if len(tc.want.err) > 0 {
				assert.Error(t, err, tc.want.err)
			}

			if tc.want.publicKey != nil {
				assert.DeepEqual(t, publicKey.Signer.PublicKey().Marshal(), tc.want.publicKey.Signer.PublicKey().Marshal())
				assert.DeepEqual(t, publicKey.User, tc.want.publicKey.User)
				known_hosts_filepath := os.Getenv("SSH_KNOWN_HOSTS")
				known_hosts, err := os.ReadFile(known_hosts_filepath)
				assert.NilError(t, err)
				assert.DeepEqual(t, known_hosts, sshAuth[GitCredsKnownHosts])
			}
		})
	}
}
