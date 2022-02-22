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

	// ErrRateLimit means exceed GitHub access rate limit
	ErrRateLimit = NewAddonError("exceed github access rate limit")

	// ErrNotExist  means addon not exists
	ErrNotExist = NewAddonError("addon not exist")

	// ErrVersionMismatch  means addon version requirement mismatch
	ErrVersionMismatch = NewAddonError("addon version requirements mismatch")
)

// WrapErrRateLimit return ErrRateLimit if is the situation, or return error directly
func WrapErrRateLimit(err error) error {
	errRate := &github.RateLimitError{}
	if errors.As(err, &errRate) {
		return ErrRateLimit
	}
	return err
}
