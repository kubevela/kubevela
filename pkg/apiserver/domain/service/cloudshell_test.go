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

package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"math/big"
	"time"

	v1alpha1 "github.com/cloudtty/cloudtty/pkg/apis/cloudshell/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	certificatesv1 "k8s.io/api/certificates/v1"
	certificatesv1beta1 "k8s.io/api/certificates/v1beta1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"

	kubevelatypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/auth"
)

func prepare(userName string) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).Should(BeNil())
	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   userName,
			Organization: []string{},
		},
		SignatureAlgorithm: x509.SHA256WithRSA,
	}
	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, template, privateKey)
	Expect(err).Should(BeNil())
	csrPemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrBytes})

	tpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "169.264.169.254"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(2, 0, 0),
		BasicConstraintsValid: true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
	}
	derCert, err := x509.CreateCertificate(rand.Reader, &tpl, &tpl, &privateKey.PublicKey, privateKey)
	Expect(err).Should(BeNil())
	buf := &bytes.Buffer{}
	err = pem.Encode(buf, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: derCert,
	})
	Expect(err).Should(BeNil())

	cli, err := kubernetes.NewForConfig(cfg)
	Expect(err).Should(BeNil())
	info, err := cli.ServerVersion()
	kubeVersion := version.MustParseGeneric(info.String())
	if kubeVersion.AtLeast(version.MustParseSemantic("v1.19.0")) {
		fmt.Println(info.String())
		csr := &certificatesv1.CertificateSigningRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name: userName,
			},
			Spec: certificatesv1.CertificateSigningRequestSpec{
				SignerName: certificatesv1.KubeAPIServerClientSignerName,
				Usages:     []certificatesv1.KeyUsage{certificatesv1.UsageClientAuth},
				Request:    csrPemBytes,
			},
		}
		Expect(k8sClient.Create(context.TODO(), csr)).Should(BeNil())
		Expect(err).Should(BeNil())
		csr, err = cli.CertificatesV1().CertificateSigningRequests().Get(context.TODO(), csr.Name, metav1.GetOptions{})
		Expect(err).Should(BeNil())
		csr.Status = certificatesv1.CertificateSigningRequestStatus{
			Conditions: []certificatesv1.CertificateSigningRequestCondition{
				{
					Type:   certificatesv1.CertificateApproved,
					Status: corev1.ConditionTrue,
				},
			},
		}
		_, err = cli.CertificatesV1().CertificateSigningRequests().UpdateApproval(context.TODO(), csr.Name, csr, metav1.UpdateOptions{})
		Expect(err).Should(BeNil())
		csr, err = cli.CertificatesV1().CertificateSigningRequests().Get(context.TODO(), csr.Name, metav1.GetOptions{})
		Expect(err).Should(BeNil())
		csr.Status.Certificate = buf.Bytes()
		err = k8sClient.Status().Update(context.TODO(), csr)
		Expect(err).Should(BeNil())
	} else {
		var name = certificatesv1beta1.KubeAPIServerClientSignerName
		csr := &certificatesv1beta1.CertificateSigningRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name: userName,
			},
			Spec: certificatesv1beta1.CertificateSigningRequestSpec{
				SignerName: &name,
				Usages:     []certificatesv1beta1.KeyUsage{certificatesv1beta1.UsageClientAuth},
				Request:    csrPemBytes,
			},
		}
		Expect(k8sClient.Create(context.TODO(), csr)).Should(BeNil())
		Expect(err).Should(BeNil())
		csr, err = cli.CertificatesV1beta1().CertificateSigningRequests().Get(context.TODO(), csr.Name, metav1.GetOptions{})
		Expect(err).Should(BeNil())
		csr.Status = certificatesv1beta1.CertificateSigningRequestStatus{
			Conditions: []certificatesv1beta1.CertificateSigningRequestCondition{
				{
					Type:   certificatesv1beta1.CertificateApproved,
					Status: corev1.ConditionTrue,
				},
			},
		}
		_, err = cli.CertificatesV1beta1().CertificateSigningRequests().UpdateApproval(context.TODO(), csr, metav1.UpdateOptions{})
		Expect(err).Should(BeNil())
		csr, err = cli.CertificatesV1beta1().CertificateSigningRequests().Get(context.TODO(), csr.Name, metav1.GetOptions{})
		Expect(err).Should(BeNil())
		csr.Status.Certificate = buf.Bytes()
		err = k8sClient.Status().Update(context.TODO(), csr)
		Expect(err).Should(BeNil())
	}
}

