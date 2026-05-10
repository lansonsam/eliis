package contract

import "github.com/lansonsam/eliis/internal/core/types"

// Converter translates IR between two protocol namespaces (e.g. OpenAI → Anthropic).
type Converter interface {
	From() string
	To() string
	Convert(*types.UnifiedRequest) (*types.UnifiedRequest, error)
	ConvertChunk(*types.UnifiedChunk) (*types.UnifiedChunk, error)
}
