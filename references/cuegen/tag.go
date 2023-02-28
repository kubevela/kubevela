package cuegen

import (
	"reflect"
	"strings"
)

type tagOptions struct {
	// basic
	Name     string
	Inline   bool
	Optional bool

	// extended
	Default *string // nil means no default value
	Enum    []string
}

// TODO(iyear): be customizable
const (
	basicTag = "json" // same as json tag
	extTag   = "cue"  // format: cue:"key1:value1;key2:value2;boolValue1;boolValue2"
)

func (g *Generator) parseTag(tag string) *tagOptions {
	if tag == "" {
		return &tagOptions{}
	}

	name, opts := parseTag(reflect.StructTag(tag).Get(basicTag))
	ext := parseExtTag(reflect.StructTag(tag).Get(extTag))

	return &tagOptions{
		Name:     name,
		Inline:   opts.Has("inline"),
		Optional: opts.Has("omitempty"),

		Default: ext.GetX("default"),
		Enum:    strings.Split(ext.Get("enum"), ","),
	}
}

type basicTagOptions string

func parseTag(tag string) (string, basicTagOptions) {
	tag, opt, _ := strings.Cut(tag, ",")
	return tag, basicTagOptions(opt)
}

func (o basicTagOptions) Has(opt string) bool {
	if len(o) == 0 {
		return false
	}
	s := string(o)
	for s != "" {
		var name string
		name, s, _ = strings.Cut(s, ",")
		if name == opt {
			return true
		}
	}
	return false
}

func parseExtTag(str string) extTagOptions {
	sep := ";"
	settings := map[string]string{}
	names := strings.Split(str, sep)

	for i := 0; i < len(names); i++ {
		j := i
		if len(names[j]) > 0 {
			for {
				// support escape
				if names[j][len(names[j])-1] == '\\' {
					i++
					names[j] = names[j][0:len(names[j])-1] + sep + names[i]
					names[i] = ""
				} else {
					break
				}
			}
		}

		values := strings.Split(names[j], ":")
		k := strings.TrimSpace(strings.ToLower(values[0]))

		if len(values) >= 2 {
			settings[k] = strings.Join(values[1:], ":")
		} else if k != "" {
			settings[k] = k
		}
	}

	return settings
}

type extTagOptions map[string]string

// GetX returns the value of the key if it exists, otherwise nil.
func (e extTagOptions) GetX(key string) *string {
	if v, ok := e[key]; ok {
		return &v
	}
	return nil
}

// Get returns the value of the key if it exists, otherwise "".
func (e extTagOptions) Get(key string) string {
	return e[key]
}
