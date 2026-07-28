// Package cuex provides Helm-specific types for cuex.
package cuex

import (
	"fmt"
	"strings"

	"github.com/kubevela/kubevela/pkg/cue/cuex/types"
)

// CUEPostRenderParams represents the parameters for a CUE post-rendering operation.
type CUEPostRenderParams struct {
	Template string
}

// NewCUEPostRenderParams returns a new CUEPostRenderParams instance.
func NewCUEPostRenderParams(template string) *CUEPostRenderParams {
	return &CUEPostRenderParams{Template: template}
}

// Validate validates the CUEPostRenderParams instance.
func (p *CUEPostRenderParams) Validate() error {
	if p.Template == "" {
		return fmt.Errorf("template cannot be empty")
	}
	return nil
}

// String returns a string representation of the CUEPostRenderParams instance.
func (p *CUEPostRenderParams) String() string {
	return fmt.Sprintf("CUEPostRenderParams{Template:%q}", p.Template)
}