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

package definition

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/token"
	"cuelang.org/go/encoding/jsonschema"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

// PackageDiscover defines the inner CUE packages loaded from K8s cluster
type PackageDiscover struct {
	velaBuiltinPackages []*build.Instance
	pkgKinds            map[string][]string
	mutex               sync.RWMutex
	client              *rest.RESTClient
}

// NewPackageDiscover will create a PackageDiscover client with the K8s config file.
func NewPackageDiscover(config *rest.Config) (*PackageDiscover, error) {
	client, err := getClusterOpenAPIClient(config)
	if err != nil {
		return nil, err
	}
	pd := &PackageDiscover{
		client:   client,
		pkgKinds: make(map[string][]string),
	}
	if err = pd.RefreshKubePackagesFromCluster(); err != nil {
		return nil, err
	}
	return pd, nil
}

// ImportBuiltinPackagesFor will add KubeVela built-in packages into your CUE instance
func (pd *PackageDiscover) ImportBuiltinPackagesFor(bi *build.Instance) {
	pd.mutex.RLock()
	defer pd.mutex.RUnlock()
	bi.Imports = append(bi.Imports, pd.velaBuiltinPackages...)
}

// RefreshKubePackagesFromCluster will use K8s client to load/refresh all K8s open API as a reference kube package using in template
func (pd *PackageDiscover) RefreshKubePackagesFromCluster() error {
	body, err := pd.client.Get().AbsPath("/openapi/v2").Do(context.Background()).Raw()
	if err != nil {
		return err
	}
	return pd.addKubeCUEPackagesFromCluster(string(body))
}

// Exist checks if the GVK exists in the built-in packages
func (pd *PackageDiscover) Exist(gvk metav1.GroupVersionKind) bool {
	// package name equals to importPath
	importPath := genPackageName(gvk.Group, gvk.Version)
	pd.mutex.RLock()
	defer pd.mutex.RUnlock()
	pkgKinds := pd.pkgKinds[importPath]
	for _, k := range pkgKinds {
		if k == gvk.Kind {
			return true
		}
	}
	return false
}

// mount will mount the new parsed package into PackageDiscover built-in packages
func (pd *PackageDiscover) mount(pkg *pkgInstance, pkgKinds []string) {
	pd.mutex.Lock()
	defer pd.mutex.Unlock()
	for i, p := range pd.velaBuiltinPackages {
		if p.ImportPath == pkg.ImportPath {
			pd.velaBuiltinPackages[i] = pkg.Instance
			return
		}
	}
	pd.pkgKinds[pkg.ImportPath] = pkgKinds
	pd.velaBuiltinPackages = append(pd.velaBuiltinPackages, pkg.Instance)
}

func (pd *PackageDiscover) addKubeCUEPackagesFromCluster(apiSchema string) error {
	var r cue.Runtime
	oaInst, err := r.Compile("-", apiSchema)
	if err != nil {
		return err
	}
	kinds := map[string]metav1.GroupVersionKind{}
	pathValue := oaInst.Value().Lookup("paths")
	if pathValue.Exists() {
		if st, err := pathValue.Struct(); err == nil {
			iter := st.Fields()
			for iter.Next() {
				gvk := iter.Value().Lookup("post",
					"x-kubernetes-group-version-kind")
				if gvk.Exists() {
					if v, err := getGVK(gvk); err == nil {
						kinds["#"+v.Kind] = v
					}
				}
			}
		}
	}
	oaFile, err := jsonschema.Extract(oaInst, &jsonschema.Config{
		Root: "#/definitions",
		Map:  openAPIMapping,
	})
	if err != nil {
		return err
	}
	packages := make(map[string]*pkgInstance)
	groupKinds := make(map[string][]string)

	for k, v := range kinds {
		apiVersion := v.Version
		if v.Group != "" {
			apiVersion = v.Group + "/" + apiVersion
		}
		def := fmt.Sprintf(`%s: {
kind: "%s"
apiVersion: "%s",
}`, k, v.Kind, apiVersion)
		pkgName := genPackageName(v.Group, v.Version)
		pkg, ok := packages[pkgName]
		if !ok {
			pkg = newPackage(pkgName)
		}

		mykinds := groupKinds[pkgName]
		mykinds = append(mykinds, v.Kind)

		if err := pkg.AddFile(k, def); err != nil {
			return err
		}
		packages[pkgName] = pkg
		groupKinds[pkgName] = mykinds
	}
	for name, pkg := range packages {
		pkg.processOpenAPIFile(oaFile)
		if err = pkg.AddSyntax(oaFile); err != nil {
			return err
		}
		pd.mount(pkg, groupKinds[name])
	}
	return nil
}

