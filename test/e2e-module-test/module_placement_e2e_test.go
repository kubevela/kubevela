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

package controllers_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	veltypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// The fixture at testdata/module/placement-store is the module these specs
// publish. Its name and version are fixed by _module.cue, and the owned
// Application name follows the render service (module-<name> in vela-system).
const (
	placementModuleName     = "placement-store"
	placementModuleVersion  = "1.0.0"
	placementFixtureRelPath = "test/e2e-module-test/testdata/module/placement-store"
	placementInstallAppName = "module-" + placementModuleName + "-deploy"
	placementOwnedAppName   = "module-" + placementModuleName

	placementCMDefaults     = "placement-store-defaults"
	placementCMAdmission    = "placement-store-admission"
	placementCMKeepRuntime  = "placement-store-keep-runtime"
	placementCMV1Line       = "placement-store-v1-line"
	placementCMV1Controller = "placement-store-v1-controller"
	placementCMV2Migration  = "placement-store-v2-migration"
	placementCMSkipped      = "placement-store-skipped"
	placementCMNested       = "placement-store-nested"
	placementCDV1           = "placement-store-v1-note"
	placementCDV2           = "placement-store-v2-note"

	placementTierRuntime   = "placement-store-aux"
	placementTierHub       = "placement-store-aux-hub"
	placementTierV1Runtime = "placement-store-v1-aux"
	placementTierV1Hub     = "placement-store-v1-aux-hub"
	placementTierV1Defs    = "placement-store-v1-defs"
	placementTierV2Hub     = "placement-store-v2-aux-hub"
	placementTierV2Defs    = "placement-store-v2-defs"
)

// placementRejectNoteYAML is the smallest definition a dry-run publish will
// parse. These specs never install it.
const placementRejectNoteYAML = `apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: note
spec:
  workload:
    definition:
      apiVersion: v1
      kind: ConfigMap
`

