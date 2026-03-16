package ratelimit

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type RateLimiter struct {
	rdb    *redis.Client
	prefix string
}

type RateLimitResult struct {
	Allowed    bool
	Remaining  int64
	ResetTime  time.Time
	RetryAfter time.Duration
	Limit      int64
	Window     time.Duration
}

func NewRateLimiter(rdb *redis.Client) *RateLimiter {
	return &RateLimiter{
		rdb:    rdb,
		prefix: "rate_limit:",
	}
}

func (rl *RateLimiter) key(identifier string) string {
	return rl.prefix + identifier
}

// SlidingWindowRateLimit implements sliding window rate limiting using Redis
// It tracks requests in a sliding time window and allows up to 'limit' requests
// within the specified 'window' duration
func convertToInt64(v interface{}) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case int:
		return int64(val)
	case string:
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
			return i
		}
		return 0
	default:
		return 0
	}
}

func (rl *RateLimiter) SlidingWindowRateLimit(
	ctx context.Context,
	identifier string,
	limit int64,
	window time.Duration,
) (*RateLimitResult, error) {
	now := time.Now().UnixMilli()
	windowStart := now - window.Milliseconds()
	key := rl.key(identifier)

	// Lua script for atomic sliding window rate limiting
	script := `
		local key = KEYS[1]
		local now = tonumber(ARGV[1])
		local window_start = tonumber(ARGV[2])
		local limit = tonumber(ARGV[3])
		local window_ms = tonumber(ARGV[4])
		
		-- Remove expired entries
		redis.call('ZREMRANGEBYSCORE', key, 0, window_start)
		
		-- Count current requests in window
		local current = redis.call('ZCARD', key)
		
		-- Check if limit is exceeded
		if current >= limit then
			-- Get oldest request time to calculate retry after
			local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
			local retry_after = 0
			if #oldest > 0 then
				retry_after = oldest[2] - now + window_ms
				if retry_after < 0 then
					retry_after = 0
				end
			end
			
			return {
				0, -- not allowed
				limit - current, -- remaining
				oldest[2] or now, -- reset time (oldest request + window)
				retry_after
			}
		end
		
		-- Add current request
		redis.call('ZADD', key, now, now)
		redis.call('EXPIRE', key, math.ceil(window_ms / 1000) + 1)
		
		return {
			1, -- allowed
			limit - current - 1, -- remaining
			now + window_ms, -- reset time
			0 -- retry after
		}
	`

	result, err := rl.rdb.Eval(ctx, script, []string{key},
		now, windowStart, limit, window.Milliseconds()).Result()
	if err != nil {
		return nil, fmt.Errorf("rate limit script execution failed: %w", err)
	}

	resultSlice, ok := result.([]interface{})
	if !ok || len(resultSlice) != 4 {
		return nil, fmt.Errorf("unexpected script result format")
	}

	allowed := convertToInt64(resultSlice[0]) == 1
	remaining := convertToInt64(resultSlice[1])
	resetTimeMs := convertToInt64(resultSlice[2])
	retryAfterMs := convertToInt64(resultSlice[3])

	return &RateLimitResult{
		Allowed:    allowed,
		Remaining:  remaining,
		ResetTime:  time.UnixMilli(resetTimeMs),
		RetryAfter: time.Duration(retryAfterMs) * time.Millisecond,
		Limit:      limit,
		Window:     window,
	}, nil
}

// FixedWindowRateLimit implements simple fixed window rate limiting
// Less accurate than sliding window but simpler and more performant
func (rl *RateLimiter) FixedWindowRateLimit(
	ctx context.Context,
	identifier string,
	limit int64,
	window time.Duration,
) (*RateLimitResult, error) {
	key := rl.key(identifier)

	// Use INCR with expiration for fixed window
	pipe := rl.rdb.TxPipeline()

	// Increment counter
	incrCmd := pipe.Incr(ctx, key)

	// Set expiration only for new keys (first request in window)
	pipe.Expire(ctx, key, window)

	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("rate limit check failed: %w", err)
	}

	current := incrCmd.Val()
	allowed := current <= limit
	remaining := limit - current
	if remaining < 0 {
		remaining = 0
	}

	// Calculate reset time (start of next window)
	now := time.Now()
	resetTime := now.Add(window)
	retryAfter := time.Duration(0)

	if !allowed {
		// Get TTL to calculate retry after
		ttl := rl.rdb.TTL(ctx, key).Val()
		if ttl > 0 {
			retryAfter = ttl
		}
	}

	return &RateLimitResult{
		Allowed:    allowed,
		Remaining:  remaining,
		ResetTime:  resetTime,
		RetryAfter: retryAfter,
		Limit:      limit,
		Window:     window,
	}, nil
}

