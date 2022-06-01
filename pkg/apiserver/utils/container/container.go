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

package container

import (
	"github.com/barnettZQG/inject"
	"helm.sh/helm/v3/pkg/time"

	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
)

// NewContainer new a IoC container
func NewContainer() *Container {
	return &Container{
		graph: inject.Graph{},
	}
}

// Container the IoC container
type Container struct {
	graph inject.Graph
}

// Provides provide some beans with default name
func (c *Container) Provides(beans ...interface{}) error {
	for _, bean := range beans {
		if err := c.graph.Provide(&inject.Object{Value: bean}); err != nil {
			return err
		}
	}
	return nil
}

// ProvideWithName provide the bean with name
func (c *Container) ProvideWithName(name string, bean interface{}) error {
	return c.graph.Provide(&inject.Object{Name: name, Value: bean})
}

// Populate populate dependency fields for all beans.
// this function must be called after providing all beans
func (c *Container) Populate() error {
	start := time.Now()
	defer func() {
		log.Logger.Infof("populate the bean container take time %s", time.Now().Sub(start))
	}()
	return c.graph.Populate()
}
