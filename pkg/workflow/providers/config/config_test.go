/*
Copyright 2023 The KubeVela Authors.

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

package config

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kubevela/workflow/pkg/mock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/config"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	oamprovidertypes "github.com/oam-dev/kubevela/pkg/workflow/providers/types"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var factory config.Factory

func TestProvider(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Test Config Suite")
}

var _ = BeforeSuite(func() {
	By("Bootstrapping test environment")
	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
		UseExistingCluster:       ptr.To(false),
		// the Config and ConfigTemplate CRDs, so the steps take the CR path; no controller
		// runs here, so a Config's status is whatever a spec writes into it
		CRDDirectoryPaths: []string{"../../../../charts/vela-core/crds"},
	}
	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())
	// the shared scheme: the migration library the adopt-first step uses reads core.oam.dev
	// objects (the distribution Application) as well as config.oam.dev ones
	By("Create the k8s client")
	k8sClient, err = client.New(cfg, client.Options{Scheme: common.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	factory = config.NewConfigFactory(k8sClient)
})

var _ = AfterSuite(func() {
	By("Tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

// recordingDispatcher stands in for the Application controller's resource tracker: it
// notes what the factory applied, then applies it the way the keeper does, stamped with
// the application's labels and behind its ownership check, so an object the application
// does not own is refused here as it is there.
type recordingDispatcher struct {
	kinds   []string
	batches int // dispatcher calls: each is dry-run as a whole before it is written
	deleted []string
}

// deleter stands in for the Application's delete handler: it notes what the step asked to
// delete, then deletes it.
func (d *recordingDispatcher) deleter(ctx context.Context, cli client.Client, _, _ string, obj *unstructured.Unstructured) error {
	d.deleted = append(d.deleted, obj.GetKind()+"/"+obj.GetName())
	return client.IgnoreNotFound(cli.Delete(ctx, obj))
}

// testApp is the Application the dispatcher applies for. It reads ownership from the app
// labels the generator merges onto every dispatched manifest.
type testApp struct{ namespace, name string }

func (a testApp) Kind() string { return "application" }
func (a testApp) Key() string  { return a.namespace + "/" + a.name }
func (a testApp) ControlledBy(obj client.Object) string {
	labels := obj.GetLabels()
	if labels[oam.LabelAppName] == "" || labels[oam.LabelAppNamespace] == "" {
		return ""
	}
	return labels[oam.LabelAppNamespace] + "/" + labels[oam.LabelAppName]
}

var dispatchingApp = testApp{namespace: "default", name: "wf-app"}

func (d *recordingDispatcher) factory(cli client.Client) config.Factory {
	applicator := apply.NewAPIApplicator(cli)
	return config.NewConfigFactoryWithDispatcher(cli, func(ctx context.Context, objects []*unstructured.Unstructured, opts []apply.ApplyOption) error {
		d.batches++
		for _, obj := range objects {
			d.kinds = append(d.kinds, obj.GetKind())
			stampApp(obj)
			// the keeper's order: the caller's options, then the ownership check
			if err := applicator.Apply(ctx, obj, append(opts, apply.MustBeControlledBy(dispatchingApp))...); err != nil {
				return err
			}
		}
		return nil
	})
}

// legacyConfigOf writes a config the way the 1.11 provider did for an Application: a
// legacy Secret carrying the app labels its dispatcher stamped.
func legacyConfigOf(ctx context.Context, name, value string) corev1.Secret {
	return legacyOf(ctx, "test-image-registry", templateContent, name, map[string]interface{}{"registry": value})
}

// legacyOf is legacyConfigOf for any template: the legacy factory writes the Secret (and
// the outputs the template has), and the app labels go on as the 1.11 dispatcher put them.
func legacyOf(ctx context.Context, templateName, templateCUE, name string, properties map[string]interface{}) corev1.Secret {
	template, err := factory.ParseTemplate(ctx, templateName, []byte(templateCUE))
	Expect(err).ToNot(HaveOccurred())
	Expect(factory.CreateOrUpdateConfigTemplate(ctx, "default", template)).Should(Succeed())
	item, err := factory.ParseConfig(ctx, config.NamespacedName{Name: templateName, Namespace: "default"}, config.Metadata{
		NamespacedName: config.NamespacedName{Name: name, Namespace: "default"},
		Properties:     properties,
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(factory.CreateOrUpdateConfig(ctx, item, "default")).Should(Succeed())
	var secret corev1.Secret
	Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: name}, &secret)).Should(Succeed())
	labelForApp(ctx, &secret)
	Expect(secret.OwnerReferences).Should(BeEmpty())
	return secret
}

// stampApp puts the app labels on an object, as the generator's dispatcher does.
func stampApp(obj client.Object) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[oam.LabelAppName], labels[oam.LabelAppNamespace] = dispatchingApp.name, dispatchingApp.namespace
	obj.SetLabels(labels)
}

// labelForApp stamps the app labels on a live object, as the 1.11 dispatcher left them on
// what it applied.
func labelForApp(ctx context.Context, obj client.Object) {
	stampApp(obj)
	Expect(k8sClient.Update(ctx, obj)).Should(Succeed())
}

// expectClaimed checks a config the step made its own: the Config carries the app labels
// the dispatcher stamps, the Secret kept its UID, is the Config's and no longer the app's.
func expectClaimed(ctx context.Context, name string, before corev1.Secret) configv1alpha1.Config {
	var got configv1alpha1.Config
	Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: name}, &got)).Should(Succeed())
	Expect(got.Labels[oam.LabelAppName]).Should(Equal(dispatchingApp.name), "the dispatcher took the migration's Config over and stamped it")
	Expect(got.Labels[oam.LabelAppNamespace]).Should(Equal(dispatchingApp.namespace))
	Expect(config.CreatedByMigration(&got)).Should(BeTrue(), "the migration label stays")
	var after corev1.Secret
	Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: name}, &after)).Should(Succeed())
	Expect(after.UID).Should(Equal(before.UID), "adopted, not recreated")
	Expect(after.OwnerReferences).Should(HaveLen(1))
	Expect(after.OwnerReferences[0].Kind).Should(Equal(configv1alpha1.ConfigKind))
	Expect(after.OwnerReferences[0].Name).Should(Equal(name))
	Expect(after.Labels).ShouldNot(HaveKey(oam.LabelAppName), "released from the application: its GC would otherwise delete the Secret with the retired tracker")
	Expect(after.Labels).ShouldNot(HaveKey(oam.LabelAppNamespace))
	Expect(after.Labels).Should(HaveKeyWithValue(types.LabelConfigCatalog, types.VelaCoreConfig), "the config labels stay")
	return got
}

func depsWith(f config.Factory, act *mock.Action) Deps {
	return Deps{Factory: f, Client: k8sClient, Action: act, AppLabels: map[string]string{oam.LabelAppName: dispatchingApp.name, oam.LabelAppNamespace: dispatchingApp.namespace}}
}

// depsOfApp is depsWith for a step that runs inside an Application: the dispatcher's
// factory and its delete handler.
func depsOfApp(d *recordingDispatcher, act *mock.Action) Deps {
	deps := depsWith(d.factory(k8sClient), act)
	deps.Delete = d.deleter
	return deps
}

// propsOf is a create-config step's properties for a config in default from template, with
// the registry the fixtures expect.
func propsOf(name, template string) CreateConfigProperties {
	return CreateConfigProperties{Name: name, Namespace: "default", Template: template, Config: map[string]interface{}{"registry": "ghcr.io/kubevela"}}
}

func ensureNamespace(ctx context.Context, name string) {
	if err := k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}); err != nil {
		Expect(apierrors.IsAlreadyExists(err)).Should(BeTrue())
	}
}

func setStatus(ctx context.Context, name string, phase configv1alpha1.ConfigPhase, message string) {
	var got configv1alpha1.Config
	Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: name}, &got)).Should(Succeed())
	got.Status.ObservedGeneration = got.Generation
	got.Status.Phase = phase
	if message != "" {
		got.Status.SetConditions(condition.ReconcileError(errors.New(message)))
	} else {
		got.Status.SetConditions(condition.ReconcileSuccess())
	}
	Expect(k8sClient.Status().Update(ctx, &got)).Should(Succeed())
}

var _ = Describe("Test the config provider", func() {
	ctx := context.Background()

	It("registers the four steps", func() {
		providers := GetProviders()
		Expect(providers).To(HaveKey("create"))
		Expect(providers).To(HaveKey("read"))
		Expect(providers).To(HaveKey("list"))
		Expect(providers).To(HaveKey("delete"))
		Expect(providers).To(HaveLen(4))
	})

	It("create fails before writing anything when the template is missing", func() {
		act := &mock.Action{}
		err := Create(ctx, depsWith(factory, act), CreateConfigProperties{
			Name: "hub-kubevela", Namespace: "default", Template: "default/test-image-registry",
			Config: map[string]interface{}{"registry": "ghcr.io/kubevela"},
		})
		Expect(err).Should(HaveOccurred())
		Expect(strings.Contains(err.Error(), "the template does not exist")).Should(BeTrue())
		Expect(act.Phase).Should(BeEmpty())
		exists, err := config.ConfigCRExists(ctx, k8sClient, "default", "hub-kubevela")
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).Should(BeFalse())
	})

	It("create writes a Config CR through the dispatcher and waits for the controller", func() {
		template, err := factory.ParseTemplate(ctx, "test-image-registry", []byte(templateContent))
		Expect(err).ToNot(HaveOccurred())
		Expect(factory.CreateOrUpdateConfigTemplate(ctx, "default", template)).ToNot(HaveOccurred())

		dispatcher := &recordingDispatcher{}
		f := dispatcher.factory(k8sClient)
		props := CreateConfigProperties{
			Name: "hub-kubevela", Namespace: "default", Template: "default/test-image-registry",
			Config: map[string]interface{}{"registry": "ghcr.io/kubevela"},
		}

		By("the first pass writes the CR and waits: no controller has rendered it")
		act := &mock.Action{}
		Expect(Create(ctx, depsWith(f, act), props)).Should(Succeed())
		Expect(dispatcher.kinds).Should(Equal([]string{"Config"}), "the Config went through the dispatcher, nothing else")
		Expect(dispatcher.batches).Should(Equal(1))
		var got configv1alpha1.Config
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "hub-kubevela"}, &got)).Should(Succeed())
		Expect(got.Spec.TemplateRef).Should(Equal(&configv1alpha1.ConfigTemplateReference{Name: "test-image-registry", Namespace: "default"}))
		Expect(string(got.Spec.Properties.Raw)).Should(MatchJSON(`{"registry":"ghcr.io/kubevela"}`))
		Expect(act.Phase).Should(Equal("Wait"))
		Expect(act.Msg).Should(ContainSubstring("waiting for config default/hub-kubevela to be rendered"))
		Expect(factory.IsExist(ctx, "default", "hub-kubevela")).Should(BeFalse(), "no legacy Secret was written")

		By("the controller reports Available for this spec: the step ends")
		setStatus(ctx, "hub-kubevela", configv1alpha1.ConfigPhaseAvailable, "")
		act = &mock.Action{}
		Expect(Create(ctx, depsWith(f, act), props)).Should(Succeed())
		Expect(act.Phase).Should(BeEmpty())

		By("the controller reports Error: the step fails with its message")
		setStatus(ctx, "hub-kubevela", configv1alpha1.ConfigPhaseError, "secret default/hub-kubevela already exists and is not owned by this Config")
		act = &mock.Action{}
		err = Create(ctx, depsWith(f, act), props)
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).Should(ContainSubstring("already exists and is not owned by this Config"))
		Expect(act.Phase).Should(BeEmpty())

		By("a status for the previous spec is not this spec's: the step waits")
		setStatus(ctx, "hub-kubevela", configv1alpha1.ConfigPhaseAvailable, "")
		changed := props
		changed.Config = map[string]interface{}{"registry": "quay.io/kubevela"}
		act = &mock.Action{}
		Expect(Create(ctx, depsWith(f, act), changed)).Should(Succeed())
		Expect(act.Phase).Should(Equal("Wait"))
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "hub-kubevela"}, &got)).Should(Succeed())
		Expect(got.Generation).Should(BeNumerically(">", got.Status.ObservedGeneration))
		setStatus(ctx, "hub-kubevela", configv1alpha1.ConfigPhaseAvailable, "")
	})

	// reviewingClient answers every SubjectAccessReview with allowed and records what it was
	// asked; every other call goes to the suite's API server.
	reviewingClient := func(allowed bool, reviews *[]authorizationv1.ResourceAttributes) client.Client {
		watching, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).ToNot(HaveOccurred())
		return interceptor.NewClient(watching, interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if review, ok := obj.(*authorizationv1.SubjectAccessReview); ok {
					*reviews = append(*reviews, *review.Spec.ResourceAttributes)
					review.Status.Allowed = allowed
					return nil
				}
				return c.Create(ctx, obj, opts...)
			},
		})
	}
	asAlice := request.WithUser(ctx, &user.DefaultInfo{Name: "alice", Groups: []string{"scope-users"}})

	It("create without a user refuses a namespace-scoped template elsewhere, judged from the application's namespace", func() {
		ensureNamespace(ctx, "wf-b")
		template, err := factory.ParseTemplate(ctx, "elsewhere", []byte(templateContent))
		Expect(err).ToNot(HaveOccurred())
		Expect(factory.CreateOrUpdateConfigTemplate(ctx, "wf-b", template)).ToNot(HaveOccurred(), "a legacy ConfigMap template, namespace-scoped outside vela-system whatever its label")

		By("a config in the application's namespace from the other team's template: refused before anything is written")
		act := &mock.Action{}
		err = Create(ctx, depsWith(factory, act), propsOf("from-elsewhere", "wf-b/elsewhere"))
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).Should(ContainSubstring("config template wf-b/elsewhere is namespace-scoped: a config in default may use it only if its author can get it"))
		Expect(err.Error()).Should(ContainSubstring("the AuthenticateApplication feature gate is off"))
		exists, err := config.ConfigCRExists(ctx, k8sClient, "default", "from-elsewhere")
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).Should(BeFalse(), "nothing written")

		By("the step's namespace property set to the template's namespace: still judged from the application's, so still refused")
		moved := propsOf("into-wf-b", "wf-b/elsewhere")
		moved.Namespace = "wf-b"
		act = &mock.Action{}
		err = Create(ctx, depsWith(factory, act), moved)
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).Should(ContainSubstring("a config in default may use it only if its author can get it"))
		exists, err = config.ConfigCRExists(ctx, k8sClient, "wf-b", "into-wf-b")
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).Should(BeFalse())
	})

	It("create with a user reviews a namespace-scoped template elsewhere for that user before reading it", func() {
		ensureNamespace(ctx, "wf-b")
		template, err := factory.ParseTemplate(ctx, "theirs", []byte(templateContent))
		Expect(err).ToNot(HaveOccurred())
		Expect(factory.CreateOrUpdateConfigTemplate(ctx, "wf-b", template)).ToNot(HaveOccurred())

		By("denied: refused with the reason and the user, nothing written, whatever the controller's cache could read")
		var reviews []authorizationv1.ResourceAttributes
		denying := reviewingClient(false, &reviews)
		act := &mock.Action{}
		err = Create(asAlice, Deps{Factory: config.NewConfigFactory(denying), Client: denying, Action: act}, propsOf("from-theirs", "wf-b/theirs"))
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).Should(ContainSubstring("config template wf-b/theirs is namespace-scoped"))
		Expect(err.Error()).Should(ContainSubstring("alice may not"))
		Expect(reviews).Should(ConsistOf(authorizationv1.ResourceAttributes{Verb: "get", Group: "config.oam.dev", Version: "v1alpha1", Resource: "configtemplates", Namespace: "wf-b", Name: "theirs"}))
		exists, err := config.ConfigCRExists(ctx, k8sClient, "default", "from-theirs")
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).Should(BeFalse())

		By("allowed: the step writes, as the user")
		reviews = nil
		allowing := reviewingClient(true, &reviews)
		dispatcher := &recordingDispatcher{}
		act = &mock.Action{}
		Expect(Create(asAlice, Deps{Factory: dispatcher.factory(allowing), Client: allowing, Action: act}, propsOf("from-theirs", "wf-b/theirs"))).Should(Succeed())
		Expect(act.Phase).Should(Equal("Wait"))
		Expect(reviews).Should(HaveLen(1))
	})

	It("create uses a system template in vela-system from any namespace with no review, and refuses a template elsewhere that is not usable yet", func() {
		ensureNamespace(ctx, "vela-system")
		ensureNamespace(ctx, "wf-b")
		shared, err := factory.ParseTemplate(ctx, "shared", []byte(templateContent))
		Expect(err).ToNot(HaveOccurred())
		Expect(factory.CreateOrUpdateConfigTemplate(ctx, "vela-system", shared)).ToNot(HaveOccurred(), "a legacy ConfigMap in vela-system is system whatever its label (this one says project)")
		DeferCleanup(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: config.TemplateConfigMapNamePrefix + "shared", Namespace: types.DefaultKubeVelaNS}}))).Should(Succeed())
		})
		dispatcher := &recordingDispatcher{}
		act := &mock.Action{}
		Expect(Create(ctx, depsWith(dispatcher.factory(k8sClient), act), propsOf("from-shared", "vela-system/shared"))).Should(Succeed())
		Expect(act.Phase).Should(Equal("Wait"), "a system template needs no user: the step wrote and waits for the controller")

		By("as a user: no review for a system template, and the adoption of a legacy config made from it goes through")
		var reviews []authorizationv1.ResourceAttributes
		denying := reviewingClient(false, &reviews)
		act = &mock.Action{}
		Expect(Create(asAlice, Deps{Factory: config.NewConfigFactory(denying), Client: denying, Action: act}, propsOf("from-shared-as-alice", "vela-system/shared"))).Should(Succeed())
		item, err := factory.ParseConfig(ctx, config.NamespacedName{Name: "shared", Namespace: types.DefaultKubeVelaNS}, config.Metadata{
			NamespacedName: config.NamespacedName{Name: "lc-shared", Namespace: "default"},
			Properties:     map[string]interface{}{"registry": "old.example"},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(factory.CreateOrUpdateConfig(ctx, item, "default")).Should(Succeed(), "a legacy Secret from the CLI: no app labels, anyone's to adopt")
		adopting := propsOf("lc-shared", "vela-system/shared")
		adopting.Config = map[string]interface{}{"registry": "new.example"}
		act = &mock.Action{}
		Expect(Create(asAlice, Deps{Factory: config.NewConfigFactory(denying), Client: denying, Action: act}, adopting)).Should(Succeed())
		var adopted corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "lc-shared"}, &adopted)).Should(Succeed())
		Expect(adopted.OwnerReferences).Should(HaveLen(1))
		Expect(adopted.OwnerReferences[0].Kind).Should(Equal(configv1alpha1.ConfigKind))
		Expect(reviews).Should(BeEmpty())

		By("a namespace-scoped ConfigTemplate in vela-system is not a system one: reviewed, and refused before any adoption")
		nsct := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "ns-in-sys", Namespace: types.DefaultKubeVelaNS},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: templateContent, Scope: configv1alpha1.ConfigTemplateScopeNamespace},
			Status:     configv1alpha1.ConfigTemplateStatus{},
		}
		Expect(k8sClient.Create(ctx, nsct)).Should(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, nsct))).Should(Succeed()) })
		nsct.Status.Phase = configv1alpha1.ConfigTemplatePhaseAvailable
		nsct.Status.Schema = &runtime.RawExtension{Raw: []byte(`{}`)}
		Expect(k8sClient.Status().Update(ctx, nsct)).Should(Succeed())
		act = &mock.Action{}
		err = Create(asAlice, Deps{Factory: config.NewConfigFactory(denying), Client: denying, Action: act}, propsOf("from-ns-in-sys", "vela-system/ns-in-sys"))
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).Should(ContainSubstring("alice may not"))

		By("a ConfigTemplate elsewhere the controller has not made usable yet: refused until it is, as the webhook would")
		pending := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "pending", Namespace: "wf-b"},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: templateContent},
		}
		Expect(k8sClient.Create(ctx, pending)).Should(Succeed())
		act = &mock.Action{}
		err = Create(ctx, depsWith(factory, act), propsOf("from-pending", "wf-b/pending"))
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).Should(ContainSubstring("config template wf-b/pending is not available yet: a config in default may reference a template in another namespace once it is"))
		Expect(act.Phase).Should(BeEmpty())

		By("a system ConfigTemplate in vela-system that is not usable yet: known from its spec, the step writes and waits (the sweep's fresh CRs)")
		pendingShared := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "pending-shared", Namespace: types.DefaultKubeVelaNS},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: templateContent, Scope: configv1alpha1.ConfigTemplateScopeSystem},
		}
		Expect(k8sClient.Create(ctx, pendingShared)).Should(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, pendingShared))).Should(Succeed()) })
		act = &mock.Action{}
		Expect(Create(ctx, depsWith(dispatcher.factory(k8sClient), act), propsOf("from-pending-shared", "vela-system/pending-shared"))).Should(Succeed())
		Expect(act.Phase).Should(Equal("Wait"))
	})

	It("create of a config the template grandfathers needs no review: the binding predates the template's adoption", func() {
		ensureNamespace(ctx, "wf-b")
		old := &configv1alpha1.ConfigTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "old-shared", Namespace: "wf-b", Annotations: map[string]string{config.AnnotationGrandfatheredConfigs: "default/grandfathered"}},
			Spec:       configv1alpha1.ConfigTemplateSpec{Template: templateContent, Scope: configv1alpha1.ConfigTemplateScopeNamespace},
		}
		Expect(k8sClient.Create(ctx, old)).Should(Succeed())
		DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, old))).Should(Succeed()) })
		old.Status.Phase = configv1alpha1.ConfigTemplatePhaseAvailable
		old.Status.Schema = &runtime.RawExtension{Raw: []byte(`{}`)}
		Expect(k8sClient.Status().Update(ctx, old)).Should(Succeed())

		dispatcher := &recordingDispatcher{}
		act := &mock.Action{}
		Expect(Create(ctx, depsWith(dispatcher.factory(k8sClient), act), propsOf("grandfathered", "wf-b/old-shared"))).Should(Succeed())
		act = &mock.Action{}
		err := Create(ctx, depsWith(factory, act), propsOf("newcomer", "wf-b/old-shared"))
		Expect(err).Should(HaveOccurred(), "a config the template does not list is judged as any other")
		Expect(err.Error()).Should(ContainSubstring("is namespace-scoped"))
	})

	It("create without a template is not a template question: the step writes the Config", func() {
		dispatcher := &recordingDispatcher{}
		act := &mock.Action{}
		Expect(Create(ctx, depsWith(dispatcher.factory(k8sClient), act), CreateConfigProperties{
			Name: "no-template", Namespace: "default", Config: map[string]interface{}{"value": "plain"},
		})).Should(Succeed())
		Expect(act.Phase).Should(Equal("Wait"))
		var got configv1alpha1.Config
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "no-template"}, &got)).Should(Succeed())
		Expect(got.Spec.TemplateRef).Should(BeNil())
	})

	It("a sensitive template keeps the properties out of the Config", func() {
		template, err := factory.ParseTemplate(ctx, "test-password", []byte(sensitiveTemplateContent))
		Expect(err).ToNot(HaveOccurred())
		Expect(factory.CreateOrUpdateConfigTemplate(ctx, "default", template)).ToNot(HaveOccurred())

		dispatcher := &recordingDispatcher{}
		act := &mock.Action{}
		Expect(Create(ctx, depsWith(dispatcher.factory(k8sClient), act), CreateConfigProperties{
			Name: "db-password", Namespace: "default", Template: "default/test-password",
			Config: map[string]interface{}{"password": "s3cr3t"},
		})).Should(Succeed())
		Expect(dispatcher.kinds).Should(Equal([]string{"Secret", "Config"}), "the companion Secret is dispatched with the Config, so the app tracks both")
		Expect(dispatcher.batches).Should(Equal(2), "in its own batch, first: the Config's webhook reads the Secret, which a dry run of one batch would not have created")
		var got configv1alpha1.Config
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "db-password"}, &got)).Should(Succeed())
		Expect(got.Spec.Properties).Should(BeNil())
		Expect(got.Spec.PropertiesFrom.SecretRef.Name).Should(Equal("db-password-properties"))
		var secret corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "db-password-properties"}, &secret)).Should(Succeed())
		Expect(string(secret.Data[config.DefaultPropertiesSecretKey])).Should(MatchJSON(`{"password":"s3cr3t"}`))
	})

	It("a legacy config of the same name is adopted before the step updates it", func() {
		By("a config the 1.11 provider wrote for this app: a legacy Secret with the app labels, no Config")
		before := legacyConfigOf(ctx, "wf-cfg", "old.example")

		By("the step on the new controller, through the app's dispatcher")
		dispatcher := &recordingDispatcher{}
		f := dispatcher.factory(k8sClient)
		act := &mock.Action{}
		Expect(Create(ctx, depsWith(f, act), CreateConfigProperties{
			Name: "wf-cfg", Namespace: "default", Template: "default/test-image-registry",
			Config: map[string]interface{}{"registry": "new.example"},
		})).Should(Succeed())
		got := expectClaimed(ctx, "wf-cfg", before)
		Expect(string(got.Spec.Properties.Raw)).Should(MatchJSON(`{"registry":"new.example"}`), "the step's spec, applied after the adoption")
		Expect(dispatcher.kinds).Should(Equal([]string{"Config"}))
		Expect(act.Phase).Should(Equal("Wait"), "the controller still has to render the new spec")

		By("a plain Secret in the way is left alone for the controller to refuse")
		Expect(k8sClient.Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "plain", Namespace: "default"}, StringData: map[string]string{"value": "theirs"}})).Should(Succeed())
		act = &mock.Action{}
		Expect(Create(ctx, depsWith(f, act), CreateConfigProperties{
			Name: "plain", Namespace: "default", Template: "default/test-image-registry",
			Config: map[string]interface{}{"registry": "x"},
		})).Should(Succeed())
		var plain corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "plain"}, &plain)).Should(Succeed())
		Expect(plain.OwnerReferences).Should(BeEmpty())
		Expect(act.Phase).Should(Equal("Wait"))
	})

	It("a Config the startup sweep made of the app's legacy config is taken over, not refused", func() {
		By("the sweep adopted the config at the upgrade: a migration-created Config, nobody's in the dispatcher's eyes")
		before := legacyConfigOf(ctx, "swept", "old.example")
		report, err := config.NewMigrator(k8sClient).MigrateConfig(ctx, "default", "swept", config.MigrationOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(report.Count(config.MigrationAdopted)).Should(Equal(1))

		By("the workflow re-runs: the step's apply would be refused without the take-over")
		dispatcher := &recordingDispatcher{}
		act := &mock.Action{}
		Expect(Create(ctx, depsWith(dispatcher.factory(k8sClient), act), CreateConfigProperties{
			Name: "swept", Namespace: "default", Template: "default/test-image-registry",
			Config: map[string]interface{}{"registry": "new.example"},
		})).Should(Succeed())
		got := expectClaimed(ctx, "swept", before)
		Expect(string(got.Spec.Properties.Raw)).Should(MatchJSON(`{"registry":"new.example"}`))
		Expect(act.Phase).Should(Equal("Wait"))
	})

	It("a legacy sensitive config is adopted and its properties Secret claimed with the Config", func() {
		before := legacyOf(ctx, "test-password", sensitiveTemplateContent, "wf-sensitive-legacy", map[string]interface{}{"password": "old"})
		dispatcher := &recordingDispatcher{}
		act := &mock.Action{}
		Expect(Create(ctx, depsWith(dispatcher.factory(k8sClient), act), CreateConfigProperties{
			Name: "wf-sensitive-legacy", Namespace: "default", Template: "default/test-password",
			Config: map[string]interface{}{"password": "new"},
		})).Should(Succeed())
		got := expectClaimed(ctx, "wf-sensitive-legacy", before)
		Expect(got.Spec.Properties).Should(BeNil())
		Expect(got.Spec.PropertiesFrom.SecretRef.Name).Should(Equal("wf-sensitive-legacy-properties"))
		var props corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "wf-sensitive-legacy-properties"}, &props)).Should(Succeed())
		Expect(props.Labels[oam.LabelAppName]).Should(Equal(dispatchingApp.name), "the migration's properties Secret is taken over too, or the dispatch of the step's own would be refused")
		Expect(string(props.Data[config.DefaultPropertiesSecretKey])).Should(MatchJSON(`{"password":"new"}`))
		Expect(dispatcher.kinds).Should(Equal([]string{"Secret", "Config"}))
		Expect(act.Phase).Should(Equal("Wait"))
	})

	It("the output objects of an adopted legacy config are released from the application too", func() {
		before := legacyOf(ctx, "test-with-output", outputTemplateContent, "wf-outlegacy", map[string]interface{}{"value": "old"})
		var out corev1.ConfigMap
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "wf-outlegacy-out"}, &out)).Should(Succeed())
		labelForApp(ctx, &out)
		act := &mock.Action{}
		Expect(Create(ctx, depsWith((&recordingDispatcher{}).factory(k8sClient), act), CreateConfigProperties{
			Name: "wf-outlegacy", Namespace: "default", Template: "default/test-with-output",
			Config: map[string]interface{}{"value": "new"},
		})).Should(Succeed())
		expectClaimed(ctx, "wf-outlegacy", before)
		var after corev1.ConfigMap
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "wf-outlegacy-out"}, &after)).Should(Succeed())
		Expect(after.UID).Should(Equal(out.UID))
		Expect(after.Labels).ShouldNot(HaveKey(oam.LabelAppName), "released by the adoption: the app's GC would otherwise delete the output the Config now owns")
	})

	It("a migration-created Config another application marked is left to the dispatcher, which refuses it", func() {
		legacyConfigOf(ctx, "theirs", "old.example")
		report, err := config.NewMigrator(k8sClient).MigrateConfig(ctx, "default", "theirs", config.MigrationOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(report.Count(config.MigrationAdopted)).Should(Equal(1))
		var theirs configv1alpha1.Config
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "theirs"}, &theirs)).Should(Succeed())
		theirs.Labels[oam.LabelAppName], theirs.Labels[oam.LabelAppNamespace] = "other-app", "default"
		Expect(k8sClient.Update(ctx, &theirs)).Should(Succeed())
		act := &mock.Action{}
		err = Create(ctx, depsWith((&recordingDispatcher{}).factory(k8sClient), act), CreateConfigProperties{
			Name: "theirs", Namespace: "default", Template: "default/test-image-registry",
			Config: map[string]interface{}{"registry": "mine.example"},
		})
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).Should(ContainSubstring("managed by other application"))
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "theirs"}, &theirs)).Should(Succeed())
		Expect(theirs.Labels[oam.LabelAppName]).Should(Equal("other-app"), "untouched: a take-over never overrides another owner")
	})

	It("the wait does not trust a read that shows a spec other than the one applied", func() {
		template, err := factory.ParseTemplate(ctx, "test-image-registry", []byte(templateContent))
		Expect(err).ToNot(HaveOccurred())
		Expect(factory.CreateOrUpdateConfigTemplate(ctx, "default", template)).Should(Succeed())
		ref := &config.NamespacedName{Name: "test-image-registry", Namespace: "default"}
		applied, _, err := config.BuildConfigCR(config.ConfigCRSpec{Name: "stale", Namespace: "default", Template: ref, Properties: map[string]interface{}{"registry": "a.example"}})
		Expect(err).ToNot(HaveOccurred())
		Expect(k8sClient.Create(ctx, applied)).Should(Succeed())
		setStatus(ctx, "stale", configv1alpha1.ConfigPhaseAvailable, "")

		By("the read shows the previous spec, Available at its own generation: a cache that has not seen the write")
		next, _, err := config.BuildConfigCR(config.ConfigCRSpec{Name: "stale", Namespace: "default", Template: ref, Properties: map[string]interface{}{"registry": "b.example"}})
		Expect(err).ToNot(HaveOccurred())
		act := &mock.Action{}
		Expect(awaitRendered(ctx, depsWith(factory, act), next)).Should(Succeed())
		Expect(act.Phase).Should(Equal("Wait"))

		By("the read shows the applied spec: the status counts")
		same, _, err := config.BuildConfigCR(config.ConfigCRSpec{Name: "stale", Namespace: "default", Template: ref, Properties: map[string]interface{}{"registry": "a.example"}})
		Expect(err).ToNot(HaveOccurred())
		act = &mock.Action{}
		Expect(awaitRendered(ctx, depsWith(factory, act), same)).Should(Succeed())
		Expect(act.Phase).Should(BeEmpty())
	})

	It("with no workflow to wait for, an unrendered Config is an error, not a success", func() {
		cfg, _, err := config.BuildConfigCR(config.ConfigCRSpec{Name: "nowait", Namespace: "default", Template: &config.NamespacedName{Name: "test-image-registry", Namespace: "default"}, Properties: map[string]interface{}{"registry": "x"}})
		Expect(err).ToNot(HaveOccurred())
		Expect(k8sClient.Create(ctx, cfg)).Should(Succeed())
		err = awaitRendered(ctx, Deps{Factory: factory, Client: k8sClient}, cfg)
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).Should(ContainSubstring("no workflow to wait for it"))
	})

	It("a legacy config another application dispatched is refused, not adopted", func() {
		before := legacyConfigOf(ctx, "not-mine", "old.example")
		before.Labels[oam.LabelAppName] = "other-app"
		Expect(k8sClient.Update(ctx, &before)).Should(Succeed())
		act := &mock.Action{}
		err := Create(ctx, depsWith((&recordingDispatcher{}).factory(k8sClient), act), CreateConfigProperties{
			Name: "not-mine", Namespace: "default", Template: "default/test-image-registry",
			Config: map[string]interface{}{"registry": "mine.example"},
		})
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).Should(ContainSubstring("managed by application default/other-app"))
		exists, err := config.ConfigCRExists(ctx, k8sClient, "default", "not-mine")
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).Should(BeFalse(), "nothing adopted")
		var after corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "not-mine"}, &after)).Should(Succeed())
		Expect(after.OwnerReferences).Should(BeEmpty())
		Expect(after.Labels[oam.LabelAppName]).Should(Equal("other-app"), "untouched")

		By("a legacy config nobody dispatched, the CLI's, is anyone's")
		theirs := legacyConfigOf(ctx, "cli-made", "old.example")
		delete(theirs.Labels, oam.LabelAppName)
		delete(theirs.Labels, oam.LabelAppNamespace)
		Expect(k8sClient.Update(ctx, &theirs)).Should(Succeed())
		act = &mock.Action{}
		Expect(Create(ctx, depsWith((&recordingDispatcher{}).factory(k8sClient), act), CreateConfigProperties{
			Name: "cli-made", Namespace: "default", Template: "default/test-image-registry",
			Config: map[string]interface{}{"registry": "mine.example"},
		})).Should(Succeed())
		expectClaimed(ctx, "cli-made", theirs)
	})

	It("delete-config lets a Config the application dispatched go through the application's delete handler, companion included", func() {
		template, err := factory.ParseTemplate(ctx, "test-password", []byte(sensitiveTemplateContent))
		Expect(err).ToNot(HaveOccurred())
		Expect(factory.CreateOrUpdateConfigTemplate(ctx, "default", template)).ToNot(HaveOccurred())
		dispatcher := &recordingDispatcher{}
		deps := depsOfApp(dispatcher, &mock.Action{})
		Expect(Create(ctx, deps, CreateConfigProperties{
			Name: "to-delete", Namespace: "default", Template: "default/test-password",
			Config: map[string]interface{}{"password": "p"},
		})).Should(Succeed())

		Expect(Delete(ctx, deps, config.NamespacedName{Namespace: "default", Name: "to-delete"})).Should(Succeed())
		Expect(dispatcher.deleted).Should(Equal([]string{"Secret/to-delete-properties", "Config/to-delete"}), "through the handler, so the tracker's records are marked deleted and state-keep does not bring them back")
		exists, err := config.ConfigCRExists(ctx, k8sClient, "default", "to-delete")
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).Should(BeFalse())
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "to-delete-properties"}, &corev1.Secret{}))).Should(BeTrue())

		By("a Config the application did not dispatch takes the factory's delete")
		Expect(k8sClient.Create(ctx, &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "not-dispatched", Namespace: "default"},
			Spec:       configv1alpha1.ConfigSpec{Properties: rawJSON(`{"a":1}`)},
		})).Should(Succeed())
		dispatcher.deleted = nil
		Expect(Delete(ctx, deps, config.NamespacedName{Namespace: "default", Name: "not-dispatched"})).Should(Succeed())
		Expect(dispatcher.deleted).Should(BeEmpty())
		exists, err = config.ConfigCRExists(ctx, k8sClient, "default", "not-dispatched")
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).Should(BeFalse())
	})

	It("a Config of that name nobody manages and migration did not create is refused by the dispatcher", func() {
		Expect(k8sClient.Create(ctx, &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "handmade", Namespace: "default"},
			Spec: configv1alpha1.ConfigSpec{
				TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: "test-image-registry", Namespace: "default"},
				Properties:  &runtime.RawExtension{Raw: []byte(`{"registry":"theirs.example"}`)},
			},
		})).Should(Succeed())
		act := &mock.Action{}
		err := Create(ctx, depsWith((&recordingDispatcher{}).factory(k8sClient), act), CreateConfigProperties{
			Name: "handmade", Namespace: "default", Template: "default/test-image-registry",
			Config: map[string]interface{}{"registry": "mine.example"},
		})
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).Should(ContainSubstring("exists but not managed by any application"))
		var got configv1alpha1.Config
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "handmade"}, &got)).Should(Succeed())
		Expect(string(got.Spec.Properties.Raw)).Should(MatchJSON(`{"registry":"theirs.example"}`), "untouched")
		Expect(got.Labels).ShouldNot(HaveKey(oam.LabelAppName))
		Expect(act.Phase).Should(BeEmpty())
	})

	It("takes the legacy path when the Config kind is not served", func() {
		noConfig := runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(noConfig)).Should(Succeed())
		bare := fake.NewClientBuilder().WithScheme(noConfig).Build()
		bareFactory := config.NewConfigFactory(bare)
		template, err := bareFactory.ParseTemplate(ctx, "test-image-registry", []byte(templateContent))
		Expect(err).ToNot(HaveOccurred())
		Expect(bareFactory.CreateOrUpdateConfigTemplate(ctx, "default", template)).ToNot(HaveOccurred())

		act := &mock.Action{}
		Expect(Create(ctx, Deps{Factory: bareFactory, Client: bare, Action: act}, CreateConfigProperties{
			Name: "legacy-only", Namespace: "default", Template: "default/test-image-registry",
			Config: map[string]interface{}{"registry": "ghcr.io/kubevela"},
		})).Should(Succeed())
		var secret corev1.Secret
		Expect(bare.Get(ctx, client.ObjectKey{Namespace: "default", Name: "legacy-only"}, &secret)).Should(Succeed())
		Expect(secret.Labels[types.LabelConfigCatalog]).Should(Equal(types.VelaCoreConfig))
		Expect(act.Phase).Should(BeEmpty(), "the legacy write is synchronous")
	})

	It("read waits for a Config that is still rendering and fails on one in Error", func() {
		Expect(k8sClient.Create(ctx, &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "rendering", Namespace: "default"},
			Spec:       configv1alpha1.ConfigSpec{Properties: rawJSON(`{"a":1}`)},
		})).Should(Succeed())

		act := &mock.Action{}
		content, err := Read(ctx, depsWith(factory, act), config.NamespacedName{Namespace: "default", Name: "rendering"})
		Expect(err).ToNot(HaveOccurred())
		Expect(content).Should(BeNil())
		Expect(act.Phase).Should(Equal("Wait"))

		setStatus(ctx, "rendering", configv1alpha1.ConfigPhaseError, "template default/x not found")
		act = &mock.Action{}
		_, err = Read(ctx, depsWith(factory, act), config.NamespacedName{Namespace: "default", Name: "rendering"})
		Expect(err).Should(HaveOccurred())
		Expect(err.Error()).Should(ContainSubstring("template default/x not found"))

		By("Available, but for the previous spec: the read waits rather than returning what that spec rendered")
		setStatus(ctx, "rendering", configv1alpha1.ConfigPhaseAvailable, "")
		var stale configv1alpha1.Config
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "rendering"}, &stale)).Should(Succeed())
		stale.Spec.Properties = rawJSON(`{"a":2}`)
		Expect(k8sClient.Update(ctx, &stale)).Should(Succeed())
		act = &mock.Action{}
		content, err = Read(ctx, depsWith(factory, act), config.NamespacedName{Namespace: "default", Name: "rendering"})
		Expect(err).ToNot(HaveOccurred())
		Expect(content).Should(BeNil())
		Expect(act.Phase).Should(Equal("Wait"))

		act = &mock.Action{}
		_, err = Read(ctx, depsWith(factory, act), config.NamespacedName{Namespace: "default", Name: "nowhere"})
		Expect(err).Should(HaveOccurred(), "neither backend holds it: the original not-found")
		Expect(act.Phase).Should(BeEmpty())
	})

	It("list returns legacy and CR-backed configs once each, read sees both", func() {
		By("a legacy config next to it")
		item, err := factory.ParseConfig(ctx, config.NamespacedName{Name: "test-image-registry", Namespace: "default"}, config.Metadata{
			NamespacedName: config.NamespacedName{Name: "legacy-only-envtest", Namespace: "default"},
			Properties:     map[string]interface{}{"registry": "legacy.example"},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(factory.CreateOrUpdateConfig(ctx, item, "default")).Should(Succeed())

		By("a Secret as the controller materialises it for a Config")
		owner := &configv1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{Name: "cr-backed", Namespace: "default"},
			Spec:       configv1alpha1.ConfigSpec{TemplateRef: &configv1alpha1.ConfigTemplateReference{Name: "test-image-registry", Namespace: "default"}},
		}
		Expect(k8sClient.Create(ctx, owner)).Should(Succeed())
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cr-backed", Namespace: "default",
				Labels:      map[string]string{types.LabelConfigCatalog: types.VelaCoreConfig, types.LabelConfigType: "test-image-registry"},
				Annotations: map[string]string{types.AnnotationConfigTemplateNamespace: "default"},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: configv1alpha1.SchemeGroupVersion.String(), Kind: configv1alpha1.ConfigKind,
					Name: owner.Name, UID: owner.UID, Controller: ptr.To(true),
				}},
			},
			Data: map[string][]byte{config.SaveInputPropertiesKey: []byte(`{"registry":"cr.example"}`)},
		})).Should(Succeed())

		contents, err := List(ctx, depsWith(factory, &mock.Action{}), ListVars{Namespace: "default", Template: "default/test-image-registry"})
		Expect(err).ToNot(HaveOccurred())
		names := map[string]string{}
		for _, c := range contents {
			name, _ := c["name"].(string)
			Expect(name).ShouldNot(BeEmpty(), "every listed config has a name: %v", c)
			Expect(names).ShouldNot(HaveKey(name), "listed once")
			properties, _ := c["config"].(map[string]interface{})
			registry, _ := properties["registry"].(string)
			names[name] = registry
		}
		Expect(names).Should(HaveKeyWithValue("cr-backed", "cr.example"))
		Expect(names).Should(HaveKeyWithValue("legacy-only-envtest", "legacy.example"), "the legacy one is listed too")

		setStatus(ctx, "cr-backed", configv1alpha1.ConfigPhaseAvailable, "") // the read trusts the Secret once the Config says it is rendered
		content, err := Read(ctx, depsWith(factory, &mock.Action{}), config.NamespacedName{Namespace: "default", Name: "cr-backed"})
		Expect(err).ToNot(HaveOccurred())
		Expect(content["registry"]).Should(Equal("cr.example"))
	})

	It("delete removes a CR-backed config through the Config", func() {
		Expect(factory.DeleteConfig(ctx, "default", "cr-backed")).Should(Succeed())
		exists, err := config.ConfigCRExists(ctx, k8sClient, "default", "cr-backed")
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).Should(BeFalse())
	})

	// the provider-function wrappers carry the runtime params through
	It("the CueX wrappers pass the runtime params", func() {
		res, err := ListConfig(ctx, &oamprovidertypes.Params[ListVars]{
			Params:        ListVars{Namespace: "default", Template: "test-image-registry"},
			RuntimeParams: oamprovidertypes.RuntimeParams{ConfigFactory: factory, KubeClient: k8sClient, Action: &mock.Action{}},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(res.Returns.Configs).ShouldNot(BeEmpty())
		_, err = ListConfig(ctx, &oamprovidertypes.Params[ListVars]{Params: ListVars{}, RuntimeParams: oamprovidertypes.RuntimeParams{ConfigFactory: factory}})
		Expect(err).Should(Equal(ErrRequestInvalid))
	})
})

// the legacy config the list spec expects, written the way the 1.11 provider wrote it

func rawJSON(s string) *runtime.RawExtension { return &runtime.RawExtension{Raw: []byte(s)} }

var sensitiveTemplateContent = `
metadata: {
	name:        "test-password"
	alias:       "Password"
	scope:       "project"
	description: "a sensitive template"
	sensitive:   true
}

template: {
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {
			name:      context.name
			namespace: context.namespace
		}
		stringData: password: parameter.password
	}
	parameter: {
		password: string
	}
}
`

var outputTemplateContent = `
metadata: {
	name:      "test-with-output"
	alias:     "With output"
	scope:     "namespace"
	sensitive: false
}
template: {
	parameter: value: string
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {name: context.name, namespace: context.namespace}
		stringData: value: parameter.value
	}
	outputs: {
		cm: {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: {name: context.name + "-out", namespace: context.namespace}
			data: value: parameter.value
		}
	}
}
`

var templateContent = `
import (
	"encoding/base64"
	"encoding/json"
	"strconv"
)

metadata: {
	name:        "image-registry"
	alias:       "Image Registry"
	scope:       "project"
	description: "Config information to authenticate image registry"
	sensitive:   false
}

template: {
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {
			name:      context.name
			namespace: context.namespace
			labels: {
				"config.oam.dev/catalog": "velacore-config"
				"config.oam.dev/type":    "image-registry"
			}
		}
		if parameter.auth != _|_ {
			type: "kubernetes.io/dockerconfigjson"
		}
		if parameter.auth == _|_ {
			type: "Opaque"
		}
		stringData: {
			if parameter.auth != _|_ && parameter.auth.username != _|_ {
				".dockerconfigjson": json.Marshal({
					"auths": (parameter.registry): {
						"username": parameter.auth.username
						"password": parameter.auth.password
						if parameter.auth.email != _|_ {
							"email": parameter.auth.email
						}
						"auth": base64.Encode(null, (parameter.auth.username + ":" + parameter.auth.password))
					}
				})
			}
			if parameter.insecure != _|_ {
				"insecure-skip-verify": strconv.FormatBool(parameter.insecure)
			}
			if parameter.useHTTP != _|_ {
				"protocol-use-http": strconv.FormatBool(parameter.useHTTP)
			}
		}
	}

	parameter: {
		// +usage=Image registry FQDN, such as: index.docker.io
		registry: *"index.docker.io" | string
		// +usage=Authenticate the image registry
		auth?: {
			// +usage=Private Image registry username
			username: string
			// +usage=Private Image registry password
			password: string
			// +usage=Private Image registry email
			email?: string
		}
		// +usage=For the registry server that uses the self-signed certificate
		insecure?: bool
		// +usage=For the registry server that uses the HTTP protocol
		useHTTP?: bool
	}
}
`
