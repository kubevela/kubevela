package common

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/oam-dev/kubevela/pkg/dsl/definition"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
)

// Args is args for controller-runtime client
type Args struct {
	Config *rest.Config
	Schema *runtime.Scheme
	Client client.Client
	dm     discoverymapper.DiscoveryMapper
	pd     *definition.PackageDiscover
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

// GetDiscoveryMapper get discoveryMapper client if exist, create if not exist.
func (a *Args) GetDiscoveryMapper() (discoverymapper.DiscoveryMapper, error) {
	if a.Config == nil {
		if err := a.SetConfig(); err != nil {
			return nil, err
		}
	}
	if a.dm != nil {
		return a.dm, nil
	}
	dm, err := discoverymapper.New(a.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create CRD discovery client %w", err)
	}
	a.dm = dm
	return dm, nil
}

// GetPackageDiscover get PackageDiscover client if exist, create if not exist.
func (a *Args) GetPackageDiscover() (*definition.PackageDiscover, error) {
	if a.Config == nil {
		if err := a.SetConfig(); err != nil {
			return nil, err
		}
	}
	if a.pd != nil {
		return a.pd, nil
	}
	pd, err := definition.NewPackageDiscover(a.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create CRD discovery for CUE package client %w", err)
	}
	a.pd = pd
	return pd, nil
}
