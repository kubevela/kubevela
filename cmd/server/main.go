package main

import (
	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
	vela "github.com/oam-dev/kubevela/apis/core/v1alpha2"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"os"
)


var (
	scheme = runtime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = core.AddToScheme(scheme)
	_ = crdv1.AddToScheme(scheme)
	_ = vela.AddToScheme(scheme)
}
func main() {
	command:=newServerCommand()
	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}
