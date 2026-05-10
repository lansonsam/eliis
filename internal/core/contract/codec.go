package contract

import (
	"net/http"

	"github.com/lansonsam/eliis/internal/core/types"
)

// Codec encodes and decodes one wire protocol (OpenAI, Anthropic, Gemini, ...).
type Codec interface {
	Name() string
	DecodeRequest(*http.Request) (*types.UnifiedRequest, error)
	EncodeResponse(*types.UnifiedResponse, http.ResponseWriter) error

	DecodeStreamChunk([]byte) (*types.UnifiedChunk, error)
	EncodeStreamChunk(*types.UnifiedChunk) ([]byte, error)
}
