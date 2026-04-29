package pkgsite

import (
	"fmt"
	"net/http"
	"testing"
)

func TestHTTPErrorCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
		wantOk   bool
	}{
		{
			name:     "nil error",
			err:      nil,
			wantCode: 0,
			wantOk:   false,
		},
		{
			name:     "APIError with code",
			err:      &APIError{Code: http.StatusNotFound, Message: "not found"},
			wantCode: http.StatusNotFound,
			wantOk:   true,
		},
		{
			name:     "APIError with 200 code",
			err:      &APIError{Code: http.StatusOK, Message: "ok"},
			wantCode: http.StatusOK,
			wantOk:   true,
		},
		{
			name:     "HTTPError status not found",
			err:      HTTPError(http.StatusNotFound),
			wantCode: http.StatusNotFound,
			wantOk:   true,
		},
		{
			name:     "HTTPError status internal server error",
			err:      HTTPError(http.StatusInternalServerError),
			wantCode: http.StatusInternalServerError,
			wantOk:   true,
		},
		{
			name:     "HTTPError status bad request",
			err:      HTTPError(http.StatusBadRequest),
			wantCode: http.StatusBadRequest,
			wantOk:   true,
		},
		{
			name:     "non-matching error type",
			err:      fmt.Errorf("some error"),
			wantCode: 0,
			wantOk:   false,
		},
		{
			name:     "wrapped APIError",
			err:      fmt.Errorf("wrapped: %w", &APIError{Code: http.StatusForbidden, Message: "forbidden"}),
			wantCode: http.StatusForbidden,
			wantOk:   true,
		},
		{
			name:     "wrapped HTTPError",
			err:      fmt.Errorf("wrapped: %w", HTTPError(http.StatusUnauthorized)),
			wantCode: http.StatusUnauthorized,
			wantOk:   true,
		},
		{
			name:     "APIError with candidates",
			err:      &APIError{Code: http.StatusBadRequest, Message: "ambiguous", Candidates: []Candidate{{ModulePath: "example.com/a"}, {ModulePath: "example.com/b"}}},
			wantCode: http.StatusBadRequest,
			wantOk:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, ok := HTTPErrorCode(tt.err)
			if code != tt.wantCode || ok != tt.wantOk {
				t.Errorf("HTTPErrorCode(%v) = (%d, %v), want (%d, %v)",
					tt.err, code, ok, tt.wantCode, tt.wantOk)
			}
		})
	}
}
