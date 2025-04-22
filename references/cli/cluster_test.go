package cli

import (
	"context"
	"fmt"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"
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
		It("app should not have update app annotation set", func() {
			err := createApplication(appWithoutTopologyPolicyYaml)
			Expect(err).Should(BeNil())

			err = updateAppsWithTopologyPolicy(context.Background(), k8sClient)
			Expect(err).Should(BeNil())

			err, result := hasUpdateTimeAnnotation("app-without-policies", "vela-system")
			Expect(err).Should(BeNil())
			Expect(result).Should(BeFalse())
		})
	})

	var _ = When("app has topology policy without clusterLabelSelector", func() {
		It("app should not have update app annotation set", func() {
			err := createApplication(appWithTopologyClustersYaml)
			Expect(err).Should(BeNil())

			err = updateAppsWithTopologyPolicy(context.Background(), k8sClient)
			Expect(err).Should(BeNil())

			err, result := hasUpdateTimeAnnotation("basic-topology", "default")
			Expect(err).Should(BeNil())
			Expect(result).Should(BeFalse())
		})
	})

	var _ = When("app has topology policy with clusterLabelSelector", func() {
		It("app should have update app annotation set", func() {
			err := createApplication(appWithTopologyClusterLabelSelectorYaml)
			Expect(err).Should(BeNil())

			err = updateAppsWithTopologyPolicy(context.Background(), k8sClient)
			Expect(err).Should(BeNil())

			err, result := hasUpdateTimeAnnotation("region-selector", "vela-system")
			Expect(err).Should(BeNil())
			Expect(result).Should(BeTrue())
		})
	})

	var _ = When("app has topology policy with empty clusterLabelSelector", func() {
		It("app should have update app annotation set", func() {
			err := createApplication(appWithEmptyTopologyClusterLabelSelectorYaml)
			Expect(err).Should(BeNil())

			err = updateAppsWithTopologyPolicy(context.Background(), k8sClient)
			Expect(err).Should(BeNil())

			err, result := hasUpdateTimeAnnotation("empty-cluster-selector", "default")
			Expect(err).Should(BeNil())
			Expect(result).Should(BeTrue())
		})
	})

})

func createApplication(appYaml string) error {
	app := v1beta1.Application{}
	if err := yaml.Unmarshal([]byte(appYaml), &app); err != nil {
		return fmt.Errorf("unmarshal error for yaml %s: %w", appYaml, err)
	}
	if err := k8sClient.Create(context.Background(), &app); err != nil {
		return fmt.Errorf("error in creating app %v: %w", app, err)
	}
	return nil
}

func hasUpdateTimeAnnotation(name, namespace string) (error, bool) {
	app := &v1beta1.Application{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: name, Namespace: namespace}, app); err != nil {
		return fmt.Errorf("error in getting application %s in namespace %s: %w", name, namespace, err), false
	}
	annotations := app.GetAnnotations()
	if annotations != nil && annotations[ClusterUpdateTime] != "" {
		return nil, true
	}
	return nil, false
}
