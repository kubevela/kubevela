package definition

import (
	"context"
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/token"
	"cuelang.org/go/encoding/jsonschema"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
)

var velaBuiltinPkgs []*build.Instance

// AddVelaInternalPackagesFor will add KubeVela built-in packages into your CUE instance
func AddVelaInternalPackagesFor(bi *build.Instance) {
	bi.Imports = append(bi.Imports, velaBuiltinPkgs...)
}

// AddKubeCUEPackagesFromCluster use  K8s native API and CRD definition as a reference package in template rendering
func AddKubeCUEPackagesFromCluster(config *rest.Config) error {
	copyConfig := *config
	apiSchema, err := getClusterOpenAPI(&copyConfig)
	if err != nil {
		return err
	}
	kubePkg := newPackage("kube")
	if err := kubePkg.addOpenAPI(apiSchema); err != nil {
		return err
	}
	kubePkg.mount()
	return nil
}

func getClusterOpenAPI(config *rest.Config) (string, error) {
	config.NegotiatedSerializer = serializer.NegotiatedSerializerWrapper(runtime.SerializerInfo{})
	restClient, err := rest.UnversionedRESTClientFor(config)
	if err != nil {
		return "", err
	}

	body, err := restClient.Get().AbsPath("/openapi/v2").Do(context.Background()).Raw()
	if err != nil {
		return "", err
	}
	return string(body), nil
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

func (pkg *pkgInstance) addOpenAPI(apiSchema string) error {
	var r cue.Runtime

	oaInst, err := r.Compile("-", apiSchema)
	if err != nil {
		return err
	}

	kinds := map[string]metav1.GroupVersionKind{}
	pathv := oaInst.Value().Lookup("paths")
	if pathv.Exists() {
		if st, err := pathv.Struct(); err == nil {
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

	for k, v := range kinds {
		apiversion := v.Version
		if v.Group != "" {
			apiversion = v.Group + "/" + apiversion
		}
		def := fmt.Sprintf(`%s: {
kind: "%s"
apiVersion: "%s",
}`, k, v.Kind, apiversion)
		if err := pkg.AddFile(k, def); err != nil {
			return err
		}
	}

	pkg.processOpenAPIFile(oaFile)
	return pkg.AddSyntax(oaFile)
}

func (pkg *pkgInstance) mount() {
	for i := range velaBuiltinPkgs {
		if velaBuiltinPkgs[i].ImportPath == pkg.ImportPath {
			velaBuiltinPkgs[i] = pkg.Instance
			return
		}
	}
	velaBuiltinPkgs = append(velaBuiltinPkgs, pkg.Instance)
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
