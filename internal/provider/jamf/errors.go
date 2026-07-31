package jamf

import (
	"errors"
	"fmt"
	"strings"
)

const maxErrorBodyBytes = 4 << 10

type httpError struct {
	status int
	body   string
}

func (e *httpError) Error() string {
	if e.body == "" {
		return fmt.Sprintf("Jamf API returned HTTP %d", e.status)
	}
	return fmt.Sprintf("Jamf API returned HTTP %d: %s", e.status, e.body)
}

func newHTTPError(status int, body []byte) error {
	if len(body) > maxErrorBodyBytes {
		body = body[:maxErrorBodyBytes]
	}
	return &httpError{status: status, body: strings.TrimSpace(string(body))}
}

// StatusCode extracts an HTTP status from a Jamf API error.
func StatusCode(err error) (int, bool) {
	var apiError *httpError
	if !errors.As(err, &apiError) {
		return 0, false
	}
	return apiError.status, true
}
