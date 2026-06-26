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
	"path/filepath"
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
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
	assert.NoError(t, err)

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
	assert.NoError(t, err)

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
	assert.NoError(t, err)

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
	assert.NoError(t, err)

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
				assert.Equal(t, publicKey.Signer.PublicKey().Marshal(), tc.want.publicKey.Signer.PublicKey().Marshal())
				assert.Equal(t, publicKey.User, tc.want.publicKey.User)
				known_hosts_filepath := os.Getenv("SSH_KNOWN_HOSTS")
				known_hosts, err := os.ReadFile(known_hosts_filepath)
				assert.NoError(t, err)
				assert.Equal(t, known_hosts, sshAuth[GitCredsKnownHosts])
			}
		})
	}
}

// TestReadTerraformConfigFromDir guards the remote Terraform loader against
// GHSA-fmgp-q6jx-gg3x: a variables.tf (or a parent directory) that symlinks
// outside the clone cache, and an oversized configuration file. The loader
// must read normal regular files but refuse anything that escapes the cache
// or exceeds the size cap, before any content is returned.
func TestReadTerraformConfigFromDir(t *testing.T) {
	// A sentinel file outside the clone cache. A successful escape would leak
	// its content; the guards must prevent that.
	outside := t.TempDir()
	secretPath := filepath.Join(outside, "secret.txt")
	assert.NoError(t, os.WriteFile(secretPath, []byte("top-secret-host-data"), 0600))

	t.Run("reads variables.tf", func(t *testing.T) {
		dir := t.TempDir()
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "variables.tf"), []byte(`variable "x" {}`), 0600))
		got, err := readTerraformConfigFromDir(dir, "")
		assert.NoError(t, err)
		assert.Equal(t, `variable "x" {}`, got)
	})

	t.Run("falls back to main.tf", func(t *testing.T) {
		dir := t.TempDir()
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(`resource "x" "y" {}`), 0600))
		got, err := readTerraformConfigFromDir(dir, "")
		assert.NoError(t, err)
		assert.Equal(t, `resource "x" "y" {}`, got)
	})

	t.Run("errors when neither file is present", func(t *testing.T) {
		dir := t.TempDir()
		_, err := readTerraformConfigFromDir(dir, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to find")
	})

	t.Run("refuses a variables.tf that symlinks outside the cache", func(t *testing.T) {
		dir := t.TempDir()
		// This is the advisory's attack: variables.tf -> a path outside the clone.
		assert.NoError(t, os.Symlink(secretPath, filepath.Join(dir, "variables.tf")))
		got, err := readTerraformConfigFromDir(dir, "")
		assert.Error(t, err)
		assert.NotContains(t, got, "top-secret-host-data")
	})

	t.Run("refuses a symlinked subdirectory that escapes the cache", func(t *testing.T) {
		dir := t.TempDir()
		outDir := t.TempDir()
		assert.NoError(t, os.WriteFile(filepath.Join(outDir, "variables.tf"), []byte("secret-config"), 0600))
		assert.NoError(t, os.Symlink(outDir, filepath.Join(dir, "evil")))
		got, err := readTerraformConfigFromDir(dir, "evil")
		assert.Error(t, err)
		assert.NotContains(t, got, "secret-config")
	})

	t.Run("refuses a file larger than the size cap", func(t *testing.T) {
		dir := t.TempDir()
		big := make([]byte, maxTerraformConfigBytes+1024)
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "variables.tf"), big, 0600))
		_, err := readTerraformConfigFromDir(dir, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds the maximum")
	})

	t.Run("accepts a file exactly at the size cap", func(t *testing.T) {
		dir := t.TempDir()
		atCap := make([]byte, maxTerraformConfigBytes)
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "variables.tf"), atCap, 0600))
		got, err := readTerraformConfigFromDir(dir, "")
		assert.NoError(t, err)
		assert.Len(t, got, int(maxTerraformConfigBytes))
	})

	t.Run("refuses a file one byte over the size cap", func(t *testing.T) {
		dir := t.TempDir()
		overCap := make([]byte, maxTerraformConfigBytes+1)
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "variables.tf"), overCap, 0600))
		_, err := readTerraformConfigFromDir(dir, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds the maximum")
	})

	t.Run("refuses lexical ../ traversal via remotePath with no symlink", func(t *testing.T) {
		dir := t.TempDir()
		// A real variables.tf in a sibling of the cache dir, reached purely by a
		// cleaned ".." in remotePath, with no symlink anywhere. The containment
		// check (not the symlink check) must reject this.
		sibling := filepath.Join(filepath.Dir(dir), "sibling-"+filepath.Base(dir))
		assert.NoError(t, os.MkdirAll(sibling, 0700))
		defer func() { _ = os.RemoveAll(sibling) }()
		assert.NoError(t, os.WriteFile(filepath.Join(sibling, "variables.tf"), []byte("escaped-config"), 0600))
		got, err := readTerraformConfigFromDir(dir, "../"+filepath.Base(sibling))
		assert.Error(t, err)
		assert.NotContains(t, got, "escaped-config")
	})

	t.Run("refuses even a contained in-cache symlink", func(t *testing.T) {
		dir := t.TempDir()
		// Pins the deliberate reject-all-symlinks policy: a symlink whose target
		// is a regular file inside the cache is still refused.
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "real.tf"), []byte(`variable "x" {}`), 0600))
		assert.NoError(t, os.Symlink(filepath.Join(dir, "real.tf"), filepath.Join(dir, "variables.tf")))
		got, err := readTerraformConfigFromDir(dir, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "symlinked")
		assert.NotContains(t, got, "variable")
	})
}

