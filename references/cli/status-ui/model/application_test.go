package model

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestApplicationList_Header(t *testing.T) {
	appList := &ApplicationList{title: []string{"Name", "Namespace", "Phase", "CreateTime"}}
	assert.Equal(t, appList.Header(), []string{"Name", "Namespace", "Phase", "CreateTime"})
}

func TestApplicationList_Body(t *testing.T) {
	appList := &ApplicationList{data: []Application{{"name", "namespace", "phase", "createTime"}}}
	assert.Equal(t, len(appList.data), 1)
	assert.Equal(t, appList.Body()[0], []string{"name", "namespace", "phase", "createTime"})
}

var _ = Describe("test Application", func() {
	var err error
	ctx := context.Background()

	It("list applications", func() {
		k8sClient, err = client.New(cfg, client.Options{Scheme: common.Scheme})
		Expect(err).NotTo(HaveOccurred())

		applicationsList, err := ListApplications(ctx, k8sClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(applicationsList.Header()).To(Equal([]string{"Name", "Namespace", "Phase", "CreateTime"}))
		Expect(len(applicationsList.Body())).To(Equal(1))
	})
	It("load application info", func() {
		application, err := LoadApplication(k8sClient, "first-vela-app", "default")
		Expect(err).NotTo(HaveOccurred())
		Expect(application.Name).To(Equal("first-vela-app"))
		Expect(application.Namespace).To(Equal("default"))
		Expect(application.ClusterName).To(Equal(""))
		Expect(len(application.Spec.Components)).To(Equal(1))
	})
})
