package types

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// Args is args for controller-runtime client
type Args struct {
	Config *rest.Config
	Schema *runtime.Scheme
	Client client.Client
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

// GetClient get client if exist
func (a *Args) GetClient() (client.Client, error) {
	if a.Config == nil {
		if err := a.SetConfig(); err != nil {
			return nil, err
		}
	}
	if a.Client != nil {
		return a.Client, nil
	}
	newClient, err := client.New(a.Config, client.Options{Scheme: a.Schema})
	if err != nil {
		return nil, err
	}
	a.Client = newClient
	return a.Client, nil
}
