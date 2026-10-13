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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("redaction", func() {
	It("replaces values of secret-like keys at any depth", func() {
		in := map[string]any{
			"image":    "nginx",
			"password": "x",
			"nested":   map[string]any{"apiKey": "k", "port": 80, "list": []any{map[string]any{"secretRef": "s", "name": "n"}}},
		}
		out := redactMap(in).(map[string]any)
		Expect(out).To(HaveKeyWithValue("image", "nginx"))
		Expect(out).To(HaveKeyWithValue("password", redactedValue))
		nested := out["nested"].(map[string]any)
		Expect(nested).To(HaveKeyWithValue("apiKey", redactedValue))
		Expect(nested).To(HaveKeyWithValue("port", 80))
		item := nested["list"].([]any)[0].(map[string]any)
		Expect(item).To(HaveKeyWithValue("secretRef", redactedValue))
		Expect(item).To(HaveKeyWithValue("name", "n"))
		Expect(in["password"]).To(Equal("x"), "input is not mutated")
	})

	It("strips credential-shaped substrings from lines", func() {
		Expect(redactLine("GET / Authorization: Bearer abc.def-ghi done")).To(Equal("GET / Authorization: Bearer " + redactedValue + " done"))
		Expect(redactLine(`token="t0k3n", user="bob"`)).To(Equal(`token="` + redactedValue + `", user="bob"`))
		Expect(redactLine("authorization: Basic dXNlcjpwYXNz")).To(Equal("authorization: Basic " + redactedValue))
		Expect(redactLine("API_KEY: sk-live-123")).To(Equal("API_KEY: " + redactedValue))
		Expect(redactLine("key AKIAIOSFODNN7EXAMPLE here")).To(Equal("key AKIA" + redactedValue + " here"))
		Expect(redactLine("plain line with no secrets")).To(Equal("plain line with no secrets"))
		pem := "-----BEGIN RSA PRIVATE KEY-----\nMIIE...\n-----END RSA PRIVATE KEY-----"
		Expect(redactLine(pem)).To(Equal("-----BEGIN RSA PRIVATE KEY-----" + redactedValue + "-----END RSA PRIVATE KEY-----"))
	})

	It("decodes raw properties and keeps undecodable content visible", func() {
		Expect(rawToRedactedMap(nil)).To(BeEmpty())
		Expect(rawToRedactedMap(&runtime.RawExtension{Raw: []byte(`{"a":1,"token":"t"}`)})).To(Equal(map[string]any{"a": float64(1), "token": redactedValue}))
		Expect(rawToRedactedMap(&runtime.RawExtension{Raw: []byte(`not json token=abc`)})).To(HaveKeyWithValue("_raw", "not json token="+redactedValue))
	})

	It("bounds log reads by bytes and reports truncation", func() {
		lines, truncated, err := readLines(strings.NewReader("one\ntwo\nthree\n"), 1<<10)
		Expect(err).NotTo(HaveOccurred())
		Expect(truncated).To(BeFalse())
		Expect(lines).To(Equal([]string{"one", "two", "three"}))

		lines, truncated, err = readLines(strings.NewReader("one\ntwo\nthree\n"), 7)
		Expect(err).NotTo(HaveOccurred())
		Expect(truncated).To(BeTrue())
		Expect(lines).To(Equal([]string{"one"}))
	})
})