// TokenBucketRateLimit implements token bucket algorithm
// Good for burst traffic while maintaining average rate
func (rl *RateLimiter) TokenBucketRateLimit(
	ctx context.Context,
	identifier string,
	capacity int64,
	refillRate float64, // tokens per second
) (*RateLimitResult, error) {
	key := rl.key(identifier)
	now := time.Now().UnixMilli()

	// Lua script for token bucket algorithm
	script := `
		local key = KEYS[1]
		local now = tonumber(ARGV[1])
		local capacity = tonumber(ARGV[2])
		local refill_rate = tonumber(ARGV[3])
		local refill_interval = tonumber(ARGV[4])
		
		-- Get current bucket state
		local bucket = redis.call('HMGET', key, 'tokens', 'last_refill')
		local tokens = tonumber(bucket[1]) or capacity
		local last_refill = tonumber(bucket[2]) or now
		
		-- Calculate time passed and tokens to add
		local time_passed = now - last_refill
		local tokens_to_add = math.floor(time_passed / refill_interval * refill_rate)
		
		-- Refill tokens
		tokens = math.min(capacity, tokens + tokens_to_add)
		
		-- Check if request can be processed
		local allowed = tokens >= 1
		
		if allowed then
			tokens = tokens - 1
		end
		
		-- Update bucket state
		redis.call('HMSET', key, 'tokens', tokens, 'last_refill', now)
		redis.call('EXPIRE', key, math.ceil(capacity / refill_rate) + 60)
		
		-- Calculate retry after if not allowed
		local retry_after = 0
		if not allowed then
			retry_after = math.ceil((1 - tokens) / refill_rate * 1000)
		end
		
		return {
			allowed and 1 or 0,
			tokens,
			now + 1000, -- reset time (next second)
			retry_after
		}
	`

	refillInterval := 1000.0 // 1 second in milliseconds

	result, err := rl.rdb.Eval(ctx, script, []string{key},
		now, capacity, refillRate, refillInterval).Result()
	if err != nil {
		return nil, fmt.Errorf("token bucket script execution failed: %w", err)
	}

	resultSlice, ok := result.([]interface{})
	if !ok || len(resultSlice) != 4 {
		return nil, fmt.Errorf("unexpected script result format")
	}

	allowed := convertToInt64(resultSlice[0]) == 1
	remaining := convertToInt64(resultSlice[1])
	resetTimeMs := convertToInt64(resultSlice[2])
	retryAfterMs := convertToInt64(resultSlice[3])

	return &RateLimitResult{
		Allowed:    allowed,
		Remaining:  remaining,
		ResetTime:  time.UnixMilli(resetTimeMs),
		RetryAfter: time.Duration(retryAfterMs) * time.Millisecond,
		Limit:      capacity,
		Window:     time.Duration(float64(capacity)/refillRate) * time.Second,
	}, nil
}

// MultiRateLimit allows checking multiple rate limits at once
// Useful for implementing tiered rate limiting (e.g., per minute and per hour)
func (rl *RateLimiter) MultiRateLimit(
	ctx context.Context,
	identifier string,
	limits []struct {
		Limit  int64
		Window time.Duration
	},
) (*RateLimitResult, error) {
	var mostRestrictive *RateLimitResult
	var minResetTime time.Time

	for _, limitConfig := range limits {
		result, err := rl.SlidingWindowRateLimit(
			ctx,
			fmt.Sprintf("%s:%d", identifier, limitConfig.Window.Milliseconds()),
			limitConfig.Limit,
			limitConfig.Window,
		)
		if err != nil {
			return nil, err
		}

		if !result.Allowed {
			// Return immediately if any limit is exceeded
			return result, nil
		}

		if mostRestrictive == nil || result.Remaining < mostRestrictive.Remaining {
			mostRestrictive = result
		}

		if result.ResetTime.Before(minResetTime) || minResetTime.IsZero() {
			minResetTime = result.ResetTime
		}
	}

	if mostRestrictive != nil {
		mostRestrictive.ResetTime = minResetTime
	}

	return mostRestrictive, nil
}

// GetUserRateLimit gets current rate limit status without consuming a request
func (rl *RateLimiter) GetUserRateLimit(
	ctx context.Context,
	identifier string,
	limit int64,
	window time.Duration,
) (*RateLimitResult, error) {
	key := rl.key(identifier)
	now := time.Now().UnixMilli()
	windowStart := now - window.Milliseconds()

	// Count current requests in window without adding new one
	pipe := rl.rdb.TxPipeline()
	pipe.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(windowStart, 10))
	pipe.ZCard(ctx, key)

	results, err := pipe.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check rate limit status: %w", err)
	}

	current := results[1].(*redis.IntCmd).Val()
	remaining := limit - current
	if remaining < 0 {
		remaining = 0
	}

	allowed := current < limit

	// Get oldest request time for reset calculation
	oldestStr, err := rl.rdb.ZRange(ctx, key, 0, 0).Result()
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get oldest request: %w", err)
	}

	var resetTime time.Time
	var retryAfter time.Duration

	if len(oldestStr) > 0 {
		if oldestMs, err := strconv.ParseInt(oldestStr[0], 10, 64); err == nil {
			resetTime = time.UnixMilli(oldestMs + window.Milliseconds())
			if !allowed {
				retryAfter = time.Until(resetTime)
				if retryAfter < 0 {
					retryAfter = 0
				}
			}
		}
	}

	if resetTime.IsZero() {
		resetTime = time.Now().Add(window)
	}

	return &RateLimitResult{
		Allowed:    allowed,
		Remaining:  remaining,
		ResetTime:  resetTime,
		RetryAfter: retryAfter,
		Limit:      limit,
		Window:     window,
	}, nil
}
