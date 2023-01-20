/*
Copyright 2023 The KubeVela Authors.

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

package sharding_test

import (
	"context"
	"testing"
	"time"

	"github.com/kubevela/pkg/util/singleton"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/cache"

	"github.com/kubevela/pkg/util/test/bootstrap"

	"github.com/oam-dev/kubevela/pkg/controller/sharding"
)

func TestSharding(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Run client package test")
}

var _ = bootstrap.InitKubeBuilderForTest()

var _ = Describe("Test sharding", func() {

	It("Test scheduler", func() {
		fs := pflag.NewFlagSet("-", pflag.ExitOnError)
		sharding.AddFlags(fs)
		Ω(fs.Parse([]string{"--enable-sharding", "--shard-id=s", "--schedulable-shards=s,t"})).To(Succeed())
		Ω(sharding.SchedulableShards).To(Equal([]string{"s", "t"}))

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cfg, cli := singleton.KubeConfig.Get(), singleton.KubeClient.Get()

		By("Test static scheduler")
		scheduler := sharding.NewStaticScheduler([]string{"s"})
		go scheduler.Start(ctx)
		cm1 := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "scheduled", Namespace: metav1.NamespaceDefault}}
		Ω(scheduler.Schedule(cm1)).To(BeTrue())
		Ω(cli.Create(ctx, cm1)).To(Succeed())
		scheduler = sharding.NewStaticScheduler([]string{""})
		go scheduler.Start(ctx)
		cm2 := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "unscheduled", Namespace: metav1.NamespaceDefault}}
		Ω(scheduler.Schedule(cm1)).To(BeFalse())
		Ω(cli.Create(ctx, cm2)).To(Succeed())

		By("Test cache")
		store, err := sharding.BuildCache(scheme.Scheme, &corev1.ConfigMap{})(cfg, cache.Options{})
		Ω(err).To(Succeed())
		go func() { _ = store.Start(ctx) }()
		Eventually(func(g Gomega) {
			cms := &corev1.ConfigMapList{}
			g.Expect(store.List(ctx, cms)).To(Succeed())
			g.Expect(len(cms.Items)).To(Equal(1))
			g.Expect(cms.Items[0].Name).To(Equal("scheduled"))
			g.Expect(kerrors.IsNotFound(store.Get(ctx, types.NamespacedName{Name: cm2.Name, Namespace: cm2.Namespace}, &corev1.ConfigMap{}))).To(BeTrue())
		}).WithTimeout(5 * time.Second).Should(Succeed())
	})

})
