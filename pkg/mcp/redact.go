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

package mcp

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode"

	"k8s.io/apimachinery/pkg/runtime"
)

const (
	// redactedValue replaces anything the redactor removes. It is a fixed
	// string so a reader can tell a removed value from an empty one.
	redactedValue = "[redacted]"
	// maxTextLen caps free-text fields such as messages and reasons. Logs
	// have their own byte budget.
	maxTextLen = 2048
	truncMark  = "...[truncated]"
)

// Redaction here is pattern based and therefore best effort: it removes
// credential shapes that are common in Kubernetes objects, logs and error
// text, and it cannot know that an arbitrary string is a secret. Secret
// objects and ConfigMap data are handled structurally instead, by never
// returning their values.
var (
	// secretKey matches map keys and env names whose values are credentials
	// by convention. The cost of matching on names is that a harmless field
	// such as tokenCount is hidden too.
	secretKey = regexp.MustCompile(`(?i)(passw|passwd|pwd|secret|token|api[_-]?key|access[_-]?key|private[_-]?key|credential|authorization|cookie|dockerconfigjson|kubeconfig|tls\.key|(^|[^a-z0-9])pass([^a-z0-9]|$)|[_.-]key$)`)

	// secretLine patterns strip credentials that leak into text. Each
	// replacement keeps group 1 (the context that says what was removed)
	// and, for the PEM pattern, group 2. Order matters: the Authorization
	// header runs before the generic key pattern so the scheme word stays.
	secretLine = []struct {
		re   *regexp.Regexp
		repl string
	}{
		{regexp.MustCompile(`(?i)(authorization["']?\s*[:=]\s*["']?(?:(?:bearer|basic|token)\s+)?)[^\s"',;]+`), "${1}" + redactedValue},
		{regexp.MustCompile(`(?i)(bearer\s+)[a-z0-9._~+/=-]+`), "${1}" + redactedValue},
		{regexp.MustCompile(`(?i)((?:^|[^a-z0-9])[a-z0-9_.-]*(?:password|passwd|pwd|secret|token|api[_-]?key|access[_-]?key|private[_-]?key|credential)[a-z0-9_.-]*["']?\s*[:=]\s*["']?)[^\s"',;&]+`), "${1}" + redactedValue},
		{regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://[^/\s:@"']+:)[^@/\s"']+@`), "${1}" + redactedValue + "@"},
		{regexp.MustCompile(`(?i)([?&](?:access_token|id_token|refresh_token|token|api[_-]?key|apikey|key|secret|client_secret|password|passwd|sig|signature|private_token)=)[^&\s"']+`), "${1}" + redactedValue},
		{regexp.MustCompile(`()\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`), "${1}" + redactedValue},
		{regexp.MustCompile(`\b(gh[pousr]_)[A-Za-z0-9]{20,}`), "${1}" + redactedValue},
		{regexp.MustCompile(`\b(glpat-)[A-Za-z0-9_-]{20,}`), "${1}" + redactedValue},
		{regexp.MustCompile(`\b(xox[abprs]-)[A-Za-z0-9-]{10,}`), "${1}" + redactedValue},
		{regexp.MustCompile(`\b(sk-)[A-Za-z0-9_-]{20,}`), "${1}" + redactedValue},
		{regexp.MustCompile(`(hooks\.slack\.com/services/)[A-Za-z0-9/]+`), "${1}" + redactedValue},
		{regexp.MustCompile(`\b(AKIA)[0-9A-Z]{16}\b`), "${1}" + redactedValue},
		{regexp.MustCompile(`(-----BEGIN [A-Z ]*PRIVATE KEY-----)[\s\S]*?(-----END [A-Z ]*PRIVATE KEY-----)`), "${1}" + redactedValue + "${2}"},
	}

	// ansiEscape matches terminal control sequences that logs often carry.
	ansiEscape = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b[@-Z\\-_]`)

	// embeddedObjectAnnotations hold a serialized copy of an object, Secret
	// data included, so they are dropped rather than redacted.
	embeddedObjectAnnotations = []string{
		"kubectl.kubernetes.io/last-applied-configuration",
		"app.oam.dev/last-applied-configuration",
		"kubevela.io/last-applied-configuration",
	}
)

// redactMap returns a deep copy of v with the values of secret-like keys
// replaced, env-style {name, value} entries redacted when the name is
// secret-like, and every string leaf passed through redactLine. Nested
// maps and lists are walked.
func redactMap(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		secretName := false
		if name, ok := t["name"].(string); ok && secretKey.MatchString(name) {
			secretName = true
		}
		for k, val := range t {
			switch {
			case secretKey.MatchString(k):
				out[k] = redactedValue
			case secretName && k == "value":
				out[k] = redactedValue
			default:
				out[k] = redactMap(val)
			}
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = redactMap(val)
		}
		return out
	case string:
		return redactLine(t)
	default:
		return v
	}
}

// redactStringMap redacts labels, annotations and detail maps: embedded
// objects are dropped, secret-like keys and credential-shaped values are
// replaced.
func redactStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	for _, a := range embeddedObjectAnnotations {
		delete(out, a)
	}
	for k, v := range out {
		if secretKey.MatchString(k) {
			out[k] = redactedValue
			continue
		}
		out[k] = text(v)
	}
	return out
}

// redactObject prepares a live object for return: managedFields and
// annotations that embed a serialized object are removed, ConfigMap data
// is reduced to its keys, then the remainder is redacted like any other map.
func redactObject(content map[string]any) map[string]any {
	if meta, ok := content["metadata"].(map[string]any); ok {
		delete(meta, "managedFields")
		if annos, ok := meta["annotations"].(map[string]any); ok {
			for _, a := range embeddedObjectAnnotations {
				delete(annos, a)
			}
		}
	}
	if kind, _ := content["kind"].(string); kind == "ConfigMap" {
		for _, field := range []string{"data", "binaryData"} {
			if data, ok := content[field].(map[string]any); ok {
				keys := make([]any, 0, len(data))
				for k := range data {
					keys = append(keys, k)
				}
				content[field+"Keys"] = keys
				delete(content, field)
			}
		}
	}
	out, _ := redactMap(content).(map[string]any)
	return out
}

// redactLine removes credential-shaped substrings and terminal or Unicode
// control characters from a piece of text.
func redactLine(s string) string {
	s = sanitize(s)
	for _, p := range secretLine {
		s = p.re.ReplaceAllString(s, p.repl)
	}
	return s
}

// text is redactLine with a length cap, for message-like fields.
func text(s string) string {
	s = redactLine(s)
	if len(s) > maxTextLen {
		s = s[:maxTextLen] + truncMark
	}
	return s
}

// sanitize strips ANSI escapes and the control, bidirectional, zero-width
// and tag characters that can hide or reorder text in an agent's view.
func sanitize(s string) string {
	if !strings.ContainsFunc(s, isSuspectRune) {
		return s
	}
	s = ansiEscape.ReplaceAllString(s, "")
	return strings.Map(func(r rune) rune {
		if isSuspectRune(r) {
			return -1
		}
		return r
	}, s)
}

func isSuspectRune(r rune) bool {
	switch {
	case r == '\n' || r == '\t' || r == '\r':
		return false
	case r < 0x20 || r == 0x7f:
		return true
	case r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069: // bidi controls
		return true
	case r == 0x200B || r == 0x200C || r == 0x200D || r == 0x200E || r == 0x200F || r == 0x2060 || r == 0xFEFF: // zero width
		return true
	case r >= 0xE0000 && r <= 0xE007F: // Unicode tags
		return true
	case unicode.Is(unicode.Cf, r) && r != 0x00AD:
		return true
	}
	return false
}

// rawToRedactedMap decodes a RawExtension, such as component properties,
// into a map with secret-like values removed. A nil or empty extension
// gives an empty map; undecodable content is returned as a single
// "_raw" entry so nothing is silently dropped.
func rawToRedactedMap(raw *runtime.RawExtension) map[string]any {
	if raw == nil || len(raw.Raw) == 0 {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(raw.Raw, &m); err != nil {
		return map[string]any{"_raw": text(string(raw.Raw))}
	}
	out, _ := redactMap(m).(map[string]any)
	return out
}
