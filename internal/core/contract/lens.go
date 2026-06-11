package contract

import "github.com/lansonsam/eliis/internal/core/types"

// Lens is an atomic IR-to-IR transformation applied between codec.Decode
// (inbound) and codec.Encode (outbound). Lenses are composed declaratively
// per route to absorb cross-protocol differences (e.g. EnsureMaxTokens for
// Anthropic, DropExtraNamespace for cross-protocol forwarding).
//
// Lenses MUST be pure functions of the input IR; side effects (logging,
// metrics) belong in middleware. They mutate the request in place and
// return an error only on validation failure.
//
// Streaming chunk transformation is intentionally out of scope for v1:
// most cross-protocol differences are request-shape concerns. If a future
// lens needs to rewrite per-chunk payloads, extend this interface with
// ApplyChunk(*types.UnifiedChunk) error rather than introducing a new type.
type Lens interface {
	Name() string
	Apply(*types.UnifiedRequest) error
}

// LensChain is a sequenced composition of Lens, applied left-to-right.
// A nil or empty chain is a valid no-op.
type LensChain []Lens

// Apply runs every lens in order, short-circuiting on the first error.
func (c LensChain) Apply(r *types.UnifiedRequest) error {
	for _, l := range c {
		if err := l.Apply(r); err != nil {
			return err
		}
	}
	return nil
}