var _ = Describe("Module placement", Serial, func() {
	var repoRoot string

	BeforeEach(func() {
		repoRoot = modulePublishRepoRoot()
	})

	// PackageModule runs before the registry is resolved, so a positional
	// reference and --dry-run reject a bad tree without a cluster write and
	// without a push.
	DescribeTable("rejects empty and invalid placement before anything is published",
		func(prepare func(dir string), wants []string) {
			dir := writePlacementModule(nil)
			prepare(dir)
			out, err := runVelaCommand(repoRoot, "module", "publish", dir, "oci://registry.example.com/modules", "--dry-run")
			Expect(err).To(HaveOccurred(), "expected publish to fail\noutput:\n%s", out)
			for _, want := range wants {
				Expect(out).To(ContainSubstring(want), "output:\n%s", out)
			}
		},
		Entry("empty module field", func(dir string) {
			writePlacementFile(dir, "_module.cue", "module: \"reject-place\"\nversion: \"1.0.0\"\nplacement: \"\"\n")
		}, []string{"_module.cue", "empty"}),
		Entry("invalid module field", func(dir string) {
			writePlacementFile(dir, "_module.cue", "module: \"reject-place\"\nversion: \"1.0.0\"\nplacement: \"spoke\"\n")
		}, []string{"_module.cue", "invalid", "spoke"}),
		Entry("empty line field", func(dir string) {
			writePlacementFile(dir, "v1/_version.cue", "apiVersion: \"v1\"\nplacement: \"\"\n")
		}, []string{"v1/_version.cue", "empty"}),
		Entry("empty object annotation", func(dir string) {
			writePlacementFile(dir, "auxiliary/blank.yaml", `apiVersion: v1
kind: ConfigMap
metadata:
  name: blank
  annotations:
    resource.oam.dev/placement: ""
`)
		}, []string{"auxiliary/blank.yaml", "empty"}),
		Entry("invalid object annotation", func(dir string) {
			writePlacementFile(dir, "auxiliary/bad.yaml", `apiVersion: v1
kind: ConfigMap
metadata:
  name: bad
  annotations:
    resource.oam.dev/placement: spoke
`)
		}, []string{"auxiliary/bad.yaml", "invalid", "spoke"}),
		Entry("empty definition annotation", func(dir string) {
			writePlacementFile(dir, "v1/definitions/note.yaml", `apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: note
  annotations:
    resource.oam.dev/placement: ""
spec:
  workload:
    definition:
      apiVersion: v1
      kind: ConfigMap
`)
		}, []string{"v1/definitions/note.yaml", "empty"}),
	)

	It("accepts a module that leaves placement unset", func() {
		dir := writePlacementModule(nil)
		out := runVelaCommandSucceed(repoRoot, "module", "publish", dir, "oci://registry.example.com/modules", "--dry-run")
		Expect(out).To(ContainSubstring("Would publish"))
		Expect(out).NotTo(ContainSubstring("Warning:"))
	})

	It("accepts a placement marker on a definition and does not warn", func() {
		dir := writePlacementModule(map[string]string{
			"v1/definitions/note.yaml": `apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: note
  annotations:
    resource.oam.dev/placement: hub
spec:
  workload:
    definition:
      apiVersion: v1
      kind: ConfigMap
`,
		})
		out := runVelaCommandSucceed(repoRoot, "module", "publish", dir, "oci://registry.example.com/modules", "--dry-run")
		Expect(out).To(ContainSubstring("Would publish"))
		Expect(out).NotTo(ContainSubstring("Warning:"))
	})

	It("publishes a module and installs hub and runtime objects on the local cluster", func() {
		ctx := context.Background()
		testNamespace := randomNamespaceName("placement")
		registryName := randomNamespaceName("placement-reg")

		By("Removing a leftover owned Application from an earlier run")
		deletePlacementApp(ctx, placementOwnedAppName, veltypes.DefaultKubeVelaNS)

		By("Creating the test namespace: " + testNamespace)
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, ns, client.PropagationPolicy(metav1.DeletePropagationForeground))
		})

		By("Applying the in-cluster OCI registry and waiting until it answers")
		Expect(createOCIRegistry(ctx, k8sClient, testNamespace)).To(Succeed())
		waitForOCIRegistryDeploymentAvailable(ctx, testNamespace)
		registryURL := moduleE2ERegistryURL(ctx, testNamespace)
		registryBase, err := registryHTTPBase(registryURL)
		Expect(err).NotTo(HaveOccurred(), "registry URL: %s", registryURL)
		waitForOCIRegistryReachable(registryBase + "/v2/")

		By("Publishing the fixture, including the placement warnings")
		out := runVelaCommandSucceed(repoRoot, "module", "publish", placementFixtureRelPath, registryURL)
		Expect(out).To(ContainSubstring(placementModuleName + ":" + placementModuleVersion))
		Expect(out).To(ContainSubstring("auxiliary/skipped is not a recognized folder"))
		Expect(out).To(ContainSubstring("auxiliary/hub/nested is not read"))
		Expect(out).To(ContainSubstring("auxiliary/hub/20-keep-runtime.yaml is in hub/"))
		Expect(out).To(ContainSubstring("the annotation wins"))

		By("Registering that registry for the controller")
		out = runVelaCommandSucceed(repoRoot, "module", "registry", "add", registryName, registryURL, "--type", "oci")
		DeferCleanup(func() {
			_, _ = runVelaCommand(repoRoot, "module", "registry", "delete", registryName)
		})
		Expect(out).To(ContainSubstring(registryName))

		// vela module deploy cannot pass placement. Both classes are asked for
		// the local cluster, so the two deploy steps can succeed without a
		// second cluster. Which component holds each object is the split.
		By("Installing the module with placement.clusters set to the local cluster")
		props, err := json.Marshal(map[string]any{
			"module":   placementModuleName,
			"registry": registryName,
			"version":  placementModuleVersion,
			"placement": map[string]any{
				"clusters": []string{pkgmulticluster.Local},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		installer := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: placementInstallAppName, Namespace: testNamespace},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{{
					Name:       placementModuleName,
					Type:       "module",
					Properties: &runtime.RawExtension{Raw: props},
				}},
			},
		}
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: placementInstallAppName, Namespace: testNamespace}})
			_ = k8sClient.Delete(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: placementOwnedAppName, Namespace: veltypes.DefaultKubeVelaNS}})
		})
		Expect(k8sClient.Create(ctx, installer)).To(Succeed())

		By("Waiting until the owned Application is running")
		var owned v1beta1.Application
		Eventually(func(g Gomega) {
			var parent v1beta1.Application
			g.Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: placementInstallAppName, Namespace: testNamespace}, &parent)).To(Succeed())
			err := k8sClient.Get(ctx, k8stypes.NamespacedName{Name: placementOwnedAppName, Namespace: veltypes.DefaultKubeVelaNS}, &owned)
			g.Expect(err).To(Succeed(), "installer phase %q", parent.Status.Phase)
			workflowMessage := ""
			if owned.Status.Workflow != nil {
				workflowMessage = owned.Status.Workflow.Message
			}
			g.Expect(owned.Status.Phase).To(Equal(common.ApplicationRunning), "workflow: %s", workflowMessage)
			g.Expect(owned.Status.Services).NotTo(BeEmpty())
			for _, svc := range owned.Status.Services {
				g.Expect(svc.Healthy).To(BeTrue(), "tier %q: %s", svc.Name, svc.Message)
			}
		}, 5*time.Minute, 2*time.Second).Should(Succeed())

		Expect(owned.Labels[veltypes.LabelDefinitionModule]).To(Equal(placementModuleName))
		Expect(owned.Annotations[veltypes.AnnoDefinitionModuleVersion]).To(Equal(placementModuleVersion))

		By("Asserting the two topology policies both select the local cluster")
		Expect(placementPolicy(owned, "module-hub").Clusters).To(Equal([]string{pkgmulticluster.Local}))
		Expect(placementPolicy(owned, "module-runtime").Clusters).To(Equal([]string{pkgmulticluster.Local}))
		Expect(placementPolicy(owned, "module-hub-override").Selector).To(ConsistOf(
			placementTierHub,
			placementTierV1Hub,
			placementTierV1Defs,
			placementTierV2Hub,
			placementTierV2Defs,
		))
		Expect(placementPolicy(owned, "module-runtime-override").Selector).To(ConsistOf(
			placementTierRuntime,
			placementTierV1Runtime,
		))

		By("Asserting both deploy steps are on the owned Application")
		Expect(owned.Spec.Workflow).NotTo(BeNil())
		var steps []string
		for _, step := range owned.Spec.Workflow.Steps {
			steps = append(steps, step.Name)
		}
		Expect(steps).To(ConsistOf("deploy-module-hub", "deploy-module-runtime"))

		By("Asserting each object landed in the tier its placement selects")
		Expect(componentHolding(&owned, placementCMDefaults)).To(Equal(placementTierRuntime))
		Expect(componentHolding(&owned, placementCMKeepRuntime)).To(Equal(placementTierRuntime))
		Expect(componentHolding(&owned, placementCMAdmission)).To(Equal(placementTierHub))
		Expect(componentHolding(&owned, placementCMV1Line)).To(Equal(placementTierV1Runtime))
		Expect(componentHolding(&owned, placementCMV1Controller)).To(Equal(placementTierV1Hub))
		Expect(componentHolding(&owned, placementCMV2Migration)).To(Equal(placementTierV2Hub))
		Expect(componentHolding(&owned, placementCDV1)).To(Equal(placementTierV1Defs))
		Expect(componentHolding(&owned, placementCDV2)).To(Equal(placementTierV2Defs))

		raw, err := json.Marshal(owned.Spec)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(raw)).NotTo(ContainSubstring(placementCMSkipped))
		Expect(string(raw)).NotTo(ContainSubstring(placementCMNested))

		By("Asserting the objects exist, with the placement annotation the rule produced")
		expectPlacementConfigMap(ctx, testNamespace, placementCMDefaults, "")
		expectPlacementConfigMap(ctx, testNamespace, placementCMKeepRuntime, oam.PlacementRuntime)
		expectPlacementConfigMap(ctx, testNamespace, placementCMAdmission, oam.PlacementHub)
		expectPlacementConfigMap(ctx, testNamespace, placementCMV1Line, "")
		expectPlacementConfigMap(ctx, testNamespace, placementCMV1Controller, oam.PlacementHub)
		expectPlacementConfigMap(ctx, testNamespace, placementCMV2Migration, "")
		expectPlacementConfigMapAbsent(ctx, testNamespace, placementCMSkipped)
		expectPlacementConfigMapAbsent(ctx, testNamespace, placementCMNested)

		By("Asserting definitions installed on the hub, keeping a marker that does not move them")
		var v1Note v1beta1.ComponentDefinition
		Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: placementCDV1, Namespace: testNamespace}, &v1Note)).To(Succeed())
		Expect(v1Note.Annotations[oam.AnnotationPlacement]).To(Equal(oam.PlacementRuntime))
		var v2Note v1beta1.ComponentDefinition
		Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: placementCDV2, Namespace: testNamespace}, &v2Note)).To(Succeed())
		Expect(v2Note.Annotations).NotTo(HaveKey(oam.AnnotationPlacement))
	})
})

