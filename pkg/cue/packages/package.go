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

package packages

import (
	"fmt"
	"path/filepath"
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

	"github.com/oam-dev/kubevela/pkg/stdlib"
)

const (
	// BuiltinPackageDomain Specify the domain of the built-in package
	BuiltinPackageDomain = "kube"
	// K8sResourcePrefix Indicates that the definition comes from kubernetes
	K8sResourcePrefix = "io_k8s_api_"

	// ParseJSONSchemaErr describes the error that occurs when cue parses json
	ParseJSONSchemaErr ParseErrType = "parse json schema of k8s crds error"
)

// PackageDiscover defines the inner CUE packages loaded from K8s cluster
type PackageDiscover struct {
	velaBuiltinPackages []*build.Instance
	pkgKinds            map[string][]VersionKind
	mutex               sync.RWMutex
	client              *rest.RESTClient
}

// VersionKind contains the resource metadata and reference name
type VersionKind struct {
	DefinitionName string
	APIVersion     string
	Kind           string
}

// ParseErrType represents the type of CUEParseError
type ParseErrType string

// CUEParseError describes an error when CUE parse error
type CUEParseError struct {
	err     error
	errType ParseErrType
}

// Error implements the Error interface.
func (cueErr CUEParseError) Error() string {
	return fmt.Sprintf("%s: %s", cueErr.errType, cueErr.err.Error())
}

// IsCUEParseErr returns true if the specified error is CUEParseError type.
func IsCUEParseErr(err error) bool {
	return errors.As(err, &CUEParseError{})
}

// NewPackageDiscover will create a PackageDiscover client with the K8s config file.
func NewPackageDiscover(config *rest.Config) (*PackageDiscover, error) {
	client, err := getClusterOpenAPIClient(config)
	if err != nil {
		return nil, err
	}
	pd := &PackageDiscover{
		client:   client,
		pkgKinds: make(map[string][]VersionKind),
	}
	if err = pd.RefreshKubePackagesFromCluster(); err != nil {
		return pd, err
	}
	return pd, nil
}

// ImportBuiltinPackagesFor will add KubeVela built-in packages into your CUE instance
func (pd *PackageDiscover) ImportBuiltinPackagesFor(bi *build.Instance) {
	pd.mutex.RLock()
	defer pd.mutex.RUnlock()
	bi.Imports = append(bi.Imports, pd.velaBuiltinPackages...)
}

// ImportPackagesAndBuildInstance Combine import built-in packages and build cue template together to avoid data race
func (pd *PackageDiscover) ImportPackagesAndBuildInstance(bi *build.Instance) (inst *cue.Instance, err error) {
	pd.ImportBuiltinPackagesFor(bi)
	if err := stdlib.AddImportsFor(bi, ""); err != nil {
		return nil, err
	}
	var r cue.Runtime
	pd.mutex.Lock()
	defer pd.mutex.Unlock()
	cueInst, err := r.Build(bi)
	if err != nil {
		return nil, err
	}
	return cueInst, err
}

// ListPackageKinds list packages and their kinds
func (pd *PackageDiscover) ListPackageKinds() map[string][]VersionKind {
	pd.mutex.RLock()
	defer pd.mutex.RUnlock()
	return pd.pkgKinds
}

// RefreshKubePackagesFromCluster will use K8s client to load/refresh all K8s open API as a reference kube package using in template
func (pd *PackageDiscover) RefreshKubePackagesFromCluster() error {
	return nil
	// body, err := pd.client.Get().AbsPath("/openapi/v2").Do(context.Background()).Raw()
	// if err != nil {
	//	 return err
	// }
	// return pd.addKubeCUEPackagesFromCluster(string(body))
}

// Exist checks if the GVK exists in the built-in packages
func (pd *PackageDiscover) Exist(gvk metav1.GroupVersionKind) bool {
	dgvk := convert2DGVK(gvk)
	// package name equals to importPath
	importPath := genStandardPkgName(dgvk)
	pd.mutex.RLock()
	defer pd.mutex.RUnlock()
	pkgKinds, ok := pd.pkgKinds[importPath]
	if !ok {
		pkgKinds = pd.pkgKinds[genOpenPkgName(dgvk)]
	}
	for _, v := range pkgKinds {
		if v.Kind == dgvk.Kind {
			return true
		}
	}
	return false
}

// mount will mount the new parsed package into PackageDiscover built-in packages
func (pd *PackageDiscover) mount(pkg *pkgInstance, pkgKinds []VersionKind) {
	pd.mutex.Lock()
	defer pd.mutex.Unlock()
	if pkgKinds == nil {
		pkgKinds = []VersionKind{}
	}
	for i, p := range pd.velaBuiltinPackages {
		if p.ImportPath == pkg.ImportPath {
			pd.pkgKinds[pkg.ImportPath] = pkgKinds
			pd.velaBuiltinPackages[i] = pkg.Instance
			return
		}
	}
	pd.pkgKinds[pkg.ImportPath] = pkgKinds
	pd.velaBuiltinPackages = append(pd.velaBuiltinPackages, pkg.Instance)
}

func (pd *PackageDiscover) pkgBuild(packages map[string]*pkgInstance, pkgName string,
	dGVK domainGroupVersionKind, def string, kubePkg *pkgInstance, groupKinds map[string][]VersionKind) error {
	pkg, ok := packages[pkgName]
	if !ok {
		pkg = newPackage(pkgName)
		pkg.Imports = []*build.Instance{kubePkg.Instance}
	}

	mykinds := groupKinds[pkgName]
	mykinds = append(mykinds, VersionKind{
		APIVersion:     dGVK.APIVersion,
		Kind:           dGVK.Kind,
		DefinitionName: "#" + dGVK.Kind,
	})

	if err := pkg.AddFile(dGVK.reverseString(), def); err != nil {
		return err
	}

	packages[pkgName] = pkg
	groupKinds[pkgName] = mykinds
	return nil
}