var _ = Describe("Test cloudshell service function", func() {
	var (
		ds                datastore.DataStore
		cloudShellService *cloudShellServiceImpl
		userService       *userServiceImpl
		projectService    *projectServiceImpl
		err               error
		database          string
	)

	BeforeEach(func() {
		database = "cloudshell-test-kubevela"
		ds, err = NewDatastore(datastore.Config{Type: "kubeapi", Database: database})
		Expect(err).Should(Succeed())
		envService := &envServiceImpl{
			Store:      ds,
			KubeClient: k8sClient,
		}
		userService = &userServiceImpl{
			Store:          ds,
			SysService:     &systemInfoServiceImpl{Store: ds},
			ProjectService: projectService,
		}
		projectService = &projectServiceImpl{
			Store: ds,
			RbacService: &rbacServiceImpl{
				Store: ds,
			},
			TargetService: &targetServiceImpl{
				Store:     ds,
				K8sClient: k8sClient,
			},
			EnvService:  envService,
			UserService: userService,
		}
		userService.ProjectService = projectService
		userService.RbacService = projectService.RbacService
		envService.ProjectService = projectService

		cloudShellService = &cloudShellServiceImpl{
			KubeClient:     k8sClient,
			KubeConfig:     cfg,
			ProjectService: projectService,
			TargetService: &targetServiceImpl{
				Store:     ds,
				K8sClient: k8sClient,
			},
			EnvService:  envService,
			UserService: userService,
			RBACService: projectService.RbacService,
		}
	})

	It("test prepareKubeConfig", func() {
		err = userService.Init(context.TODO())
		Expect(err).Should(BeNil())
		err = projectService.Init(context.TODO())
		Expect(err).Should(BeNil())

		By("test the developer users")

		_, err = userService.CreateUser(context.TODO(), apisv1.CreateUserRequest{Name: "test-dev", Password: "test"})
		Expect(err).Should(BeNil())

		_, err = projectService.AddProjectUser(context.TODO(), "default", apisv1.AddProjectUserRequest{
			UserName:  "test-dev",
			UserRoles: []string{"app-developer"},
		})
		Expect(err).Should(BeNil())

		permissions, err := projectService.RbacService.GetUserPermissions(context.TODO(), &model.User{Name: "test-dev"}, "default", false)
		Expect(err).Should(BeNil())
		Expect(checkReadOnly("default", permissions)).Should(BeFalse())

		ctx := context.WithValue(context.TODO(), &apisv1.CtxKeyUser, "test-dev")
		prepare("test-dev")
		err = cloudShellService.prepareKubeConfig(ctx)
		Expect(err).Should(BeNil())

		var rb rbacv1.RoleBinding
		err = k8sClient.Get(context.Background(), types.NamespacedName{Name: "kubevela:writer:application:binding", Namespace: "default"}, &rb)
		Expect(err).Should(BeNil())
		err = k8sClient.Get(context.Background(), types.NamespacedName{Name: "kubevela:writer:binding", Namespace: "default"}, &rb)
		Expect(err).Should(BeNil())

		By("test the viewer users")

		_, err = userService.CreateUser(context.TODO(), apisv1.CreateUserRequest{Name: "test-viewer", Password: "test"})
		Expect(err).Should(BeNil())

		_, err = projectService.AddProjectUser(context.TODO(), "default", apisv1.AddProjectUserRequest{
			UserName:  "test-viewer",
			UserRoles: []string{"project-viewer"},
		})
		Expect(err).Should(BeNil())

		permissions, err = projectService.RbacService.GetUserPermissions(ctx, &model.User{Name: "test-viewer"}, "default", false)
		Expect(err).Should(BeNil())
		Expect(checkReadOnly("default", permissions)).Should(BeTrue())

		ctx = context.WithValue(context.TODO(), &apisv1.CtxKeyUser, "test-viewer")
		prepare("test-viewer")
		err = cloudShellService.prepareKubeConfig(ctx)
		Expect(err).Should(BeNil())

		err = k8sClient.Get(context.Background(), types.NamespacedName{Name: "kubevela:reader:application:binding", Namespace: "default"}, &rb)
		Expect(err).Should(BeNil())
		Expect(len(rb.Subjects)).Should(Equal(1))
		Expect(rb.Subjects[0].Name).Should(Equal("test-viewer"))
		Expect(rb.Subjects[0].Kind).Should(Equal("User"))
		err = k8sClient.Get(context.Background(), types.NamespacedName{Name: "kubevela:reader:binding", Namespace: "default"}, &rb)
		Expect(err).Should(BeNil())

		By("test the administrator users")

		_, err = userService.CreateUser(context.TODO(), apisv1.CreateUserRequest{Name: "admin-test", Password: "test", Roles: []string{"admin"}})
		Expect(err).Should(BeNil())
		ctx = context.WithValue(context.TODO(), &apisv1.CtxKeyUser, "admin-test")
		prepare("admin-test")
		err = cloudShellService.prepareKubeConfig(ctx)
		Expect(err).Should(BeNil())
		var cm corev1.ConfigMap
		err = k8sClient.Get(context.TODO(), types.NamespacedName{Namespace: kubevelatypes.DefaultKubeVelaNS, Name: makeUserConfigName("admin-test")}, &cm)
		Expect(err).Should(BeNil())
		Expect(len(cm.Data["identity"]) > 0).Should(BeTrue())
		var identity auth.Identity
		err = yaml.Unmarshal([]byte(cm.Data["identity"]), &identity)
		Expect(err).Should(BeNil())
		Expect(utils.StringsContain(identity.Groups, utils.KubeVelaAdminGroupPrefix+"admin")).Should(BeTrue())
	})

	It("test prepare", func() {
		By("With not CRD")
		_, err = userService.CreateUser(context.TODO(), apisv1.CreateUserRequest{Name: "test", Password: "test"})
		Expect(err).Should(BeNil())
		ctx := context.WithValue(context.TODO(), &apisv1.CtxKeyUser, "test")
		_, err := cloudShellService.Prepare(ctx)
		Expect(err).ShouldNot(BeNil())
		Expect(err.Error()).Should(Equal(bcode.ErrCloudShellAddonNotEnabled.Error()))

		cloudshellCRDBytes, err := ioutil.ReadFile("./testdata/cloudshell-crd.yaml")
		Expect(err).Should(BeNil())

		crd := apiextensionsv1.CustomResourceDefinition{}
		Expect(yaml.Unmarshal(cloudshellCRDBytes, &crd)).Should(BeNil())
		Expect(k8sClient.Create(context.TODO(), &crd)).Should(BeNil())

		prepare("test")
		re, err := cloudShellService.Prepare(ctx)
		Expect(err).Should(BeNil())
		Expect(re.Status).Should(Equal(StatusPreparing))

		var cloudShell v1alpha1.CloudShell
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: kubevelatypes.DefaultKubeVelaNS, Name: makeUserCloudShellName("test")}, &cloudShell)
		Expect(err).Should(BeNil())
		cloudShell.Status.Phase = v1alpha1.PhaseReady
		cloudShell.Status.AccessURL = "10.10.1.1:8765"
		err = k8sClient.Status().Update(context.Background(), &cloudShell)
		Expect(err).Should(BeNil())

		re, err = cloudShellService.Prepare(ctx)
		Expect(err).Should(BeNil())
		Expect(re.Status).Should(Equal(StatusCompleted))

		endpoint, err := cloudShellService.GetCloudShellEndpoint(ctx)
		Expect(err).Should(BeNil())
		Expect(endpoint).Should(Equal("10.10.1.1:8765"))
	})
})
