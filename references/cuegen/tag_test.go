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
	"testing"

	"github.com/stretchr/testify/assert"
)

func str(s string) *string {
	return &s
}

func TestGeneratorParseTag(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		opts *tagOptions
	}{
		{"empty", "", &tagOptions{}},
		{"only_name", `json:"name"`, &tagOptions{Name: "name", Enum: []string{}}},
		{"only_name_2", `json:""`, &tagOptions{Name: "", Enum: []string{}}},
		{"only_name_3", `json:"-"`, &tagOptions{Name: "-", Enum: []string{}}},
		{"json_omitempty", `json:"name,omitempty"`, &tagOptions{Name: "name", Optional: true, Enum: []string{}}},
		{"json_omitempty_2", `json:"name,omitempty,omitempty"`, &tagOptions{Name: "name", Optional: true, Enum: []string{}}},
		{"json_omitempty_3", `json:",omitempty"`, &tagOptions{Name: "", Optional: true, Enum: []string{}}},
		{"json_inline", `json:",inline"`, &tagOptions{Name: "", Inline: true, Enum: []string{}}},
		{"json_inline_2", `json:"name,inline"`, &tagOptions{Name: "name", Inline: true, Enum: []string{}}},
		{"json_inline_3", `json:"name,inline,inline"`, &tagOptions{Name: "name", Inline: true, Enum: []string{}}},
		{"json_omitempty_inline", `json:"name,omitempty,inline"`, &tagOptions{Name: "name", Optional: true, Inline: true, Enum: []string{}}},
		{"json_omitempty_inline_2", `json:",omitempty,inline"`, &tagOptions{Name: "", Optional: true, Inline: true, Enum: []string{}}},
		{"cue_default", `cue:"default:default_value"`, &tagOptions{Default: str("default_value"), Enum: []string{}}},
		{"cue_default_2", `cue:"default:default_value;default:default_value2"`, &tagOptions{Default: str("default_value2"), Enum: []string{}}},
		{"cue_default_3", `cue:"default:1.11"`, &tagOptions{Default: str("1.11"), Enum: []string{}}},
		{"cue_default_4", `cue:"default:va,lue"`, &tagOptions{Default: str(`va,lue`), Enum: []string{}}},
		{"cue_enum", `cue:"enum:enum1,enum2"`, &tagOptions{Enum: []string{"enum1", "enum2"}}},
		{"cue_enum_2", `cue:"enum:enum1,enum2;enum:enum3,enum4"`, &tagOptions{Enum: []string{"enum3", "enum4"}}},
		{"cue_enum_empty", `cue:""`, &tagOptions{Enum: []string{}}},
		{"cue_enum_empty_2", `cue:"enum:"`, &tagOptions{Enum: []string{}}},
		{"cue_escape", `cue:"default:\"default_value\""`, &tagOptions{Default: str(`"default_value"`), Enum: []string{}}},
		{"cue_escape_2", `cue:"default:\"default_value\\\"\""`, &tagOptions{Default: str(`"default_value\""`), Enum: []string{}}},
		{"cue_escape_3", `cue:"default:value\\;vv;enum:enum1,enum2"`, &tagOptions{Default: str(`value;vv`), Enum: []string{"enum1", "enum2"}}},
		{"cue_escape_default_semicolon", `cue:"default:va\\;lue\\"`, &tagOptions{Default: str(`va;lue\`), Enum: []string{}}},
		{"cue_escape_default_colon", `cue:"default:va\\:lue\\"`, &tagOptions{Default: str(`va:lue\`), Enum: []string{}}},
		{"cue_escape_enum_semicolon", `cue:"enum:e\\;num1,enum2"`, &tagOptions{Enum: []string{`e;num1`, "enum2"}}},
		{"cue_escape_enum_colon", `cue:"enum:e\\:num1,enum2"`, &tagOptions{Enum: []string{`e:num1`, "enum2"}}},
		{"cue_escape_enum_colon_2", `cue:"enum:enum1\\,enum2"`, &tagOptions{Enum: []string{"enum1,enum2"}}},
		{"cue_escape_all", `cue:"default:va\\;lue\\:;enum:e\\;num1,e\\:num2\\,enum3"`, &tagOptions{Default: str(`va;lue:`), Enum: []string{`e;num1`, `e:num2,enum3`}}},
		{"json_cue", `json:"name" cue:"default:default_value"`, &tagOptions{Name: "name", Default: str("default_value"), Enum: []string{}}},
		{"json_cue_2", `json:"name" cue:"default:default_value;enum:enum1,enum2"`, &tagOptions{Name: "name", Default: str("default_value"), Enum: []string{"enum1", "enum2"}}},
		{"json_cue_3", `json:",omitempty" cue:"default:default_value"`, &tagOptions{Name: "", Optional: true, Default: str("default_value"), Enum: []string{}}},
		{"json_cue_4", `json:",inline" cue:"default:default_value"`, &tagOptions{Name: "", Inline: true, Default: str("default_value"), Enum: []string{}}},
		{"json_cue_5", `json:"name,omitempty,inline" cue:"default:default_value"`, &tagOptions{Name: "name", Optional: true, Inline: true, Default: str("default_value"), Enum: []string{}}},
		{"json_cue_6", `json:",omitempty,inline" cue:"default:default_value;enum:enum1,enum2"`, &tagOptions{Name: "", Optional: true, Inline: true, Default: str("default_value"), Enum: []string{"enum1", "enum2"}}},
	}

	g := &Generator{}
	for _, tt := range tests {
		got := g.parseTag(tt.tag)
		assert.Equal(t, tt.opts, got, tt.name)
	}
}

func TestParseBasicTag(t *testing.T) {
	tests := []struct {
		name     string
		tag      string
		wantName string
		wantOpt  string
	}{
		{"empty", "", "", ""},
		{"only_name", "name", "name", ""},
		{"name_and_opt", "name,opt,opt2", "name", "opt,opt2"},
		{"name_and_opt_2", "name,opt1;opt2.opt3", "name", "opt1;opt2.opt3"},
		{"name_and_opt_with_space", "name, opt", "name", " opt"},
		{"only_opt", ",opt,opt2", "", "opt,opt2"},
	}

	for _, tt := range tests {
		gotName, gotOpt := parseTag(tt.tag)
		assert.EqualValues(t, tt.wantName, gotName, tt.name)
		assert.EqualValues(t, tt.wantOpt, gotOpt, tt.name)
	}
}

func TestBasicTagOptHas(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		opt  string
		want bool
	}{
		{"empty", "", "", false},
		{"empty_opt", "name", "", false},
		{"empty_tag", "", "opt", false},
		{"single_opt", "name,opt", "opt", true},
		{"multi_opt", "name,opt1,opt2,opt3", "opt1", true},
		{"multi_opt_2", "name,opt1,opt2,opt3", "opt2", true},
		{"multi_opt_3", "name,opt1,opt2,opt3", "opt3", true},
		{"only_multi_opt", ",opt1,opt2,opt3", "opt1", true},
		{"only_multi_opt_2", ",opt1,opt2,opt3", "opt2", true},
		{"only_multi_opt_3", ",opt1,opt2,opt3", "opt3", true},
	}

	for _, tt := range tests {
		_, opt := parseTag(tt.tag)
		assert.Equal(t, tt.want, opt.Has(tt.opt), tt.name)
	}
}

func TestParseExtTag(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		want map[string]string
	}{
		{"empty", "", map[string]string{}},
		{"only_key", "key", map[string]string{"key": ""}},
		{"one_kv", "key:value", map[string]string{"key": "value"}},
		{"multi_kv", "key1:value1;key2:value2", map[string]string{"key1": "value1", "key2": "value2"}},
		{"bool", "key1;key2", map[string]string{"key1": "", "key2": ""}},
		{"bool_and_kv", "key1;key2:value2", map[string]string{"key1": "", "key2": "value2"}},
		{"kv_and_bool", "key1:value1;key2", map[string]string{"key1": "value1", "key2": ""}},
		{"multi_kv_and_bool", "key1:value1;key2:value2;key3", map[string]string{"key1": "value1", "key2": "value2", "key3": ""}},
		{"escape_semicolon_value", `key1:value\;1`, map[string]string{"key1": "value;1"}},
		{"escape_semicolon_key", `key\;1:value1`, map[string]string{"key;1": "value1"}},
		{"escape_semicolon_pairs", `key\;1:value\;1;key3:va\lue3\`, map[string]string{"key;1": "value;1", "key3": `va\lue3\`}},
		{"escape_semicolon_last", `key\1:value1\`, map[string]string{`key\1`: `value1\`}},
		{"escape_semicolon_last_2", `k\ey1:va\lue1\1`, map[string]string{`k\ey1`: `va\lue1\1`}},
		{"escape_semicolon_last_3", `key1:value1\;`, map[string]string{"key1": `value1;`}},
		{"escape_colon_value", `key1:value\:1`, map[string]string{"key1": "value:1"}},
		{"escape_colon_key", `key\:1:value1`, map[string]string{"key:1": "value1"}},
		{"escape_colon_pairs", `key\:1:value\:1;key3:va\lue3\`, map[string]string{"key:1": "value:1", "key3": `va\lue3\`}},
		{"escape_colon_last", `key\1:value1\:`, map[string]string{`key\1`: `value1:`}},
		{"escape_colon_last_2", `k\ey1:va\lue1\:1`, map[string]string{`k\ey1`: `va\lue1:1`}},
		{"escape_colon_last_3", `key1:value1\:`, map[string]string{"key1": `value1:`}},
		{"invalid_pair", `key1:value1:invalid;key2:value2`, map[string]string{"key2": "value2"}},
	}

	for _, tt := range tests {
		got := parseExtTag(tt.tag)
		assert.EqualValues(t, tt.want, got, tt.name)
	}
}

func TestExtTagGet(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		key  string
		want string
	}{
		{"empty", "", "", ""},
		{"empty_key", "key", "", ""},
		{"empty_tag", "", "key", ""},
		{"one_kv", "key:value", "key", "value"},
		{"multi_kv", "key1:value1;key2:value2", "key1", "value1"},
		{"multi_kv_2", "key1:value1;key2:value2", "key2", "value2"},
		{"bool", "key1;key2", "key1", ""},
		{"bool2", "key1;key2", "key2", ""},
		{"escape", "key1:value1\\;vv", "key1", "value1;vv"},
		{"escape_2", "key1:\"value2\"", "key1", `"value2"`},
	}

	for _, tt := range tests {
		got := parseExtTag(tt.tag)
		assert.EqualValues(t, tt.want, got.Get(tt.key), tt.name)
	}
}

func TestExtTagGetX(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		key  string
		want *string
	}{
		{"empty", "", "", nil},
		{"empty_key", "key", "", nil},
		{"empty_tag", "", "key", nil},
		{"one_kv", "key:value", "key", str("value")},
		{"multi_kv", "key1:value1;key2:value2", "key1", str("value1")},
		{"multi_kv_2", "key1:value1;key2:value2", "key2", str("value2")},
		{"bool", "key1;key2", "key1", str("")},
		{"bool2", "key1;key2", "key2", str("")},
		{"escape", "key1:value1\\;vv", "key1", str("value1;vv")},
		{"escape_2", "key1:\"value2\"", "key1", str(`"value2"`)},
	}

	for _, tt := range tests {
		got := parseExtTag(tt.tag)
		assert.EqualValues(t, tt.want, got.GetX(tt.key), tt.name)
	}
}

func TestUnescapeSplit(t *testing.T) {
	tests := []struct {
		name string
		s    string
		sep  string
		want []string
	}{
		{"empty", "", "", []string{}},
		{"empty_sep", "key:value", "", []string{"k", "e", "y", ":", "v", "a", "l", "u", "e"}},
		{"one", "key:value", ":", []string{"key", "value"}},
		{"escape_sep", `key\:value`, ":", []string{"key:value"}},
		{"escape_sep_2", `key\:value\:`, ":", []string{"key:value:"}},
		{"escape_last", `key\:value\`, ":", []string{`key:value\`}},
		{"escape_multi", `key\:value\:;key2:value2`, ":", []string{"key:value:;key2", "value2"}},
		{"escape_multi_2", `key\:value\:;key2:value2`, ";", []string{`key\:value\:`, "key2:value2"}},
	}

	for _, tt := range tests {
		got := unescapeSplit(tt.s, tt.sep)
		assert.EqualValues(t, tt.want, got, tt.name)
	}
}
