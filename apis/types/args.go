package types

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// Args is args for controller-runtime client
type Args struct {
	Config *rest.Config
	Schema *runtime.Scheme
}

// SetConfig insert kubeconfig into Args
func (a *Args) SetConfig() error {
	restConf, err := config.GetConfig()
	if err != nil {
		return err
	}
	a.Config = restConf
	return nil
}