// TestEnsureDirWithinLimits covers the clone caps that bound how large (bytes)
// and how many files a cloned remote Terraform module may retain on disk
// (GHSA-fmgp-q6jx-gg3x).
func TestEnsureDirWithinLimits(t *testing.T) {
	t.Run("passes when under the limits", func(t *testing.T) {
		dir := t.TempDir()
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "a"), make([]byte, 1024), 0600))
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "b"), make([]byte, 1024), 0600))
		assert.NoError(t, ensureDirWithinLimits(dir, 4096, 100))
	})

	t.Run("passes when cumulative size is exactly at the limit", func(t *testing.T) {
		dir := t.TempDir()
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "a"), make([]byte, 4096), 0600))
		assert.NoError(t, ensureDirWithinLimits(dir, 4096, 100))
	})

	t.Run("fails when cumulative size is one byte over the limit", func(t *testing.T) {
		dir := t.TempDir()
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "a"), make([]byte, 4097), 0600))
		err := ensureDirWithinLimits(dir, 4096, 100)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds the maximum allowed size")
	})

	t.Run("sums regular files across nested subdirectories", func(t *testing.T) {
		dir := t.TempDir()
		sub := filepath.Join(dir, "modules", "vpc")
		assert.NoError(t, os.MkdirAll(sub, 0700))
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), make([]byte, 3000), 0600))
		assert.NoError(t, os.WriteFile(filepath.Join(sub, "vars.tf"), make([]byte, 2000), 0600))
		assert.Error(t, ensureDirWithinLimits(dir, 4096, 100))
	})

	t.Run("fails when the entry count exceeds the limit", func(t *testing.T) {
		dir := t.TempDir()
		for i := 0; i < 5; i++ {
			assert.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%d", i)), []byte("x"), 0600))
		}
		err := ensureDirWithinLimits(dir, 4096, 3)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "entry count")
	})

	t.Run("counts directories toward the entry limit (inode-exhaustion guard)", func(t *testing.T) {
		dir := t.TempDir()
		// Many empty directories carry no regular-file bytes and no regular files,
		// but each is an inode, so they must still trip the entry cap.
		for i := 0; i < 10; i++ {
			assert.NoError(t, os.MkdirAll(filepath.Join(dir, fmt.Sprintf("d%d", i)), 0700))
		}
		err := ensureDirWithinLimits(dir, 4096, 3)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "entry count")
	})

	t.Run("does not follow or count symlinks", func(t *testing.T) {
		dir := t.TempDir()
		outside := t.TempDir()
		big := filepath.Join(outside, "big")
		assert.NoError(t, os.WriteFile(big, make([]byte, 8192), 0600))
		assert.NoError(t, os.Symlink(big, filepath.Join(dir, "link")))
		// The symlink is skipped (not a regular file), so the 8192-byte target is
		// neither followed nor counted against the 4096 cap.
		assert.NoError(t, ensureDirWithinLimits(dir, 4096, 100))
	})

	t.Run("returns an error when the root does not exist", func(t *testing.T) {
		assert.Error(t, ensureDirWithinLimits(filepath.Join(t.TempDir(), "missing"), 4096, 100))
	})
}

// TestCacheMatchesRemote verifies the URL-keyed cache reuse decision.
func TestCacheMatchesRemote(t *testing.T) {
	populated := func(t *testing.T, markerURL string, withMarker bool) string {
		t.Helper()
		cache := filepath.Join(t.TempDir(), "mod")
		assert.NoError(t, os.MkdirAll(cache, 0700))
		assert.NoError(t, os.WriteFile(filepath.Join(cache, "variables.tf"), []byte("x"), 0600))
		if withMarker {
			assert.NoError(t, os.WriteFile(cache+cacheRemoteMarkerSuffix, []byte(markerURL), 0600))
		}
		return cache
	}

	t.Run("matches when populated and the URL marker matches", func(t *testing.T) {
		cache := populated(t, "git://example/repo.git", true)
		assert.True(t, cacheMatchesRemote(cache, "git://example/repo.git"))
	})

	t.Run("does not match when the URL changed", func(t *testing.T) {
		cache := populated(t, "git://example/old.git", true)
		assert.False(t, cacheMatchesRemote(cache, "git://example/new.git"))
	})

	t.Run("does not match when the marker is missing", func(t *testing.T) {
		cache := populated(t, "", false)
		assert.False(t, cacheMatchesRemote(cache, "git://example/repo.git"))
	})

	t.Run("does not match when the cache is empty", func(t *testing.T) {
		assert.False(t, cacheMatchesRemote(t.TempDir(), "git://example/repo.git"))
	})
}

