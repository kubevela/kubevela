package application

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

var _ = Describe("Test Application workflow generator", func() {
	var namespaceName string
	var ns corev1.Namespace
	var ctx context.Context

	BeforeEach(func() {
		namespaceName = "generate-test-" + strconv.Itoa(time.Now().Second()) + "-" + strconv.Itoa(time.Now().Nanosecond())
		ctx = context.WithValue(context.TODO(), util.AppDefinitionNamespace, namespaceName)
		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespaceName,
			},
		}
		By("Create the Namespace for test")
		Expect(k8sClient.Create(ctx, &ns)).Should(Succeed())

		healthComponentDef := &oamcore.ComponentDefinition{}
		hCDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(hCDefJson, healthComponentDef)).Should(BeNil())
		healthComponentDef.Name = "worker-with-health"
		healthComponentDef.Namespace = namespaceName

		By("Create the Component Definition for test")
		Expect(k8sClient.Create(ctx, healthComponentDef)).Should(Succeed())
	})

	AfterEach(func() {
		By("[TEST] Clean up resources after an integration test")
		Expect(k8sClient.Delete(context.TODO(), &ns)).Should(Succeed())
	})

	It("Test generate application workflow with inputs and outputs", func() {
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-input-output",
				Namespace: namespaceName,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
						Inputs: common.StepInputs{
							{
								From:         "message",
								ParameterKey: "properties.enemies",
							},
							{
								From:         "message",
								ParameterKey: "properties.lives",
							},
						},
					},
					{
						Name:       "myweb2",
						Type:       "worker-with-health",
						Properties: runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
						Outputs: common.StepOutputs{
							{
								Name:      "message",
								ExportKey: "output.status.conditions[0].message+\",\"+outputs.gameconfig.data.lives",
							},
						},
					},
				},
			},
		}
		af, err := appParser.GenerateAppFile(ctx, app)
		Expect(err).Should(BeNil())
		_, err = af.PrepareWorkflowAndPolicy()
		Expect(err).Should(BeNil())
		appRev := &v1beta1.ApplicationRevision{}
		dm, err := discoverymapper.New(cfg)
		Expect(err).To(BeNil())
		pd, err := packages.NewPackageDiscover(cfg)
		Expect(err).To(BeNil())

		handler := &AppHandler{
			r:      reconciler,
			app:    app,
			parser: appParser,
		}

		taskRunner, err := handler.GenerateApplicationSteps(ctx, app, appParser, af, appRev, k8sClient, dm, pd)
		Expect(err).To(BeNil())
		Expect(len(taskRunner)).Should(BeEquivalentTo(2))
		Expect(taskRunner[0].Name()).Should(BeEquivalentTo("myweb1"))
		Expect(taskRunner[1].Name()).Should(BeEquivalentTo("myweb2"))
	})

	It("Test generate application workflow without inputs and outputs", func() {
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-without-input-output",
				Namespace: namespaceName,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
					{
						Name:       "myweb2",
						Type:       "worker-with-health",
						Properties: runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
				},
			},
		}
		af, err := appParser.GenerateAppFile(ctx, app)
		Expect(err).Should(BeNil())
		_, err = af.PrepareWorkflowAndPolicy()
		Expect(err).Should(BeNil())
		appRev := &v1beta1.ApplicationRevision{}
		dm, err := discoverymapper.New(cfg)
		Expect(err).To(BeNil())
		pd, err := packages.NewPackageDiscover(cfg)
		Expect(err).To(BeNil())

		handler := &AppHandler{
			r:      reconciler,
			app:    app,
			parser: appParser,
		}

		taskRunner, err := handler.GenerateApplicationSteps(ctx, app, appParser, af, appRev, k8sClient, dm, pd)
		Expect(err).To(BeNil())
		Expect(len(taskRunner)).Should(BeEquivalentTo(2))
		Expect(taskRunner[0].Name()).Should(BeEquivalentTo("myweb1"))
		Expect(taskRunner[1].Name()).Should(BeEquivalentTo("myweb2"))
	})
})
