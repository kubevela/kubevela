/*
Copyright 2021 The KubeVela Authors.

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

package application

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/yaml"

	"github.com/kubevela/workflow/pkg/cue/packages"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var testScheme = runtime.NewScheme()
var decoder *admission.Decoder
var dm discoverymapper.DiscoveryMapper
var pd *packages.PackageDiscover
var ctx = context.Background()
var handler *ValidatingHandler

func TestAPIs(t *testing.T) {

	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func(done Done) {
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))
	By("bootstrapping test environment")
	var yamlPath string
	if _, set := os.LookupEnv("COMPATIBILITY_TEST"); set {
		yamlPath = "../../../../../test/compatibility-test/testdata"
	} else {
		yamlPath = filepath.Join("../../../../..", "charts", "vela-core", "crds")
	}
	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
		CRDDirectoryPaths:        []string{yamlPath},
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = v1alpha2.SchemeBuilder.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())

	err = v1beta1.SchemeBuilder.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())

	err = scheme.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())
	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	dm, err = discoverymapper.New(cfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(dm).ToNot(BeNil())

	pd, err = packages.NewPackageDiscover(cfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(pd).ToNot(BeNil())

	handler = &ValidatingHandler{
		dm: dm,
		pd: pd,
	}

	decoder, err = admission.NewDecoder(testScheme)
	Expect(err).Should(BeNil())
	Expect(decoder).ToNot(BeNil())

	ctx := context.Background()
	ns := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vela-system"}}
	Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
	wd := &v1beta1.ComponentDefinition{}
	wDDefJson, _ := yaml.YAMLToJSON([]byte(cDDefYaml))
	Expect(json.Unmarshal(wDDefJson, wd)).Should(BeNil())
	Expect(k8sClient.Create(ctx, wd)).Should(BeNil())
	td := &v1beta1.TraitDefinition{}
	tDDefJson, _ := yaml.YAMLToJSON([]byte(tDDefYaml))
	Expect(json.Unmarshal(tDDefJson, td)).Should(BeNil())
	Expect(k8sClient.Create(ctx, td)).Should(BeNil())

	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

const (
	cDDefYaml = `
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: worker
  namespace: vela-system
  annotations:
    definition.oam.dev/description: "Long-running scalable backend worker without network endpoint"
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  extension:
    template: |
      output: {
      	apiVersion: "apps/v1"
      	kind:       "Deployment"
      	spec: {
      		selector: matchLabels: {
      			"app.oam.dev/component": context.name
      		}

      		template: {
      			metadata: labels: {
      				"app.oam.dev/component": context.name
      			}

      			spec: {
      				containers: [{
      					name:  context.name
      					image: parameter.image

      					if parameter["cmd"] != _|_ {
      						command: parameter.cmd
      					}
      				}]
      			}
      		}

      		selector:
      			matchLabels:
      				"app.oam.dev/component": context.name
      	}
      }

      parameter: {
      	// +usage=Which image would you like to use for your service
      	// +short=i
      	image: string

      	cmd?: [...string]
      }`

	tDDefYaml = `
# Code generated by KubeVela templates. DO NOT EDIT. Please edit the original cue file.
# Definition source cue file: vela-templates/definitions/internal/scaler.cue
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: Manually scale K8s pod for your workload which follows the pod spec in path 'spec.template'.
  name: scaler
  namespace: vela-system
spec:
  appliesToWorkloads:
    - deployments.apps
    - statefulsets.apps
  podDisruptive: false
  schematic:
    cue:
      template: |
        parameter: {
        	// +usage=Specify the number of workload
        	replicas: *1 | int
        }
        // +patchStrategy=retainKeys
        patch: spec: replicas: parameter.replicas
`
)
