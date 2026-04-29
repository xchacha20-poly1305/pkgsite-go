//go:build !go1.26

package pkgsite

import (
	"errors"
)

// HTTPErrorCode extracts the HTTP status code from an error.
// It returns the code and true if the error is an APIError or HTTPError,
// otherwise returns 0 and false.
func HTTPErrorCode(err error) (code int, ok bool) {
	var apiError *APIError
	if errors.As(err, &apiError) {
		return apiError.Code, true
	}
	var httpError HTTPError
	if errors.As(err, &httpError) {
		return int(httpError), true
	}
	return 0, false
}