func (pd *PackageDiscover) addKubeCUEPackagesFromCluster(apiSchema string) error {
	var r cue.Runtime
	oaInst, err := r.Compile("-", apiSchema)
	if err != nil {
		return err
	}
	dgvkMapper := make(map[string]domainGroupVersionKind)
	pathValue := oaInst.Value().Lookup("paths")
	if pathValue.Exists() {
		if st, err := pathValue.Struct(); err == nil {
			iter := st.Fields()
			for iter.Next() {
				gvk := iter.Value().Lookup("post",
					"x-kubernetes-group-version-kind")
				if gvk.Exists() {
					if v, err := getDGVK(gvk); err == nil {
						dgvkMapper[v.reverseString()] = v
					}
				}
			}
		}
	}
	oaFile, err := jsonschema.Extract(oaInst, &jsonschema.Config{
		Root: "#/definitions",
		Map:  openAPIMapping(dgvkMapper),
	})
	if err != nil {
		return CUEParseError{
			err:     err,
			errType: ParseJSONSchemaErr,
		}
	}
	kubePkg := newPackage("kube")
	kubePkg.processOpenAPIFile(oaFile)
	if err := kubePkg.AddSyntax(oaFile); err != nil {
		return err
	}
	packages := make(map[string]*pkgInstance)
	groupKinds := make(map[string][]VersionKind)

	for k := range dgvkMapper {
		v := dgvkMapper[k]
		apiVersion := v.APIVersion
		def := fmt.Sprintf(`
import "kube"

#%s: kube.%s & {
kind: "%s"
apiVersion: "%s",
}`, v.Kind, k, v.Kind, apiVersion)

		if err := pd.pkgBuild(packages, genStandardPkgName(v), v, def, kubePkg, groupKinds); err != nil {
			return err
		}
		if err := pd.pkgBuild(packages, genOpenPkgName(v), v, def, kubePkg, groupKinds); err != nil {
			return err
		}
	}
	for name, pkg := range packages {
		pd.mount(pkg, groupKinds[name])
	}
	return nil
}

func genOpenPkgName(v domainGroupVersionKind) string {
	return BuiltinPackageDomain + "/" + v.APIVersion
}

func genStandardPkgName(v domainGroupVersionKind) string {
	res := []string{v.Group, v.Version}
	if v.Domain != "" {
		res = []string{v.Domain, v.Group, v.Version}
	}

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

func openAPIMapping(dgvkMapper map[string]domainGroupVersionKind) func(pos token.Pos, a []string) ([]ast.Label, error) {
	return func(pos token.Pos, a []string) ([]ast.Label, error) {
		if len(a) < 2 {
			return nil, errors.New("openAPIMapping format invalid")
		}

		name := strings.ReplaceAll(a[1], ".", "_")
		name = strings.ReplaceAll(name, "-", "_")
		if _, ok := dgvkMapper[name]; !ok && strings.HasPrefix(name, K8sResourcePrefix) {
			trimName := strings.TrimPrefix(name, K8sResourcePrefix)
			if v, ok := dgvkMapper[trimName]; ok {
				v.Domain = "k8s.io"
				dgvkMapper[name] = v
				delete(dgvkMapper, trimName)
			}
		}

		if strings.HasSuffix(a[1], ".JSONSchemaProps") && pos != token.NoPos {
			return []ast.Label{ast.NewIdent("_")}, nil
		}

		return []ast.Label{ast.NewIdent(name)}, nil
	}

}

type domainGroupVersionKind struct {
	Domain     string
	Group      string
	Version    string
	Kind       string
	APIVersion string
}

func (dgvk domainGroupVersionKind) reverseString() string {
	var s = []string{dgvk.Kind, dgvk.Version}
	s = append(s, strings.Split(dgvk.Group, ".")...)
	domain := dgvk.Domain
	if domain == "k8s.io" {
		domain = "api.k8s.io"
	}

	if domain != "" {
		s = append(s, strings.Split(domain, ".")...)
	}

	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
	return strings.ReplaceAll(strings.Join(s, "_"), "-", "_")
}

type pkgInstance struct {
	*build.Instance
}

func newPackage(name string) *pkgInstance {
	return &pkgInstance{
		&build.Instance{
			PkgName:    filepath.Base(name),
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

func getDGVK(v cue.Value) (ret domainGroupVersionKind, err error) {
	gvk := metav1.GroupVersionKind{}
	gvk.Group, err = v.Lookup("group").String()
	if err != nil {
		return
	}
	gvk.Version, err = v.Lookup("version").String()
	if err != nil {
		return
	}

	gvk.Kind, err = v.Lookup("kind").String()
	if err != nil {
		return
	}

	ret = convert2DGVK(gvk)
	return
}

func convert2DGVK(gvk metav1.GroupVersionKind) domainGroupVersionKind {
	ret := domainGroupVersionKind{
		Version:    gvk.Version,
		Kind:       gvk.Kind,
		APIVersion: gvk.Version,
	}
	if gvk.Group == "" {
		ret.Group = "core"
		ret.Domain = "k8s.io"
	} else {
		ret.APIVersion = gvk.Group + "/" + ret.APIVersion
		sv := strings.Split(gvk.Group, ".")
		// Domain must contain dot
		if len(sv) > 2 {
			ret.Domain = strings.Join(sv[1:], ".")
			ret.Group = sv[0]
		} else {
			ret.Group = gvk.Group
		}
	}
	return ret
}
