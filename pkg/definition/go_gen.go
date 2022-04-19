package definition

import (
	"bytes"
	"fmt"
	"go/format"
	"strings"
	"unicode"

	"cuelang.org/go/cue"

	"github.com/fatih/camelcase"
	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/apis/types"
	velacue "github.com/oam-dev/kubevela/pkg/cue"
)

// StructParameter is a parameter that can be printed as a struct.
type StructParameter struct {
	types.Parameter
	// GoType is the same to parameter.Type but can be print in Go
	GoType string
	Fields []Field
}

// Field is a field of a struct.
type Field struct {
	Name   string
	Type   string
	GoType string
}

//nolint:gochecknoglobals
var (
	WellKnownAbbreviations = map[string]bool{
		"API":   true,
		"DB":    true,
		"HTTP":  true,
		"HTTPS": true,
		"ID":    true,
		"JSON":  true,
		"OS":    true,
		"SQL":   true,
		"SSH":   true,
		"URI":   true,
		"URL":   true,
		"XML":   true,
		"YAML":  true,

		"CPU": true,
		"PVC": true,
	}

	dm = &AbbreviationHandlingFieldNamer{
		Abbreviations: WellKnownAbbreviations,
	}
)

// A FieldNamer generates a Go field name from a CUE label.
type FieldNamer interface {
	FieldName(label string) string
}

var structs []StructParameter

// GeneratorParameterStructs generates structs for parameters in cue.
func GeneratorParameterStructs(param cue.Value) ([]StructParameter, error) {
	structs = []StructParameter{}
	err := parseParameters(param, "Parameter")
	return structs, err
}

// NewStructParameter creates a StructParameter
func NewStructParameter() StructParameter {
	return StructParameter{
		Parameter: types.Parameter{},
		GoType:    "",
		Fields:    []Field{},
	}
}

func parseParameters(paraValue cue.Value, paramKey string) error {
	param := NewStructParameter()
	param.Name = paramKey
	param.Type = paraValue.IncompleteKind()
	param.Short, param.Usage, param.Alias, param.Ignore = velacue.RetrieveComments(paraValue)
	if def, ok := paraValue.Default(); ok && def.IsConcrete() {
		param.Default = velacue.GetDefault(def)
	}

	if param.Type == cue.StructKind {
		arguments, err := paraValue.Struct()
		if err != nil {
			return fmt.Errorf("augument not as struct %w", err)
		}
		if arguments.Len() == 0 {
			var SubParam StructParameter
			SubParam.Name = "-"
			SubParam.Required = true
			tl := paraValue.Template()
			if tl != nil { // is map type
				kind, err := trimIncompleteKind(tl("").IncompleteKind().String())
				if err != nil {
					return errors.Wrap(err, "invalid parameter kind")
				}
				SubParam.GoType = kind
				// TODO: better way to name to subParam
				SubParam.Name = "Item"
				param.GoType = fmt.Sprintf("map[string]%s", SubParam.Name)
				structs = append(structs, SubParam)
			}
		}
		for i := 0; i < arguments.Len(); i++ {
			var subParam Field
			fi := arguments.Field(i)
			if fi.IsDefinition {
				continue
			}
			val := fi.Value
			name := fi.Name
			subParam.Name = name
			switch val.IncompleteKind() {
			case cue.StructKind:
				if subField, err := val.Struct(); err == nil && subField.Len() == 0 { // err cannot be not nil,so ignore it
					if mapValue, ok := val.Elem(); ok {
						// In the future we could recursively call to support complex map-value(struct or list)
						subParam.GoType = fmt.Sprintf("map[string]%s", mapValue.IncompleteKind().String())
					} else {
						return fmt.Errorf("failed to got Map kind from %s", subParam.Name)
					}
				} else {
					if err := parseParameters(val, name); err != nil {
						return err
					}
					subParam.GoType = dm.FieldName(name)
				}
			case cue.ListKind:
				elem, success := val.Elem()
				if !success {
					// fail to get elements, use the value of ListKind to be the type
					subParam.GoType = val.IncompleteKind().String()
					break
				}
				switch elem.Kind() {
				case cue.StructKind:
					subParam.GoType = fmt.Sprintf("[]%s", dm.FieldName(name))
					if err := parseParameters(elem, name); err != nil {
						return err
					}
				default:
					subParam.GoType = fmt.Sprintf("[]%s", elem.IncompleteKind().String())
				}
			default:
				subParam.GoType = val.IncompleteKind().String()
			}
			param.Fields = append(param.Fields, Field{
				Name:   subParam.Name,
				GoType: subParam.GoType,
			})
		}
	}
	structs = append(structs, param)
	return nil
}

