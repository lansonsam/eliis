package pipeline

import "github.com/lansonsam/eliis/internal/core/types"

// Pipeline orchestrates middleware and protocol handlers (M0 placeholder).
type Pipeline struct{}

// New constructs an empty pipeline shell.
func New() *Pipeline {
	return &Pipeline{}
}

// Handle is reserved for the future unified entrypoint; not used in M0.
func (p *Pipeline) Handle(ctx *types.Context) error {
	_ = ctx
	return nil
}
