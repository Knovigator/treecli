package api

import (
	"fmt"
	"sync/atomic"
)

// HTTPError preserves the response status and safe response body for callers that
// need to distinguish a rejected request from an ambiguous transport failure.
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) HTTPStatus() int { return e.StatusCode }

// RequestError is an HTTP failure from the transport itself (DoJSON, /mcp),
// keeping its existing message while exposing the status for exit codes.
type RequestError struct {
	StatusCode int
	Message    string
}

func (e *RequestError) Error() string   { return e.Message }
func (e *RequestError) HTTPStatus() int { return e.StatusCode }

func (e *HTTPError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("received status code %d", e.StatusCode)
	}
	return fmt.Sprintf("received status code %d: %s", e.StatusCode, e.Body)
}

var networkUsed atomic.Bool

// MarkNetwork records that this process contacted a server. A command that fails
// without ever doing so failed on its own arguments (see cmd.ExitCode).
func MarkNetwork() { networkUsed.Store(true) }

// NetworkUsed reports whether MarkNetwork was called.
func NetworkUsed() bool { return networkUsed.Load() }
