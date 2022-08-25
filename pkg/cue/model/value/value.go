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

package value

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/literal"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/pkg/cue/model/sets"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/stdlib"
)

// DefaultPackageHeader describes the default package header for CUE files.
const DefaultPackageHeader = "package main\n"

// Value is an object with cue.runtime and vendors
type Value struct {
	v          cue.Value
	r          *cue.Context
	addImports func(instance *build.Instance) error
}

// String return value's cue format string
func (val *Value) String(opts ...func(node ast.Node) ast.Node) (string, error) {
	opts = append(opts, sets.OptBytesToString)
	return sets.ToString(val.v, opts...)
}

// Error return value's error information.
func (val *Value) Error() error {
	v := val.CueValue()
	if !v.Exists() {
		return errors.New("empty value")
	}
	if err := val.v.Err(); err != nil {
		return err
	}
	var gerr error
	v.Walk(func(value cue.Value) bool {
		if err := value.Eval().Err(); err != nil {
			gerr = err
			return false
		}
		return true
	}, nil)
	return gerr
}

// UnmarshalTo unmarshal value into golang object
func (val *Value) UnmarshalTo(x interface{}) error {
	data, err := val.v.MarshalJSON()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, x)
}

// NewValueWithMainAndFiles new a value from main and appendix files
func NewValueWithMainAndFiles(main string, slaveFiles []string, pd *packages.PackageDiscover, tagTempl string, opts ...func(*ast.File) error) (*Value, error) {
	builder := &build.Instance{}

	mainFile, err := parser.ParseFile("main.cue", main, parser.ParseComments)
	if err != nil {
		return nil, errors.Wrap(err, "parse main file")
	}
	if mainFile.PackageName() == "" {
		// add a default package main if not exist
		mainFile, err = parser.ParseFile("main.cue", DefaultPackageHeader+main, parser.ParseComments)
		if err != nil {
			return nil, errors.Wrap(err, "parse main file with added package main header")
		}
	}
	for _, opt := range opts {
		if err := opt(mainFile); err != nil {
			return nil, errors.Wrap(err, "run option func for main file")
		}
	}
	if err := builder.AddSyntax(mainFile); err != nil {
		return nil, errors.Wrap(err, "add main file to CUE builder")
	}

	for idx, sf := range slaveFiles {
		cueSF, err := parser.ParseFile("sf-"+strconv.Itoa(idx)+".cue", sf, parser.ParseComments)
		if err != nil {
			return nil, errors.Wrap(err, "parse added file "+strconv.Itoa(idx)+" \n"+sf)
		}
		if cueSF.PackageName() != mainFile.PackageName() {
			continue
		}
		for _, opt := range opts {
			if err := opt(cueSF); err != nil {
				return nil, errors.Wrap(err, "run option func for files")
			}
		}
		if err := builder.AddSyntax(cueSF); err != nil {
			return nil, errors.Wrap(err, "add slave files to CUE builder")
		}
	}
	return newValue(builder, pd, tagTempl)
}

