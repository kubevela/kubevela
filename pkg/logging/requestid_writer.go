/*
Copyright 2025 The KubeVela Authors.

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
	"io"
	"regexp"
	"strings"
	"sync"
)

// requestIDPattern matches `requestID="<uuid>"` anywhere in a log line.
// The leading boundary is non-greedy so it also handles ` requestID=` at line
// start (klog typically prepends a space before each key=value pair).
var requestIDPattern = regexp.MustCompile(`(?:^|\s+)requestID="([^"]+)"`)

// spanIDPattern matches `spanID="<uuid>"` or `spanID="<uuid>.<op>"`.
// Only the UUID portion is hoisted into the header; the full value stays in
// the trailing fields so sub-span operation names remain queryable.
var spanIDPattern = regexp.MustCompile(`\s+spanID="([^"]+)"`)

// requestIDInjector wraps an io.Writer and rewrites every klog-formatted log
// line so the requestID field is hoisted into the header, immediately after
// the PID. The trailing requestID="..." key/value is removed to avoid
// duplication.
//
// Input format (klog default):
//
//	I0527 23:08:32.627885   98950 file.go:42] "msg" k1=v1 requestID="abc" k2=v2
//
// Output:
//
//	I0527 23:08:32.627885   98950 {abc} file.go:42] "msg" k1=v1 k2=v2
//
// Lines without a requestID, or lines that don't match the klog header shape,
// pass through unchanged. This wrapper is intended for non-dev-logs runs;
// the dev-logs colorWriter performs the same hoisting with ANSI colours.
type requestIDInjector struct {
	dst io.Writer
	mu  sync.Mutex
	buf bytes.Buffer
}

// NewRequestIDInjector wraps dst so klog output has the requestID hoisted
// into the header. Safe for concurrent Write calls. The returned writer also
// implements Flush so callers can drain any buffered partial line at shutdown.
func NewRequestIDInjector(dst io.Writer) LineFormatter {
	return &requestIDInjector{dst: dst}
}

// Write accepts klog-formatted lines, hoists the requestID/spanID into the
// header for any complete line, and buffers any trailing partial line until
// the next Write or Flush. On downstream write failure the buffered line is
// retained so callers retrying the same input don't silently drop the line.
func (w *requestIDInjector) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	written := 0
	for len(p) > 0 {
		idx := bytes.IndexByte(p, '\n')
		if idx == -1 {
			// No newline in the remaining chunk — buffer it and wait for more.
			_, _ = w.buf.Write(p)
			written += len(p)
			break
		}
		_, _ = w.buf.Write(p[:idx])
		line := w.buf.String()
		// Hold the line; only commit (reset buffer + advance written) once the
		// downstream write succeeds. On failure the buffer keeps the line for
		// the next Write/Flush attempt.
		if _, err := io.WriteString(w.dst, injectRequestIDPlain(line)+"\n"); err != nil {
			return written, err
		}
		w.buf.Reset()
		p = p[idx+1:]
		written += idx + 1
	}
	return written, nil
}

// Flush writes any buffered partial line (one without a trailing newline) to
// the underlying writer. Idempotent; safe to call multiple times.
func (w *requestIDInjector) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.buf.Len() == 0 {
		return nil
	}
	line := w.buf.String()
	if _, err := io.WriteString(w.dst, injectRequestIDPlain(line)); err != nil {
		return err
	}
	w.buf.Reset()
	return nil
}

// injectRequestIDPlain rewrites a single klog-formatted line, hoisting the
// requestID="..." and spanID="..." fields into `{<id>}` blocks placed between
// the PID and the file:line. The requestID block is stripped from the trailing
// fields (full value in header); the spanID block is hoisted as just the UUID
// portion while the full spanID (with optional ".<operation>" suffix) stays in
// the trailing fields for sub-span detail. Returns the original line if it
// can't be parsed.
func injectRequestIDPlain(line string) string {
	if line == "" {
		return line
	}

	// Locate the end of the klog header: ']' closes "file.go:NN".
	closeBracket := strings.IndexByte(line, ']')
	if closeBracket == -1 {
		return line
	}
	header := line[:closeBracket+1]
	rest := line[closeBracket+1:]

	// Extract requestID and strip it from the trailing fields.
	var requestID string
	if m := requestIDPattern.FindStringSubmatchIndex(rest); m != nil {
		requestID = rest[m[2]:m[3]]
		rest = rest[:m[0]] + rest[m[1]:]
	}

	// Extract spanID for the header (UUID portion only). Keep the original
	// trailing `spanID="..."` field intact so the full value (with suffix)
	// remains queryable in log aggregators.
	var headerSpanID string
	if m := spanIDPattern.FindStringSubmatch(rest); m != nil {
		headerSpanID = stripSpanSuffix(m[1])
	}

	if requestID == "" && headerSpanID == "" {
		return line
	}

	// Inject `{requestID} {spanID} ` into the header just before the file:line.
	// Header shape: "I0527 23:08:32.627885   98950 file.go:42]".
	// The last single space separates PID from file.
	fileStart := strings.LastIndexByte(header, ' ')
	if fileStart < 0 {
		return line
	}

	var injected strings.Builder
	if requestID != "" {
		injected.WriteString("{")
		injected.WriteString(requestID)
		injected.WriteString("} ")
	}
	if headerSpanID != "" {
		injected.WriteString("{")
		injected.WriteString(headerSpanID)
		injected.WriteString("} ")
	}

	return header[:fileStart+1] + injected.String() + header[fileStart+1:] + rest
}