// PrintParamGosStruct prints the StructParameter in Golang struct format
func PrintParamGosStruct(parameters []StructParameter) {
	for _, parameter := range parameters {
		if parameter.Usage == "" {
			parameter.Usage = "-"
		}
		fmt.Printf("// %s %s\n", dm.FieldName(parameter.Name), parameter.Usage)
		printField(parameter)
	}
}

func printField(param StructParameter) {
	buffer := &bytes.Buffer{}
	fieldName := dm.FieldName(param.Name)
	switch param.Type {
	case cue.StructKind:
		// struct in cue can be map or struct
		if strings.HasPrefix(param.GoType, "map[string]") {
			fmt.Fprintf(buffer, "type %s %s", fieldName, param.GoType)
		} else {
			fmt.Fprintf(buffer, "type %s struct {\n", fieldName)
			for _, f := range param.Fields {
				fmt.Fprintf(buffer, "    %s %s `json:\"%s\"`\n", dm.FieldName(f.Name), f.GoType, f.Name)
			}

			fmt.Fprintf(buffer, "}\n")
		}
	case cue.StringKind:
		fmt.Fprintf(buffer, "type %s string\n", fieldName)
	case cue.IntKind:
		fmt.Fprintf(buffer, "type %s int\n", fieldName)
	case cue.BoolKind:
		fmt.Fprintf(buffer, "type %s bool\n", fieldName)
	case cue.FloatKind:
		fmt.Fprintf(buffer, "type %s float64\n", fieldName)
	case cue.ListKind:
		fmt.Fprintf(buffer, "type %s []%s\n", fieldName, param.GoType)
	default:
		fmt.Fprintf(buffer, "type %s %s\n", fieldName, param.GoType)
	}
	source, err := format.Source(buffer.Bytes())
	if err != nil {
		fmt.Println("Failed to format source:", err)
	}
	fmt.Println(string(source))
}

func trimIncompleteKind(mask string) (string, error) {
	mask = strings.Trim(mask, "()")
	ks := strings.Split(mask, "|")
	if len(ks) == 1 {
		return ks[0], nil
	}
	if len(ks) == 2 && ks[0] == "null" {
		return ks[1], nil
	}
	return "", fmt.Errorf("invalid incomplete kind: %s", mask)

}

// An AbbreviationHandlingFieldNamer generates Go field names from JSON
// properties while keeping abbreviations uppercased.
type AbbreviationHandlingFieldNamer struct {
	Abbreviations map[string]bool
}

// FieldName implements FieldNamer.FieldName.
func (a *AbbreviationHandlingFieldNamer) FieldName(property string) string {
	components := SplitComponents(property)
	for i, component := range components {
		switch {
		case component == "":
			// do nothing
		case a.Abbreviations[strings.ToUpper(component)]:
			components[i] = strings.ToUpper(component)
		case component == strings.ToUpper(component):
			runes := []rune(component)
			components[i] = string(runes[0]) + strings.ToLower(string(runes[1:]))
		default:
			runes := []rune(component)
			runes[0] = unicode.ToUpper(runes[0])
			components[i] = string(runes)
		}
	}
	runes := []rune(strings.Join(components, ""))
	for i, r := range runes {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			runes[i] = '_'
		}
	}
	fieldName := string(runes)
	if !unicode.IsLetter(runes[0]) && runes[0] != '_' {
		fieldName = "_" + fieldName
	}
	return fieldName
}

// SplitComponents splits name into components. name may be kebab case, snake
// case, or camel case.
func SplitComponents(name string) []string {
	switch {
	case strings.ContainsRune(name, '-'):
		return strings.Split(name, "-")
	case strings.ContainsRune(name, '_'):
		return strings.Split(name, "_")
	default:
		return camelcase.Split(name)
	}
}
