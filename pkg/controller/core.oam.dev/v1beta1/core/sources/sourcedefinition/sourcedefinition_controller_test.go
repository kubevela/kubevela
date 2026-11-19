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

package sourcedefinition

import (
	"regexp"
	"strings"
	"testing"

	oamctrl "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
)

func TestExtractSchemaExpr(t *testing.T) {
	template := `
schema: {
  image: string
  nested?: {
    tag: string
  }
}
output: {
  image: parameter.image
}
parameter: {
  image: string
}
`
	schema, err := extractSchemaExpr(template)
	if err != nil {
		t.Fatalf("extractSchemaExpr returned error: %v", err)
	}
	if schema == "" {
		t.Fatalf("extractSchemaExpr returned empty schema")
	}
}

func TestBuildSchemaTemplateNameStable(t *testing.T) {
	a := buildSchemaTemplateName("img-source", "abc123456789")
	b := buildSchemaTemplateName("img-source", "abc123456789")
	c := buildSchemaTemplateName("img-source", "def456456789")
	if a != b {
		t.Fatalf("expected deterministic name, got %s vs %s", a, b)
	}
	if a == c {
		t.Fatalf("expected different hash to produce different name")
	}
	if !strings.HasPrefix(a, "source-img-source-abc12345") {
		t.Fatalf("expected source name + short schema hash in template name, got %s", a)
	}
}

func TestBuildSchemaTemplateNameTruncatesAndSanitizes(t *testing.T) {
	name := "My_Source.Definition.With$Long@@Name-That-Keeps-Going-Longer-And-Longer"
	got := buildSchemaTemplateName(name, "0011223344556677")
	if len(got) > 63 {
		t.Fatalf("template name exceeds max length: %d", len(got))
	}
	if !strings.HasPrefix(got, "source-") {
		t.Fatalf("template name prefix invalid: %s", got)
	}
	if !strings.HasSuffix(got, "-00112233") {
		t.Fatalf("template hash suffix invalid: %s", got)
	}
}

// The name is a Kubernetes object name, so the failures are hard ones: too long
// and the ConfigTemplate cannot be created at all, unstable and every reconcile
// creates another.
func TestBuildSchemaTemplateNameAlwaysProducesALegalName(t *testing.T) {
	legal := regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	for _, tc := range []struct {
		name, hash string
	}{
		{"http-get", "abc123def456"},
		{"HTTP_Get", "abc123def456"},
		{strings.Repeat("a", 200), "abc123def456"},
		{strings.Repeat("-", 40), "abc123def456"},
		{"...", "abc123def456"},
		{"", "abc123def456"},
		{"日本語", "abc123def456"},
		// The hash is always a full sha256 hex here: reconcileSchemaTemplate
		// returns early when there is no schema to hash, so an empty one never
		// reaches this. Left untested rather than handled, since handling an
		// input that cannot occur is code nothing will ever exercise.
	} {
		got := buildSchemaTemplateName(tc.name, tc.hash)
		if len(got) > maxK8sNameLen {
			t.Errorf("%q: name is %d chars, over the %d limit: %s", tc.name, len(got), maxK8sNameLen, got)
		}
		if !legal.MatchString(got) {
			t.Errorf("%q: %q is not a legal object name", tc.name, got)
		}
		if !strings.HasPrefix(got, sourceTemplateNamePrefix) {
			t.Errorf("%q: %q lost its prefix, so it is not selectable as a source template", tc.name, got)
		}
	}
}

// Only the first sourceTemplateHashLen characters are used, so two schemas that
// differ later in the hash must still be told apart by what is kept.
func TestBuildSchemaTemplateNameUsesTheShortHash(t *testing.T) {
	a := buildSchemaTemplateName("http-get", "aaaaaaaabbbbbbbb")
	b := buildSchemaTemplateName("http-get", "aaaaaaaacccccccc")
	if a != b {
		t.Errorf("hashes agreeing in their first %d chars should give one name: %s vs %s",
			sourceTemplateHashLen, a, b)
	}
	c := buildSchemaTemplateName("http-get", "bbbbbbbbaaaaaaaa")
	if a == c {
		t.Error("hashes differing in the kept prefix must give different names")
	}
}

func TestSanitizeName(t *testing.T) {
	for in, want := range map[string]string{
		"http-get":       "http-get",
		"HTTP_Get":       "http-get",
		"a..b":           "a-b",
		"--lead-trail--": "lead-trail",
		"":               "",
		"...":            "",
		"a1b2":           "a1b2",
	} {
		if got := sanitizeName(in); got != want {
			t.Errorf("sanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
}

// A template with no schema block yields nothing, not an error: the webhook
// reports that, and the controller has no ConfigTemplate to register.
func TestExtractSchemaExprOnTemplatesWithoutOne(t *testing.T) {
	got, err := extractSchemaExpr(`output: {host: "x"}`)
	if err != nil || got != "" {
		t.Errorf("expected no schema and no error, got %q %v", got, err)
	}

	if _, err := extractSchemaExpr(`schema: {this is not cue`); err == nil {
		t.Error("a template that will not parse must report it")
	}
}

func TestParseOptionsCarriesTheControllerArgs(t *testing.T) {
	opts := parseOptions(oamctrl.Args{
		ConcurrentReconciles:                         7,
		DefRevisionLimit:                             3,
		IgnoreDefinitionWithoutControllerRequirement: true,
	})
	if opts.concurrentReconciles != 7 {
		t.Errorf("concurrentReconciles = %d, want 7", opts.concurrentReconciles)
	}
	if opts.defRevLimit != 3 {
		t.Errorf("defRevLimit = %d, want 3", opts.defRevLimit)
	}
	if !opts.ignoreDefNoCtrlReq {
		t.Error("ignoreDefNoCtrlReq should follow the arg")
	}
	if !opts.cacheGCEnabled || opts.cacheGCInterval != defaultCacheGCInterval {
		t.Error("the cache sweep is on by default, or entries accumulate forever")
	}
}
