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

package applicationresourcetracker

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apiserver/pkg/endpoints/request"

	"github.com/oam-dev/kubevela/apis/apiextensions.core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

var _ = Describe("Test ApplicationResourceTracker API", func() {

	It("Test usa of ResourceHandlerProvider for ApplicationResourceTracker", func() {
		By("Test create handler")
		provider := NewResourceHandlerProvider(cfg)
		_s, err := provider(nil, nil)
		Expect(err).Should(Succeed())
		s, ok := _s.(*storage)
		Expect(ok).Should(BeTrue())

		By("Test meta info")
		Expect(s.New()).Should(Equal(&v1alpha1.ApplicationResourceTracker{}))
		Expect(s.NamespaceScoped()).Should(BeTrue())
		Expect(s.ShortNames()).Should(ContainElement("apprt"))
		Expect(s.NewList()).Should(Equal(&v1alpha1.ApplicationResourceTrackerList{}))

		ctx := context.Background()

		By("Create RT")
		createRt := func(name, ns, val string) *v1beta1.ResourceTracker {
			rt := &v1beta1.ResourceTracker{}
			rt.SetName(name + "-" + ns)
			rt.SetLabels(map[string]string{
				oam.LabelAppNamespace: ns,
				oam.LabelAppName:      name,
				"key":                 val,
			})
			Expect(k8sClient.Create(ctx, rt)).Should(Succeed())
			return rt
		}
		createRt("app-1", "example", "x")
		createRt("app-2", "example", "y")
		createRt("app-1", "default", "x")
		createRt("app-2", "default", "x")
		createRt("app-3", "default", "x")

		By("Test Get")
		_appRt1, err := s.Get(request.WithNamespace(ctx, "default"), "app-1", nil)
		Expect(err).Should(Succeed())
		appRt1, ok := _appRt1.(*v1alpha1.ApplicationResourceTracker)
		Expect(ok).Should(BeTrue())
		Expect(appRt1.GetLabels()["key"]).Should(Equal("x"))
		_, err = s.Get(request.WithNamespace(ctx, "no"), "app-1", nil)
		Expect(errors.IsNotFound(err)).Should(BeTrue())

		By("Test List")
		_appRts1, err := s.List(request.WithNamespace(ctx, "example"), nil)
		Expect(err).Should(Succeed())
		appRts1, ok := _appRts1.(*v1alpha1.ApplicationResourceTrackerList)
		Expect(ok).Should(BeTrue())
		Expect(len(appRts1.Items)).Should(Equal(2))

		_appRts2, err := s.List(ctx, &metainternalversion.ListOptions{LabelSelector: labels.SelectorFromValidatedSet(map[string]string{"key": "x"})})
		Expect(err).Should(Succeed())
		appRts2, ok := _appRts2.(*v1alpha1.ApplicationResourceTrackerList)
		Expect(ok).Should(BeTrue())
		Expect(len(appRts2.Items)).Should(Equal(4))

		_appRts3, err := s.List(request.WithNamespace(ctx, "default"), &metainternalversion.ListOptions{LabelSelector: labels.SelectorFromValidatedSet(map[string]string{"key": "x"})})
		Expect(err).Should(Succeed())
		appRts3, ok := _appRts3.(*v1alpha1.ApplicationResourceTrackerList)
		Expect(ok).Should(BeTrue())
		Expect(len(appRts3.Items)).Should(Equal(3))
	})

})
