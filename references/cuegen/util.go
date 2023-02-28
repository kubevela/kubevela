package cuegen

import (
	"fmt"
	gotypes "go/types"
	"strconv"
	"unicode"

	cueast "cuelang.org/go/cue/ast"
	cuetoken "cuelang.org/go/cue/token"
)

func ident(name string, isDef bool) *cueast.Ident {
	if isDef {
		r := []rune(name)[0]
		name = "#" + name
		if !unicode.Is(unicode.Lu, r) {
			name = "_" + name
		}
	}
	return cueast.NewIdent(name)
}

func basicType(x *gotypes.Basic) cueast.Expr {
	switch t := x.String(); t {
	case "uintptr":
		return ident("uint64", false)
	case "byte":
		return ident("uint8", false)
	default:
		return ident(t, false)
	}
}

func anyLit() cueast.Expr {
	return &cueast.StructLit{Elts: []cueast.Decl{&cueast.Ellipsis{}}}
}

func basicLabel(t *gotypes.Basic, v string) (cueast.Expr, error) {
	if t.Info()&gotypes.IsInteger != 0 {
		if _, err := strconv.ParseInt(v, 10, 64); err != nil {
			return nil, err
		}
		return &cueast.BasicLit{Kind: cuetoken.INT, Value: v}, nil
	} else if t.Info()&gotypes.IsFloat != 0 {
		if _, err := strconv.ParseFloat(v, 64); err != nil {
			return nil, err
		}
		return &cueast.BasicLit{Kind: cuetoken.FLOAT, Value: v}, nil
	} else if t.Info()&gotypes.IsBoolean != 0 {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, err
		}
		return cueast.NewBool(b), nil
	} else if t.Info()&gotypes.IsString != 0 {
		return cueast.NewString(v), nil
	}

	return nil, fmt.Errorf("unsupported basic type %s", t)
}
