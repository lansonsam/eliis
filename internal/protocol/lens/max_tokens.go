package lens

import (
	"errors"

	"github.com/lansonsam/eliis/internal/core/types"
)

// EnsureMaxTokens fills UnifiedRequest.MaxTokens when the target protocol
// requires it and the inbound protocol left it unset.
type EnsureMaxTokens struct {
	Default int
}

func (l EnsureMaxTokens) Name() string {
	return "ensure_max_tokens"
}

func (l EnsureMaxTokens) Apply(req *types.UnifiedRequest) error {
	if req == nil {
		return errors.New("nil request")
	}
	if req.MaxTokens != nil {
		return nil
	}

	defaultMaxTokens := l.Default
	if defaultMaxTokens <= 0 {
		defaultMaxTokens = 1024
	}
	req.MaxTokens = &defaultMaxTokens
	return nil
}
