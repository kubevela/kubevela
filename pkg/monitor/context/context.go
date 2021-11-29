/*
 Copyright 2021. The KubeVela Authors.
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

package context

import (
	stdctx "context"
	"fmt"
	"time"

	"github.com/oam-dev/kubevela/pkg/utils"

	"k8s.io/klog/v2"
)

const (
	// spanTagID is the tag name of span ID.
	spanTagID = "spanID"
)

// Context keep the trace info
type Context interface {
	stdctx.Context
	Logger
	GetContext() stdctx.Context
	SetContext(ctx stdctx.Context)
	AddTag(keysAndValues ...interface{}) Context
	Fork(name string, exporters ...Exporter) Context
	Commit(msg string)
}

// Logger represents the ability to log messages, both errors and not.
type Logger interface {
	InfoDepth(depth int, msg string, keysAndValues ...interface{})
	Info(msg string, keysAndValues ...interface{})
	Error(err error, msg string, keysAndValues ...interface{})
	ErrorDepth(depth int, err error, msg string, keysAndValues ...interface{})
	Printf(format string, args ...interface{})
	V(level int)
}

type traceContext struct {
	stdctx.Context

	id             string
	beginTimestamp time.Time
	logLevel       int

	tags      []interface{}
	exporters []Exporter
	parent    *traceContext
}

// Fork a child Context extends parent Context
func (t *traceContext) Fork(id string, exporters ...Exporter) Context {
	if id == "" {
		id = t.id
	} else {
		id = t.id + "." + id
	}

	return &traceContext{
		Context:        t.Context,
		id:             id,
		tags:           copySlice(t.tags),
		logLevel:       t.logLevel,
		parent:         t,
		beginTimestamp: time.Now(),
		exporters:      exporters,
	}
}

// Commit finish the span record
func (t *traceContext) Commit(msg string) {
	msg = fmt.Sprintf("[Finished]: %s(%s)", t.id, msg)
	duration := time.Since(t.beginTimestamp)
	for _, export := range t.exporters {
		export(t, duration.Microseconds())
	}
	klog.InfoSDepth(1, msg, t.getTagsWith("duration", duration.String())...)
}

func (t *traceContext) getTagsWith(keysAndValues ...interface{}) []interface{} {
	tags := t.tags
	tags = append(tags, keysAndValues...)
	return append(tags, spanTagID, t.id)
}

// Info logs a non-error message with the given key/value pairs as context.
func (t *traceContext) Info(msg string, keysAndValues ...interface{}) {
	klog.InfoSDepth(1, msg, t.getTagsWith(keysAndValues...)...)
}

// GetContext get raw context.
func (t *traceContext) GetContext() stdctx.Context {
	return t.Context
}

// SetContext set raw context.
func (t *traceContext) SetContext(ctx stdctx.Context) {
	t.Context = ctx
}

// InfoDepth acts as Info but uses depth to determine which call frame to log.
func (t *traceContext) InfoDepth(depth int, msg string, keysAndValues ...interface{}) {
	klog.InfoSDepth(depth+1, msg, t.getTagsWith(keysAndValues...)...)
}

// Error logs an error, with the given message and key/value pairs as context.
func (t *traceContext) Error(err error, msg string, keysAndValues ...interface{}) {
	klog.ErrorSDepth(1, err, msg, t.getTagsWith(keysAndValues...)...)
}

// ErrorDepth acts as Error but uses depth to determine which call frame to log.
func (t *traceContext) ErrorDepth(depth int, err error, msg string, keysAndValues ...interface{}) {
	klog.ErrorSDepth(depth+1, err, msg, t.getTagsWith(keysAndValues...)...)
}

// Printf formats according to a format specifier and logs.
func (t *traceContext) Printf(format string, args ...interface{}) {
	klog.InfoSDepth(1, fmt.Sprintf(format, args...), t.getTagsWith()...)
}

// V reports whether verbosity at the call site is at least the requested level.
func (t *traceContext) V(level int) {
	t.logLevel = level
}

// AddTag adds some key-value pairs of context to a logger.
func (t *traceContext) AddTag(keysAndValues ...interface{}) Context {
	t.tags = append(t.tags, keysAndValues...)
	return t
}

// NewTraceContext new a TraceContext
func NewTraceContext(ctx stdctx.Context, id string) Context {
	if id == "" {
		id = "i-" + utils.RandomString(8)
	}
	return &traceContext{
		Context:        ctx,
		id:             id,
		beginTimestamp: time.Now(),
	}
}

func copySlice(in []interface{}) []interface{} {
	out := make([]interface{}, len(in))
	copy(out, in)
	return out
}

// Exporter export context info.
type Exporter func(t *traceContext, duration int64)

// DurationMetric export context duration metric.
func DurationMetric(h func(v float64)) Exporter {
	return func(t *traceContext, duration int64) {
		h(float64(duration / 1000))
	}
}
