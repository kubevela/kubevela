/*
Copyright 2026 The KubeVela Authors.

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

// Package velaconfig provides a CueX provider that reads a KubeVela Config's
// properties.
//
// It goes through pkg/config's read path rather than the Secret a Config
// currently lives in, for two reasons: KEP-2.18 plans to graduate Config to a
// CRD, and that path already refuses a Config marked sensitive - a property
// worth inheriting rather than reimplementing.
package velaconfig

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/kubevela/pkg/cue/cuex/providers"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/runtime"

	di "github.com/oam-dev/kubevela/pkg/registry"
)

// DefaultNamespace is where Configs live unless one is named.
const DefaultNamespace = "vela-system"

// Reader reads a Config.
//
// The interface lives here and the implementation does not, because reaching
// pkg/config from this package closes a cycle - the same one the registry
// provider hits. cmd/core/app/bootstrap.go registers the implementation.
type Reader interface {
	ReadConfig(ctx context.Context, namespace, name string) (*ReadResult, error)
}

// ReadVars are the inputs to a Config read.
type ReadVars struct {
	Name string `json:"name"`
	// Namespace defaults to DefaultNamespace.
	Namespace string `json:"namespace,omitempty"`
}

// TemplateRef names the ConfigTemplate a Config was rendered from. It is what
// tells a consumer which contract the properties satisfy.
type TemplateRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// ObjectRef identifies one object a Config's template produced.
//
// Deliberately a reference and not the object: a Config's outputs are routinely
// Secrets, so returning their contents here would make every Config a way to
// read credentials. A definition that wants the objects themselves fetches them
// with kube.#Get, which keeps that decision visible in the definition and
// subject to the controller's own RBAC.
type ObjectRef struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace,omitempty"`
}

// ReadResult is what a Config yields when read.
//
// The shape follows a ConfigTemplate's own: `output` is the single Secret a
// template renders for the Config itself, and `outputs` are the additional
// objects it declares, each under the name the template gave it.
//
// Outputs are keyed by that name rather than positionally because position is
// not an identity. pkg/config builds the stored reference list by ranging a Go
// map, so its order is whatever that iteration produced and can change when the
// Config is updated - `outputs[0]` would silently start meaning something else.
type ReadResult struct {
	Properties map[string]interface{} `json:"properties"`
	Template   TemplateRef            `json:"template"`
	Output     ObjectRef              `json:"output"`
	Outputs    map[string]ObjectRef   `json:"outputs"`
}

// ReadParams is the params for a Config read.
type ReadParams providers.Params[ReadVars]

// ReadReturns is the returns for a Config read.
type ReadReturns providers.Returns[ReadResult]

// Read fetches one Config's properties.
func Read(ctx context.Context, params *ReadParams) (*ReadReturns, error) {
	in := params.Params
	if in.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	namespace := in.Namespace
	if namespace == "" {
		namespace = DefaultNamespace
	}

	reader, ok := di.Get[Reader]()
	if !ok {
		return nil, fmt.Errorf("no config reader is registered; " +
			"cmd/core/app/bootstrap.go should register one")
	}

	// Errors are returned rather than swallowed. A Config that is missing, or one
	// marked sensitive, must fail the source loudly - resolving to an empty set
	// of properties would be cached, and the emptiness would then look like data.
	res, err := reader.ReadConfig(ctx, namespace, in.Name)
	if err != nil {
		return nil, fmt.Errorf("config %q in %q: %w", in.Name, namespace, err)
	}
	// A Config with no extra outputs must still present an empty map rather
	// than null, so a template can range over it unconditionally.
	if res.Outputs == nil {
		res.Outputs = map[string]ObjectRef{}
	}

	return &ReadReturns{Returns: *res}, nil
}

// ProviderName is the name a template references this provider by.
const ProviderName = "velaconfig"

//go:embed velaconfig.cue
var template string

// Package is the provider package, registered with the compilers that offer it.
var Package = runtime.Must(cuexruntime.NewInternalPackage(ProviderName, template, map[string]cuexruntime.ProviderFn{
	"read": cuexruntime.GenericProviderFn[ReadParams, ReadReturns](Read),
}))
