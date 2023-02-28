package cuegen

import (
	"fmt"
	"strings"

	"golang.org/x/tools/go/packages"
)

// loadPackage loads a package from given path.
func loadPackage(p string) (*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedTypes | packages.NeedTypesSizes |
			packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedDeps |
			packages.NeedModule,
	}

	pkgs, err := packages.Load(cfg, []string{p}...)
	if err != nil {
		return nil, err
	}
	if len(pkgs) != 1 {
		return nil, fmt.Errorf("expected one package, got %d", len(pkgs))
	}

	// only need to check the first package
	pkg := pkgs[0]
	if pkg.Errors != nil {
		errs := make([]string, 0, len(pkg.Errors))
		for _, e := range pkg.Errors {
			errs = append(errs, fmt.Sprintf("\t%s: %v", pkg.PkgPath, e))
		}
		return nil, fmt.Errorf("could not load Go packages:\n%s", strings.Join(errs, "\n"))
	}

	return pkg, nil
}
