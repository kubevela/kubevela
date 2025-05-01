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

package cli

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

var (
	appWithoutTopologyPolicyYaml = `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: app-without-policies
  namespace: vela-system
spec:
  components:
    - name: nginx-basic
      type: webservice
      properties:
        image: nginx
`
	appWithTopologyClustersYaml = `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: basic-topology
  namespace: default
spec:
  components:
    - name: nginx-basic
      type: webservice
      properties:
        image: nginx
  policies:
    - name: topology-hangzhou-clusters
      type: topology
      properties:
        clusters: ["hangzhou-1", "hangzhou-2"]
`
	appWithTopologyClusterLabelSelectorYaml = `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: region-selector
  namespace: vela-system
spec:
  components:
    - name: nginx-basic
      type: webservice
      properties:
        image: nginx
  policies:
    - name: topology-hangzhou-clusters
      type: topology
      properties:
        clusterLabelSelector:
          region: hangzhou
`
	appWithEmptyTopologyClusterLabelSelectorYaml = `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: empty-cluster-selector
  namespace: default
spec:
  components:
    - name: nginx-basic
      type: webservice
      properties:
        image: nginx
  policies:
    - name: topology-hangzhou-clusters
      type: topology
      properties:
        clusterLabelSelector: {}
`
)

var _ = Describe("Test updateAppsWithTopologyPolicy", func() {

	var _ = When("app does not have topology policy", func() {
		It("app should not have publish version annotation set", func() {
			err := createApplication(appWithoutTopologyPolicyYaml)
			Expect(err).Should(BeNil())

			cmd := &cobra.Command{}
			err = updateAppsWithTopologyPolicy(context.Background(), cmd, k8sClient)
			Expect(err).Should(BeNil())

			matched, err := hasPublishVersionAnnotation("app-without-policies", "vela-system")
			Expect(err).Should(BeNil())
			Expect(matched).Should(BeFalse())
		})
	})

	var _ = When("app has topology policy without clusterLabelSelector", func() {
		It("app should not have publish version annotation set", func() {
			err := createApplication(appWithTopologyClustersYaml)
			Expect(err).Should(BeNil())

			cmd := &cobra.Command{}
			err = updateAppsWithTopologyPolicy(context.Background(), cmd, k8sClient)
			Expect(err).Should(BeNil())

			matched, err := hasPublishVersionAnnotation("basic-topology", "default")
			Expect(err).Should(BeNil())
			Expect(matched).Should(BeFalse())
		})
	})

	var _ = When("app has topology policy with clusterLabelSelector", func() {
		It("app should have publish version annotation set", func() {
			err := createApplication(appWithTopologyClusterLabelSelectorYaml)
			Expect(err).Should(BeNil())

			cmd := &cobra.Command{}
			err = updateAppsWithTopologyPolicy(context.Background(), cmd, k8sClient)
			Expect(err).Should(BeNil())

			matched, err := hasPublishVersionAnnotation("region-selector", "vela-system")
			Expect(err).Should(BeNil())
			Expect(matched).Should(BeTrue())
		})
	})

	var _ = When("app has topology policy with empty clusterLabelSelector", func() {
		It("app should have publish version annotation set", func() {
			err := createApplication(appWithEmptyTopologyClusterLabelSelectorYaml)
			Expect(err).Should(BeNil())

			cmd := &cobra.Command{}
			err = updateAppsWithTopologyPolicy(context.Background(), cmd, k8sClient)
			Expect(err).Should(BeNil())

			matched, err := hasPublishVersionAnnotation("empty-cluster-selector", "default")
			Expect(err).Should(BeNil())
			Expect(matched).Should(BeTrue())
		})
	})

})

func createApplication(appYaml string) error {
	app := v1beta1.Application{}
	if err := yaml.Unmarshal([]byte(appYaml), &app); err != nil {
		return fmt.Errorf("unmarshal error for yaml %s: %w", appYaml, err)
	}
	if err := k8sClient.Create(context.Background(), &app); err != nil {
		return fmt.Errorf("error in creating app %s in namespace %s: %w", app.Name, app.Namespace, err)
	}
	return nil
}

func hasPublishVersionAnnotation(name, namespace string) (bool, error) {
	app := &v1beta1.Application{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: name, Namespace: namespace}, app); err != nil {
		return false, fmt.Errorf("error in getting application %s in namespace %s: %w", name, namespace, err)
	}
	annotations := app.GetAnnotations()
	if annotations != nil && annotations[oam.AnnotationPublishVersion] != "" {
		return true, nil
	}
	return false, nil
}
