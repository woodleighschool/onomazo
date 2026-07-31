package microsoft

import (
	"errors"

	abstractions "github.com/microsoft/kiota-abstractions-go"
)

// StatusCode extracts an HTTP status from a Graph SDK API error.
func StatusCode(err error) (int, bool) {
	var apiError abstractions.ApiErrorable
	if !errors.As(err, &apiError) {
		return 0, false
	}
	status := apiError.GetStatusCode()
	return status, status != 0
}
