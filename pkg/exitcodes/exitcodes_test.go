package exitcodes

import (
	"errors"
	"fmt"
	"testing"
)

func TestExitCodeError_Error(t *testing.T) {
	testCases := []struct {
		name     string
		code     int
		err      error
		expected string
	}{
		{
			name:     "with simple error message",
			code:     ExitChartNotFound,
			err:      errors.New("chart not found"),
			expected: "exit code 4: chart not found",
		},
		{
			name:     "with formatted error message",
			code:     ExitIOError,
			err:      fmt.Errorf("failed to read file %s", "config.yaml"),
			expected: "exit code 21: failed to read file config.yaml",
		},
		{
			name:     "with nil error",
			code:     ExitSuccess,
			err:      nil,
			expected: "exit code 0: <nil>",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			exitErr := &ExitCodeError{
				Code: tc.code,
				Err:  tc.err,
			}
			if got := exitErr.Error(); got != tc.expected {
				t.Errorf("Error() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestExitCodeError_Unwrap(t *testing.T) {
	originalErr := errors.New("original error")
	exitErr := &ExitCodeError{
		Code: ExitIOError,
		Err:  originalErr,
	}

	if unwrapped := exitErr.Unwrap(); !errors.Is(unwrapped, originalErr) {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, originalErr)
	}

	// Test with nil error
	exitErrWithNil := &ExitCodeError{
		Code: ExitSuccess,
		Err:  nil,
	}
	if unwrapped := exitErrWithNil.Unwrap(); unwrapped != nil {
		t.Errorf("Unwrap() with nil error = %v, want nil", unwrapped)
	}
}

func TestIsExitCodeError(t *testing.T) {
	testCases := []struct {
		name       string
		err        error
		wantCode   int
		wantIsExit bool
	}{
		{
			name:       "exit code error",
			err:        &ExitCodeError{Code: ExitChartNotFound, Err: errors.New("chart not found")},
			wantCode:   ExitChartNotFound,
			wantIsExit: true,
		},
		{
			name:       "wrapped exit code error",
			err:        fmt.Errorf("context: %w", &ExitCodeError{Code: ExitIOError, Err: errors.New("io error")}),
			wantCode:   ExitIOError,
			wantIsExit: true,
		},
		{
			name:       "regular error",
			err:        errors.New("regular error"),
			wantCode:   0,
			wantIsExit: false,
		},
		{
			name:       "nil error",
			err:        nil,
			wantCode:   0,
			wantIsExit: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotCode, gotIsExit := IsExitCodeError(tc.err)
			if gotCode != tc.wantCode || gotIsExit != tc.wantIsExit {
				t.Errorf("IsExitCodeError() = (%d, %v), want (%d, %v)",
					gotCode, gotIsExit, tc.wantCode, tc.wantIsExit)
			}
		})
	}
}