func deletePlacementApp(ctx context.Context, name, namespace string) {
	app := &v1beta1.Application{}
	key := k8stypes.NamespacedName{Name: name, Namespace: namespace}
	err := k8sClient.Get(ctx, key, app)
	if k8serrors.IsNotFound(err) {
		return
	}
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient.Delete(ctx, app)).To(Succeed())
	Eventually(func() bool {
		return k8serrors.IsNotFound(k8sClient.Get(ctx, key, &v1beta1.Application{}))
	}, 90*time.Second, 2*time.Second).Should(BeTrue())
}

func expectPlacementConfigMap(ctx context.Context, namespace, name, placement string) {
	var cm corev1.ConfigMap
	Expect(k8sClient.Get(ctx, k8stypes.NamespacedName{Name: name, Namespace: namespace}, &cm)).To(Succeed())
	if placement == "" {
		Expect(cm.Annotations).NotTo(HaveKey(oam.AnnotationPlacement), "configmap %s", name)
		return
	}
	Expect(cm.Annotations[oam.AnnotationPlacement]).To(Equal(placement), "configmap %s", name)
}

func expectPlacementConfigMapAbsent(ctx context.Context, namespace, name string) {
	err := k8sClient.Get(ctx, k8stypes.NamespacedName{Name: name, Namespace: namespace}, &corev1.ConfigMap{})
	Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "configmap %s: %v", name, err)
}

