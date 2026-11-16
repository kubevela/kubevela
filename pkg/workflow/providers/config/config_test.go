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
	"fmt"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/config"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
	oamprovidertypes "github.com/oam-dev/kubevela/pkg/workflow/providers/types"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var scheme = runtime.NewScheme()
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
	}
	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())
	Expect(clientgoscheme.AddToScheme(scheme)).Should(BeNil())
	// +kubebuilder:scaffold:scheme
	By("Create the k8s client")
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	factory = config.NewConfigFactory(k8sClient)
})

var _ = AfterSuite(func() {
	By("Tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

var _ = Describe("Test the config provider", func() {

	It("test creating a config", func() {
		ctx := context.Background()
		params := &CreateParams{
			Params: CreateConfigProperties{
				Name:      "hub-kubevela",
				Namespace: "default",
				Template:  "default/test-image-registry",
				Config: map[string]interface{}{
					"registry": "ghcr.io/kubevela",
				},
			},
			RuntimeParams: oamprovidertypes.RuntimeParams{
				ConfigFactory: factory,
			},
		}
		_, err := CreateConfig(ctx, params)
		Expect(strings.Contains(err.Error(), "the template does not exist")).Should(BeTrue())

		template, err := factory.ParseTemplate(ctx, "test-image-registry", []byte(templateContent))
		Expect(err).ToNot(HaveOccurred())
		Expect(factory.CreateOrUpdateConfigTemplate(ctx, "default", template)).ToNot(HaveOccurred())

		_, err = CreateConfig(ctx, params)
		Expect(err).ToNot(HaveOccurred())
	})

	It("test creating a config without the template", func() {
		params := &CreateParams{
			Params: CreateConfigProperties{
				Name:      "www-kubevela",
				Namespace: "default",
				Config: map[string]interface{}{
					"url": "kubevela.net",
				},
			},
			RuntimeParams: oamprovidertypes.RuntimeParams{
				ConfigFactory: factory,
			},
		}
		_, err := CreateConfig(context.Background(), params)
		Expect(err).ToNot(HaveOccurred())
	})

	It("test listing the config", func() {
		ctx := context.Background()
		res, err := ListConfig(ctx, &oamprovidertypes.Params[ListVars]{
			Params: ListVars{
				Namespace: "default",
				Template:  "test-image-registry",
			},
			RuntimeParams: oamprovidertypes.RuntimeParams{
				ConfigFactory: factory,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		contents := res.Returns.Configs
		Expect(len(contents)).To(Equal(1))
		Expect(contents[0]["config"].(map[string]interface{})["registry"]).To(Equal("ghcr.io/kubevela"))
	})

	It("test reading the config", func() {
		ctx := context.Background()
		res, err := ReadConfig(ctx, &oamprovidertypes.Params[config.NamespacedName]{
			Params: config.NamespacedName{
				Namespace: "default",
				Name:      "hub-kubevela",
			},
			RuntimeParams: oamprovidertypes.RuntimeParams{
				ConfigFactory: factory,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(res.Returns.Config["registry"]).To(Equal("ghcr.io/kubevela"))
	})

	It("test deleting the config", func() {
		ctx := context.Background()
		_, err := DeleteConfig(ctx, &oamprovidertypes.Params[config.NamespacedName]{
			Params: config.NamespacedName{
				Namespace: "default",
				Name:      "hub-kubevela",
			},
			RuntimeParams: oamprovidertypes.RuntimeParams{
				ConfigFactory: factory,
			},
		})
		Expect(err).ToNot(HaveOccurred())

		configs, err := factory.ListConfigs(context.Background(), "default", "", "", false)
		Expect(err).ToNot(HaveOccurred())
		Expect(len(configs)).To(Equal(1))
		Expect(configs[0].Properties["url"]).To(Equal("kubevela.net"))
	})
	It("reaches configs outside the application's namespace only to read shared system configs", func() {
		ctx := context.Background()
		for _, ns := range []string{"vela-system", "wf-b"} {
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}))).Should(Succeed())
		}
		writeConfig := func(name, namespace, scope string) {
			Expect(k8sClient.Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: map[string]string{
				types.LabelConfigCatalog: types.VelaCoreConfig, types.LabelConfigType: "shared-read", types.LabelConfigScope: scope,
			}}, StringData: map[string]string{"input-properties": `{"registry":"` + name + `.example"}`}})).Should(Succeed())
		}
		writeConfig("tenant-db", "wf-b", "project")
		writeConfig("platform-registry", "vela-system", "system")
		rt := oamprovidertypes.RuntimeParams{ConfigFactory: factory, KubeClient: k8sClient,
			AppLabels: map[string]string{oam.LabelAppNamespace: "default"}}
		nn := func(namespace, name string) *oamprovidertypes.Params[config.NamespacedName] {
			return &oamprovidertypes.Params[config.NamespacedName]{Params: config.NamespacedName{Namespace: namespace, Name: name}, RuntimeParams: rt}
		}
		gateOff := "the AuthenticateApplication feature gate is off"

		By("another tenant's config: read, list and delete refused, nothing deleted")
		_, err := ReadConfig(ctx, nn("wf-b", "tenant-db"))
		Expect(err).Should(MatchError(ContainSubstring("config wf-b/tenant-db is outside the application's namespace default")))
		Expect(err).Should(MatchError(ContainSubstring(gateOff)))
		_, err = ListConfig(ctx, &oamprovidertypes.Params[ListVars]{Params: ListVars{Namespace: "wf-b", Template: "shared-read"}, RuntimeParams: rt})
		Expect(err).Should(MatchError(ContainSubstring("configs in namespace wf-b is outside the application's namespace default")))
		_, err = DeleteConfig(ctx, nn("wf-b", "tenant-db"))
		Expect(err).Should(MatchError(ContainSubstring(gateOff)))
		Expect(factory.IsExist(ctx, "wf-b", "tenant-db")).Should(BeTrue(), "not deleted")

		By("a system-scoped config in vela-system: read from anywhere, not deleted")
		_, err = ReadConfig(ctx, nn("vela-system", "platform-registry"))
		Expect(err).ToNot(HaveOccurred())
		_, err = DeleteConfig(ctx, nn("vela-system", "platform-registry"))
		Expect(err).Should(MatchError(ContainSubstring(gateOff)))
		Expect(factory.IsExist(ctx, "vela-system", "platform-registry")).Should(BeTrue(), "not deleted")

		By("no config written into another namespace")
		_, err = CreateConfig(ctx, &CreateParams{Params: CreateConfigProperties{Name: "planted", Namespace: "wf-b",
			Config: map[string]interface{}{"url": "x"}}, RuntimeParams: rt})
		Expect(err).Should(MatchError(ContainSubstring("config wf-b/planted is outside the application's namespace default")))
		Expect(factory.IsExist(ctx, "wf-b", "planted")).Should(BeFalse())

		// Under authentication the step acts for its Application's user, who is reviewed.
		featuregatetesting.SetFeatureGateDuringTest(GinkgoT(), utilfeature.DefaultMutableFeatureGate, features.AuthenticateApplication, true)
		asAlice := request.WithUser(ctx, &user.DefaultInfo{Name: "alice"})
		watching, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).ToNot(HaveOccurred())

		By("a shared system config is read as the controller once allowed, not as the application's user")
		refuseUsers := func(ctx context.Context) error {
			if u, ok := request.UserFrom(ctx); ok && u != nil {
				return apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "", fmt.Errorf("%s may not", u.GetName()))
			}
			return nil
		}
		asItself := interceptor.NewClient(watching, interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if err := refuseUsers(ctx); err != nil {
					return err
				}
				return c.Get(ctx, key, obj, opts...)
			},
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if err := refuseUsers(ctx); err != nil {
					return err
				}
				return c.List(ctx, list, opts...)
			},
		})
		shared := oamprovidertypes.RuntimeParams{ConfigFactory: config.NewConfigFactory(asItself), KubeClient: asItself,
			AppLabels: map[string]string{oam.LabelAppNamespace: "default"}}
		_, err = ReadConfig(asAlice, &oamprovidertypes.Params[config.NamespacedName]{Params: config.NamespacedName{Namespace: "vela-system", Name: "platform-registry"}, RuntimeParams: shared})
		Expect(err).ToNot(HaveOccurred())

		By("a write elsewhere creates or overwrites, so it is reviewed for both")
		var verbs []string
		reviewing := interceptor.NewClient(watching, interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if review, ok := obj.(*authorizationv1.SubjectAccessReview); ok {
					verbs = append(verbs, review.Spec.ResourceAttributes.Verb)
					review.Status.Allowed = true
					return nil
				}
				return c.Create(ctx, obj, opts...)
			},
		})
		_, err = CreateConfig(asAlice, &CreateParams{Params: CreateConfigProperties{Name: "planted", Namespace: "wf-b", Config: map[string]interface{}{"url": "x"}},
			RuntimeParams: oamprovidertypes.RuntimeParams{ConfigFactory: config.NewConfigFactory(reviewing), KubeClient: reviewing, AppLabels: map[string]string{oam.LabelAppNamespace: "default"}}})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(verbs).Should(ContainElements("create", "update"))
	})
})

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
