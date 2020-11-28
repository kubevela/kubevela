package main

import (
	"os"

	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	vela "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = crdv1.AddToScheme(scheme)
	_ = vela.SchemeBuilder.AddToScheme(scheme)
}
func main() {
	command := newServerCommand()
	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}
