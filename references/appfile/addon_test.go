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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	commontype "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

var _ = It("Test ApplyTerraform", func() {
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
	Expect(err).Should(BeNil())
	_, err = ApplyTerraform(app, k8sClient, ioStream, addonNamespace, arg)
	Expect(err).Should(BeNil())
})

var _ = Describe("Test generateSecretFromTerraformOutput", func() {
	var name = "test-addon-secret"
	It("namespace doesn't exist", func() {
		badNamespace := "a-not-existed-namespace"
		err := generateSecretFromTerraformOutput(k8sClient, nil, name, badNamespace)
		Expect(err).Should(Equal(fmt.Errorf("namespace %s doesn't exist", badNamespace)))
	})
	It("valid output list", func() {
		outputList := []string{"name=aaa", "age=1"}
		err := generateSecretFromTerraformOutput(k8sClient, outputList, name, addonNamespace)
		Expect(err).Should(BeNil())
	})

	It("invalid output list", func() {
		outputList := []string{"name"}
		err := generateSecretFromTerraformOutput(k8sClient, outputList, name, addonNamespace)
		Expect(err).Should(Equal(fmt.Errorf("terraform output isn't in the right format")))
	})
})
