/*
Copyright 2026 The KubeVela Authors.

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
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/cue/script"
)

// nacosTemplate is the shape of pkg/config/testdata/mysql-db-nacos.cue: the endpoint is
// concrete, the metadata references the parameter, the content is the parameter.
const nacosTemplate = `
template: {
	nacos: {
		endpoint: {
			name:      "nacos"
			namespace: "vela-system"
		}
		format: "properties"
		metadata: {
			dataId: parameter.dataId
			group:  parameter.group
			if parameter.appName != _|_ {
				appName: parameter.appName
			}
		}
		content: parameter.content
	}
	parameter: {
		dataId:   string
		group:    *"DEFAULT_GROUP" | string
		appName?: string
		content: {
			mysqlHost: string
			mysqlPort: int
		}
	}
}
`

func templateField(t *testing.T, cueScript string) cue.Value {
	t.Helper()
	v, err := script.CUE(cueScript).ParseToValue()
	require.NoError(t, err)
	return v.LookupPath(cue.ParsePath("template"))
}

func TestValidateExpandedWriter(t *testing.T) {
	cases := []struct {
		name    string
		cue     string
		wantErr string
	}{
		{name: "no nacos block", cue: `template: { output: {} , parameter: {} }`},
		{name: "the real template", cue: nacosTemplate},
		{name: "format left to the parameter", cue: `template: {
			nacos: { endpoint: name: "nacos", format: parameter.fmt, metadata: { dataId: "d", group: "g" }, content: {} }
			parameter: fmt: string
		}`},
		{name: "not a struct", cue: `template: { nacos: "nacos", parameter: {} }`, wantErr: "must be a struct"},
		{name: "no endpoint", cue: `template: { nacos: { metadata: { dataId: "d", group: "g" } }, parameter: {} }`, wantErr: "nacos.endpoint.name is required"},
		{name: "endpoint name from the parameter", cue: `template: {
			nacos: { endpoint: name: parameter.ep, metadata: { dataId: "d", group: "g" } }
			parameter: ep: string
		}`, wantErr: "concrete, non-empty string"},
		{name: "empty endpoint name", cue: `template: { nacos: { endpoint: name: "", metadata: { dataId: "d", group: "g" } }, parameter: {} }`, wantErr: "concrete, non-empty string"},
		{name: "endpoint namespace set", cue: `template: { nacos: { endpoint: { name: "nacos", namespace: "team-b" }, metadata: { dataId: "d", group: "g" } }, parameter: {} }`},
		{name: "endpoint namespace from the parameter", cue: `template: {
			nacos: { endpoint: { name: "nacos", namespace: parameter.ns }, metadata: { dataId: "d", group: "g" } }
			parameter: ns: string
		}`, wantErr: "nacos.endpoint.namespace must be a concrete, non-empty string"},
		{name: "empty endpoint namespace", cue: `template: { nacos: { endpoint: { name: "nacos", namespace: "" }, metadata: { dataId: "d", group: "g" } }, parameter: {} }`, wantErr: "nacos.endpoint.namespace must be a concrete, non-empty string"},
		{name: "no metadata", cue: `template: { nacos: { endpoint: name: "nacos" }, parameter: {} }`, wantErr: "nacos.metadata is required"},
		{name: "metadata not a struct", cue: `template: { nacos: { endpoint: name: "nacos", metadata: "m" }, parameter: {} }`, wantErr: "nacos.metadata must be a struct"},
		{name: "metadata without group", cue: `template: { nacos: { endpoint: name: "nacos", metadata: { dataId: "d" } }, parameter: {} }`},
		{name: "metadata without dataId", cue: `template: { nacos: { endpoint: name: "nacos", metadata: { group: "g" } }, parameter: {} }`, wantErr: "nacos.metadata.dataId is required"},
		{name: "group not a string", cue: `template: { nacos: { endpoint: name: "nacos", metadata: { dataId: "d", group: 42 } }, parameter: {} }`, wantErr: "nacos.metadata.group must be a string"},
		{name: "dataId from an interpolation", cue: `template: {
			nacos: { endpoint: name: "nacos", metadata: { dataId: "\(context.name)-db", group: "g" }, content: {} }
			parameter: {}
		}`},
		{name: "group from an optional parameter", cue: `template: {
			nacos: { endpoint: name: "nacos", metadata: { dataId: "d", group: parameter.g }, content: {} }
			parameter: g?: string
		}`},
		{name: "dataId not a string", cue: `template: { nacos: { endpoint: name: "nacos", metadata: { dataId: 42, group: "g" } }, parameter: {} }`, wantErr: "nacos.metadata.dataId must be a string"},
		{name: "group from an int parameter", cue: `template: {
			nacos: { endpoint: name: "nacos", metadata: { dataId: "d", group: parameter.n } }
			parameter: n: int
		}`, wantErr: "nacos.metadata.group must be a string"},
		{name: "format not a string", cue: `template: { nacos: { endpoint: name: "nacos", format: 42, metadata: { dataId: "d", group: "g" } }, parameter: {} }`, wantErr: "nacos.format must be a string"},
		{name: "unknown format", cue: `template: { nacos: { endpoint: name: "nacos", format: "xml", metadata: { dataId: "d", group: "g" } }, parameter: {} }`, wantErr: `nacos.format "xml" is not one of json, yaml, toml, properties`},
		{name: "format case-insensitive", cue: `template: { nacos: { endpoint: name: "nacos", format: "JSON", metadata: { dataId: "d", group: "g" } }, parameter: {} }`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateExpandedWriter(templateField(t, tc.cue))
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// ValidateTemplate is the shared entry point: a vela import is no obstacle, a block
// behind it is checked, and a script that does not compile is left to the template
// controller.
func TestValidateTemplate(t *testing.T) {
	ctx := context.Background()
	r := require.New(t)
	r.NoError(ValidateTemplate(ctx, script.CUE(nacosTemplate)))
	r.NoError(ValidateTemplate(ctx, script.CUE("template: { output: {} , parameter: {} }")))
	r.NoError(ValidateTemplate(ctx, script.CUE("template: { nacos: { endpoint: name: } }")), "a script that does not compile is not this check's concern")
	imported := `import "vela/config"

template: {
	validation: config.#ImageRegistry & { $params: registry: "index.docker.io" }
	nacos: { endpoint: name: "nacos", format: "ini", metadata: { dataId: parameter.dataId, group: "g" }, content: {} }
	parameter: dataId: string
}
`
	err := ValidateTemplate(ctx, script.CUE(imported))
	r.Error(err, "the plain compiler cannot read the import; the CueX parse must still see the block")
	r.Contains(err.Error(), `nacos.format "ini" is not one of`)
}

func TestValidateExpandedWriterAgreesWithTheLenientParse(t *testing.T) {
	r := require.New(t)
	tf := templateField(t, nacosTemplate)
	r.NoError(ValidateExpandedWriter(tf))
	ewc := ParseExpandedWriterConfig(tf)
	r.NotNil(ewc.Nacos)
	r.Equal("nacos", ewc.Nacos.Endpoint.Name)
	r.Equal("vela-system", ewc.Nacos.Endpoint.Namespace)
	r.Equal("properties", ewc.Nacos.Format)
}

func TestEncodingOutputPropertiesIsDeterministic(t *testing.T) {
	r := require.New(t)
	v := cuecontext.New().CompileString(`{ mysqlPort: 3306, mysqlHost: "db", nested: { b: true, a: "x" } }`)
	r.NoError(v.Err())
	first, err := encodingOutput(v, "properties")
	r.NoError(err)
	r.Equal("mysqlHost = db\nmysqlPort = 3306\nnested.a = x\nnested.b = true\n", string(first))
	for i := 0; i < 20; i++ {
		again, err := encodingOutput(v, "properties")
		r.NoError(err)
		r.Equal(string(first), string(again))
	}
}

func nacosData(mutate ...func(d *NacosData)) *NacosData {
	d := &NacosData{
		NacosConfig: NacosConfig{
			Endpoint: ConfigRef{Name: "nacos", Namespace: "vela-system"},
			Format:   "properties",
			Metadata: NacosConfigMetadata{DataID: "nc-db", Group: "DEFAULT_GROUP"},
		},
		Content: []byte("mysqlHost = db\nmysqlPort = 3306\n"),
	}
	for _, m := range mutate {
		m(d)
	}
	return d
}

func TestContentHash(t *testing.T) {
	r := require.New(t)
	base := ContentHash(nacosData())
	r.Len(base, 64)
	r.Equal(base, ContentHash(nacosData()), "stable across calls")

	t.Run("changes with each part of the target and the content", func(t *testing.T) {
		r := require.New(t)
		for name, mutate := range map[string]func(d *NacosData){
			"content":     func(d *NacosData) { d.Content = []byte("mysqlHost = other\nmysqlPort = 3306\n") },
			"dataId":      func(d *NacosData) { d.Metadata.DataID = "nc-db-2" },
			"group":       func(d *NacosData) { d.Metadata.Group = "OTHER" },
			"namespaceId": func(d *NacosData) { d.Metadata.NamespaceID = "tenant-a" },
			"appName":     func(d *NacosData) { d.Metadata.AppName = "billing" },
			"tag":         func(d *NacosData) { d.Metadata.Tag = "canary" },
			"format":      func(d *NacosData) { d.Format = "yaml" },
			"endpoint":    func(d *NacosData) { d.Endpoint.Name = "nacos-b" },
			"endpoint ns": func(d *NacosData) { d.Endpoint.Namespace = "other" },
		} {
			r.NotEqual(base, ContentHash(nacosData(mutate)), name)
		}
	})

	t.Run("not with the order or spacing of properties pairs", func(t *testing.T) {
		r := require.New(t)
		r.Equal(base, ContentHash(nacosData(func(d *NacosData) { d.Content = []byte("mysqlPort = 3306\nmysqlHost = db\n") })))
		r.Equal(base, ContentHash(nacosData(func(d *NacosData) { d.Content = []byte("mysqlPort=3306\nmysqlHost=db\n") })))
	})

	t.Run("the format case does not matter", func(t *testing.T) {
		r := require.New(t)
		r.Equal(base, ContentHash(nacosData(func(d *NacosData) { d.Format = "Properties" })))
	})

	t.Run("other formats hash their bytes", func(t *testing.T) {
		r := require.New(t)
		a := ContentHash(nacosData(func(d *NacosData) { d.Format = "json"; d.Content = []byte(`{"a":1,"b":2}`) }))
		b := ContentHash(nacosData(func(d *NacosData) { d.Format = "json"; d.Content = []byte(`{"b":2,"a":1}`) }))
		r.NotEqual(a, b)
	})
}

func TestEndpointHash(t *testing.T) {
	r := require.New(t)
	endpoint := func(port float64, client map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{
			EndpointServersKey: []interface{}{map[string]interface{}{"ipAddr": "10.0.0.1", "port": port}},
			EndpointClientKey:  client,
		}
	}
	creds := map[string]interface{}{"username": "nacos", "password": "secret"}
	hash := func(m map[string]interface{}) string {
		h, err := EndpointHash(m)
		r.NoError(err)
		return h
	}
	base := hash(endpoint(8848, creds))
	r.Len(base, 64)
	r.Equal(base, hash(endpoint(8848, creds)), "stable across calls and key order")
	r.NotEqual(base, hash(endpoint(8849, creds)), "another server is another endpoint")
	r.Equal(base, hash(endpoint(8848, map[string]interface{}{"username": "nacos", "password": "rotated"})), "credentials are not part of the digest: it is readable in the status, and rotating them changes nothing in Nacos")
	r.Equal(base, hash(endpoint(8848, map[string]interface{}{})), "with or without credentials, the same endpoint")
	r.NotEqual(base, hash(endpoint(8848, map[string]interface{}{"endpoint": "acm.example"})), "the discovery endpoint is part of the server side")
	r.Len(hash(nil), 64)
	original := endpoint(8848, creds)
	_, _ = EndpointHash(original)
	r.Contains(creds, "password", "the caller's maps are not modified")
	_, err := EndpointHash(map[string]interface{}{EndpointServersKey: make(chan int)})
	r.ErrorContains(err, "cannot be digested")
}

func TestSameContent(t *testing.T) {
	r := require.New(t)
	r.True(SameContent("properties", []byte("a = 1\nb = 2\n"), []byte("b=2\na=1\n")))
	r.False(SameContent("properties", []byte("a = 1\nb = 2\n"), []byte("a = 1\nb = 3\n")))
	r.False(SameContent("properties", []byte("a = 1\n"), []byte("a = 1\nb = 2\n")))
	r.True(SameContent("yaml", []byte("a: 1\n"), []byte("a: 1\n")))
	r.False(SameContent("yaml", []byte("a: 1\n"), []byte("a:  1\n")))
	r.True(SameContent("properties", nil, []byte("")), "nothing published yet compares equal to empty")
}
