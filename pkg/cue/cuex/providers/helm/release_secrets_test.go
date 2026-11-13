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
	"fmt"
	"strings"

	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clientsetfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
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
	p.kubeClientFactory = func(context.Context) (kubernetes.Interface, error) { return cs, nil }
	return p, cs
}

var _ = Describe("release_secrets", func() {

	Describe("InvalidateRelease", func() {
		It("should clear all cache entries for a release", func() {
			p := NewProviderWithConfig(nil)

			cacheKey := releaseCacheKey(pkgmulticluster.Local, "default", "test-rel")
			p.releaseMu.Lock()
			p.releaseFingerprints[cacheKey] = "fp1"
			p.releaseManifests[cacheKey] = "manifest"
			p.releaseVersions[cacheKey] = 1
			p.releaseMu.Unlock()

			Expect(p.releaseFingerprints[cacheKey]).To(Equal("fp1"))

			p.InvalidateRelease(pkgmulticluster.Local, "test-rel", "default")

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
			names := p.listReleaseSecretNames(context.Background(), ns, "r")
			Expect(names).To(ConsistOf("sh.helm.release.v1.r.v1", "sh.helm.release.v1.r.v2"))
		})

		It("returns empty when no matching helm secrets exist", func() {
			p, _ := providerWithFakeClientset()
			Expect(p.listReleaseSecretNames(context.Background(), "default", "missing")).To(BeEmpty())
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

			p.labelReleaseSecrets(context.Background(), ns, "r", velaCtx)

			patched, err := cs.CoreV1().Secrets(ns).Get(GinkgoT().Context(), "sh.helm.release.v1.r.v1", metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(patched.Labels).To(HaveKeyWithValue("app.oam.dev/name", "app"))
			Expect(patched.Labels).To(HaveKeyWithValue("app.oam.dev/namespace", ns))
			Expect(patched.Labels).To(HaveKeyWithValue("app.oam.dev/component", "comp"))
		})

		It("replaces a departed owner's labels", func() {
			ns := "rel-ns"
			stale := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sh.helm.release.v1.r.v1",
					Namespace: ns,
					Labels: map[string]string{
						"owner": "helm", "name": "r",
						"app.oam.dev/name": "old-app", "app.oam.dev/namespace": "elsewhere", "app.oam.dev/component": "comp",
					},
				},
			}
			p, cs := providerWithFakeClientset(stale)
			p.ownerAppExists = func(context.Context, string, string) (bool, error) { return false, nil }
			velaCtx := &ContextParams{AppName: "new-app", AppNamespace: ns, Name: "comp"}

			p.labelReleaseSecrets(context.Background(), ns, "r", velaCtx)

			got, err := cs.CoreV1().Secrets(ns).Get(GinkgoT().Context(), "sh.helm.release.v1.r.v1", metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(got.Labels).To(HaveKeyWithValue("app.oam.dev/name", "new-app"))
			Expect(got.Labels).To(HaveKeyWithValue("app.oam.dev/namespace", ns))
		})

		It("leaves secrets already labelled for this component untouched", func() {
			ns := "rel-ns"
			owned := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sh.helm.release.v1.r.v1",
					Namespace: ns,
					Labels: map[string]string{
						"owner": "helm", "name": "r",
						"app.oam.dev/name": "app", "app.oam.dev/namespace": ns, "app.oam.dev/component": "comp",
					},
				},
			}
			p, cs := providerWithFakeClientset(owned)
			p.labelReleaseSecrets(context.Background(), ns, "r", &ContextParams{AppName: "app", AppNamespace: ns, Name: "comp"})

			for _, a := range cs.Actions() {
				Expect(a.GetVerb()).To(Equal("list"), "nothing but the list")
			}
		})

		It("leaves a release Secret a live Application owns as it is", func() {
			ns := "rel-ns"
			theirs := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
				Name: "sh.helm.release.v1.r.v1", Namespace: ns,
				Labels: map[string]string{"owner": "helm", "name": "r",
					"app.oam.dev/name": "other", "app.oam.dev/namespace": ns, "app.oam.dev/component": "db"},
			}}
			p, cs := providerWithFakeClientset(theirs)
			p.ownerAppExists = func(context.Context, string, string) (bool, error) { return true, nil }
			p.labelReleaseSecrets(context.Background(), ns, "r", &ContextParams{AppName: "app", AppNamespace: ns, Name: "comp"})

			got, err := cs.CoreV1().Secrets(ns).Get(GinkgoT().Context(), theirs.Name, metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(got.Labels).To(Equal(theirs.Labels))
		})

		It("is a no-op for nil context", func() {
			p, _ := providerWithFakeClientset()
			Expect(func() { p.labelReleaseSecrets(context.Background(), "default", "r", nil) }).ToNot(Panic())
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
			Expect(p.deleteReleaseSecretsDirect(context.Background(), ns, "r", nil)).To(Succeed())

			remaining, err := cs.CoreV1().Secrets(ns).List(GinkgoT().Context(), metav1.ListOptions{})
			Expect(err).ShouldNot(HaveOccurred())
			names := make([]string, 0, len(remaining.Items))
			for _, s := range remaining.Items {
				names = append(names, s.Name)
			}
			Expect(names).To(ConsistOf("other"))
		})
		It("leaves a release another live Application owns, and recovers one whose owner is gone", func() {
			ns := "rel-ns"
			theirs := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
				Name: "sh.helm.release.v1.r.v1", Namespace: ns,
				Labels: map[string]string{"owner": "helm", "name": "r",
					"app.oam.dev/name": "other", "app.oam.dev/namespace": ns, "app.oam.dev/component": "db"},
			}}
			velaCtx := &ContextParams{AppName: "app", AppNamespace: ns, Name: "comp"}

			p, cs := providerWithFakeClientset(theirs.DeepCopy())
			p.ownerAppExists = func(context.Context, string, string) (bool, error) { return true, nil }
			Expect(p.deleteReleaseSecretsDirect(context.Background(), ns, "r", velaCtx)).To(MatchError(ContainSubstring("belongs to Application rel-ns/other")))
			_, err := cs.CoreV1().Secrets(ns).Get(GinkgoT().Context(), theirs.Name, metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred(), "left in place")

			p, cs = providerWithFakeClientset(theirs.DeepCopy())
			p.ownerAppExists = func(context.Context, string, string) (bool, error) { return false, nil }
			Expect(p.deleteReleaseSecretsDirect(context.Background(), ns, "r", velaCtx)).To(Succeed())
			remaining, err := cs.CoreV1().Secrets(ns).List(GinkgoT().Context(), metav1.ListOptions{})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(remaining.Items).To(BeEmpty())
		})

		It("refuses a release whose owner labels are incomplete", func() {
			ns := "rel-ns"
			partial := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
				Name: "sh.helm.release.v1.r.v1", Namespace: ns,
				Labels: map[string]string{"owner": "helm", "name": "r", "app.oam.dev/name": "app", "app.oam.dev/component": "comp"},
			}}
			p, cs := providerWithFakeClientset(partial)
			p.ownerAppExists = func(context.Context, string, string) (bool, error) { return true, nil }
			err := p.deleteReleaseSecretsDirect(context.Background(), ns, "r", &ContextParams{AppName: "app", AppNamespace: ns, Name: "comp"})
			Expect(err).To(MatchError(ContainSubstring("incomplete ownership labels")))
			_, err = cs.CoreV1().Secrets(ns).Get(GinkgoT().Context(), partial.Name, metav1.GetOptions{})
			Expect(err).ShouldNot(HaveOccurred(), "left in place")
		})

		It("deletes this component's placeholder release Secrets, which carry no owner label", func() {
			ns := "rel-ns"
			placeholder := func(name, comp string) *corev1.Secret {
				return &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: name, Namespace: ns,
						Labels: map[string]string{"app.oam.dev/name": "app", "app.oam.dev/namespace": ns, "app.oam.dev/component": comp},
					},
					Type: "helm.sh/release.v1",
				}
			}
			p, cs := providerWithFakeClientset(
				placeholder("sh.helm.release.v1.r.v1", "comp"),
				placeholder("sh.helm.release.v1.r.v2", "comp"),
				placeholder("sh.helm.release.v1.r.v3", "other-comp"),
				placeholder("sh.helm.release.v1.rr.v1", "comp"),
			)
			velaCtx := &ContextParams{AppName: "app", AppNamespace: ns, Name: "comp"}
			Expect(p.deleteReleaseSecretsDirect(context.Background(), ns, "r", velaCtx)).To(Succeed())

			remaining, err := cs.CoreV1().Secrets(ns).List(GinkgoT().Context(), metav1.ListOptions{})
			Expect(err).ShouldNot(HaveOccurred())
			names := make([]string, 0, len(remaining.Items))
			for _, s := range remaining.Items {
				names = append(names, s.Name)
			}
			Expect(names).To(ConsistOf("sh.helm.release.v1.r.v3", "sh.helm.release.v1.rr.v1"))
		})

		It("fails when a placeholder Secret cannot be deleted, so the retry does not run on it", func() {
			ns := "rel-ns"
			p, cs := providerWithFakeClientset(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{
				Name: "sh.helm.release.v1.r.v1", Namespace: ns,
				Labels: map[string]string{"app.oam.dev/name": "app", "app.oam.dev/namespace": ns, "app.oam.dev/component": "comp"},
			}, Type: "helm.sh/release.v1"})
			cs.PrependReactor("delete", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, fmt.Errorf("delete refused")
			})
			err := p.deleteReleaseSecretsDirect(context.Background(), ns, "r", &ContextParams{AppName: "app", AppNamespace: ns, Name: "comp"})
			Expect(err).To(MatchError(ContainSubstring("delete refused")))
		})

		It("still deletes helm's own release Secrets when listing the placeholders fails", func() {
			ns := "rel-ns"
			p, cs := providerWithFakeClientset(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{
				Name: "sh.helm.release.v1.r.v1", Namespace: ns, Labels: map[string]string{"owner": "helm", "name": "r"},
			}})
			cs.PrependReactor("list", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
				if strings.Contains(action.(k8stesting.ListAction).GetListRestrictions().Labels.String(), "!owner") {
					return true, nil, fmt.Errorf("list refused")
				}
				return false, nil, nil
			})
			velaCtx := &ContextParams{AppName: "app", AppNamespace: ns, Name: "comp"}
			Expect(p.deleteReleaseSecretsDirect(context.Background(), ns, "r", velaCtx)).To(Succeed())

			remaining, err := cs.CoreV1().Secrets(ns).List(GinkgoT().Context(), metav1.ListOptions{})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(remaining.Items).To(BeEmpty())
		})
	})

})
