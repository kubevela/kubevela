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
	"context"
	"testing"

	"github.com/go-logr/logr/funcr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/pkg/oam"
)

func TestRequestIDContext(t *testing.T) {
	tests := []struct {
		name      string
		requestID string
		wantID    string
		wantOK    bool
		wantSame  bool
	}{
		{
			name:      "round-trip",
			requestID: "request-1",
			wantID:    "request-1",
			wantOK:    true,
		},
		{
			name:     "empty is no-op",
			wantOK:   false,
			wantSame: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base := context.Background()
			ctx := WithRequestID(base, tc.requestID)
			if tc.wantSame && ctx != base {
				t.Fatalf("expected context to be unchanged")
			}

			gotID, gotOK := RequestIDFrom(ctx)
			if gotOK != tc.wantOK {
				t.Fatalf("ok: want %v, got %v", tc.wantOK, gotOK)
			}
			if gotID != tc.wantID {
				t.Fatalf("id: want %q, got %q", tc.wantID, gotID)
			}
		})
	}
}

func TestSpanIDContext(t *testing.T) {
	tests := []struct {
		name     string
		spanID   string
		wantID   string
		wantOK   bool
		wantSame bool
	}{
		{
			name:   "round-trip",
			spanID: "span-1",
			wantID: "span-1",
			wantOK: true,
		},
		{
			name:     "empty is no-op",
			wantOK:   false,
			wantSame: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base := context.Background()
			ctx := WithSpanID(base, tc.spanID)
			if tc.wantSame && ctx != base {
				t.Fatalf("expected context to be unchanged")
			}

			gotID, gotOK := SpanIDFrom(ctx)
			if gotOK != tc.wantOK {
				t.Fatalf("ok: want %v, got %v", tc.wantOK, gotOK)
			}
			if gotID != tc.wantID {
				t.Fatalf("id: want %q, got %q", tc.wantID, gotID)
			}
		})
	}
}

func TestTraceIDFromObject(t *testing.T) {
	tests := []struct {
		name   string
		obj    metav1.Object
		wantID string
		wantOK bool
	}{
		{
			name:   "nil object returns false",
			obj:    nil,
			wantOK: false,
		},
		{
			name:   "nil annotations returns false",
			obj:    &metav1.ObjectMeta{},
			wantOK: false,
		},
		{
			name:   "missing key returns false",
			obj:    &metav1.ObjectMeta{Annotations: map[string]string{"other": "value"}},
			wantOK: false,
		},
		{
			name:   "empty value returns false",
			obj:    &metav1.ObjectMeta{Annotations: map[string]string{oam.AnnotationTraceID: ""}},
			wantOK: false,
		},
		{
			name:   "present value returns it",
			obj:    &metav1.ObjectMeta{Annotations: map[string]string{oam.AnnotationTraceID: "trace-1"}},
			wantID: "trace-1",
			wantOK: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotID, gotOK := TraceIDFromObject(tc.obj)
			if gotOK != tc.wantOK {
				t.Fatalf("ok: want %v, got %v", tc.wantOK, gotOK)
			}
			if gotID != tc.wantID {
				t.Fatalf("id: want %q, got %q", tc.wantID, gotID)
			}
		})
	}
}

func TestEnsureTraceIDAnnotation(t *testing.T) {
	tests := []struct {
		name         string
		obj          metav1.Object
		traceID      string
		wantMutated  bool
		wantStoredID string
	}{
		{
			name:        "nil object returns false",
			obj:         nil,
			traceID:     "trace-1",
			wantMutated: false,
		},
		{
			name:        "empty traceID is no-op",
			obj:         &metav1.ObjectMeta{},
			traceID:     "",
			wantMutated: false,
		},
		{
			name:         "writes when annotations is nil",
			obj:          &metav1.ObjectMeta{},
			traceID:      "trace-1",
			wantMutated:  true,
			wantStoredID: "trace-1",
		},
		{
			name:         "writes when annotation key missing",
			obj:          &metav1.ObjectMeta{Annotations: map[string]string{"keep": "me"}},
			traceID:      "trace-2",
			wantMutated:  true,
			wantStoredID: "trace-2",
		},
		{
			name:         "writes when annotation value is empty",
			obj:          &metav1.ObjectMeta{Annotations: map[string]string{oam.AnnotationTraceID: ""}},
			traceID:      "trace-3",
			wantMutated:  true,
			wantStoredID: "trace-3",
		},
		{
			name:         "leaves existing value alone",
			obj:          &metav1.ObjectMeta{Annotations: map[string]string{oam.AnnotationTraceID: "original"}},
			traceID:      "ignored",
			wantMutated:  false,
			wantStoredID: "original",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := EnsureTraceIDAnnotation(tc.obj, tc.traceID)
			if got != tc.wantMutated {
				t.Fatalf("mutated: want %v, got %v", tc.wantMutated, got)
			}
			if tc.obj == nil {
				return
			}
			stored := tc.obj.GetAnnotations()[oam.AnnotationTraceID]
			if stored != tc.wantStoredID {
				t.Fatalf("stored id: want %q, got %q", tc.wantStoredID, stored)
			}
		})
	}
}

func TestEnsureTraceIDAnnotationPreservesOtherKeys(t *testing.T) {
	meta := &metav1.ObjectMeta{Annotations: map[string]string{"keep": "me"}}
	if mutated := EnsureTraceIDAnnotation(meta, "trace-x"); !mutated {
		t.Fatalf("expected mutated=true")
	}
	annotations := meta.GetAnnotations()
	if annotations["keep"] != "me" {
		t.Fatalf("expected existing annotations to be preserved, got %v", annotations)
	}
	if annotations[oam.AnnotationTraceID] != "trace-x" {
		t.Fatalf("expected traceID to be set, got %v", annotations)
	}
}

func TestFromContextAttachesRequestAndSpanIDsToOutput(t *testing.T) {
	var buf bytes.Buffer
	ctx := captureLogger(t, context.Background(), &buf)
	ctx = WithRequestID(ctx, "request-1")
	ctx = WithSpanID(ctx, "span-1")

	FromContext(ctx).Info("hello")

	out := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte(`"requestID":"request-1"`)) {
		t.Fatalf("expected output to contain requestID=request-1, got: %s", out)
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"spanID":"span-1"`)) {
		t.Fatalf("expected output to contain spanID=span-1, got: %s", out)
	}
}

func TestFromContextOmitsRequestAndSpanIDsWhenAbsent(t *testing.T) {
	var buf bytes.Buffer
	ctx := captureLogger(t, context.Background(), &buf)

	FromContext(ctx).Info("hello")

	if bytes.Contains(buf.Bytes(), []byte(`"requestID"`)) {
		t.Fatalf("expected no requestID key when ctx has none, got: %s", buf.String())
	}
	if bytes.Contains(buf.Bytes(), []byte(`"spanID"`)) {
		t.Fatalf("expected no spanID key when ctx has none, got: %s", buf.String())
	}
}

func captureLogger(t *testing.T, ctx context.Context, buf *bytes.Buffer) context.Context {
	t.Helper()
	base := funcr.NewJSON(func(line string) {
		buf.WriteString(line + "\n")
	}, funcr.Options{})
	return Logger{Logger: base}.IntoContext(ctx)
}
