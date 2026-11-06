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

package main

import "testing"

func TestEscapeProse(t *testing.T) {
	cases := map[string]string{
		"a config-template-<name> ConfigMap":   "a config-template-&lt;name&gt; ConfigMap",
		"run `vela up <file>` to deploy":       "run `vela up <file>` to deploy",
		"`<a>` and <b> and `<c>`":              "`<a>` and &lt;b&gt; and `<c>`",
		"* [vela mcp](vela_mcp.md)\t - Serve.": "* [vela mcp](vela_mcp.md)\t - Serve.",
		"":                                     "",
	}
	for in, want := range cases {
		if got := escapeProse(in); got != want {
			t.Errorf("escapeProse(%q) = %q, want %q", in, got, want)
		}
	}
}
