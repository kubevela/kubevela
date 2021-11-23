package addon

import (
	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"
)

// NewAddonError will return an
func NewAddonError(msg string) error {
	return errors.New(msg)
}

var (
	// ErrRenderCueTmpl is error when render addon's cue file
	ErrRenderCueTmpl = NewAddonError("fail to render cue tmpl")

	// ErrRateLimit means exceed github access rate limit
	ErrRateLimit = NewAddonError("exceed github access rate limit")
)

// WrapErrRateLimit return ErrRateLimit if is the situation, or return error directly
func WrapErrRateLimit(err error) error {
	errRate := &github.RateLimitError{}
	if errors.As(err, &errRate) {
		return ErrRateLimit
	}
	return err
}
