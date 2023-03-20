/*
Copyright 2023 The KubeVela Authors.

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
		Enum:    unescapeSplit(ext.Get("enum"), ","),
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
	settings := map[string]string{}
	if str == "" {
		return settings
	}

	pairs := unescapeSplit(str, ";")
	for _, pair := range pairs {
		switch kv := unescapeSplit(pair, ":"); len(kv) {
		case 1:
			settings[kv[0]] = ""
		case 2:
			settings[kv[0]] = kv[1]
		default:
			// ignore invalid pair
		}
	}

	return settings
}

func unescapeSplit(str string, sep string) []string {
	if str == "" {
		return []string{}
	}

	ss := strings.Split(str, sep)
	for i := 0; i < len(ss); i++ {
		j := i
		if len(ss[j]) > 0 {
			for {
				if ss[j][len(ss[j])-1] == '\\' && i+1 < len(ss) {
					i++
					ss[j] = ss[j][0:len(ss[j])-1] + sep + ss[i]
					ss[i] = ""
				} else {
					break
				}
			}
		}
	}

	// filter empty strings
	res := make([]string, 0, len(ss))
	for _, s := range ss {
		if s != "" {
			res = append(res, s)
		}
	}
	return res
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
