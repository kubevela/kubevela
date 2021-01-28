package appfile

import (
	"fmt"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
)

var _ = It("Test ApplyTerraform", func() {
	app := &v1alpha2.Application{
		ObjectMeta: v1.ObjectMeta{Name: "test-terraform-app"},
		Spec: v1alpha2.ApplicationSpec{Components: []v1alpha2.ApplicationComponent{{
			Name:         "test-terraform-svc",
			WorkloadType: "aliyun-oss",
			Settings:     runtime.RawExtension{Raw: []byte("{\"bucket\": \"oam-website\"}")},
		},
		}},
	}
	ioStream := util.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	dm, _ := discoverymapper.New(cfg)
	_, err := ApplyTerraform(app, k8sClient, ioStream, addonNamespace, dm)
	Expect(err.Error()).Should(Equal("exit status 1"))
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
		Expect(err).Should(Equal(fmt.Errorf("Terraform output isn't in the right format")))
	})
})
