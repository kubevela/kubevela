/*
Copyright 2022 The KubeVela Authors.

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

package writer

import (
	"context"
	"errors"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"

	configcontext "github.com/oam-dev/kubevela/pkg/config/context"
	"github.com/oam-dev/kubevela/pkg/cue/script"
)

func TestConvertMap2PropertiesKV(t *testing.T) {
	r := require.New(t)
	t.Run("valid conversion", func(t *testing.T) {
		re := map[string]string{}
		err := convertMap2PropertiesKV("", map[string]interface{}{
			"s":  "s",
			"n":  1,
			"nn": 1.5,
			"b":  true,
			"m": map[string]interface{}{
				"s": "s",
				"b": false,
			},
			"aa": []string{"a", "a"},
			"ai": []int64{1, 2},
			"ar": []map[string]interface{}{{
				"s2": "s2",
			}},
		}, re)
		r.Equal(err, nil)
		r.Equal(re, map[string]string{
			"s":       "s",
			"n":       "1",
			"nn":      "1.5",
			"b":       "true",
			"m.s":     "s",
			"m.b":     "false",
			"aa":      "a,a",
			"ai":      "1,2",
			"ar.0.s2": "s2",
		})
	})

	t.Run("unsupported type", func(t *testing.T) {
		re := map[string]string{}
		err := convertMap2PropertiesKV("", map[string]interface{}{"unsupported": make(chan int)}, re)
		r.Error(err)
		r.Contains(err.Error(), "can not be supported")
	})
}

func TestEncodingOutput(t *testing.T) {
	r := require.New(t)
	t.Run("all formats", func(t *testing.T) {
		testValue := `
			context: {
				key1: "hello"
				key2: 2
				key3: true
				key4: 4.4
				key5: ["hello"]
				key6: [{"hello": 1}]
				key7: [1, 2]
				key8: [1.2, 1]
				key9: {key10: [{"wang": true}]}
			}
		`
		v := cuecontext.New().CompileString(testValue)
		r.Equal(v.Err(), nil)

		_, err := encodingOutput(v, "yaml")
		r.Equal(err, nil)

		_, err = encodingOutput(v, "properties")
		r.Equal(err, nil)

		_, err = encodingOutput(v, "toml")
		r.Equal(err, nil)

		json, err := encodingOutput(v, "json")
		r.Equal(err, nil)
		r.Equal(string(json), `{"context":{"key1":"hello","key2":2,"key3":true,"key4":4.4,"key5":["hello"],"key6":[{"hello":1}],"key7":[1,2],"key8":[1.2,1],"key9":{"key10":[{"wang":true}]}}}`)
	})

	t.Run("specific formats", func(t *testing.T) {
		testValue := `
            {
                key1: "hello"
                key2: 123
                key3: {
                    subkey: "sub"
                }
            }
        `
		v := cuecontext.New().CompileString(testValue)
		r.NoError(v.Err())

		tomlBytes, err := encodingOutput(v, "toml")
		r.NoError(err)
		expectedToml := "key1 = \"hello\"\nkey2 = 123.0\n\n[key3]\n  subkey = \"sub\"\n"
		r.Equal(expectedToml, string(tomlBytes))

		propsBytes, err := encodingOutput(v, "properties")
		r.NoError(err)
		r.Contains(string(propsBytes), "key1 = hello")
		r.Contains(string(propsBytes), "key2 = 123")
		r.Contains(string(propsBytes), "key3.subkey = sub")
	})

	t.Run("invalid cue value", func(t *testing.T) {
		v := cuecontext.New().CompileString(`{key: > 1}`)
		_, err := encodingOutput(v, "json")
		r.Error(err)
	})
}

func TestParseExpandedWriterConfig(t *testing.T) {
	r := require.New(t)
	t.Run("missing nacos block", func(t *testing.T) {
		v := cuecontext.New().CompileString(
			`
        template: {}
        `)
		r.NoError(v.Err())
		ewc := ParseExpandedWriterConfig(v)
		r.Nil(ewc.Nacos)
	})

	t.Run("malformed nacos block", func(t *testing.T) {
		v := cuecontext.New().CompileString(`
        nacos: {
            endpoint: 123 // should be a struct
        }
        `)
		r.NoError(v.Err())
		ewc := ParseExpandedWriterConfig(v)
		r.True(ewc.Nacos == nil || ewc.Nacos.Endpoint.Name == "")
	})
}

func TestRenderForExpandedWriter(t *testing.T) {
	r := require.New(t)
	t.Run("no nacos config", func(t *testing.T) {
		ewc := ExpandedWriterConfig{}
		data, err := RenderForExpandedWriter(ewc, script.CUE(""), configcontext.ConfigRenderContext{}, nil)
		r.NoError(err)
		r.Nil(data.Nacos)
	})

	t.Run("render nacos error", func(t *testing.T) {
		ewc := ExpandedWriterConfig{
			Nacos: &NacosConfig{},
		}
		s := script.CUE("parameter: {}")
		_, err := RenderForExpandedWriter(ewc, s, configcontext.ConfigRenderContext{}, nil)
		r.Error(err)
	})
}

func TestWrite(t *testing.T) {
	r := require.New(t)
	t.Run("error reading config", func(t *testing.T) {
		ewd := &ExpandedWriterData{
			Nacos: &NacosData{},
		}
		errs := Write(context.Background(), ewd, func(ctx context.Context, namespace, name string) (map[string]interface{}, error) {
			return nil, errors.New("read-config-error")
		})
		r.Len(errs, 1)
		r.Equal("fail to read the config of the nacos server:read-config-error", errs[0].Error())
	})
}