func genPackageName(group, version string) string {
	res := []string{"kube"}
	if group != "" {
		res = append(res, group)
	}
	// version should never be empty
	res = append(res, version)
	return strings.Join(res, "/")
}

func setDiscoveryDefaults(config *rest.Config) {
	config.APIPath = ""
	config.GroupVersion = nil
	if config.Timeout == 0 {
		config.Timeout = 32 * time.Second
	}
	if config.Burst == 0 && config.QPS < 100 {
		// discovery is expected to be bursty, increase the default burst
		// to accommodate looking up resource info for many API groups.
		// matches burst set by ConfigFlags#ToDiscoveryClient().
		// see https://issue.k8s.io/86149
		config.Burst = 100
	}
	codec := runtime.NoopEncoder{Decoder: clientgoscheme.Codecs.UniversalDecoder()}
	config.NegotiatedSerializer = serializer.NegotiatedSerializerWrapper(runtime.SerializerInfo{Serializer: codec})
	if len(config.UserAgent) == 0 {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}
}

func getClusterOpenAPIClient(config *rest.Config) (*rest.RESTClient, error) {
	copyConfig := *config
	setDiscoveryDefaults(&copyConfig)
	return rest.UnversionedRESTClientFor(&copyConfig)
}

func openAPIMapping(pos token.Pos, a []string) ([]ast.Label, error) {
	if len(a) < 2 {
		return nil, errors.New("openAPIMapping format invalid")
	}

	spl := strings.Split(a[1], ".")
	name := spl[len(spl)-1]

	if name == "JSONSchemaProps" && pos != token.NoPos {
		return []ast.Label{ast.NewIdent("_")}, nil
	}
	return []ast.Label{ast.NewIdent("#" + name)}, nil
}

type pkgInstance struct {
	*build.Instance
}

func newPackage(name string) *pkgInstance {
	return &pkgInstance{
		&build.Instance{
			PkgName:    name,
			ImportPath: name,
		},
	}
}

func (pkg *pkgInstance) processOpenAPIFile(f *ast.File) {
	ast.Walk(f, func(node ast.Node) bool {
		if st, ok := node.(*ast.StructLit); ok {
			hasEllipsis := false
			for index, elt := range st.Elts {
				if _, isEllipsis := elt.(*ast.Ellipsis); isEllipsis {
					if hasEllipsis {
						st.Elts = st.Elts[:index]
						return true
					}
					if index > 0 {
						st.Elts = st.Elts[:index]
						return true
					}
					hasEllipsis = true
				}
			}
		}
		return true
	}, nil)

	for _, decl := range f.Decls {
		if field, ok := decl.(*ast.Field); ok {
			if val, ok := field.Value.(*ast.Ident); ok && val.Name == "string" {
				field.Value = ast.NewBinExpr(token.OR, ast.NewIdent("int"), ast.NewIdent("string"))
			}
		}
	}
}

func getGVK(v cue.Value) (metav1.GroupVersionKind, error) {
	ret := metav1.GroupVersionKind{}
	var err error
	ret.Group, err = v.Lookup("group").String()
	if err != nil {
		return ret, err
	}
	ret.Version, err = v.Lookup("version").String()
	if err != nil {
		return ret, err
	}
	ret.Kind, err = v.Lookup("kind").String()
	return ret, err
}
