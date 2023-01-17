/*
 Copyright 2022 The KubeVela Authors.

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
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestCreateMarkdownForCUE(t *testing.T) {
	go func() {
		svr := http.NewServeMux()
		svr.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "Hello, %s!", r.URL.Path[1:])
		})
		http.ListenAndServe(":65501", svr)
	}()

	time.Sleep(time.Millisecond)

	mr := MarkdownReference{}
	mr.Local = &FromLocal{Paths: []string{"./testdata/testdef.cue"}}
	capp, err := ParseLocalFile(mr.Local.Paths[0], common.Args{})
	assert.NoError(t, err)
	got, err := mr.GenerateMarkdownForCap(context.Background(), *capp, nil, false)
	assert.NoError(t, err)
	fmt.Println(got)
	assert.Contains(t, got, "A test key")
	assert.Contains(t, got, "title:  Testdef")
	assert.Contains(t, got, "K8s-objects allow users to specify raw K8s objects in properties")
	assert.Contains(t, got, "Examples")
	assert.Contains(t, got, "Hello, examples/applications/create-namespace.yaml!")

	mr.Local = &FromLocal{Paths: []string{"./testdata/testdeftrait.cue"}}
	capp, err = ParseLocalFile(mr.Local.Paths[0], common.Args{})
	assert.NoError(t, err)
	got, err = mr.GenerateMarkdownForCap(context.Background(), *capp, nil, false)
	assert.NoError(t, err)
	assert.Contains(t, got, "Specify the hostAliases to add")
	assert.Contains(t, got, "title:  Testdeftrait")
	assert.Contains(t, got, "Add host aliases on K8s pod for your workload which follows the pod spec in path 'spec.template'.")
}

func TestCreateMarkdown(t *testing.T) {
	ctx := context.Background()
	ref := &MarkdownReference{}
	ref.I18N = &En

	refZh := &MarkdownReference{}
	refZh.I18N = &Zh

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
			want: fmt.Errorf("type(scope) of the capability(scope1) is not supported for now"),
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
			got := tc.ref.CreateMarkdown(ctx, tc.capabilities, RefTestDir, false, nil)
			if diff := cmp.Diff(tc.want, got, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nCreateMakrdown(...): -want error, +got error:\n%s", tc.reason, diff)
			}
		})
	}

}
