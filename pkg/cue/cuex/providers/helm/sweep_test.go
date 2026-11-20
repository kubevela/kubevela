/*
Copyright 2026 The KubeVela Authors.

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

package helm

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/release"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/endpoints/request"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

var _ = Describe("sweepHubReleases", func() {
	const appNS = "team-a"
	shopApp := func(name string) *v1beta1.Application {
		return &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: appNS}}
	}

	// seed stores a deployed hub release for component comp of Application shop, with its
	// release Secret in the fake clientset, as an install leaves them.
	seed := func(cfg *action.Configuration, p *Provider, name, comp string) {
		labels := map[string]string{"app.oam.dev/name": "shop", "app.oam.dev/namespace": appNS, "app.oam.dev/component": comp}
		Expect(cfg.Releases.Create(&release.Release{
			Name: name, Namespace: appNS, Version: 1,
			Info: &release.Info{Status: release.StatusDeployed}, Chart: minimalChart("c", "1.0.0"), Labels: labels,
		})).To(Succeed())
		secretLabels := map[string]string{"owner": "helm", "name": name}
		for k, v := range labels {
			secretLabels[k] = v
		}
		cs, _ := p.kubeClientFactory(context.Background())
		_, err := cs.CoreV1().Secrets(appNS).Create(context.Background(), &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "sh.helm.release.v1." + name + ".v1", Namespace: appNS, Labels: secretLabels},
		}, metav1.CreateOptions{})
		Expect(err).ShouldNot(HaveOccurred())
	}
	deployed := func(cfg *action.Configuration, name string) bool {
		_, err := cfg.Releases.Deployed(name)
		return err == nil
	}

	It("uninstalls a hub release whose component is placed only on other clusters", func() {
		cfg := fakeActionConfig()
		p := installProviderWithFake(cfg)
		seed(cfg, p, "redis", "redis")
		seed(cfg, p, "web", "web")
		seed(cfg, p, "db", "db")

		placement := func(comp string) (onHub, elsewhere bool) {
			switch comp {
			case "redis":
				return false, true // moved to a managed cluster: the hub copy is left behind
			case "web":
				return true, true // on the hub as well
			default:
				return false, false // not rendered this run: nothing to judge by
			}
		}
		Expect(p.sweepHubReleases(context.Background(), shopApp("shop"), placement)).To(Succeed())

		Expect(deployed(cfg, "redis")).To(BeFalse())
		Expect(deployed(cfg, "web")).To(BeTrue())
		Expect(deployed(cfg, "db")).To(BeTrue())
	})

	It("uninstalls as the Application's user, as it was installed", func() {
		cfg := fakeActionConfig()
		p := installProviderWithFake(cfg)
		seed(cfg, p, "redis", "redis")
		var users []string
		p.actionConfigFactory = func(ctx context.Context, _ string) (*action.Configuration, error) {
			if u, ok := request.UserFrom(ctx); ok && u != nil {
				users = append(users, u.GetName())
			}
			return cfg, nil
		}
		app := shopApp("shop")
		app.Annotations = map[string]string{oam.AnnotationApplicationUsername: "alice"}

		Expect(p.sweepHubReleases(context.Background(), app, func(string) (bool, bool) { return false, true })).To(Succeed())
		Expect(users).To(Equal([]string{"alice"}))
	})

	It("leaves releases another Application owns", func() {
		cfg := fakeActionConfig()
		p := installProviderWithFake(cfg)
		seed(cfg, p, "redis", "redis")

		Expect(p.sweepHubReleases(context.Background(), shopApp("blog"), func(string) (bool, bool) { return false, true })).To(Succeed())
		Expect(deployed(cfg, "redis")).To(BeTrue())
	})
})

var _ = Describe("tracksManagedClusterRelease", func() {
	ref := func(kind, name, cluster string) common.ClusterObjectReference {
		return common.ClusterObjectReference{Cluster: cluster, ObjectReference: corev1.ObjectReference{Kind: kind, Name: name}}
	}

	It("is true only for a release Secret tracked on a managed cluster", func() {
		Expect(tracksManagedClusterRelease([]common.ClusterObjectReference{ref("Secret", "sh.helm.release.v1.redis.v1", "eu-west")})).To(BeTrue())
		Expect(tracksManagedClusterRelease([]common.ClusterObjectReference{
			ref("Secret", "sh.helm.release.v1.redis.v1", ""),
			ref("Secret", "sh.helm.release.v1.redis.v1", "local"),
			ref("Secret", "db-password", "eu-west"),
			ref("ConfigMap", "sh.helm.release.v1.redis.v1", "eu-west"),
		})).To(BeFalse())
	})
})
