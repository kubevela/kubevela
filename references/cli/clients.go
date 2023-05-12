/*
Copyright 2021 The KubeVela Authors.

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

// clients.go is a file that contains the clients used by the CLI. We needn't create clients in every command.

package cli

import (
	"github.com/kubevela/workflow/pkg/cue/packages"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// Tool is a tool object that can be initialized
type Tool string

// OptionTree is a tree of options, it is used to describe which commands need which options
type OptionTree struct {
	Option
	Nodes map[string]OptionTree
}

// Option is a option of one single command, show which objects must be initialized
type Option struct {
	// Must means some objects must be initialized
	Must    []Tool
	MustAll bool
}

const (
	// DynamicClient is the option of dynamic client
	DynamicClient Tool = "kube"
	// Config is the option of rest config
	Config Tool = "config"
	// PackageDiscover is the option of package discover
	PackageDiscover Tool = "packageDiscover"
	// DiscoveryMapper is the option of discovery mapper
	DiscoveryMapper Tool = "discoveryMapper"
)

var (
	// Four common client objects as global variables, they are initialized when the CLI starts accordingly.
	cfg *rest.Config
	cli client.Client
	dm  discoverymapper.DiscoveryMapper
	pd  *packages.PackageDiscover

	mustAll = Option{
		MustAll: true,
	}
	mustClient = OptionTree{
		Option: Option{
			Must: []Tool{DynamicClient},
		},
	}

	err error

	// VelaOption is the option tree of vela CLI
	// In this tree, the lower node takes precedence over the higher one
	VelaOption = OptionTree{
		Nodes: func() map[string]OptionTree {
			res := make(map[string]OptionTree)
			needClient := []string{"init", "ql", "port-forward", "uischema", "workflow", "revision", "system", "status", "cluster"}
			all := []string{"trait", "component", "log", "revision", "live-diff", "debug", "up"}
			for _, v := range needClient {
				res[v] = OptionTree{
					Option: Option{
						Must: []Tool{DynamicClient},
					},
				}
			}
			for _, v := range all {
				res[v] = OptionTree{
					Option: mustAll,
				}
			}
			res["addon"] = OptionTree{
				Nodes: map[string]OptionTree{
					"enable":   mustClient,
					"disable":  mustClient,
					"list":     mustClient,
					"status":   mustClient,
					"register": mustClient,
					"upgrade":  mustClient,
				},
			}
			res["def"] = OptionTree{
				Nodes: map[string]OptionTree{
					"get":   mustClient,
					"apply": mustClient,
					"vet":   mustClient,
					"edit":  mustClient,
				},
			}
			return res
		}(),
	}
	// All is all options
	All = []Tool{
		DynamicClient,
		Config,
		PackageDiscover,
		DiscoveryMapper,
	}
)

var (
	mustInitFunc = map[Tool]func(){
		Config: func() {
			cfg = common.Config()
		},
		DynamicClient: func() {
			cli = common.DynamicClient()
		},
		PackageDiscover: func() {
			pd = common.PackageDiscover()
		},
		DiscoveryMapper: func() {
			dm = common.DiscoveryMapper()
		},
	}
	tryInitFunc = map[Tool]func(){
		Config: func() {
			cfg = common.ConfigOrNil()
		},
		DynamicClient: func() {
			cli = common.DynamicClientOrNil()
		},
		PackageDiscover: func() {
			pd = common.PackageDiscoverOrNil()
		},
		DiscoveryMapper: func() {
			dm = common.DiscoveryMapperOrNil()
		},
	}
)

func (o Option) init() {
	if o.MustAll {
		for _, opt := range All {
			if mustInitFunc[opt] != nil {
				mustInitFunc[opt]()
			}
		}
		return
	}
	for _, opt := range o.Must {
		if mustInitFunc[opt] != nil {
			mustInitFunc[opt]()
		}
	}
	for _, opt := range All {
		if tryInitFunc[opt] != nil {
			tryInitFunc[opt]()
		}
	}

}

func parseOption(command []string) Option {
	var opt = VelaOption
	for _, cmd := range command[1:] {
		subOpt, ok := opt.Nodes[cmd]
		if !ok {
			break
		}
		opt = subOpt
	}
	return opt.Option
}

// InitClients is always called on the beginning of every command
func InitClients(command []string) {
	// comand is like ["vela", "status"]
	parseOption(command).init()
}
