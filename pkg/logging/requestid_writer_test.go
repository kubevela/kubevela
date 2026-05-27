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

package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestInjectRequestIDPlain(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "hoists requestID after PID",
			in:   `I0527 23:08:32.627885   98950 file.go:42] "msg" k=v requestID="abc-123" k2=v2`,
			want: `I0527 23:08:32.627885   98950 {abc-123} file.go:42] "msg" k=v k2=v2`,
		},
		{
			name: "requestID at end of fields",
			in:   `I0527 23:08:32.627885   98950 file.go:42] "msg" k=v requestID="abc-123"`,
			want: `I0527 23:08:32.627885   98950 {abc-123} file.go:42] "msg" k=v`,
		},
		{
			name: "line without requestID is unchanged",
			in:   `I0527 23:08:32.627885   98950 file.go:42] "msg" k=v`,
			want: `I0527 23:08:32.627885   98950 file.go:42] "msg" k=v`,
		},
		{
			name: "no closing bracket - unchanged",
			in:   "broken line without bracket",
			want: "broken line without bracket",
		},
		{
			name: "empty line",
			in:   "",
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := injectRequestIDPlain(tc.in)
			if got != tc.want {
				t.Fatalf("input:\n  %s\nwant:\n  %s\ngot:\n  %s", tc.in, tc.want, got)
			}
		})
	}
}

func TestRequestIDInjector_Write(t *testing.T) {
	var buf bytes.Buffer
	w := NewRequestIDInjector(&buf)

	// Write two lines, one with a requestID, one without.
	input := `I0527 23:08:32.627885   98950 file.go:42] "msg-one" requestID="trace-7"` + "\n" +
		`I0527 23:08:32.627886   98950 file.go:43] "msg-two" k=v` + "\n"

	if _, err := w.Write([]byte(input)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, `{trace-7} file.go:42]`) {
		t.Fatalf("expected hoisted requestID, got: %s", out)
	}
	if strings.Contains(out, `requestID="trace-7"`) {
		t.Fatalf("expected trailing requestID stripped, got: %s", out)
	}
	if !strings.Contains(out, `file.go:43] "msg-two" k=v`) {
		t.Fatalf("expected second line unchanged, got: %s", out)
	}
}

func TestRequestIDInjector_HandlesSplitWrites(t *testing.T) {
	var buf bytes.Buffer
	w := NewRequestIDInjector(&buf)

	// Single logical line, split mid-stream across two Write calls.
	chunks := []string{
		`I0527 23:08:32.627885   98950 file.go:42] "msg" requestID="trace-`,
		`split-id"` + "\n",
	}
	for _, c := range chunks {
		if _, err := w.Write([]byte(c)); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	out := buf.String()
	if !strings.Contains(out, `{trace-split-id}`) {
		t.Fatalf("expected hoisted requestID across split writes, got: %s", out)
	}
}

func TestInjectRequestIDPlain_HoistsSpanIDAfterTraceID(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantHdr  string // header substring that must appear
		wantKeep string // trailing field that must NOT be stripped
		wantGone string // trailing field that MUST be stripped (empty = no check)
	}{
		{
			name:     "root span hoists bare UUID",
			in:       `I0527 23:08:32.627885   98950 file.go:42] "Start reconcile" requestID="trace-abc" application="ns/app" spanID="span-xyz"`,
			wantHdr:  `{trace-abc} {span-xyz} file.go:42]`,
			wantKeep: `spanID="span-xyz"`,
			wantGone: `requestID="trace-abc"`,
		},
		{
			name:     "sub-span hoists UUID portion only",
			in:       `I0527 23:08:32.627885   98950 file.go:42] "[Finished]: span-xyz.apply-policies(...)" requestID="trace-abc" spanID="span-xyz.apply-policies"`,
			wantHdr:  `{trace-abc} {span-xyz} file.go:42]`,
			wantKeep: `spanID="span-xyz.apply-policies"`,
			wantGone: `requestID="trace-abc"`,
		},
		{
			name:     "no spanID — only requestID hoisted (webhook log shape)",
			in:       `I0527 23:08:32.627885   98950 file.go:42] "validate" requestID="trace-abc" handler="ApplicationValidator"`,
			wantHdr:  `{trace-abc} file.go:42]`,
			wantKeep: `handler="ApplicationValidator"`,
			wantGone: `requestID="trace-abc"`,
		},
		{
			name:     "no requestID, only spanID",
			in:       `I0527 23:08:32.627885   98950 file.go:42] "msg" spanID="span-xyz.something"`,
			wantHdr:  `{span-xyz} file.go:42]`,
			wantKeep: `spanID="span-xyz.something"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := injectRequestIDPlain(tc.in)
			if !strings.Contains(got, tc.wantHdr) {
				t.Fatalf("header mismatch\n  in:   %s\n  want: %s\n  got:  %s", tc.in, tc.wantHdr, got)
			}
			if tc.wantKeep != "" && !strings.Contains(got, tc.wantKeep) {
				t.Fatalf("expected trailing field preserved\n  want: %s\n  got:  %s", tc.wantKeep, got)
			}
			if tc.wantGone != "" && strings.Contains(got, tc.wantGone) {
				t.Fatalf("expected trailing field stripped\n  gone: %s\n  got:  %s", tc.wantGone, got)
			}
		})
	}
}

func TestStripSpanSuffix(t *testing.T) {
	tests := map[string]string{
		"":                          "",
		"abc-123":                   "abc-123",
		"abc-123.create-app-handler": "abc-123",
		"abc-123.execute application workflow.55s33": "abc-123",
	}
	for in, want := range tests {
		if got := stripSpanSuffix(in); got != want {
			t.Errorf("stripSpanSuffix(%q) = %q, want %q", in, got, want)
		}
	}
}
