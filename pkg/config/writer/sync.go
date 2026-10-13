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
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"github.com/magiconair/properties"
)

// Formats the nacos writer encodes. Anything else renders as yaml today, silently;
// ValidateExpandedWriter names the choice instead.
var supportedFormats = []string{"json", "yaml", "toml", "properties"}

// ValidateExpandedWriter is the strict reading of a template's nacos block, for the
// ConfigTemplate webhook and, as the second line of defence, the Config reconciler.
// ParseExpandedWriterConfig stays lenient for the CLI. A template without the block is
// valid. With it: the block is a struct, endpoint.name is a concrete non-empty string,
// metadata is a struct that defines dataId and group (incomplete is fine, they usually
// reference the parameter), and format, when concrete, is one of the supported ones.
// The checks are structural because the block is not decodable before the parameters
// are merged in.
func ValidateExpandedWriter(template cue.Value) error {
	nacos := template.LookupPath(cue.ParsePath("nacos"))
	if !nacos.Exists() {
		return nil
	}
	if err := nacos.Err(); err != nil {
		return fmt.Errorf("the nacos block is invalid: %w", err)
	}
	if nacos.IncompleteKind() != cue.StructKind {
		return errors.New("the nacos block must be a struct")
	}
	name := nacos.LookupPath(cue.ParsePath("endpoint.name"))
	if !name.Exists() {
		return errors.New("nacos.endpoint.name is required: the name of the config made from the nacos-server template")
	}
	if s, err := name.String(); err != nil || s == "" {
		return errors.New("nacos.endpoint.name must be a concrete, non-empty string: it cannot reference the parameter")
	}
	metadata := nacos.LookupPath(cue.ParsePath("metadata"))
	if !metadata.Exists() {
		return errors.New("nacos.metadata is required, with dataId and group")
	}
	if metadata.IncompleteKind() != cue.StructKind {
		return errors.New("nacos.metadata must be a struct")
	}
	for _, field := range []string{"dataId", "group"} {
		if !metadata.LookupPath(cue.ParsePath(field)).Exists() {
			return fmt.Errorf("nacos.metadata.%s is required", field)
		}
	}
	format := nacos.LookupPath(cue.ParsePath("format"))
	if format.Exists() {
		if s, err := format.String(); err == nil && !isSupportedFormat(s) {
			return fmt.Errorf("nacos.format %q is not one of %s", s, strings.Join(supportedFormats, ", "))
		}
	}
	return nil
}

func isSupportedFormat(format string) bool {
	for _, f := range supportedFormats {
		if strings.EqualFold(f, format) {
			return true
		}
	}
	return false
}

// ContentHash identifies what a NacosData would publish and where: the endpoint, the
// dataId, group and namespaceId, the format and the content. The Config status stores
// it and the reconciler compares it, so a change to any of them publishes again and a
// reconcile that changes none of them does not. Properties content is hashed in its
// canonical form (see SameContent) so that bytes written before the renderer sorted its
// keys still compare equal.
func ContentHash(d *NacosData) string {
	h := sha256.New()
	for _, part := range []string{
		d.Endpoint.Namespace + "/" + d.Endpoint.Name,
		d.Metadata.DataID,
		d.Metadata.Group,
		d.Metadata.NamespaceID,
		strings.ToLower(d.Format),
	} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	h.Write(canonicalContent(d.Format, d.Content))
	return hex.EncodeToString(h.Sum(nil))
}

// SameContent reports whether two renderings of the given format carry the same
// configuration. Properties files compare as key/value pairs, because the bytes the
// CLI published before the renderer sorted its keys list the same pairs in another
// order and with "key = value" spacing; every other format compares byte for byte.
func SameContent(format string, a, b []byte) bool {
	return bytes.Equal(canonicalContent(format, a), canonicalContent(format, b))
}

// canonicalContent returns the bytes to compare or hash. For properties it is the
// sorted "key=value" lines; content that does not parse as properties stays as it is.
func canonicalContent(format string, content []byte) []byte {
	if !strings.EqualFold(format, "properties") {
		return content
	}
	p, err := properties.Load(content, properties.UTF8)
	if err != nil {
		return content
	}
	kv := p.Map()
	keys := make([]string, 0, len(kv))
	for k := range kv {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	for _, k := range keys {
		buf.WriteString(k)
		buf.WriteByte('=')
		buf.WriteString(kv[k])
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}
