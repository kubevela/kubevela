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

package mcp

import (
	"github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	clustercommon "github.com/oam-dev/cluster-gateway/pkg/common"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func clusterFixtures() []client.Object {
	return append(platformFixtures(),
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "prod-east",
				Namespace:   types.DefaultKubeVelaNS,
				Labels:      map[string]string{clustercommon.LabelKeyClusterCredentialType: string(v1alpha1.CredentialTypeX509Certificate), "region": "east"},
				Annotations: map[string]string{v1alpha1.AnnotationClusterAlias: "Production East"},
			},
			Data: map[string][]byte{"endpoint": []byte("https://10.0.0.1:6443"), "tls.key": []byte("never-returned")},
		},
		&v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "addon-velaux",
				Namespace: types.DefaultKubeVelaNS,
				Labels:    map[string]string{oam.LabelAddonName: "velaux", oam.LabelAddonVersion: "v1.11.0", oam.LabelAddonRegistry: "KubeVela"},
			},
			Status: oamcommon.AppStatus{
				Phase:            oamcommon.ApplicationRunning,
				Services:         []oamcommon.ApplicationComponentStatus{{Name: "velaux", Healthy: true}},
				AppliedResources: []oamcommon.ClusterObjectReference{{ObjectReference: corev1.ObjectReference{Kind: "Deployment", Name: "velaux"}}, {Cluster: "prod-east", ObjectReference: corev1.ObjectReference{Kind: "Deployment", Name: "velaux"}}},
			},
		},
		&v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "addon-fluxcd", Namespace: types.DefaultKubeVelaNS, Labels: map[string]string{oam.LabelAddonName: "fluxcd", oam.LabelAddonVersion: "2.4.0"}},
			Status:     oamcommon.AppStatus{Phase: oamcommon.ApplicationWorkflowFailed},
		},
	)
}

var _ = Describe("cluster and addon tools", func() {
	var h *harness

	AfterEach(func() {
		if h != nil {
			h.close()
			h = nil
		}
	})

	It("lists the local cluster first and joined clusters without credentials", func() {
		h = startHarness(Options{}, clusterFixtures()...)
		var out ListClustersOutput
		res := h.call("list_clusters", nil, &out)
		Expect(res.IsError).To(BeFalse())
		Expect(out.Clusters).To(HaveLen(2))
		Expect(out.Clusters[0].Name).To(Equal("local"))
		Expect(out.Clusters[0].Local).To(BeTrue())
		east := out.Clusters[1]
		Expect(east.Name).To(Equal("prod-east"))
		Expect(east.Alias).To(Equal("Production East"))
		Expect(east.Type).To(Equal("X509Certificate"))
		Expect(east.Endpoint).To(Equal("https://10.0.0.1:6443"))
		Expect(east.Labels).To(HaveKeyWithValue("region", "east"))
		raw, _ := res.StructuredContent.(map[string]any)
		Expect(raw).NotTo(BeNil())
		Expect(toJSON(raw)).NotTo(ContainSubstring("never-returned"))
	})

	It("lists enabled addons with version, phase, health and clusters", func() {
		h = startHarness(Options{}, clusterFixtures()...)
		var out ListAddonsOutput
		h.call("list_addons", nil, &out)
		Expect(out.Addons).To(HaveLen(2))
		Expect(out.Addons[0].Name).To(Equal("fluxcd"))
		Expect(out.Addons[0].Healthy).To(BeFalse())
		Expect(out.Addons[0].Phase).To(Equal("workflowFailed"))
		velaux := out.Addons[1]
		Expect(velaux.Version).To(Equal("v1.11.0"))
		Expect(velaux.Registry).To(Equal("KubeVela"))
		Expect(velaux.Healthy).To(BeTrue())
		Expect(velaux.Clusters).To(Equal([]string{"local", "prod-east"}))
		Expect(velaux.Application).To(Equal("vela-system/addon-velaux"))
	})
})
