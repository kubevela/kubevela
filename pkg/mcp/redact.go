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

	"k8s.io/apimachinery/pkg/runtime"
)

// redactedValue replaces anything the redactor removes. It is a fixed
// string so a reader can tell a removed value from an empty one.
const redactedValue = "[redacted]"

var (
	// secretKey matches map keys whose values are credentials by convention.
	// Matching is on the key, not the value, so an unusual password still
	// goes; the cost is that a field called tokenCount is also hidden.
	secretKey = regexp.MustCompile(`(?i)(password|passwd|secret|token|api[_-]?key|access[_-]?key|private[_-]?key|credential|authorization|cookie)`)

	// secretLine patterns strip credentials that leak into log lines and
	// error messages: Authorization headers with their scheme kept, bare
	// bearer tokens, key=value assignments of secret-like names, AWS access
	// key ids and PEM private keys. Order matters: the header pattern runs
	// first so the generic key pattern does not eat the scheme word.
	secretLine = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(authorization\s*[:=]\s*["']?(?:(?:bearer|basic|token)\s+)?)[^\s"',;]+`),
		regexp.MustCompile(`(?i)(bearer\s+)[a-z0-9._~+/=-]+`),
		regexp.MustCompile(`(?i)\b((?:password|passwd|secret|token|api[_-]?key|access[_-]?key)\s*[:=]\s*["']?)[^\s"',;]+`),
		regexp.MustCompile(`\b(AKIA)[0-9A-Z]{16}\b`),
		regexp.MustCompile(`(-----BEGIN [A-Z ]*PRIVATE KEY-----)[\s\S]*?(-----END [A-Z ]*PRIVATE KEY-----)`),
	}
)

// redactMap returns a deep copy of v with the values of secret-like keys
// replaced. Nested maps and lists are walked; other values are kept.
func redactMap(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if secretKey.MatchString(k) {
				out[k] = redactedValue
				continue
			}
			out[k] = redactMap(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = redactMap(val)
		}
		return out
	default:
		return v
	}
}

// redactLine removes credential-shaped substrings from a line of text.
func redactLine(s string) string {
	last := len(secretLine) - 1
	for i, re := range secretLine {
		if i == last {
			s = re.ReplaceAllString(s, "${1}"+redactedValue+"${2}")
			continue
		}
		s = re.ReplaceAllString(s, "${1}"+redactedValue)
	}
	return s
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
		return map[string]any{"_raw": redactLine(string(raw.Raw))}
	}
	out, _ := redactMap(m).(map[string]any)
	return out
}