type placementPolicyView struct {
	Clusters []string `json:"clusters"`
	Selector []string `json:"selector"`
}

func placementPolicy(app v1beta1.Application, name string) placementPolicyView {
	for _, policy := range app.Spec.Policies {
		if policy.Name != name {
			continue
		}
		raw := rawExtensionBytes(policy.Properties)
		Expect(raw).NotTo(BeEmpty(), "policy %s has no properties", name)
		var view placementPolicyView
		Expect(json.Unmarshal(raw, &view)).To(Succeed(), "policy %s: %s", name, raw)
		return view
	}
	Fail("policy " + name + " not found")
	return placementPolicyView{}
}

func componentHolding(app *v1beta1.Application, objectName string) string {
	var found string
	for _, component := range app.Spec.Components {
		raw, err := json.Marshal(component)
		Expect(err).NotTo(HaveOccurred())
		if strings.Contains(string(raw), objectName) {
			Expect(found).To(BeEmpty(), "object %s is in both %s and %s", objectName, found, component.Name)
			found = component.Name
		}
	}
	Expect(found).NotTo(BeEmpty(), "object %s is not on the owned Application", objectName)
	return found
}

func rawExtensionBytes(ext *runtime.RawExtension) []byte {
	if ext == nil {
		return nil
	}
	if len(ext.Raw) > 0 {
		return ext.Raw
	}
	if ext.Object == nil {
		return nil
	}
	raw, err := json.Marshal(ext.Object)
	Expect(err).NotTo(HaveOccurred())
	return raw
}

func writePlacementModule(overrides map[string]string) string {
	dir, err := os.MkdirTemp("", "placement-e2e-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() {
		Expect(os.RemoveAll(dir)).To(Succeed())
	})
	files := map[string]string{
		"_module.cue":              "module: \"reject-place\"\nversion: \"1.0.0\"\n",
		"v1/_version.cue":          "apiVersion: \"v1\"\n",
		"v1/definitions/note.yaml": placementRejectNoteYAML,
	}
	for rel, body := range overrides {
		files[rel] = body
	}
	for rel, body := range files {
		writePlacementFile(dir, rel, body)
	}
	return dir
}

func writePlacementFile(dir, rel, body string) {
	full := filepath.Join(dir, filepath.FromSlash(rel))
	Expect(os.MkdirAll(filepath.Dir(full), 0o750)).To(Succeed())
	Expect(os.WriteFile(full, []byte(body), 0o600)).To(Succeed())
}
