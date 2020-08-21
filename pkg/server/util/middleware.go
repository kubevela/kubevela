package util

import (
	"context"
	"mime"

	"github.com/gin-gonic/gin"
	uuid "github.com/satori/go.uuid"
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Header Keys
const (
	// ContextKey is used as key to set/get context.
	ContextKey = "context"

	// HeaderRequestID is used as key to set/get request id.
	HeaderRequestID = "x-fc-request-id"

	// ContentTypeJSON : json
	ContentTypeJSON = "application/json"

	// ContentTypeOctetStream: octet stream
	ContentTypeOctetStream = "application/octet-stream"

	// HeaderTraceID is header name for trace id.
	HeaderTraceID = "x-fc-trace-id"

	HeaderContentType = "content-Type"

	HeaderContentLength = "content-Length"

	// ServiceLogFields shared key service log fields
	ServiceLogFields = "ServiceLogFields"

	// HeaderClientIP is the real IP of the remote client
	HeaderClientIP = "clientIP"
)

const (
	// RESTful API paths
	RootPath               = "/api"
	EnvironmentPath        = "/envs"
	ApplicationPath        = "/apps"
	WorkloadDefinitionPath = "/workloads"
	ScopeDefinitionPath    = "/scopes"
	TraitDefinitionPath    = "/traits"
	RepoPath               = "/category"
	VersionPath            = "/version"
)

const contextLoggerKey = "logger"

//NoRoute is a handler which is invoked when there is no route matches.
func NoRoute() gin.HandlerFunc {
	return func(c *gin.Context) {
		SetErrorAndAbort(c, PathNotSupported, c.Request.Method, c.Request.URL.Path)
	}
}

//generateRequestID :Get request id
func generateRequestID() string {
	return uuid.NewV4().String()
}

// SetRequestID ...
func SetRequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := ""
		traceID := ""
		if traceID = c.Request.Header.Get(HeaderTraceID); traceID != "" {
			requestID = traceID
		} else if requestID = c.Request.Header.Get(HeaderRequestID); requestID != "" {
			traceID = requestID
		} else {
			requestID = generateRequestID()
			traceID = requestID
		}
		c.Set(HeaderRequestID, requestID)
		c.Set(HeaderClientIP, c.ClientIP())
		c.Set(HeaderTraceID, traceID)
	}
}

// SetContext :Set context metadata for request
// Before get request
func SetContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.MustGet(HeaderRequestID).(string)
		ctx, cancel := context.WithCancel(c.Request.Context())
		fields := make(map[string]zapcore.Field)
		mctx := context.WithValue(ctx, ServiceLogFields, fields)
		fctx := context.WithValue(mctx, HeaderRequestID, reqID)
		c.Set(ContextKey, fctx)
		c.Next()
		cancel()
	}
}

// get the context from the gin context
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

func StoreClient(kubeClient client.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("KubeClient", kubeClient)
		c.Next()
	}
}
