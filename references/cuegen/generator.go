package cuegen

import (
	goast "go/ast"
	"io"

	cueast "cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/ast/astutil"
	cueformat "cuelang.org/go/cue/format"
	"golang.org/x/tools/go/packages"
)

// Generator generates CUE schema from Go struct.
type Generator struct {
	// immutable
	pkg   *packages.Package
	types typeInfo

	anyTypes map[string]struct{}
}

var defaultAnyTypes = []string{
	"map[string]interface{}",
	"map[string]any",
	"interface{}",
	"any",
}

// NewGenerator creates a new generator with given file or package path.
func NewGenerator(f string) (*Generator, error) {
	pkg, err := loadPackage(f)
	if err != nil {
		return nil, err
	}

	types := getTypeInfo(pkg)

	g := &Generator{
		pkg:      pkg,
		types:    types,
		anyTypes: make(map[string]struct{}),
	}

	g.RegisterAny(defaultAnyTypes...)

	return g, nil
}

// Generate generates CUE schema from Go struct and writes to w.
func (g *Generator) Generate(w io.Writer) error {
	var decls []cueast.Decl

	for _, syntax := range g.pkg.Syntax {
		for _, decl := range syntax.Decls {
			if d, ok := decl.(*goast.GenDecl); ok {
				t, err := g.convertDecls(d)
				if err != nil {
					return err
				}
				decls = append(decls, t...)
			}
		}
	}

	pkg := &cueast.Package{Name: ident(g.pkg.Name, false)}

	f := &cueast.File{Decls: []cueast.Decl{pkg}}
	f.Decls = append(f.Decls, decls...)

	return g.write(w, f)
}

func (g *Generator) write(w io.Writer, f *cueast.File) error {
	if err := astutil.Sanitize(f); err != nil {
		return err
	}

	b, err := cueformat.Node(f, cueformat.Simplify())
	if err != nil {
		return err
	}

	_, err = w.Write(b)
	return err
}
