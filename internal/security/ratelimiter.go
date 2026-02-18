package security

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimiter предоставляет rate limiting функциональность
type RateLimiter struct {
	// IP rate limiter: 100 запросов в минуту
	ipRequests map[string]*TokenBucket
	// User rate limiter: 1000 запросов в минуту для аутентифицированных пользователей
	userRequests  map[string]*TokenBucket
	mu            sync.RWMutex
	cleanupTicker *time.Ticker
}

// TokenBucket реализует token bucket алгоритм
type TokenBucket struct {
	tokens     int
	maxTokens  int
	refillRate int
	lastRefill time.Time
}

// NewRateLimiter создает новый rate limiter
func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{
		ipRequests:   make(map[string]*TokenBucket),
		userRequests: make(map[string]*TokenBucket),
	}

	// Запускаем cleanup каждую минуту
	rl.cleanupTicker = time.NewTicker(1 * time.Minute)
	go func() {
		for range rl.cleanupTicker.C {
			rl.cleanup()
		}
	}()

	return rl
}

// Stop останавливает rate limiter
func (rl *RateLimiter) Stop() {
	if rl.cleanupTicker != nil {
		rl.cleanupTicker.Stop()
	}
}

// Middleware возвращает HTTP middleware для rate limiting
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := getClientIP(r)
		userID := getUserIDFromContext(r.Context())

		var bucket *TokenBucket
		var limit int
		var key string

		if userID != "" {
			// Аутентифицированный пользователь - 1000 запросов в минуту
			key = fmt.Sprintf("user:%v", userID)
			limit = 1000
			bucket = rl.getUserBucket(key, limit)
		} else {
			// Неаутентифицированный пользователь - 100 запросов в минуту
			key = clientIP
			limit = 100
			bucket = rl.getIPBucket(key, limit)
		}

		if !bucket.consume(1) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprintf(w, `{"error": "Rate limit exceeded", "limit": %d, "reset_in": "1 minute"}`, limit)
			return
		}

		// Добавляем headers для rate limiting информации
		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", bucket.tokens))
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(time.Minute).Unix()))

		next.ServeHTTP(w, r)
	})
}

// getIPBucket получает или создает bucket для IP
func (rl *RateLimiter) getIPBucket(ip string, limit int) *TokenBucket {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	bucket, exists := rl.ipRequests[ip]
	if !exists {
		bucket = &TokenBucket{
			tokens:     limit,
			maxTokens:  limit,
			refillRate: limit, // refill rate = limit per minute
			lastRefill: time.Now(),
		}
		rl.ipRequests[ip] = bucket
	}

	return bucket
}

// getUserBucket получает или создает bucket для пользователя
func (rl *RateLimiter) getUserBucket(userID string, limit int) *TokenBucket {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	bucket, exists := rl.userRequests[userID]
	if !exists {
		bucket = &TokenBucket{
			tokens:     limit,
			maxTokens:  limit,
			refillRate: limit, // refill rate = limit per minute
			lastRefill: time.Now(),
		}
		rl.userRequests[userID] = bucket
	}

	return bucket
}

// consume пытается потребовать токен
func (tb *TokenBucket) consume(tokens int) bool {
	tb.refill()

	if tb.tokens >= tokens {
		tb.tokens -= tokens
		return true
	}
	return false
}

// refill пополняет токены в bucket
func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill)

	// Добавляем токены пропорционально времени
	tokensToAdd := int(elapsed.Minutes()) * tb.refillRate
	if tokensToAdd > 0 {
		tb.tokens = min(tb.tokens+tokensToAdd, tb.maxTokens)
		tb.lastRefill = now
	}
}

// cleanup удаляет старые bucket'ы
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// Удаляем IP bucket'ы которые не использовались 5 минут
	for ip, bucket := range rl.ipRequests {
		if now.Sub(bucket.lastRefill) > 5*time.Minute {
			delete(rl.ipRequests, ip)
		}
	}

	// Удаляем user bucket'ы которые не использовались 10 минут
	for userID, bucket := range rl.userRequests {
		if now.Sub(bucket.lastRefill) > 10*time.Minute {
			delete(rl.userRequests, userID)
		}
	}
}

// getClientIP получает IP адрес клиента
func getClientIP(r *http.Request) string {
	// Проверяем X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Берем первый IP из списка
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Проверяем X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Используем RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// getUserIDFromContext получает ID пользователя из контекста
func getUserIDFromContext(ctx context.Context) string {
	if userID, ok := ctx.Value("user_id").(string); ok {
		return userID
	}
	return ""
}
