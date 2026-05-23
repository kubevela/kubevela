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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clientsetfake "k8s.io/client-go/kubernetes/fake"
)

// providerWithFakeClientset returns a fresh Provider whose kubeClientFactory
// is bound to a fake clientset pre-seeded with the supplied secrets, so the
// release-secret helpers never touch the active cluster.
func providerWithFakeClientset(seed ...*corev1.Secret) (*Provider, *clientsetfake.Clientset) {
	objs := make([]runtime.Object, 0, len(seed))
	for _, s := range seed {
		objs = append(objs, s)
	}
	cs := clientsetfake.NewSimpleClientset(objs...)
	p := NewProviderWithConfig(nil)
	p.kubeClientFactory = func() (kubernetes.Interface, error) { return cs, nil }
	return p, cs
}

var _ = Describe("release_secrets", func() {

	Describe("InvalidateRelease", func() {
		It("should clear all cache entries for a release", func() {
			p := NewProviderWithConfig(nil)

			cacheKey := "default/test-rel"
			p.releaseMu.Lock()
			p.releaseFingerprints[cacheKey] = "fp1"
			p.releaseManifests[cacheKey] = "manifest"
			p.releaseVersions[cacheKey] = 1
			p.releaseMu.Unlock()

			Expect(p.releaseFingerprints[cacheKey]).To(Equal("fp1"))

			p.InvalidateRelease("test-rel", "default")

			_, ok := p.releaseFingerprints[cacheKey]
			Expect(ok).To(BeFalse())
			_, ok = p.releaseManifests[cacheKey]
			Expect(ok).To(BeFalse())
			_, ok = p.releaseVersions[cacheKey]
			Expect(ok).To(BeFalse())
		})
	})

	Describe("listReleaseSecretNames against a fake clientset", func() {
		It("returns names of vela-owned helm release secrets", func() {
			ns := "rel-ns"
			owned1 := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sh.helm.release.v1.r.v1",
					Namespace: ns,
					Labels:    map[string]string{"owner": "helm", "name": "r", "app.oam.dev/name": "app"},
				},
			}
			owned2 := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sh.helm.release.v1.r.v2",
					Namespace: ns,
					Labels:    map[string]string{"owner": "helm", "name": "r", "app.oam.dev/name": "app"},
				},
			}
			external := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sh.helm.release.v1.r.v3",
					Namespace: ns,
					Labels:    map[string]string{"owner": "helm", "name": "r"},
				},
			}
			p, _ := providerWithFakeClientset(owned1, owned2, external)
			names := p.listReleaseSecretNames(ns, "r")
			Expect(names).To(ConsistOf("sh.helm.release.v1.r.v1", "sh.helm.release.v1.r.v2"))
		})

		It("returns empty when no matching helm secrets exist", func() {
			p, _ := providerWithFakeClientset()
			Expect(p.listReleaseSecretNames("default", "missing")).To(BeEmpty())
		})
	})

	Describe("labelReleaseSecrets against a fake clientset", func() {
		It("patches missing app.oam.dev labels onto external release secrets", func() {
			ns := "rel-ns"
			external := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sh.helm.release.v1.r.v1",
					Namespace: ns,
					Labels:    map[string]string{"owner": "helm", "name": "r"},
				},
			}
			p, cs := providerWithFakeClientset(external)
			velaCtx := &ContextParams{AppName: "app", AppNamespace: ns, Name: "comp"}

			p.labelReleaseSecrets(ns, "r", velaCtx)

			patched, err := cs.CoreV1().Secrets(ns).Get(GinkgoT().Context(), "sh.helm.release.v1.r.v1", metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(patched.Labels).To(HaveKeyWithValue("app.oam.dev/name", "app"))
			Expect(patched.Labels).To(HaveKeyWithValue("app.oam.dev/namespace", ns))
			Expect(patched.Labels).To(HaveKeyWithValue("app.oam.dev/component", "comp"))
		})

		It("skips already-labeled secrets", func() {
			ns := "rel-ns"
			already := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sh.helm.release.v1.r.v1",
					Namespace: ns,
					Labels: map[string]string{
						"owner": "helm", "name": "r",
						"app.oam.dev/name": "old-app",
					},
				},
			}
			p, cs := providerWithFakeClientset(already)
			velaCtx := &ContextParams{AppName: "new-app", AppNamespace: ns, Name: "comp"}

			p.labelReleaseSecrets(ns, "r", velaCtx)

			got, err := cs.CoreV1().Secrets(ns).Get(GinkgoT().Context(), "sh.helm.release.v1.r.v1", metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(got.Labels["app.oam.dev/name"]).To(Equal("old-app"), "must not overwrite existing app.oam.dev/name label")
		})

		It("is a no-op for nil context", func() {
			p, _ := providerWithFakeClientset()
			Expect(func() { p.labelReleaseSecrets("default", "r", nil) }).ToNot(Panic())
		})
	})

	Describe("deleteReleaseSecretsDirect against a fake clientset", func() {
		It("deletes only helm-owned secrets for the named release", func() {
			ns := "rel-ns"
			target1 := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sh.helm.release.v1.r.v1", Namespace: ns,
					Labels: map[string]string{"owner": "helm", "name": "r"},
				},
			}
			target2 := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sh.helm.release.v1.r.v2", Namespace: ns,
					Labels: map[string]string{"owner": "helm", "name": "r"},
				},
			}
			unrelated := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "other", Namespace: ns,
					Labels: map[string]string{"owner": "helm", "name": "different-release"},
				},
			}
			p, cs := providerWithFakeClientset(target1, target2, unrelated)
			Expect(p.deleteReleaseSecretsDirect(ns, "r", nil)).To(Succeed())

			remaining, err := cs.CoreV1().Secrets(ns).List(GinkgoT().Context(), metav1.ListOptions{})
			Expect(err).ShouldNot(HaveOccurred())
			names := make([]string, 0, len(remaining.Items))
			for _, s := range remaining.Items {
				names = append(names, s.Name)
			}
			Expect(names).To(ConsistOf("other"))
		})
	})

})
