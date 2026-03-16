package security

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()

	// Test IP rate limiting
	t.Run("IP Rate Limiting", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		
		// First request should pass
		rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(w, req)
		
		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}
		
		// Second request should also pass (within limit)
		w = httptest.NewRecorder()
		rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(w, req)
		
		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}
		
		// Simulate 100 requests to exceed limit
		for i := 0; i < 102; i++ {
			w = httptest.NewRecorder()
			rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})).ServeHTTP(w, req)
		}
		
		// Next request should be rate limited
		w = httptest.NewRecorder()
		rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(w, req)
		
		if w.Code != http.StatusTooManyRequests {
			t.Errorf("Expected status 429, got %d", w.Code)
		}
	})

	// Test user rate limiting
	t.Run("User Rate Limiting", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req = req.WithContext(context.WithValue(req.Context(), "user_id", "test_user"))
		w := httptest.NewRecorder()
		
		// First 1000 requests should pass
		for i := 0; i < 1000; i++ {
			w = httptest.NewRecorder()
			rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})).ServeHTTP(w, req)
			
			if w.Code != http.StatusOK {
				t.Errorf("Request %d failed with status %d", i+1, w.Code)
			}
		}
		
		// Next request should be rate limited
		w = httptest.NewRecorder()
		rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(w, req)
		
		if w.Code != http.StatusTooManyRequests {
			t.Errorf("Expected status 429 for user, got %d", w.Code)
		}
	})

	// Test cleanup
	t.Run("Cleanup", func(t *testing.T) {
		// Add some buckets
		req1 := httptest.NewRequest("GET", "/", nil)
		req2 := httptest.NewRequest("GET", "/", nil)
		req2 = req2.WithContext(context.WithValue(req2.Context(), "user_id", "test_user_2"))
		
		w1 := httptest.NewRecorder()
		w2 := httptest.NewRecorder()
		
		rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(w1, req1)
		rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(w2, req2)
		
		// Wait for cleanup
		time.Sleep(2 * time.Minute)
		
		// Buckets should still work
		w1 = httptest.NewRecorder()
		w2 = httptest.NewRecorder()
		
		rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(w1, req1)
		rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(w2, req2)
		
		if w1.Code != http.StatusOK || w2.Code != http.StatusOK {
			t.Errorf("Cleanup failed: buckets not working")
		}
	})
}