// TestGetTerraformConfigurationFromRemoteInvalidatesCacheOnRejection exercises
// the cache-reuse path with no network: a populated cache whose URL marker
// matches skips the clone, so a poisoned cached variables.tf (a symlink escaping
// the cache) is rejected AND the cache removed (GHSA-fmgp-q6jx-gg3x).
func TestGetTerraformConfigurationFromRemoteInvalidatesCacheOnRejection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	assert.NoError(t, os.WriteFile(secret, []byte("top-secret-host-data"), 0600))

	name := "poisoned-module"
	url := "git://unused.invalid/repo.git"
	cachePath := filepath.Join(home, ".vela", "terraform", name)
	assert.NoError(t, os.MkdirAll(cachePath, 0700))
	assert.NoError(t, os.Symlink(secret, filepath.Join(cachePath, "variables.tf")))
	// Matching URL marker so the populated cache is reused and the clone is skipped.
	assert.NoError(t, os.WriteFile(cachePath+cacheRemoteMarkerSuffix, []byte(url), 0600))

	got, err := GetTerraformConfigurationFromRemote(name, url, "", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "symlinked")
	assert.NotContains(t, got, "top-secret-host-data")

	_, statErr := os.Stat(cachePath)
	assert.True(t, os.IsNotExist(statErr), "cache should be removed after a rejected read")
}

// TestGetTerraformConfigurationFromRemoteCleansUpOnCloneFailure verifies that a
// failed clone (here a non-existent local repo) leaves no cache directory behind
// for the next reconcile to reuse (GHSA-fmgp-q6jx-gg3x).
func TestGetTerraformConfigurationFromRemoteCleansUpOnCloneFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	name := "clone-fail-module"
	bogus := filepath.Join(t.TempDir(), "does-not-exist.git")

	_, err := GetTerraformConfigurationFromRemote(name, bogus, "", nil)
	assert.Error(t, err)

	cachePath := filepath.Join(home, ".vela", "terraform", name)
	_, statErr := os.Stat(cachePath)
	assert.True(t, os.IsNotExist(statErr), "cache should be removed after a failed clone")
}

// TestGetTerraformConfigurationFromRemoteRejectsInvalidName ensures a name that
// could steer the cache path outside the cache directory is rejected up front.
func TestGetTerraformConfigurationFromRemoteRejectsInvalidName(t *testing.T) {
	for _, name := range []string{"", ".", "..", "a/b", "../evil"} {
		_, err := GetTerraformConfigurationFromRemote(name, "git://example/repo.git", "", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid Terraform module name")
	}
}

// TestRedactURLCredentials verifies credentials embedded in a remote URL are stripped
// before the URL is logged or written to the cache marker (GHSA-fmgp-q6jx-gg3x).
func TestRedactURLCredentials(t *testing.T) {
	cases := map[string]struct {
		in   string
		want string
	}{
		"https with user and password": {"https://user:s3cr3t@github.com/org/repo.git", "https://github.com/org/repo.git"},
		"https with token as user":     {"https://ghp_TOKEN123@github.com/org/repo.git", "https://github.com/org/repo.git"},
		"https without credentials":    {"https://github.com/org/repo.git", "https://github.com/org/repo.git"},
		"ssh url with user":            {"ssh://git@github.com/org/repo.git", "ssh://github.com/org/repo.git"},
		"scp-style ssh":                {"git@github.com:org/repo.git", "github.com:org/repo.git"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := redactURLCredentials(tc.in)
			assert.Equal(t, tc.want, got)
			assert.NotContains(t, got, "s3cr3t")
			assert.NotContains(t, got, "ghp_TOKEN123")
		})
	}
}

// TestRecordCacheRemoteRedactsCredentials verifies the persisted cache marker never
// contains credentials from an authenticated remote URL, while the populated cache is
// still reused for the same repository (GHSA-fmgp-q6jx-gg3x).
func TestRecordCacheRemoteRedactsCredentials(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "mod")
	assert.NoError(t, os.MkdirAll(cache, 0700))
	assert.NoError(t, os.WriteFile(filepath.Join(cache, "variables.tf"), []byte("x"), 0600))

	recordCacheRemote(cache, "https://user:s3cr3t@github.com/org/repo.git")

	marker, err := os.ReadFile(filepath.Clean(cache + cacheRemoteMarkerSuffix))
	assert.NoError(t, err)
	assert.NotContains(t, string(marker), "s3cr3t")
	assert.NotContains(t, string(marker), "user")
	assert.Equal(t, "https://github.com/org/repo.git", string(marker))

	assert.True(t, cacheMatchesRemote(cache, "https://user:s3cr3t@github.com/org/repo.git"))
	assert.True(t, cacheMatchesRemote(cache, "https://github.com/org/repo.git"))
	assert.False(t, cacheMatchesRemote(cache, "https://github.com/org/other.git"))
}
