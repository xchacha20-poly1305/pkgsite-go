//go:build go1.26

package pkgsite

import (
	"errors"
)

// HTTPErrorCode extracts the HTTP status code from an error.
// It returns the code and true if the error is an APIError or HTTPError,
// otherwise returns 0 and false.
func HTTPErrorCode(err error) (code int, ok bool) {
	if err == nil {
		return 0, false
	}
	if apiError, isAPIError := errors.AsType[*Error](err); isAPIError {
		return apiError.Code, true
	}
	if httpError, isHTTPError := errors.AsType[HTTPError](err); isHTTPError {
		return int(httpError), true
	}
	return 0, false
}
