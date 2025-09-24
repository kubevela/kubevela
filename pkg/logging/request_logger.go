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

package logging

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// contextKey is a private type to avoid key collisions in context.
type contextKey struct{ name string }

var requestIDKey = contextKey{name: "requestID"}

// WithRequestID returns a derived context that stores the given request ID.
func WithRequestID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestIDFrom returns the request ID stored in ctx (if any).
func RequestIDFrom(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(requestIDKey).(string)
	return v, ok && v != ""
}

// NewHandlerLogger creates a logger for a handler name that includes the requestID if present
// plus any additional structured key/value pairs. Any odd-length kv slice will have the last
// element dropped to prevent panics in logr implementations.
func NewHandlerLogger(ctx context.Context, handlerName string, req admission.Request, kv ...any) logr.Logger {
	l := log.Log
	if rid, ok := RequestIDFrom(ctx); ok {
		l = l.WithValues("requestID", rid)
	}
	if len(kv)%2 == 1 { // ensure even length
		kv = kv[:len(kv)-1]
	}
	l = l.WithValues("operation", req.Operation,
		"resource", req.Resource.String(),
		"name", req.Name,
		"namespace", req.Namespace)

	if len(kv) > 0 {
		l = l.WithValues(kv...)
	}
	return l
}

// WithValuesCtx augments a logger with context requestID (if not already present) and extra kv.
// Odd kv length will drop the last element safely.
func WithValuesCtx(ctx context.Context, base logr.Logger, kv ...interface{}) logr.Logger {
	if len(kv)%2 == 1 {
		kv = kv[:len(kv)-1]
	}
	if _, ok := RequestIDFrom(ctx); ok {
		kv = append(kv)
	}
	if len(kv) == 0 {
		return base
	}
	return base.WithValues(kv...)
}
