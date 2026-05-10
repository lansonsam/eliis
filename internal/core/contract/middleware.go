package contract

import "github.com/lansonsam/eliis/internal/core/types"

// Handler is the next step in the middleware chain.
type Handler func(ctx *types.Context) error

// Middleware participates in the request pipeline (auth, ratelimit, logging, ...).
type Middleware interface {
	Name() string
	Handle(ctx *types.Context, next Handler) error
}
