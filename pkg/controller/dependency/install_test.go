package dependency

import (
	"fmt"
	"testing"

	"github.com/pkg/errors"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
)

var (
	scheme  *runtime.Scheme
	errHelm = fmt.Errorf("err")
)

func init() {
	scheme = runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = clientgoscheme.AddToScheme(scheme)
	_ = crdv1.AddToScheme(scheme)
}

func TestSuccessfulInstall(t *testing.T) {
	helmInstallFunc = successHelmInstall
	if err := installHelmChart([]byte("{}"), log); err != nil {
		t.Errorf("failed to install dependency error: %v", err)
	}
}

func TestFailedInstall(t *testing.T) {
	helmInstallFunc = failedHelmInstall
	if err := installHelmChart([]byte("{}"), log); errors.Cause(err) != errHelm {
		t.Errorf("failed to get install dependency error: %v", err)
	}
}

func failedHelmInstall(ioStreams cmdutil.IOStreams, c types.Chart) error {
	return errHelm
}

func successHelmInstall(ioStreams cmdutil.IOStreams, c types.Chart) error {
	return nil
}
