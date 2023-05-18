/*
Copyright 2023 The KubeVela Authors.

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

package provider

import (
	"fmt"
	goast "go/ast"
	"io"
	"strings"

	cueast "cuelang.org/go/cue/ast"
	cuetoken "cuelang.org/go/cue/token"
	"golang.org/x/tools/go/packages"

	"github.com/oam-dev/kubevela/references/cuegen"
)

const (
	typeProviderFnMap          = "map[string]github.com/kubevela/pkg/cue/cuex/runtime.ProviderFn"
	typeProvidersParamsPrefix  = "github.com/kubevela/pkg/cue/cuex/providers.Params"
	typeProvidersReturnsPrefix = "github.com/kubevela/pkg/cue/cuex/providers.Returns"
)

const (
	doKey       = "do"
	providerKey = "provider"
)

type provider struct {
	name    string
	params  string
	returns string
	do      string
}

// Options is options of generation
type Options struct {
	File     string                 // Go file path
	Writer   io.Writer              // target writer
	Types    map[string]cuegen.Type // option cuegen.WithTypes
	Nullable bool                   // option cuegen.WithNullable
}

// Generate generates cue provider from Go struct
func Generate(opts Options) (rerr error) {
	g, err := cuegen.NewGenerator(opts.File)
	if err != nil {
		return err
	}

	// make options
	genOpts := make([]cuegen.Option, 0)
	// any types
	genOpts = append(genOpts, cuegen.WithTypes(opts.Types))
	// nullable
	if opts.Nullable {
		genOpts = append(genOpts, cuegen.WithNullable())
	}
	// type filter
	genOpts = append(genOpts, cuegen.WithTypeFilter(func(spec *goast.TypeSpec) bool {
		typ := g.Package().TypesInfo.TypeOf(spec.Type)
		// only process provider params and returns.
		if strings.HasPrefix(typ.String(), typeProvidersParamsPrefix) ||
			strings.HasPrefix(typ.String(), typeProvidersReturnsPrefix) {
			return true
		}

		return false
	}))

	decls, err := g.Generate(genOpts...)
	if err != nil {
		return err
	}

	providers, err := extractProviders(g.Package())
	if err != nil {
		return err
	}
	newDecls, err := modifyDecls(g.Package().Name, decls, providers)
	if err != nil {
		return err
	}

	return g.Format(opts.Writer, newDecls)
}

// extractProviders extracts the providers from map[string]cuexruntime.ProviderFn
func extractProviders(pkg *packages.Package) (providers []provider, rerr error) {
	var (
		providersMap *goast.CompositeLit
		ok           bool
	)
	// extract provider def map
	for k, v := range pkg.TypesInfo.Types {
		if v.Type.String() != typeProviderFnMap {
			continue
		}

		if providersMap, ok = k.(*goast.CompositeLit); ok {
			break
		}
	}

	if providersMap == nil {
		return nil, fmt.Errorf("no provider function map found like '%s'", typeProviderFnMap)
	}

	defer recoverAssert(&rerr, "extract providers")

	for _, e := range providersMap.Elts {
		pair := e.(*goast.KeyValueExpr)
		doName := pair.Key.(*goast.BasicLit)
		value := pair.Value.(*goast.CallExpr)

		indices := value.Fun.(*goast.IndexListExpr)
		params := indices.Indices[0].(*goast.Ident)  // params struct name
		returns := indices.Indices[1].(*goast.Ident) // returns struct name

		do := value.Args[0].(*goast.Ident)

		providers = append(providers, provider{
			name:    doName.Value,
			params:  params.Name,
			returns: returns.Name,
			do:      do.Name,
		})
	}

	return providers, nil
}

// modifyDecls re-generates cue ast decls of providers.
func modifyDecls(provider string, old []cuegen.Decl, providers []provider) (decls []cuegen.Decl, rerr error) {
	defer recoverAssert(&rerr, "modify decls failed")

	// map[StructName]StructLit
	mapping := make(map[string]cueast.Expr)
	for _, decl := range old {
		if t, ok := decl.(*cuegen.Struct); ok {
			mapping[t.Name] = t.Expr
		}
	}

	providerField := &cueast.Field{
		Label: cuegen.Ident(providerKey, true),
		Value: cueast.NewString(provider),
	}

	for _, p := range providers {
		params := mapping[p.params].(*cueast.StructLit).Elts
		returns := mapping[p.returns].(*cueast.StructLit).Elts

		doField := &cueast.Field{
			Label: cuegen.Ident(doKey, true),
			Value: cueast.NewLit(cuetoken.STRING, p.name), // p.name has contained double quotes
		}

		pdecls := []cueast.Decl{doField, providerField}
		pdecls = append(pdecls, params...)
		pdecls = append(pdecls, returns...)

		decls = append(decls, &cuegen.Struct{CommonFields: cuegen.CommonFields{
			Expr: &cueast.StructLit{
				Elts: pdecls,
			},
			Name: "#" + p.do,
			Pos:  cuetoken.NewSection.Pos(),
		}})
	}

	return decls, nil
}

// recoverAssert captures panic caused by invalid type assertion or out of range index,
// so we don't need to check each type assertion and index
func recoverAssert(err *error, msg string) {
	if r := recover(); r != nil {
		*err = fmt.Errorf("%s: panic: %v", r, msg)
	}
}
