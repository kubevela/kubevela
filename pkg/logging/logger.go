/*
Copyright 2024 The KubeVela Authors.

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

// Package logging provides structured logging utilities for KubeVela webhooks
// with focus on request traceability and observability.
package logging

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Structured logging field keys - consistent across all handlers for observability
const (
	// Core traceability fields
	FieldRequestID = "requestID"  // Unique identifier for request correlation
	FieldOperation = "operation"  // Webhook operation (CREATE/UPDATE/DELETE)
	FieldHandler   = "handler"    // Handler processing the request
	FieldStep      = "step"       // Current processing step
	FieldDuration  = "durationMs" // Operation duration in milliseconds

	// Resource identification
	FieldName       = "name"       // Resource name
	FieldNamespace  = "namespace"  // Resource namespace
	FieldKind       = "kind"       // Resource kind
	FieldGeneration = "generation" // Resource generation

	// User context
	FieldUserName = "user" // User making the request

	// Error tracking
	FieldError   = "error"   // Error indicator
	FieldSuccess = "success" // Success indicator
)

// contextKey for storing values in context
type contextKey struct{ name string }

var (
	requestIDKey = contextKey{name: "requestID"}
	loggerKey    = contextKey{name: "logger"}
)

// Logger wraps logr.Logger with structured logging methods
type Logger struct {
	logr.Logger
}

// WithValues adds key-value pairs to the logger
func (l Logger) WithValues(keysAndValues ...interface{}) Logger {
	return Logger{Logger: l.Logger.WithValues(keysAndValues...)}
}

// New creates a new Logger
func New() Logger {
	return Logger{Logger: log.Log}
}

// WithContext returns a Logger from context or creates a new one
func WithContext(ctx context.Context) Logger {
	if logger, ok := ctx.Value(loggerKey).(Logger); ok {
		return logger
	}
	return New()
}

// IntoContext stores the Logger in context
func (l Logger) IntoContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, loggerKey, l)
}

// WithRequestID stores request ID in context
func WithRequestID(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey, requestID)
}

// RequestIDFrom retrieves request ID from context
func RequestIDFrom(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(requestIDKey).(string)
	return id, ok && id != ""
}

// NewHandlerLogger creates a logger for webhook handlers with full request context
func NewHandlerLogger(ctx context.Context, req admission.Request, handlerName string) Logger {
	logger := New()

	// Use admission UID as request ID for correlation
	requestID := string(req.UID)
	if rid, ok := RequestIDFrom(ctx); ok && rid != "" {
		requestID = rid
	}

	// Build structured log with essential fields for observability
	logger = logger.WithValues(
		FieldRequestID, requestID,
		FieldHandler, handlerName,
		FieldOperation, req.Operation,
		FieldKind, req.Kind.Kind,
		FieldName, req.Name,
		FieldNamespace, req.Namespace,
		FieldUserName, req.UserInfo.Username,
	)

	return logger
}

// Helper methods that return the logger with values added
// These don't log directly, so the actual logging call site is preserved

// WithStep adds a step field to the logger
func (l Logger) WithStep(step string) Logger {
	return l.WithValues(FieldStep, step)
}

// WithSuccess adds success and duration fields to the logger
func (l Logger) WithSuccess(success bool, startTime ...time.Time) Logger {
	logger := l.WithValues(FieldSuccess, success)
	if len(startTime) > 0 {
		duration := time.Since(startTime[0])
		logger = logger.WithValues(FieldDuration, duration.Milliseconds())
	}
	return logger
}

// WithError adds error context to the logger
func (l Logger) WithError(err error) Logger {
	return l.WithValues(FieldError, err.Error(), FieldSuccess, false)
}

// V returns a logger with verbosity level (0=info, 1=debug, 2=trace)
func (l Logger) V(level int) Logger {
	return Logger{Logger: l.Logger.V(level)}
}

// Debug logs debug message (verbosity 1)
func (l Logger) Debug(msg string, keysAndValues ...interface{}) {
	l.Logger.V(1).Info(msg, keysAndValues...)
}

// Trace logs trace message (verbosity 2)
func (l Logger) Trace(msg string, keysAndValues ...interface{}) {
	l.Logger.V(2).Info(msg, keysAndValues...)
}

// Info logs info message
func (l Logger) Info(msg string, keysAndValues ...interface{}) {
	l.Logger.Info(msg, keysAndValues...)
}

// Error logs error message
func (l Logger) Error(err error, msg string, keysAndValues ...interface{}) {
	l.Logger.Error(err, msg, keysAndValues...)
}
