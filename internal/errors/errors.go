package errors

import (
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"
)

type AppError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	ErrorOp string    `json:"op"`
	Path    string    `json:"path"`
	Err     error     `json:"-"`
	Stack   []string  `json:"stack"`
}

type ErrorCode string

const (
	ErrCodeInternal    ErrorCode = "internal"
	ErrCodeNotFound    ErrorCode = "not_found"
	ErrCodeInvalid     ErrorCode = "invalid"
	ErrCodeTimeout     ErrorCode = "timeout"
	ErrCodePermission  ErrorCode = "permission"
	ErrCodeConflict    ErrorCode = "conflict"
	ErrCodeRateLimit   ErrorCode = "rate_limit"
	ErrCodeUnavailable ErrorCode = "unavailable"
)

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.ErrorOp, e.Message, e.Err)
	}

	return fmt.Sprintf("%s: %s", e.ErrorOp, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func (e *AppError) Is(target error) bool {
	if t, ok := target.(*AppError); ok {
		return e.Code == t.Code
	}
	return false
}

func As(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}

func IsCode(err error, code ErrorCode) bool {
	var appErr *AppError
	for errors.As(err, &appErr) {
		if appErr.Code == code {
			return true
		}
		err = appErr.Unwrap()
	}

	return false
}

func New(code ErrorCode, op, message string) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		ErrorOp: op,
		Stack:   stackTrace(),
	}
}

func Wrap(err error, code ErrorCode, op, message string) *AppError {
	if err == nil {
		return nil
	}

	appErr := &AppError{
		Code:    code,
		Message: message,
		ErrorOp: op,
		Err:     err,
		Stack:   stackTrace(),
	}

	if wrapped, ok := err.(*AppError); ok {
		appErr.Path = wrapped.Path
	}

	return appErr
}

func (e *AppError) WithPath(path string) *AppError {
	e.Path = path
	return e
}

func (e *AppError) HTTPStatus() int {
	switch e.Code {
	case ErrCodeNotFound:
		return http.StatusNotFound
	case ErrCodeInvalid:
		return http.StatusBadRequest
	case ErrCodePermission:
		return http.StatusForbidden
	case ErrCodeConflict:
		return http.StatusConflict
	case ErrCodeRateLimit:
		return http.StatusTooManyRequests
	case ErrCodeUnavailable:
		return http.StatusServiceUnavailable
	case ErrCodeTimeout:
		return http.StatusRequestTimeout
	default:
		return http.StatusInternalServerError
	}
}

func stackTrace() []string {
	var pcs [32]uintptr
	n := runtime.Callers(3, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])

	var stack []string
	for {
		frame, more := frames.Next()
		if !more {
			break
		}

		fn := frame.Function
		if fn == "" {
			fn = "unknown"
		}

		if idx := strings.LastIndex(fn, "/"); idx >= 0 {
			fn = fn[idx+1:]
		}

		stack = append(stack, fmt.Sprintf("%s:%d %s", frame.File, frame.Line, fn))
	}

	return stack
}
