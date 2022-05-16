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

package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"time"

	"github.com/pkg/errors"
	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/kubernetes"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/utils/pointer"

	"github.com/oam-dev/kubevela/pkg/utils/util"
)

// KubeConfigCreateOptions options for create KubeConfig
type KubeConfigCreateOptions struct {
	X509           *KubeConfigCreateX509Options
	ServiceAccount *KubeConfigCreateServiceAccountOptions
}

// KubeConfigCreateX509Options options for create X509 based KubeConfig
type KubeConfigCreateX509Options struct {
	User           string
	Groups         []string
	ExpireTime     time.Duration
	PrivateKeyBits int
}

// KubeConfigCreateServiceAccountOptions options for create ServiceAccount based KubeConfig
type KubeConfigCreateServiceAccountOptions struct {
	ServiceAccountName      string
	ServiceAccountNamespace string
}

// KubeConfigWithUserCreateOption option for setting user in KubeConfig
type KubeConfigWithUserCreateOption string

// ApplyToOptions .
func (opt KubeConfigWithUserCreateOption) ApplyToOptions(options *KubeConfigCreateOptions) {
	options.X509.User = string(opt)
}

// KubeConfigWithGroupCreateOption option for setting group in KubeConfig
type KubeConfigWithGroupCreateOption string

// ApplyToOptions .
func (opt KubeConfigWithGroupCreateOption) ApplyToOptions(options *KubeConfigCreateOptions) {
	for _, group := range options.X509.Groups {
		if group == string(opt) {
			return
		}
	}
	options.X509.Groups = append(options.X509.Groups, string(opt))
}

// KubeConfigWithServiceAccount option for setting service account in KubeConfig
type KubeConfigWithServiceAccount types.NamespacedName

// ApplyToOptions .
func (opt KubeConfigWithServiceAccount) ApplyToOptions(options *KubeConfigCreateOptions) {
	options.X509 = nil
	options.ServiceAccount = &KubeConfigCreateServiceAccountOptions{
		ServiceAccountName:      opt.Name,
		ServiceAccountNamespace: opt.Namespace,
	}
}

// KubeConfigCreateOption option for create KubeConfig
type KubeConfigCreateOption interface {
	ApplyToOptions(options *KubeConfigCreateOptions)
}

func newKubeConfigCreateOptions(options ...KubeConfigCreateOption) *KubeConfigCreateOptions {
	opts := &KubeConfigCreateOptions{
		X509: &KubeConfigCreateX509Options{
			User:           user.Anonymous,
			Groups:         []string{KubeVelaClientGroup},
			ExpireTime:     time.Hour * 24 * 365,
			PrivateKeyBits: 2048,
		},
		ServiceAccount: nil,
	}
	for _, op := range options {
		op.ApplyToOptions(opts)
	}
	return opts
}

const (
	// KubeVelaClientGroup the default group to be added to the generated X509 KubeConfig
	KubeVelaClientGroup = "kubevela:client"
)

// CreateKubeConfig create KubeConfig for users with given options
func CreateKubeConfig(ctx context.Context, cli kubernetes.Interface, cfg *clientcmdapi.Config, ioStream util.IOStreams, options ...KubeConfigCreateOption) (*clientcmdapi.Config, error) {
	opts := newKubeConfigCreateOptions(options...)
	if opts.X509 != nil {
		return createX509KubeConfig(ctx, cli, cfg, ioStream, opts.X509)
	} else if opts.ServiceAccount != nil {
		return createServiceAccountKubeConfig(ctx, cli, cfg, ioStream, opts.ServiceAccount)
	}
	return nil, errors.New("either x509 or serviceaccount must be set for creating KubeConfig")
}

func generateKubeConfig(cfg *clientcmdapi.Config, authInfo *clientcmdapi.AuthInfo, caData []byte) *clientcmdapi.Config {
	exportCfg := cfg.DeepCopy()
	exportContext := cfg.Contexts[cfg.CurrentContext].DeepCopy()
	exportCfg.Contexts = map[string]*clientcmdapi.Context{cfg.CurrentContext: exportContext}
	exportCluster := cfg.Clusters[exportContext.Cluster].DeepCopy()
	if caData != nil {
		exportCluster.CertificateAuthorityData = caData
	}
	exportCfg.Clusters = map[string]*clientcmdapi.Cluster{exportContext.Cluster: exportCluster}
	exportCfg.AuthInfos = map[string]*clientcmdapi.AuthInfo{exportContext.AuthInfo: authInfo}
	return exportCfg
}

