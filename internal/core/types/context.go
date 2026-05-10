package types

import "net/http"

// Context carries per-request state through the pipeline (middleware, codecs, forwarders).
type Context struct {
	Request *http.Request
	// RequestID is a stable correlation id for logs and tracing (set by pipeline later).
	RequestID string
}
