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

package appfile

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	commontype "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

func TestApplyTerraform(t *testing.T) {
	app := &v1beta1.Application{
		ObjectMeta: v1.ObjectMeta{Name: "test-terraform-app"},
		Spec: v1beta1.ApplicationSpec{Components: []commontype.ApplicationComponent{{
			Name:       "test-terraform-svc",
			Type:       "aliyun-oss",
			Properties: &runtime.RawExtension{Raw: []byte("{\"bucket\": \"oam-website\"}")},
		},
		}},
	}
	ioStream := util.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	arg := common.Args{
		Schema: scheme,
	}
	err := arg.SetConfig(cfg)
	assert.NoError(t, err)
	_, err = ApplyTerraform(app, k8sClient, ioStream, addonNamespace, arg)
	assert.NoError(t, err)
}

func TestGenerateSecretFromTerraformOutput(t *testing.T) {
	var name = "test-addon-secret"

	t.Run("namespace doesn't exist", func(t *testing.T) {
		badNamespace := "a-not-existed-namespace"
		err := generateSecretFromTerraformOutput(k8sClient, "", name, badNamespace)
		assert.EqualError(t, err, fmt.Sprintf("namespace %s doesn't exist", badNamespace))
	})

	t.Run("valid output list", func(t *testing.T) {
		rawOutput := "name=aaa\nage=1"
		err := generateSecretFromTerraformOutput(k8sClient, rawOutput, name, addonNamespace)
		assert.NoError(t, err)
	})

	t.Run("invalid output list", func(t *testing.T) {
		rawOutput := "name"
		err := generateSecretFromTerraformOutput(k8sClient, rawOutput, name, addonNamespace)
		assert.EqualError(t, err, fmt.Sprintf("terraform output isn't in the right format: %q", rawOutput))
	})
}