func createX509KubeConfig(ctx context.Context, cli kubernetes.Interface, cfg *clientcmdapi.Config, ioStream util.IOStreams, opts *KubeConfigCreateX509Options) (*clientcmdapi.Config, error) {
	// generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, opts.PrivateKeyBits)
	if err != nil {
		return nil, err
	}
	keyBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	ioStream.Infof("Private key generated.")

	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   opts.User,
			Organization: opts.Groups,
		},
		SignatureAlgorithm: x509.SHA256WithRSA,
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, template, privateKey)
	if err != nil {
		return nil, err
	}
	csrPemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrBytes})
	ioStream.Infof("Certificate request generated.")

	csr := &certificatesv1.CertificateSigningRequest{}
	csr.Name = opts.User
	csr.Spec.SignerName = certificatesv1.KubeAPIServerClientSignerName
	csr.Spec.Usages = []certificatesv1.KeyUsage{certificatesv1.UsageClientAuth}
	csr.Spec.Request = csrPemBytes
	csr.Spec.ExpirationSeconds = pointer.Int32(int32(opts.ExpireTime.Seconds()))
	if csr, err = cli.CertificatesV1().CertificateSigningRequests().Create(ctx, csr, metav1.CreateOptions{}); err != nil {
		return nil, err
	}
	ioStream.Infof("Certificate signing request %s generated.", opts.User)
	defer func() {
		_ = cli.CertificatesV1().CertificateSigningRequests().Delete(ctx, csr.Name, metav1.DeleteOptions{})
	}()
	csr.Status.Conditions = append(csr.Status.Conditions, certificatesv1.CertificateSigningRequestCondition{
		Type:           certificatesv1.CertificateApproved,
		Status:         corev1.ConditionTrue,
		Reason:         "Self-generated and auto-approved by KubeVela",
		Message:        "This CSR was approved by KubeVela",
		LastUpdateTime: metav1.Now(),
	})
	if csr, err = cli.CertificatesV1().CertificateSigningRequests().UpdateApproval(ctx, csr.Name, csr, metav1.UpdateOptions{}); err != nil {
		return nil, err
	}
	ioStream.Infof("Certificate signing request %s approved.", opts.User)
	if err = wait.Poll(time.Second, time.Minute, func() (done bool, err error) {
		if csr, err = cli.CertificatesV1().CertificateSigningRequests().Get(ctx, opts.User, metav1.GetOptions{}); err != nil {
			return false, err
		}
		if csr.Status.Certificate == nil {
			return false, nil
		}
		return true, nil
	}); err != nil {
		return nil, err
	}
	ioStream.Infof("Signed certificate retrieved.")

	return generateKubeConfig(cfg, &clientcmdapi.AuthInfo{
		ClientKeyData:         keyBytes,
		ClientCertificateData: csr.Status.Certificate,
	}, nil), nil
}

func createServiceAccountKubeConfig(ctx context.Context, cli kubernetes.Interface, cfg *clientcmdapi.Config, ioStream util.IOStreams, opts *KubeConfigCreateServiceAccountOptions) (*clientcmdapi.Config, error) {
	sa, err := cli.CoreV1().ServiceAccounts(opts.ServiceAccountNamespace).Get(ctx, opts.ServiceAccountName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	ioStream.Infof("ServiceAccount %s/%s found.", opts.ServiceAccountNamespace, opts.ServiceAccountName)
	if len(sa.Secrets) == 0 {
		return nil, errors.Errorf("no secret found in serviceaccount %s/%s", opts.ServiceAccountNamespace, opts.ServiceAccountName)
	}
	secretKey := sa.Secrets[0]
	if secretKey.Namespace == "" {
		secretKey.Namespace = sa.Namespace
	}
	secret, err := cli.CoreV1().Secrets(secretKey.Namespace).Get(ctx, secretKey.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	ioStream.Infof("ServiceAccount secret %s/%s found.", secretKey.Namespace, secret.Name)
	if len(secret.Data["token"]) == 0 {
		return nil, errors.Errorf("no token found in secret %s/%s", secret.Namespace, secret.Name)
	}
	ioStream.Infof("ServiceAccount token found.")
	return generateKubeConfig(cfg, &clientcmdapi.AuthInfo{
		Token: string(secret.Data["token"]),
	}, secret.Data["ca.crt"]), nil
}
