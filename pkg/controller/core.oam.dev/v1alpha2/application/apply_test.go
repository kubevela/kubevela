package application

import (
	"context"
	"math/rand"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test Application apply", func() {
	var handler appHandler
	app := &v1alpha2.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "core.oam.dev/v1alpha2",
		},
	}
	var appConfig *v1alpha2.ApplicationConfiguration
	var namespaceName string
	var componentName string
	var ns corev1.Namespace

	BeforeEach(func() {
		ctx := context.TODO()
		namespaceName = "apply-test-" + strconv.Itoa(rand.Intn(1000))
		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespaceName,
			},
		}
		app.Namespace = namespaceName
		app.Spec = v1alpha2.ApplicationSpec{
			Components: []v1alpha2.ApplicationComponent{{
				WorkloadType: "webservice",
				Name:         "express-server",
				Scopes:       map[string]string{"healthscopes.core.oam.dev": "myapp-default-health"},
				Settings: runtime.RawExtension{
					Raw: []byte(`{"image": "oamdev/testapp:v1", "cmd": ["node", "server.js"]}`),
				},
				Traits: []v1alpha2.ApplicationTrait{{
					Name: "route",
					Properties: runtime.RawExtension{
						Raw: []byte(`{"domain": "example.com", "http":{"/": 8080}}`),
					},
				},
				},
			}},
		}
		handler = appHandler{
			r:      reconciler,
			app:    app,
			logger: reconciler.Log.WithValues("application", "unit-test"),
		}

		By("Create the Namespace for test")
		Expect(k8sClient.Create(ctx, &ns)).Should(Succeed())

		appConfig = &v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      app.Name,
				Namespace: namespaceName,
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{
					{
						ComponentName: componentName,
					},
				},
			},
		}
	})

	AfterEach(func() {
		By("[TEST] Clean up resources after an integration test")
		Expect(k8sClient.Delete(context.TODO(), &ns)).Should(Succeed())
	})

	It("Test creating applicationConfiguration revision", func() {
		ctx := context.TODO()

		By("[TEST] Test application without AC revision")
		app.Name = "test-revision"
		Expect(handler.r.Create(ctx, app)).NotTo(HaveOccurred())
		// Test create or update
		err := handler.createOrUpdateAppConfig(ctx, appConfig.DeepCopy())
		Expect(err).ToNot(HaveOccurred())
		// verify
		curApp := &v1alpha2.Application{}
		Eventually(
			func() error {
				return handler.r.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: app.Name},
					curApp)
			},
			time.Second*10, time.Millisecond*500).Should(BeNil())

		By("Verify that the application status has the lastRevision name ")
		Expect(curApp.Status.LatestRevision.Revision).Should(BeEquivalentTo(1))
		Expect(curApp.Status.LatestRevision.Name).Should(Equal(utils.ConstructRevisionName(app.Name, 1)))
		curAC := &v1alpha2.ApplicationConfiguration{}
		Expect(handler.r.Get(ctx,
			types.NamespacedName{Namespace: ns.Name, Name: utils.ConstructRevisionName(app.Name, 1)},
			curAC)).NotTo(HaveOccurred())
		// check that the annotation/labels are correctly applied
		Expect(curAC.GetLabels()[oam.LabelAppConfigHash]).ShouldNot(BeEmpty())
		hashValue := curAC.GetLabels()[oam.LabelAppConfigHash]
		Expect(hashValue).ShouldNot(BeEmpty())
		Expect(curApp.Status.LatestRevision.RevisionHash).Should(Equal(hashValue))

		// TODO: verify that label and annotation change will be passed down

		By("[TEST] apply the same appConfig mimic application controller, should do nothing")
		// this should not lead to a new AC
		err = handler.createOrUpdateAppConfig(ctx, appConfig.DeepCopy())
		Expect(err).ToNot(HaveOccurred())
		// verify the app latest revision is not changed
		Eventually(
			func() error {
				return handler.r.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: app.Name},
					curApp)
			},
			time.Second*10, time.Millisecond*500).Should(BeNil())

		By("Verify that the lastest revision does not change")
		Expect(curApp.Status.LatestRevision.Revision).Should(BeEquivalentTo(1))
		Expect(curApp.Status.LatestRevision.Name).Should(Equal(utils.ConstructRevisionName(app.Name, 1)))
		Expect(curApp.Status.LatestRevision.RevisionHash).Should(Equal(hashValue))
		Expect(handler.r.Get(ctx,
			types.NamespacedName{Namespace: ns.Name, Name: curApp.Status.LatestRevision.Name},
			curAC)).NotTo(HaveOccurred())

		By("[TEST] Modify the applicationConfiguration mimic AC controller, should only update")
		// update the status of the AC which is expected after AC controller takes over
		curAC.Status.SetConditions(readyCondition("newType"))
		Expect(handler.r.Status().Update(ctx, curAC)).NotTo(HaveOccurred())
		// set the new AppConfig annotation as false AC controller would do
		cl := make(map[string]string)
		cl[oam.AnnotationAppRollout] = strconv.FormatBool(false)
		curAC.SetAnnotations(cl)
		Expect(handler.r.Update(ctx, curAC)).NotTo(HaveOccurred())
		// this should not lead to a new AC
		err = handler.createOrUpdateAppConfig(ctx, curAC.DeepCopy())
		Expect(err).ToNot(HaveOccurred())
		// verify the app latest revision is not changed
		Eventually(
			func() error {
				return handler.r.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: app.Name},
					curApp)
			},
			time.Second*10, time.Millisecond*500).Should(BeNil())

		By("Verify that the lastest revision does not change")
		Expect(curApp.Status.LatestRevision.Revision).Should(BeEquivalentTo(1))
		Expect(curApp.Status.LatestRevision.Name).Should(Equal(utils.ConstructRevisionName(app.Name, 1)))
		Expect(curApp.Status.LatestRevision.RevisionHash).Should(Equal(hashValue))
		Expect(handler.r.Get(ctx,
			types.NamespacedName{Namespace: ns.Name, Name: curApp.Status.LatestRevision.Name},
			curAC)).NotTo(HaveOccurred())
		// check that the new app annotation is false
		Expect(curAC.GetAnnotations()[oam.AnnotationAppRollout]).Should(Equal(strconv.FormatBool(false)))
		Expect(curAC.GetLabels()[oam.LabelAppConfigHash]).Should(Equal(hashValue))
		Expect(curAC.GetCondition("newType").Status).Should(BeEquivalentTo(corev1.ConditionTrue))
		// check that no new appConfig created
		Expect(handler.r.Get(ctx, types.NamespacedName{Namespace: ns.Name,
			Name: utils.ConstructRevisionName(app.Name, 2)}, curAC)).Should(&oamutil.NotFoundMatcher{})

		By("[TEST] Modify the applicationConfiguration spec, should lead to a new AC")
		// update the spec of the AC which should lead to a new AC being created
		appConfig.Spec.Components[0].Traits = []v1alpha2.ComponentTrait{
			{
				Trait: runtime.RawExtension{
					Object: &v1alpha1.MetricsTrait{
						TypeMeta: metav1.TypeMeta{
							Kind:       "MetricsTrait",
							APIVersion: "standard.oam.dev/v1alpha1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      app.Name,
							Namespace: namespaceName,
						},
					},
				},
			},
		}
		// this should lead to a new AC
		err = handler.createOrUpdateAppConfig(ctx, appConfig)
		Expect(err).ToNot(HaveOccurred())
		// verify the app latest revision is not changed
		Eventually(
			func() error {
				return handler.r.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: app.Name},
					curApp)
			},
			time.Second*10, time.Millisecond*500).Should(BeNil())

		By("Verify that the lastest revision is advanced")
		Expect(curApp.Status.LatestRevision.Revision).Should(BeEquivalentTo(2))
		Expect(curApp.Status.LatestRevision.Name).Should(Equal(app.Name + "-v2"))
		Expect(curApp.Status.LatestRevision.RevisionHash).ShouldNot(Equal(hashValue))

		// check that the new app annotation exist and the hash value has changed
		Expect(handler.r.Get(ctx,
			types.NamespacedName{Namespace: ns.Name, Name: curApp.Status.LatestRevision.Name},
			curAC)).NotTo(HaveOccurred())
		Expect(curAC.GetLabels()[oam.LabelAppConfigHash]).ShouldNot(BeEmpty())
		Expect(curAC.GetLabels()[oam.LabelAppConfigHash]).ShouldNot(Equal(hashValue))
		// check that no more new appConfig created
		Expect(handler.r.Get(ctx, types.NamespacedName{Namespace: ns.Name, Name: app.Name + "-v3"},
			curAC)).Should(&oamutil.NotFoundMatcher{})
	})

	It("Test update or create component", func() {
		ctx := context.TODO()
		By("[TEST] Setting up the testing environment")
		imageV1 := "wordpress:4.6.1-apache"
		imageV2 := "wordpress:4.6.2-apache"
		cwV1 := v1alpha2.ContainerizedWorkload{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ContainerizedWorkload",
				APIVersion: "core.oam.dev/v1alpha2",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespaceName,
			},
			Spec: v1alpha2.ContainerizedWorkloadSpec{
				Containers: []v1alpha2.Container{
					{
						Name:  "wordpress",
						Image: imageV1,
						Ports: []v1alpha2.ContainerPort{
							{
								Name: "wordpress",
								Port: 80,
							},
						},
					},
				},
			},
		}
		component := &v1alpha2.Component{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Component",
				APIVersion: "core.oam.dev/v1alpha2",
			}, ObjectMeta: metav1.ObjectMeta{
				Name:      "myweb",
				Namespace: namespaceName,
				Labels:    map[string]string{"application.oam.dev": "test"},
			},
			Spec: v1alpha2.ComponentSpec{
				Workload: runtime.RawExtension{
					Object: &cwV1,
				},
			}}

		By("[TEST] Creating a component the first time")
		// take a copy so the component's workload still uses object instead of raw data
		// just like the way we use it in prod. The raw data will be filled by the k8s for some reason.
		revision, newRevision, err := handler.createOrUpdateComponent(ctx, component.DeepCopy())
		By("verify that the revision is the set correctly and newRevision is true")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(newRevision).Should(BeTrue())
		// verify the revision actually contains the right component
		Expect(utils.CompareWithRevision(ctx, handler.r, logging.NewLogrLogger(handler.logger), component.GetName(),
			component.GetNamespace(), revision, &component.Spec)).Should(BeTrue())
		preRevision := revision

		By("[TEST] update the component without any changes (mimic reconcile behavior)")
		revision, newRevision, err = handler.createOrUpdateComponent(ctx, component.DeepCopy())
		By("verify that the revision is the same and newRevision is false")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(newRevision).Should(BeFalse())
		Expect(revision).Should(BeIdenticalTo(preRevision))

		By("[TEST] update the component")
		// modify the component spec through object
		cwV2 := cwV1.DeepCopy()
		cwV2.Spec.Containers[0].Image = imageV2
		component.Spec.Workload.Object = cwV2
		revision, newRevision, err = handler.createOrUpdateComponent(ctx, component.DeepCopy())
		By("verify that the revision is changed and newRevision is true")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(newRevision).Should(BeTrue())
		Expect(revision).ShouldNot(BeIdenticalTo(preRevision))
		Expect(utils.CompareWithRevision(ctx, handler.r, logging.NewLogrLogger(handler.logger), component.GetName(),
			component.GetNamespace(), revision, &component.Spec)).Should(BeTrue())
		// revision increased
		Expect(strings.Compare(revision, preRevision) > 0).Should(BeTrue())
	})

})
