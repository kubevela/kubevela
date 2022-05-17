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
	"fmt"
	"io"
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
)

// KubeConfigGenerateOptions options for create KubeConfig
type KubeConfigGenerateOptions struct {
	X509           *KubeConfigGenerateX509Options
	ServiceAccount *KubeConfigGenerateServiceAccountOptions
}

// KubeConfigGenerateX509Options options for create X509 based KubeConfig
type KubeConfigGenerateX509Options struct {
	User           string
	Groups         []string
	ExpireTime     time.Duration
	PrivateKeyBits int
}

// KubeConfigGenerateServiceAccountOptions options for create ServiceAccount based KubeConfig
type KubeConfigGenerateServiceAccountOptions struct {
	ServiceAccountName      string
	ServiceAccountNamespace string
}

// KubeConfigWithUserGenerateOption option for setting user in KubeConfig
type KubeConfigWithUserGenerateOption string

// ApplyToOptions .
func (opt KubeConfigWithUserGenerateOption) ApplyToOptions(options *KubeConfigGenerateOptions) {
	options.X509.User = string(opt)
}

// KubeConfigWithGroupGenerateOption option for setting group in KubeConfig
type KubeConfigWithGroupGenerateOption string

// ApplyToOptions .
func (opt KubeConfigWithGroupGenerateOption) ApplyToOptions(options *KubeConfigGenerateOptions) {
	for _, group := range options.X509.Groups {
		if group == string(opt) {
			return
		}
	}
	options.X509.Groups = append(options.X509.Groups, string(opt))
}

// KubeConfigWithServiceAccountGenerateOption option for setting service account in KubeConfig
type KubeConfigWithServiceAccountGenerateOption types.NamespacedName

// ApplyToOptions .
func (opt KubeConfigWithServiceAccountGenerateOption) ApplyToOptions(options *KubeConfigGenerateOptions) {
	options.X509 = nil
	options.ServiceAccount = &KubeConfigGenerateServiceAccountOptions{
		ServiceAccountName:      opt.Name,
		ServiceAccountNamespace: opt.Namespace,
	}
}

// KubeConfigGenerateOption option for create KubeConfig
type KubeConfigGenerateOption interface {
	ApplyToOptions(options *KubeConfigGenerateOptions)
}

func newKubeConfigGenerateOptions(options ...KubeConfigGenerateOption) *KubeConfigGenerateOptions {
	opts := &KubeConfigGenerateOptions{
		X509: &KubeConfigGenerateX509Options{
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

// GenerateKubeConfig generate KubeConfig for users with given options.
func GenerateKubeConfig(ctx context.Context, cli kubernetes.Interface, cfg *clientcmdapi.Config, writer io.Writer, options ...KubeConfigGenerateOption) (*clientcmdapi.Config, error) {
	opts := newKubeConfigGenerateOptions(options...)
	if opts.X509 != nil {
		return generateX509KubeConfig(ctx, cli, cfg, writer, opts.X509)
	} else if opts.ServiceAccount != nil {
		return generateServiceAccountKubeConfig(ctx, cli, cfg, writer, opts.ServiceAccount)
	}
	return nil, errors.New("either x509 or serviceaccount must be set for creating KubeConfig")
}

func genKubeConfig(cfg *clientcmdapi.Config, authInfo *clientcmdapi.AuthInfo, caData []byte) *clientcmdapi.Config {
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

func generateX509KubeConfig(ctx context.Context, cli kubernetes.Interface, cfg *clientcmdapi.Config, writer io.Writer, opts *KubeConfigGenerateX509Options) (*clientcmdapi.Config, error) {
	// generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, opts.PrivateKeyBits)
	if err != nil {
		return nil, err
	}
	keyBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	_, _ = fmt.Fprintf(writer, "Private key generated.\n")

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
	_, _ = fmt.Fprintf(writer, "Certificate request generated.\n")

	csr := &certificatesv1.CertificateSigningRequest{}
	csr.Name = opts.User
	csr.Spec.SignerName = certificatesv1.KubeAPIServerClientSignerName
	csr.Spec.Usages = []certificatesv1.KeyUsage{certificatesv1.UsageClientAuth}
	csr.Spec.Request = csrPemBytes
	csr.Spec.ExpirationSeconds = pointer.Int32(int32(opts.ExpireTime.Seconds()))
	if csr, err = cli.CertificatesV1().CertificateSigningRequests().Create(ctx, csr, metav1.CreateOptions{}); err != nil {
		return nil, err
	}
	_, _ = fmt.Fprintf(writer, "Certificate signing request %s generated.\n", opts.User)
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
	_, _ = fmt.Fprintf(writer, "Certificate signing request %s approved.\n", opts.User)
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
	_, _ = fmt.Fprintf(writer, "Signed certificate retrieved.\n")

	return genKubeConfig(cfg, &clientcmdapi.AuthInfo{
		ClientKeyData:         keyBytes,
		ClientCertificateData: csr.Status.Certificate,
	}, nil), nil
}

func generateServiceAccountKubeConfig(ctx context.Context, cli kubernetes.Interface, cfg *clientcmdapi.Config, writer io.Writer, opts *KubeConfigGenerateServiceAccountOptions) (*clientcmdapi.Config, error) {
	sa, err := cli.CoreV1().ServiceAccounts(opts.ServiceAccountNamespace).Get(ctx, opts.ServiceAccountName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	_, _ = fmt.Fprintf(writer, "ServiceAccount %s/%s found.\n", opts.ServiceAccountNamespace, opts.ServiceAccountName)
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
	_, _ = fmt.Fprintf(writer, "ServiceAccount secret %s/%s found.\n", secretKey.Namespace, secret.Name)
	if len(secret.Data["token"]) == 0 {
		return nil, errors.Errorf("no token found in secret %s/%s", secret.Namespace, secret.Name)
	}
	_, _ = fmt.Fprintf(writer, "ServiceAccount token found.\n")
	return genKubeConfig(cfg, &clientcmdapi.AuthInfo{
		Token: string(secret.Data["token"]),
	}, secret.Data["ca.crt"]), nil
}
