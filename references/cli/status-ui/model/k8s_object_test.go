package model

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
)

func TestK8SObjectList_Header(t *testing.T) {
	list := K8SObjectList{title: []string{"name", "namespace", "kind", "APIVersion", "cluster", "status"}}
	assert.Equal(t, len(list.Header()), 6)
	assert.Equal(t, list.Header(), []string{"name", "namespace", "kind", "APIVersion", "cluster", "status"})
}

func TestK8SObjectList_Body(t *testing.T) {
	list := K8SObjectList{data: []K8SObject{{"", "", "", "", "", ""}}}
	assert.Equal(t, len(list.Body()), 1)
	assert.Equal(t, list.Body(), [][]string{{"", "", "", "", "", ""}})
}

var _ = Describe("test k8s object", func() {
	ctx := context.Background()
	ctx = context.WithValue(ctx, "appName", "first-vela-app")
	ctx = context.WithValue(ctx, "appNs", "default")
	ctx = context.WithValue(ctx, "cluster", "local")

	It("list k8s object", func() {
		list := ListObjects(ctx, k8sClient)
		Expect(len(list.Header())).To(Equal(6))
		Expect(len(list.Body())).To(Equal(2))
	})
})
