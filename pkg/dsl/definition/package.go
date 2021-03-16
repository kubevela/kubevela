package definition

import (
	"context"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/token"
	"cuelang.org/go/encoding/jsonschema"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
)

var velaBuiltinPkgs []*build.Instance

func addImportsFor(bi *build.Instance) {
	bi.Imports = append(bi.Imports, velaBuiltinPkgs...)
}

// AddImportFromCluster use  K8s native API and CRD definition as a reference package in template rendering
func AddImportFromCluster(config *rest.Config, scheme *runtime.Scheme) error {
	apiSchema, err := getClusterOpenAPI(config, scheme)
	if err != nil {
		return err
	}
	kubePkg := newPackage("kube")
	kubePkg.addOpenAPI(apiSchema)
	kubePkg.mount()
	return nil
}

func getClusterOpenAPI(config *rest.Config, scheme *runtime.Scheme) (string, error) {
	codec := runtime.NoopEncoder{Decoder: serializer.NewCodecFactory(scheme).UniversalDeserializer()}
	config.NegotiatedSerializer = serializer.NegotiatedSerializerWrapper(runtime.SerializerInfo{Serializer: codec})
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

var rootDefs = "#SchemaMap"

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

func processOpenAPIFile(f *ast.File) {
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

func (pkg *pkgInstance) addOpenAPI(apiSchema string) error {
	var r cue.Runtime

	oaInst, err := r.Compile("-", apiSchema)
	if err != nil {
		return err
	}
	oaFile, err := jsonschema.Extract(oaInst, &jsonschema.Config{
		Root: "#/definitions",
		Map:  openAPIMapping,
	})
	if err != nil {
		return err
	}
	processOpenAPIFile(oaFile)
	return pkg.AddSyntax(oaFile)
}

func (pkg *pkgInstance) mount() {
	for i := range velaBuiltinPkgs {
		if velaBuiltinPkgs[i].ImportPath == pkg.ImportPath {
			velaBuiltinPkgs[i] = pkg.Instance
			break
		}
	}
	velaBuiltinPkgs = append(velaBuiltinPkgs, pkg.Instance)
}
