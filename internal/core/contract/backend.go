package contract

import (
	"context"
	"io"

	"github.com/lansonsam/eliis/internal/core/types"
)

// Backend delivers IR requests to one concrete upstream implementation
// (OpenAI HTTP, Anthropic HTTP, Bedrock, Vertex, custom SDK, ...).
type Backend interface {
	Name() string
	Invoke(context.Context, *types.UnifiedRequest) (*types.UnifiedResponse, error)
	InvokeStream(context.Context, *types.UnifiedRequest) (StreamReader, error)
}

// StreamReader yields already-normalised IR chunks from a streaming backend.
type StreamReader interface {
	Recv() (*types.UnifiedChunk, error)
	Close() error
}

// EOF reports whether an error means the stream is exhausted.
func EOF(err error) bool {
	return err == io.EOF
}
