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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"github.com/magiconair/properties"

	"github.com/oam-dev/kubevela/pkg/cue/script"
)

// Formats the nacos writer encodes. Anything else renders as yaml today, silently;
// ValidateExpandedWriter names the choice instead.
var supportedFormats = []string{"json", "yaml", "toml", "properties"}

// ValidateTemplate is the Config reconciler's entry to ValidateExpandedWriter, the check it
// shares with the ConfigTemplate webhook so the two cannot drift: it parses the whole
// template script the way the Secret render does (CueX, provider functions disabled, so a
// template that imports a vela package is read like any other) and validates its nacos
// block. The webhook parses once itself, for this and for the endpoint rule, and calls
// ValidateExpandedWriter on the same value. A script that does not compile passes here:
// the ConfigTemplate controller reports that through status.phase, and in the reconciler
// the Secret render fails on it right after this check.
func ValidateTemplate(ctx context.Context, template script.CUE) error {
	if v, err := template.ParseToValueWithCueX(ctx); err == nil {
		return ValidateExpandedWriter(v.LookupPath(cue.ParsePath("template")))
	}
	return nil
}

// ValidateExpandedWriter is the strict reading of a template's nacos block, behind
// ValidateTemplate. ParseExpandedWriterConfig stays lenient for the CLI. A template
// without the block is valid. With it: the block is a struct, endpoint.name is a
// concrete non-empty string, metadata is a struct that defines dataId as a string and
// may define group as one (incomplete is fine, they usually reference the parameter, but
// a concrete non-string is refused; a missing group means DEFAULT_GROUP at render time),
// and format, a string, is one of the supported ones when concrete. The checks are
// structural because the block is not decodable before the parameters are merged in.
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
		return errors.New("nacos.metadata is required, with dataId")
	}
	if metadata.IncompleteKind() != cue.StructKind {
		return errors.New("nacos.metadata must be a struct")
	}
	dataID := metadata.LookupPath(cue.ParsePath("dataId"))
	if !dataID.Exists() {
		return errors.New("nacos.metadata.dataId is required")
	}
	if !mayBeString(dataID) {
		return errors.New("nacos.metadata.dataId must be a string")
	}
	// group is optional: the SDK publishes to and reads from DEFAULT_GROUP when it is
	// empty, and templates written before the controller published never had to set it
	if group := metadata.LookupPath(cue.ParsePath("group")); group.Exists() && !mayBeString(group) {
		return errors.New("nacos.metadata.group must be a string")
	}
	format := nacos.LookupPath(cue.ParsePath("format"))
	if format.Exists() {
		if !mayBeString(format) {
			return errors.New("nacos.format must be a string")
		}
		if s, err := format.String(); err == nil && !isSupportedFormat(s) {
			return fmt.Errorf("nacos.format %q is not one of %s", s, strings.Join(supportedFormats, ", "))
		}
	}
	return nil
}

// mayBeString reports whether v can still become a string. Only a value whose kind is
// settled and is not a string is refused: 42, or a reference to an int parameter. A
// concrete string, a reference to a string parameter and an open value pass, and so
// does anything CUE cannot evaluate before the parameters are merged in, such as an
// interpolation "\(context.name)-db" or a reference to an optional parameter: those
// carry an incomplete error here and the render decides. The error can only be the
// incomplete kind: a settled conflict would have failed the template's compile before
// anything looked at this field.
func mayBeString(v cue.Value) bool {
	return v.Err() != nil || v.IncompleteKind()&cue.StringKind != 0
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
// dataId, group, namespaceId, appName and tag, the format and the content. The Config
// status stores it and the reconciler compares it, so a change to any of them publishes
// again and a reconcile that changes none of them does not. appName and tag reach the
// Nacos client and the publish request, so a change to either must publish too.
// Properties content is hashed in its canonical form (see SameContent) so that bytes
// written before the renderer sorted its keys still compare equal.
func ContentHash(d *NacosData) string {
	h := sha256.New()
	for _, part := range []string{
		d.Endpoint.Namespace + "/" + d.Endpoint.Name,
		d.Metadata.DataID,
		d.Metadata.Group,
		d.Metadata.NamespaceID,
		d.Metadata.AppName,
		d.Metadata.Tag,
		strings.ToLower(d.Format),
	} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	h.Write(canonicalContent(d.Format, d.Content))
	return hex.EncodeToString(h.Sum(nil))
}

// endpointCredentialKeys are the keys of the endpoint config's client section that never
// enter EndpointHash: the digest lands in the Config status, which anyone who can read
// Configs can read, and a digest of a password is a password to brute-force offline.
// Rotating them changes nothing in Nacos, so they need no re-sync either.
var endpointCredentialKeys = []string{"username", "password", "accessKey", "secretKey"}

// EndpointHash is a digest of the server part of the endpoint config the content is
// synced through: the properties of the config made from the nacos-server template, in
// canonical JSON, with the client credentials left out. The Config status stores it next
// to the content hash, so a changed endpoint (another server) is treated like changed
// content: Nacos is read first and the content published only when the server does not
// hold it. The error is for a value json cannot marshal, which a config decoded from
// JSON never carries.
func EndpointHash(endpointConfig map[string]interface{}) (string, error) {
	projected := maps.Clone(endpointConfig)
	if client, ok := projected[EndpointClientKey].(map[string]interface{}); ok {
		server := maps.Clone(client)
		maps.DeleteFunc(server, func(key string, _ interface{}) bool { return slices.Contains(endpointCredentialKeys, key) })
		projected[EndpointClientKey] = server
	}
	canonical, err := json.Marshal(projected)
	if err != nil {
		return "", fmt.Errorf("the endpoint config cannot be digested: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
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
