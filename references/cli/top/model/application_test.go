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

package model

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
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
	ctx := context.Background()
	ctx = context.WithValue(ctx, &CtxKeyNamespace, "")

	It("application num", func() {
		num, err := applicationNum(ctx, k8sClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(num).To(Equal(1))
	})
	It("running application num", func() {
		num, err := runningApplicationNum(ctx, k8sClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(num).To(Equal(1))
	})
	It("application running ratio", func() {
		num := ApplicationRunningNum(cfg)
		Expect(num).To(Equal("1/1"))
	})
	It("list applications", func() {
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
