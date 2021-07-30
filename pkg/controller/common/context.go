package common

import (
	"context"
	"time"
)

const (
	reconcileTimeout = 60 * time.Second
)

// NewReconcileContext creates a context with timeout
func NewReconcileContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), reconcileTimeout)
}
