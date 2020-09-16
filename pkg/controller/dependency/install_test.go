package dependency

import (
	"fmt"
	"testing"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/api/v1alpha1"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
)

var (
	scheme         *runtime.Scheme
	errHelm        = fmt.Errorf("err")
	velaConfigBase = v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      VelaConfigName,
			Namespace: types.DefaultAppNamespace,
			Labels:    map[string]string{"vela": "dependency"},
		},
		Data: map[string]string{
			"certificates.cert-manager.io": `{
				"repo": "jetstack",
				"urL": "https://charts.jetstack.io",
				"name": "cert-manager",
				"version": "v1.0.0"
			}`,
			"prometheuses.monitoring.coreos.com": `{
				"repo": "jetstack",
				"urL": "https://charts.jetstack.io",
				"name": "cert-manager",
				"version": "v1.0.0"
			}`,
		},
	}
)

func init() {
	scheme = runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)
	_ = crdv1.AddToScheme(scheme)
}

func TestSuccessfulInstall(t *testing.T) {
	helmInstallFunc = successHelmInstall
	velaConfig := velaConfigBase.DeepCopy()
	crd := crdv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "certificates.cert-manager.io",
		},
	}
	client := fake.NewFakeClientWithScheme(scheme, velaConfig, &crd)
	if err := Install(client); err != nil {
		t.Errorf("failed to install dependency error: %v", err)
	}
}

func TestFailedInstall(t *testing.T) {
	helmInstallFunc = failedHelmInstall
	velaConfig := velaConfigBase.DeepCopy()
	client := fake.NewFakeClientWithScheme(scheme, velaConfig)
	if err := Install(client); errors.Cause(err) != errHelm {
		t.Errorf("failed to get install dependency error: %v", err)
	}
}

func TestNoNeedToInstall(t *testing.T) {
	// set it to fail as we won't get to it
	helmInstallFunc = failedHelmInstall
	velaConfig := velaConfigBase.DeepCopy()
	certManagerCRD := crdv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "certificates.cert-manager.io",
		},
	}
	prometheusCRD := crdv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "prometheuses.monitoring.coreos.com",
		},
	}
	client := fake.NewFakeClientWithScheme(scheme, velaConfig, &certManagerCRD, &prometheusCRD)
	if err := Install(client); err != nil {
		t.Errorf("failed to install dependency error: %v", err)
	}
}

func failedHelmInstall(ioStreams cmdutil.IOStreams, c types.Chart) error {
	return errHelm
}

func successHelmInstall(ioStreams cmdutil.IOStreams, c types.Chart) error {
	return nil
}
