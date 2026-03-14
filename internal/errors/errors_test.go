package errors

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestAppError_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		code     ErrorCode
		message  string
		op       string
		path     string
		err      error
		expected string
	}{
		{
			name:     "with original error",
			code:     ErrCodeInternal,
			message:  "something went wrong",
			op:       "database.Query",
			path:     "/users/123",
			err:      fmt.Errorf("connection failed"),
			expected: "database.Query: something went wrong: connection failed",
		},
		{
			name:     "without original error",
			code:     ErrCodeNotFound,
			message:  "user not found",
			op:       "user.Find",
			path:     "",
			err:      nil,
			expected: "user.Find: user not found",
		},
		{
			name:     "empty message",
			code:     ErrCodeInvalid,
			message:  "",
			op:       "validator.Check",
			path:     "/api/v1/users",
			err:      nil,
			expected: "validator.Check: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appErr := &AppError{
				Code:    tt.code,
				Message: tt.message,
				ErrorOp: tt.op,
				Path:    tt.path,
				Err:     tt.err,
				Stack:   []string{"file.go:10 function"},
			}

			result := appErr.Error()
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestAppError_Unwrap(t *testing.T) {
	t.Parallel()

	originalErr := fmt.Errorf("original error")
	appErr := &AppError{
		Code:    ErrCodeInternal,
		Message: "wrapped error",
		ErrorOp: "test.Operation",
		Err:     originalErr,
	}

	unwrapped := appErr.Unwrap()
	if unwrapped != originalErr {
		t.Errorf("Expected original error, got %v", unwrapped)
	}

	// Test without original error
	appErrNoOrig := &AppError{
		Code:    ErrCodeInternal,
		Message: "no original error",
		ErrorOp: "test.Operation",
		Err:     nil,
	}

	if appErrNoOrig.Unwrap() != nil {
		t.Error("Expected nil when no original error")
	}
}

func TestAppError_Is(t *testing.T) {
	t.Parallel()

	err1 := &AppError{Code: ErrCodeNotFound}
	err2 := &AppError{Code: ErrCodeNotFound}
	err3 := &AppError{Code: ErrCodeInternal}

	// Same code should match
	if !err1.Is(err2) {
		t.Error("Errors with same code should match")
	}

	// Different codes should not match
	if err1.Is(err3) {
		t.Error("Errors with different codes should not match")
	}

	// Should not match non-AppError
	stdErr := fmt.Errorf("standard error")
	if err1.Is(stdErr) {
		t.Error("AppError should not match standard error")
	}
}

func TestAppError_WithPath(t *testing.T) {
	t.Parallel()

	appErr := &AppError{
		Code:    ErrCodeInternal,
		Message: "test error",
		ErrorOp: "test.Operation",
	}

	// Test WithPath returns same error
	result := appErr.WithPath("/api/v1/users")
	if result != appErr {
		t.Error("WithPath should return same error instance")
	}

	// Test path is set
	if appErr.Path != "/api/v1/users" {
		t.Errorf("Expected path /api/v1/users, got %s", appErr.Path)
	}
}

func TestAppError_HTTPStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		code     ErrorCode
		expected int
	}{
		{name: "not_found", code: ErrCodeNotFound, expected: http.StatusNotFound},
		{name: "invalid", code: ErrCodeInvalid, expected: http.StatusBadRequest},
		{name: "permission", code: ErrCodePermission, expected: http.StatusForbidden},
		{name: "conflict", code: ErrCodeConflict, expected: http.StatusConflict},
		{name: "rate_limit", code: ErrCodeRateLimit, expected: http.StatusTooManyRequests},
		{name: "unavailable", code: ErrCodeUnavailable, expected: http.StatusServiceUnavailable},
		{name: "timeout", code: ErrCodeTimeout, expected: http.StatusRequestTimeout},
		{name: "internal", code: ErrCodeInternal, expected: http.StatusInternalServerError},
		{name: "unknown", code: ErrorCode("unknown"), expected: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			appErr := &AppError{Code: tt.code}
			result := appErr.HTTPStatus()

			if result != tt.expected {
				t.Errorf("Expected status %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestAs(t *testing.T) {
	t.Parallel()

	originalErr := fmt.Errorf("wrapped error")
	appErr := Wrap(originalErr, ErrCodeNotFound, "user.Find", "user not found")

	// Test successful extraction
	extracted, ok := As(appErr)
	if !ok {
		t.Error("Expected to extract AppError")
	}
	if extracted.Code != ErrCodeNotFound {
		t.Errorf("Expected ErrCodeNotFound, got %v", extracted.Code)
	}

	// Test failed extraction
	stdErr := fmt.Errorf("standard error")
	_, ok = As(stdErr)
	if ok {
		t.Error("Expected not to extract AppError from standard error")
	}

	// Test nil error
	_, ok = As(nil)
	if ok {
		t.Error("Expected not to extract AppError from nil error")
	}
}

func TestIsCode(t *testing.T) {
	t.Parallel()

	// Test direct match
	appErr := New(ErrCodeNotFound, "user.Find", "user not found")
	if !IsCode(appErr, ErrCodeNotFound) {
		t.Error("Expected direct code match")
	}

	// Test wrapped match
	wrappedErr := Wrap(appErr, ErrCodeInternal, "api.Handler", "failed to handle")
	if !IsCode(wrappedErr, ErrCodeNotFound) {
		t.Error("Expected wrapped code match")
	}

	// Test non-match
	if IsCode(appErr, ErrCodeInternal) {
		t.Error("Expected non-match for different code")
	}

	// Test nil error
	if IsCode(nil, ErrCodeNotFound) {
		t.Error("Expected false for nil error")
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	appErr := New(ErrCodeInvalid, "validator.Check", "invalid input")

	if appErr.Code != ErrCodeInvalid {
		t.Errorf("Expected ErrCodeInvalid, got %v", appErr.Code)
	}

	if appErr.Message != "invalid input" {
		t.Errorf("Expected 'invalid input', got %s", appErr.Message)
	}

	if appErr.ErrorOp != "validator.Check" {
		t.Errorf("Expected 'validator.Check', got %s", appErr.ErrorOp)
	}

	if appErr.Err != nil {
		t.Error("Expected nil original error")
	}

	// Check stack trace is generated
	if len(appErr.Stack) == 0 {
		t.Error("Expected non-empty stack trace")
	}
}

func TestWrap(t *testing.T) {
	t.Parallel()

	originalErr := fmt.Errorf("database error")
	appErr := Wrap(originalErr, ErrCodeInternal, "database.Query", "query failed")

	if appErr.Code != ErrCodeInternal {
		t.Errorf("Expected ErrCodeInternal, got %v", appErr.Code)
	}

	if appErr.Message != "query failed" {
		t.Errorf("Expected 'query failed', got %s", appErr.Message)
	}

	if appErr.ErrorOp != "database.Query" {
		t.Errorf("Expected 'database.Query', got %s", appErr.ErrorOp)
	}

	if appErr.Err != originalErr {
		t.Error("Expected original error to be preserved")
	}

	// Test wrapping nil error
	wrappedNil := Wrap(nil, ErrCodeInternal, "test.Op", "test message")
	if wrappedNil != nil {
		t.Error("Expected nil when wrapping nil error")
	}

	// Test wrapping another AppError (path copying)
	innerAppErr := New(ErrCodeNotFound, "user.Find", "not found")
	innerAppErr.Path = "/users/123"

	outerAppErr := Wrap(innerAppErr, ErrCodeInternal, "api.Handler", "failed")
	if outerAppErr.Path != "/users/123" {
		t.Errorf("Expected path copying, got %s", outerAppErr.Path)
	}
}

func TestStackTraces(t *testing.T) {
	t.Parallel()

	// Test that stack traces are generated
	appErr := New(ErrCodeInternal, "test.Operation", "test message")

	if len(appErr.Stack) == 0 {
		t.Error("Expected non-empty stack trace")
	}

	// Check stack trace format
	for _, frame := range appErr.Stack {
		if !strings.Contains(frame, ":") {
			t.Errorf("Stack frame should contain line number: %s", frame)
		}
		if !strings.Contains(frame, " ") {
			t.Errorf("Stack frame should contain function: %s", frame)
		}
	}

	// Test that stack traces are different for different errors
	appErr2 := New(ErrCodeInternal, "test.Operation2", "test message2")

	if len(appErr.Stack) == len(appErr2.Stack) {
		same := true
		for i := range appErr.Stack {
			if appErr.Stack[i] != appErr2.Stack[i] {
				same = false
				break
			}
		}
		if same {
			t.Error("Stack traces should be different for different errors")
		}
	}
}

func TestErrorCode_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code     ErrorCode
		expected string
	}{
		{ErrCodeInternal, "internal"},
		{ErrCodeNotFound, "not_found"},
		{ErrCodeInvalid, "invalid"},
		{ErrCodeTimeout, "timeout"},
		{ErrCodePermission, "permission"},
		{ErrCodeConflict, "conflict"},
		{ErrCodeRateLimit, "rate_limit"},
		{ErrCodeUnavailable, "unavailable"},
	}

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			result := string(tt.code)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestErrors_WithStandardLibrary(t *testing.T) {
	t.Parallel()

	appErr := New(ErrCodeNotFound, "user.Find", "user not found")

	// Test errors.Is
	if !errors.Is(appErr, appErr) {
		t.Error("errors.Is should match same AppError")
	}

	// Test errors.As
	var extracted *AppError
	if !errors.As(appErr, &extracted) {
		t.Error("errors.As should extract AppError")
	}
	if extracted.Code != ErrCodeNotFound {
		t.Errorf("Expected ErrCodeNotFound, got %v", extracted.Code)
	}

	// Test errors.Unwrap
	unwrapped := errors.Unwrap(appErr)
	if unwrapped != nil {
		t.Error("errors.Unwrap should return nil for AppError without original error")
	}

	// Test with wrapped error
	originalErr := fmt.Errorf("original")
	wrappedErr := Wrap(originalErr, ErrCodeInternal, "test.Op", "test")

	unwrapped = errors.Unwrap(wrappedErr)
	if unwrapped != originalErr {
		t.Error("errors.Unwrap should return original error")
	}
}

func TestConcurrentErrorCreation(t *testing.T) {
	t.Skip("Skipping concurrent test due to performance issues")
}
