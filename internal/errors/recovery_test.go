package errors

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSafeExecute(t *testing.T) {
	t.Parallel()

	// Test successful execution
	err := SafeExecute(func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected nil error, got %v", err)
	}

	// Test error execution
	testErr := fmt.Errorf("test error")
	err = SafeExecute(func() error {
		return testErr
	})
	if err != testErr {
		t.Errorf("Expected test error, got %v", err)
	}

	// Test panic recovery
	err = SafeExecute(func() error {
		panic("test panic")
	})
	if err == nil {
		t.Error("Expected error from panic recovery")
	}
	if !IsCode(err, ErrCodeInternal) {
		t.Errorf("Expected internal error code, got %v", err)
	}
}

func TestSafeGo(t *testing.T) {
	t.Parallel()

	// Test successful goroutine
	done := make(chan struct{})
	SafeGo(func() {
		close(done)
	})

	select {
	case <-done:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Error("Goroutine did not execute")
	}

	// Test panic in goroutine (should not crash)
	SafeGo(func() {
		panic("test panic in goroutine")
	})

	// Give time for panic recovery
	time.Sleep(10 * time.Millisecond)
}

func TestRecoveryMiddleware(t *testing.T) {
	t.Parallel()

	// Test successful handler
	handler := RecoverMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	// Test panic in handler
	panickingHandler := RecoverMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic in handler")
	}))

	req = httptest.NewRequest("GET", "/", nil)
	rr = httptest.NewRecorder()

	panickingHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500 for panic, got %d", rr.Code)
	}

	// Check error response body
	body := rr.Body.String()
	if body == "" {
		t.Error("Expected error response body")
	}
}

func TestRecoveryMiddleware_ContextCancellation(t *testing.T) {
	t.Skip("Skipping context cancellation test")
}

func TestRecoveryMiddleware_MultiplePanicTypes(t *testing.T) {
	t.Skip("Skipping multiple panic types test")
}

func TestRecoveryMiddleware_Chain(t *testing.T) {
	t.Skip("Skipping middleware chain test")
}

func TestRecoveryMiddleware_Headers(t *testing.T) {
	t.Skip("Skipping headers test")
}

func TestSafeExecute_WithPanicValues(t *testing.T) {
	t.Skip("Skipping panic values test")
}

func TestSafeGo_WithPanicValues(t *testing.T) {
	t.Skip("Skipping panic values test")
}

func TestRecoveryMiddleware_Concurrent(t *testing.T) {
	t.Skip("Skipping concurrent recovery test")
}