// NewValue new a value
func NewValue(s string, pd *packages.PackageDiscover, tagTempl string, opts ...func(*ast.File) error) (*Value, error) {
	builder := &build.Instance{}

	file, err := parser.ParseFile("-", s, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	for _, opt := range opts {
		if err := opt(file); err != nil {
			return nil, err
		}
	}
	if err := builder.AddSyntax(file); err != nil {
		return nil, err
	}
	return newValue(builder, pd, tagTempl)
}

func newValue(builder *build.Instance, pd *packages.PackageDiscover, tagTempl string) (*Value, error) {
	addImports := func(inst *build.Instance) error {
		if pd != nil {
			pd.ImportBuiltinPackagesFor(inst)
		}
		if err := stdlib.AddImportsFor(inst, tagTempl); err != nil {
			return err
		}
		return nil
	}

	if err := addImports(builder); err != nil {
		return nil, err
	}

	r := cuecontext.New()
	inst := r.BuildInstance(builder)
	val := new(Value)
	val.r = r
	val.v = inst
	val.addImports = addImports
	// do not check val.Err() error here, because the value may be filled later
	return val, nil
}

// AddFile add file to the instance
func AddFile(bi *build.Instance, filename string, src interface{}) error {
	if filename == "" {
		filename = "-"
	}
	file, err := parser.ParseFile(filename, src, parser.ParseComments)
	if err != nil {
		return err
	}
	if err := bi.AddSyntax(file); err != nil {
		return err
	}
	return nil
}

// TagFieldOrder add step tag.
func TagFieldOrder(root *ast.File) error {
	i := 0
	vs := &visitor{
		r: map[string]struct{}{},
	}
	for _, decl := range root.Decls {
		vs.addAttrForExpr(decl, &i)
	}
	return nil
}

// ProcessScript preprocess the script builtin function.
func ProcessScript(root *ast.File) error {
	return sets.PreprocessBuiltinFunc(root, "script", func(values []ast.Node) (ast.Expr, error) {
		for _, v := range values {
			lit, ok := v.(*ast.BasicLit)
			if ok {
				src, err := literal.Unquote(lit.Value)
				if err != nil {
					return nil, errors.WithMessage(err, "unquote script value")
				}
				expr, err := parser.ParseExpr("-", src)
				if err != nil {
					return nil, errors.Errorf("script value(%s) is invalid CueLang", src)
				}
				return expr, nil
			}
		}
		return nil, errors.New("script parameter error")
	})
}

type visitor struct {
	r map[string]struct{}
}

func (vs *visitor) done(name string) {
	vs.r[name] = struct{}{}
}

func (vs *visitor) shouldDo(name string) bool {
	_, ok := vs.r[name]
	return !ok
}
func (vs *visitor) addAttrForExpr(node ast.Node, index *int) {
	switch v := node.(type) {
	case *ast.Comprehension:
		st := v.Value.(*ast.StructLit)
		for _, elt := range st.Elts {
			vs.addAttrForExpr(elt, index)
		}
	case *ast.Field:
		basic, ok := v.Label.(*ast.Ident)
		if !ok {
			return
		}
		if !vs.shouldDo(basic.Name) {
			return
		}
		if v.Attrs == nil {
			*index++
			vs.done(basic.Name)
			v.Attrs = []*ast.Attribute{
				{Text: fmt.Sprintf("@step(%d)", *index)},
			}
		}
	}
}

// MakeValue generate an value with same runtime
func (val *Value) MakeValue(s string) (*Value, error) {
	builder := &build.Instance{}
	file, err := parser.ParseFile("-", s, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	if err := builder.AddSyntax(file); err != nil {
		return nil, err
	}
	if err := val.addImports(builder); err != nil {
		return nil, err
	}
	inst := val.r.BuildInstance(builder)
	v := new(Value)
	v.r = val.r
	v.v = inst
	v.addImports = val.addImports
	if v.Error() != nil {
		return nil, v.Error()
	}
	return v, nil
}

func (val *Value) makeValueWithFile(files ...*ast.File) (*Value, error) {
	builder := &build.Instance{}
	newFile := &ast.File{}
	imports := map[string]*ast.ImportSpec{}
	for _, f := range files {
		for _, importSpec := range f.Imports {
			if _, ok := imports[importSpec.Name.String()]; !ok {
				imports[importSpec.Name.String()] = importSpec
			}
		}
		newFile.Decls = append(newFile.Decls, f.Decls...)
	}

	for _, imp := range imports {
		newFile.Imports = append(newFile.Imports, imp)
	}

	if err := builder.AddSyntax(newFile); err != nil {
		return nil, err
	}
	if err := val.addImports(builder); err != nil {
		return nil, err
	}
	inst := val.r.BuildInstance(builder)
	v := new(Value)
	v.r = val.r
	v.v = inst
	v.addImports = val.addImports
	return v, nil
}

// FillRaw unify the value with the cue format string x at the given path.
func (val *Value) FillRaw(x string, paths ...string) error {
	file, err := parser.ParseFile("-", x, parser.ParseComments)
	if err != nil {
		return err
	}
	xInst := val.r.BuildFile(file)
	v := val.v.FillPath(FieldPath(paths...), xInst)
	if v.Err() != nil {
		return v.Err()
	}
	val.v = v
	return nil
}

// FillValueByScript unify the value x at the given script path.
func (val *Value) FillValueByScript(x *Value, path string) error {
	if !strings.Contains(path, "[") {
		newV := val.v.FillPath(FieldPath(path), x.v)
		if err := newV.Err(); err != nil {
			return err
		}
		val.v = newV
		return nil
	}
	s, err := x.String()
	if err != nil {
		return err
	}
	return val.fillRawByScript(s, path)
}

func (val *Value) fillRawByScript(x string, path string) error {
	a := newAssembler(x)
	pathExpr, err := parser.ParseExpr("path", path)
	if err != nil {
		return errors.WithMessage(err, "parse path")
	}
	if err := a.installTo(pathExpr); err != nil {
		return err
	}
	raw, err := val.String(sets.ListOpen)
	if err != nil {
		return err
	}
	v, err := val.MakeValue(raw + "\n" + a.v)
	if err != nil {
		return errors.WithMessage(err, "remake value")
	}
	if err := v.Error(); err != nil {
		return err
	}
	*val = *v
	return nil
}

// CueValue return cue.Value
func (val *Value) CueValue() cue.Value {
	return val.v
}

// FillObject unify the value with object x at the given path.
func (val *Value) FillObject(x interface{}, paths ...string) error {
	insert := x
	if v, ok := x.(*Value); ok {
		if v.r != val.r {
			return errors.New("filled value not created with same Runtime")
		}
		insert = v.v
	}
	newV := val.v.FillPath(FieldPath(paths...), insert)
	// do not check newV.Err() error here, because the value may be filled later
	val.v = newV
	return nil
}

// LookupValue reports the value at a path starting from val
func (val *Value) LookupValue(paths ...string) (*Value, error) {
	v := val.v.LookupPath(FieldPath(paths...))
	if !v.Exists() {
		return nil, errors.Errorf("failed to lookup value: var(path=%s) not exist", strings.Join(paths, "."))
	}
	return &Value{
		v:          v,
		r:          val.r,
		addImports: val.addImports,
	}, nil
}

func isScript(content string) (bool, error) {
	content = strings.TrimSpace(content)
	scriptFile, err := parser.ParseFile("-", content, parser.ParseComments)
	if err != nil {
		return false, errors.WithMessage(err, "parse script")
	}
	if len(scriptFile.Imports) != 0 {
		return true, nil
	}
	if len(scriptFile.Decls) == 0 || len(scriptFile.Decls) > 1 {
		return true, nil
	}

	return !isSelector(scriptFile.Decls[0]), nil
}

func isSelector(node ast.Node) bool {
	switch v := node.(type) {
	case *ast.EmbedDecl:
		return isSelector(v.Expr)
	case *ast.SelectorExpr, *ast.IndexExpr, *ast.Ident:
		return true
	default:
		return false
	}
}

// LookupByScript reports the value by cue script.
func (val *Value) LookupByScript(script string) (*Value, error) {
	var outputKey = "zz_output__"
	script = strings.TrimSpace(script)
	scriptFile, err := parser.ParseFile("-", script, parser.ParseComments)
	if err != nil {
		return nil, errors.WithMessage(err, "parse script")
	}
	isScriptPath, err := isScript(script)
	if err != nil {
		return nil, err
	}

	if !isScriptPath {
		return val.LookupValue(script)
	}

	raw, err := val.String()
	if err != nil {
		return nil, err
	}

	rawFile, err := parser.ParseFile("-", raw, parser.ParseComments)
	if err != nil {
		return nil, errors.WithMessage(err, "parse script")
	}

	behindKey(scriptFile, outputKey)

	newV, err := val.makeValueWithFile(rawFile, scriptFile)
	if err != nil {
		return nil, err
	}

	return newV.LookupValue(outputKey)
}

func behindKey(file *ast.File, key string) {
	var (
		implDecls []ast.Decl
		decls     []ast.Decl
	)

	for i, decl := range file.Decls {
		if _, ok := decl.(*ast.ImportDecl); ok {
			implDecls = append(implDecls, file.Decls[i])
		} else {
			decls = append(decls, file.Decls[i])
		}
	}

	file.Decls = implDecls
	if len(decls) == 1 {
		target := decls[0]
		if embed, ok := target.(*ast.EmbedDecl); ok {
			file.Decls = append(file.Decls, &ast.Field{
				Label: ast.NewIdent(key),
				Value: embed.Expr,
			})
			return
		}
	}
	file.Decls = append(file.Decls, &ast.Field{
		Label: ast.NewIdent(key),
		Value: &ast.StructLit{
			Elts: decls,
		},
	})

}

type field struct {
	Name  string
	Value *Value
	no    int64
}

// StepByList process item in list.
func (val *Value) StepByList(handle func(name string, in *Value) (bool, error)) error {
	iter, err := val.CueValue().List()
	if err != nil {
		return err
	}
	for iter.Next() {
		stop, err := handle(iter.Label(), &Value{
			v:          iter.Value(),
			r:          val.r,
			addImports: val.addImports,
		})
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
	}
	return nil
}

// StepByFields process the fields in order
func (val *Value) StepByFields(handle func(name string, in *Value) (bool, error)) error {
	iter := steps(val)
	for iter.next() {
		iter.do(handle)
	}
	return iter.err
}

type stepsIterator struct {
	queue   []*field
	index   int
	target  *Value
	err     error
	stopped bool
}

func steps(v *Value) *stepsIterator {
	return &stepsIterator{
		target: v,
	}
}

func (iter *stepsIterator) next() bool {
	if iter.stopped {
		return false
	}
	if iter.err != nil {
		return false
	}
	if iter.queue != nil {
		iter.index++
	}
	iter.assemble()
	return iter.index <= len(iter.queue)-1
}

func (iter *stepsIterator) assemble() {
	st, err := iter.target.v.Struct()
	if err != nil {
		iter.err = err
		return
	}

	filters := map[string]struct{}{}
	for _, item := range iter.queue {
		filters[item.Name] = struct{}{}
	}
	var addFields []*field
	for i := 0; i < st.Len(); i++ {
		if st.Field(i).Value.IncompleteKind() == cue.TopKind {
			continue
		}
		name := st.Field(i).Name
		attr := st.Field(i).Value.Attribute("step")
		no, err := attr.Int(0)
		if err != nil {
			no = 100
			if name == "#do" || name == "#provider" {
				no = 0
			}
		}
		if _, ok := filters[name]; !ok {
			addFields = append(addFields, &field{
				Name: name,
				no:   no,
			})
		}
	}

	suffixItems := append(addFields, iter.queue[iter.index:]...)
	sort.Sort(sortFields(suffixItems))
	iter.queue = append(iter.queue[:iter.index], suffixItems...)
}

func (iter *stepsIterator) value() *Value {
	v := iter.target.v.LookupPath(FieldPath(iter.name()))
	return &Value{
		r:          iter.target.r,
		v:          v,
		addImports: iter.target.addImports,
	}
}

func (iter *stepsIterator) name() string {
	return iter.queue[iter.index].Name
}

func (iter *stepsIterator) do(handle func(name string, in *Value) (bool, error)) {
	if iter.err != nil {
		return
	}
	v := iter.value()
	stopped, err := handle(iter.name(), v)
	if err != nil {
		iter.err = err
		return
	}
	iter.stopped = stopped
	if !isDef(iter.name()) {
		if err := iter.target.FillObject(v, iter.name()); err != nil {
			iter.err = err
			return
		}
	}
}

type sortFields []*field

func (sf sortFields) Len() int {
	return len(sf)
}
func (sf sortFields) Less(i, j int) bool {
	return sf[i].no < sf[j].no
}

func (sf sortFields) Swap(i, j int) {
	sf[i], sf[j] = sf[j], sf[i]
}

// Field return the cue value corresponding to the specified field
func (val *Value) Field(label string) (cue.Value, error) {
	v := val.v.LookupPath(cue.ParsePath(label))
	if !v.Exists() {
		return v, errors.Errorf("label %s not found", label)
	}

	if v.IncompleteKind() == cue.BottomKind {
		return v, errors.Errorf("label %s's value not computed", label)
	}
	return v, nil
}

// GetString get the string value at a path starting from v.
func (val *Value) GetString(paths ...string) (string, error) {
	v, err := val.LookupValue(paths...)
	if err != nil {
		return "", err
	}
	return v.CueValue().String()
}

// GetStringSlice get string slice from val
func (val *Value) GetStringSlice(paths ...string) ([]string, error) {
	v, err := val.LookupValue(paths...)
	if err != nil {
		return nil, err
	}
	var s []string
	err = v.UnmarshalTo(&s)
	return s, err
}

// GetInt64 get the int value at a path starting from v.
func (val *Value) GetInt64(paths ...string) (int64, error) {
	v, err := val.LookupValue(paths...)
	if err != nil {
		return 0, err
	}
	return v.CueValue().Int64()
}

// GetBool get the int value at a path starting from v.
func (val *Value) GetBool(paths ...string) (bool, error) {
	v, err := val.LookupValue(paths...)
	if err != nil {
		return false, err
	}
	return v.CueValue().Bool()
}

// OpenCompleteValue make that the complete value can be modified.
func (val *Value) OpenCompleteValue() error {
	newS, err := sets.OpenBaiscLit(val.CueValue())
	if err != nil {
		return err
	}

	v := cuecontext.New().BuildFile(newS)
	val.v = v
	return nil
}
func isDef(s string) bool {
	return strings.HasPrefix(s, "#")
}

// assembler put value under parsed expression as path.
type assembler struct {
	v string
}

func newAssembler(v string) *assembler {
	return &assembler{v: v}
}

func (a *assembler) fill2Path(p string) {
	a.v = fmt.Sprintf("%s: %s", p, a.v)
}

func (a *assembler) fill2Array(i int) {
	s := ""
	for j := 0; j < i; j++ {
		s += "_,"
	}
	if strings.Contains(a.v, ":") && !strings.HasPrefix(a.v, "{") {
		a.v = fmt.Sprintf("{ %s }", a.v)
	}
	a.v = fmt.Sprintf("[%s%s]", s, strings.TrimSpace(a.v))
}

func (a *assembler) installTo(expr ast.Expr) error {
	switch v := expr.(type) {
	case *ast.IndexExpr:
		if err := a.installTo(v.Index); err != nil {
			return err
		}
		if err := a.installTo(v.X); err != nil {
			return err
		}
	case *ast.SelectorExpr:
		if ident, ok := v.Sel.(*ast.Ident); ok {
			if err := a.installTo(ident); err != nil {
				return err
			}
		} else {
			return errors.New("invalid sel type in selector")
		}
		if err := a.installTo(v.X); err != nil {
			return err
		}

	case *ast.Ident:
		a.fill2Path(v.String())
	case *ast.BasicLit:
		switch v.Kind {
		case token.STRING:
			a.fill2Path(v.Value)
		case token.INT:
			idex, _ := strconv.Atoi(v.Value)
			a.fill2Array(idex)
		default:
			return errors.New("invalid path")
		}
	default:
		return errors.New("invalid path")
	}
	return nil
}

// makePath creates a Path from a sequence of string.
func makePath(paths ...string) string {
	mergedPath := ""
	if len(paths) == 0 {
		return mergedPath
	}
	mergedPath = paths[0]
	if mergedPath == "" || (len(paths) == 1 && (strings.Contains(mergedPath, ".") || strings.Contains(mergedPath, "["))) {
		return paths[0]
	}
	if !strings.HasPrefix(mergedPath, "_") && !strings.HasPrefix(mergedPath, "#") {
		mergedPath = fmt.Sprintf("\"%s\"", mergedPath)
	}
	for _, p := range paths[1:] {
		if !strings.HasPrefix(p, "#") {
			mergedPath += fmt.Sprintf("[\"%s\"]", p)
		} else {
			mergedPath += fmt.Sprintf(".%s", p)
		}
	}
	return mergedPath
}

func isNumber(s string) bool {
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}

// FieldPath return the cue path of the given paths
func FieldPath(paths ...string) cue.Path {
	s := makePath(paths...)
	if isNumber(s) {
		return cue.MakePath(cue.Str(s))
	}
	return cue.ParsePath(s)
}
