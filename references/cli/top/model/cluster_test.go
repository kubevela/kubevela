package model

import (
	"context"
	"testing"

	"github.com/bmizerany/assert"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestClusterList_Header(t *testing.T) {
	clusterList := &ClusterList{title: []string{"Name", "Alias", "Type", "EndPoint", "Labels"}}
	assert.Equal(t, len(clusterList.Header()), 5)
	assert.Equal(t, clusterList.Header(), []string{"Name", "Alias", "Type", "EndPoint", "Labels"})
}

func TestClusterList_Body(t *testing.T) {
	clusterList := &ClusterList{data: []Cluster{{"local", "", "", "", ""}}}
	assert.Equal(t, len(clusterList.Body()), 1)
	assert.Equal(t, clusterList.Body()[0], []string{"local", "", "", "", ""})
}

var _ = Describe("test cluster", func() {
	ctx := context.WithValue(context.Background(), &CtxKeyAppName, "first-vela-app")
	ctx = context.WithValue(ctx, &CtxKeyNamespace, "default")

	It("list clusters", func() {
		clusterList := ListClusters(ctx, k8sClient)
		Expect(len(clusterList.Header())).To(Equal(5))
		Expect(clusterList.Header()).To(Equal([]string{"Name", "Alias", "Type", "EndPoint", "Labels"}))
		Expect(len(clusterList.Body())).To(Equal(1))
		Expect(clusterList.Body()[0]).To(Equal([]string{"local", "", "Internal", "-", ""}))
	})
})
