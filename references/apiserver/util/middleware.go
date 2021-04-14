/*
Copyright 2021 The KubeVela Authors.

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

package util

import (
	"context"
	"mime"

	"github.com/gin-gonic/gin"
	uuid "github.com/satori/go.uuid"
	"go.uber.org/zap/zapcore"
)

// Header Keys
const (
	// ContextKey is used as key to set/get context.
	ContextKey = "context"

	// ContentTypeJSON : json
	ContentTypeJSON = "application/json"

	// ContentTypeOctetStream: octet stream
	ContentTypeOctetStream = "application/octet-stream"

	// HeaderTraceID is header name for trace id.
	HeaderTraceID = "x-fc-trace-id"

	HeaderContentType = "content-Type"

	HeaderContentLength = "content-Length"

	// HeaderClientIP is the real IP of the remote client
	HeaderClientIP = "clientIP"
)

// ContextKeyType defining the context key type for the middleware
type ContextKeyType string

const (
	// ServiceLogFields shared key service log fields
	ServiceLogFields ContextKeyType = "ServiceLogFields"

	// HeaderRequestID is used as key to set/get request id.
	HeaderRequestID ContextKeyType = "x-fc-request-id"
)

// RESTful API paths
const (
	RootPath                = "/api"
	EnvironmentPath         = "/envs"
	ApplicationPath         = "/apps"
	WorkloadDefinitionPath  = "/workloads"
	ComponentDefinitionPath = "/components"
	ScopeDefinitionPath     = "/scopes"
	TraitDefinitionPath     = "/traits"
	CapabilityPath          = "/capabilities"
	CapabilityCenterPath    = "/capability-centers"
	VersionPath             = "/version"
	Definition              = "/definitions"
)

// NoRoute is a handler which is invoked when there is no route matches.
func NoRoute() gin.HandlerFunc {
	return func(c *gin.Context) {
		SetErrorAndAbort(c, PathNotSupported, c.Request.Method, c.Request.URL.Path)
	}
}

// generateRequestID :Get request id
func generateRequestID() string {
	id, _ := uuid.NewV4()
	return id.String()
}

// SetRequestID ...
func SetRequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := ""
		traceID := ""
		if traceID = c.Request.Header.Get(HeaderTraceID); traceID != "" {
			requestID = traceID
		} else if requestID = c.Request.Header.Get(string(HeaderRequestID)); requestID != "" {
			traceID = requestID
		} else {
			requestID = generateRequestID()
			traceID = requestID
		}
		c.Set(string(HeaderRequestID), requestID)
		c.Set(HeaderClientIP, c.ClientIP())
		c.Set(HeaderTraceID, traceID)
	}
}

// SetContext :Set context metadata for request
// Before get request
func SetContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.MustGet(string(HeaderRequestID)).(string)
		ctx, cancel := context.WithCancel(c.Request.Context())
		fields := make(map[string]zapcore.Field)
		mctx := context.WithValue(ctx, ServiceLogFields, fields)
		fctx := context.WithValue(mctx, HeaderRequestID, reqID)
		c.Set(ContextKey, fctx)
		c.Next()
		cancel()
	}
}

// GetContext get the context from the gin context
func GetContext(c *gin.Context) context.Context {
	return c.MustGet(ContextKey).(context.Context)
}

// ValidateHeaders validates the common headers.
//
// It reports one problem at a time.
func ValidateHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		// It's ok to not specify Content-Type header, but it should be correct if it's specified.
		contentType := c.Request.Header.Get(HeaderContentType)
		if len(contentType) != 0 {
			mType, _, err := mime.ParseMediaType(contentType)
			if err != nil {
				SetErrorAndAbort(c, UnsupportedMediaType)
				return
			}
			switch mType {
			case ContentTypeJSON, ContentTypeOctetStream:
				// Passes.
			default:
				SetErrorAndAbort(c, UnsupportedMediaType)
				return
			}
		}
	}
}
