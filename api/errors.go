package api

import "fmt"

// HTTPError preserves the response status and safe response body for callers that
// need to distinguish a rejected request from an ambiguous transport failure.
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("received status code %d", e.StatusCode)
	}
	return fmt.Sprintf("received status code %d: %s", e.StatusCode, e.Body)
}
