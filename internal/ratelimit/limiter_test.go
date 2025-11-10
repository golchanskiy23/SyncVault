package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRateLimitTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	return mr, client
}

func TestSlidingWindowRateLimit_AllowWithinLimit(t *testing.T) {
	mr, client := setupRateLimitTestRedis(t)
	defer mr.Close()

	limiter := NewRateLimiter(client)
	ctx := context.Background()

	// Allow 5 requests per minute
	limit := int64(5)
	window := time.Minute

	// Make 5 requests - all should be allowed
	for i := 0; i < 5; i++ {
		result, err := limiter.SlidingWindowRateLimit(ctx, "user1", limit, window)
		require.NoError(t, err)
		assert.True(t, result.Allowed, "Request %d should be allowed", i)
		assert.Equal(t, limit-int64(i+1), result.Remaining)
	}
}

func TestSlidingWindowRateLimit_ExceedLimit(t *testing.T) {
	mr, client := setupRateLimitTestRedis(t)
	defer mr.Close()

	limiter := NewRateLimiter(client)
	ctx := context.Background()

	// Allow 3 requests per minute
	limit := int64(3)
	window := time.Minute

	// Make 3 requests - all should be allowed
	for i := 0; i < 3; i++ {
		result, err := limiter.SlidingWindowRateLimit(ctx, "user1", limit, window)
		require.NoError(t, err)
		assert.True(t, result.Allowed, "Request %d should be allowed", i)
	}

	// 4th request should be denied
	result, err := limiter.SlidingWindowRateLimit(ctx, "user1", limit, window)
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Equal(t, int64(0), result.Remaining)
	assert.Greater(t, result.RetryAfter, time.Duration(0))
}

func TestSlidingWindowRateLimit_DifferentUsers(t *testing.T) {
	mr, client := setupRateLimitTestRedis(t)
	defer mr.Close()

	limiter := NewRateLimiter(client)
	ctx := context.Background()

	// Allow 2 requests per minute
	limit := int64(2)
	window := time.Minute

	// User 1 makes 2 requests
	for i := 0; i < 2; i++ {
		result, err := limiter.SlidingWindowRateLimit(ctx, "user1", limit, window)
		require.NoError(t, err)
		assert.True(t, result.Allowed)
	}

	// User 2 should still be able to make requests
	for i := 0; i < 2; i++ {
		result, err := limiter.SlidingWindowRateLimit(ctx, "user2", limit, window)
		require.NoError(t, err)
		assert.True(t, result.Allowed, "User2 request %d should be allowed", i)
	}
}

func TestSlidingWindowRateLimit_SlidingBehavior(t *testing.T) {
	mr, client := setupRateLimitTestRedis(t)
	defer mr.Close()

	limiter := NewRateLimiter(client)
	ctx := context.Background()

	// Allow 3 requests per 2 seconds
	limit := int64(3)
	window := 2 * time.Second

	// Make 3 requests quickly
	for i := 0; i < 3; i++ {
		result, err := limiter.SlidingWindowRateLimit(ctx, "user1", limit, window)
		require.NoError(t, err)
		assert.True(t, result.Allowed)
	}

	// 4th request should be denied
	result, err := limiter.SlidingWindowRateLimit(ctx, "user1", limit, window)
	require.NoError(t, err)
	assert.False(t, result.Allowed)

	// Wait for window to slide
	time.Sleep(2 * time.Second)

	// Should be able to make requests again
	result, err = limiter.SlidingWindowRateLimit(ctx, "user1", limit, window)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestFixedWindowRateLimit_AllowWithinLimit(t *testing.T) {
	mr, client := setupRateLimitTestRedis(t)
	defer mr.Close()

	limiter := NewRateLimiter(client)
	ctx := context.Background()

	// Allow 5 requests per minute
	limit := int64(5)
	window := time.Minute

	// Make 5 requests - all should be allowed
	for i := 0; i < 5; i++ {
		result, err := limiter.FixedWindowRateLimit(ctx, "user1", limit, window)
		require.NoError(t, err)
		assert.True(t, result.Allowed, "Request %d should be allowed", i)
		assert.Equal(t, limit-int64(i+1), result.Remaining)
	}
}

func TestFixedWindowRateLimit_ExceedLimit(t *testing.T) {
	mr, client := setupRateLimitTestRedis(t)
	defer mr.Close()

	limiter := NewRateLimiter(client)
	ctx := context.Background()

	// Allow 2 requests per minute
	limit := int64(2)
	window := time.Minute

	// Make 2 requests - all should be allowed
	for i := 0; i < 2; i++ {
		result, err := limiter.FixedWindowRateLimit(ctx, "user1", limit, window)
		require.NoError(t, err)
		assert.True(t, result.Allowed)
	}

	// 3rd request should be denied
	result, err := limiter.FixedWindowRateLimit(ctx, "user1", limit, window)
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Equal(t, int64(0), result.Remaining)
}

func TestTokenBucketRateLimit_AllowWithinCapacity(t *testing.T) {
	mr, client := setupRateLimitTestRedis(t)
	defer mr.Close()

	limiter := NewRateLimiter(client)
	ctx := context.Background()

	// Bucket capacity of 5 tokens, refill rate of 2 tokens per second
	capacity := int64(5)
	refillRate := 2.0

	// Make 5 requests - all should be allowed
	for i := 0; i < 5; i++ {
		result, err := limiter.TokenBucketRateLimit(ctx, "user1", capacity, refillRate)
		require.NoError(t, err)
		assert.True(t, result.Allowed, "Request %d should be allowed", i)
	}
}

func TestTokenBucketRateLimit_ExceedCapacity(t *testing.T) {
	mr, client := setupRateLimitTestRedis(t)
	defer mr.Close()

	limiter := NewRateLimiter(client)
	ctx := context.Background()

	// Bucket capacity of 3 tokens, refill rate of 1 token per second
	capacity := int64(3)
	refillRate := 1.0

	// Make 3 requests - all should be allowed
	for i := 0; i < 3; i++ {
		result, err := limiter.TokenBucketRateLimit(ctx, "user1", capacity, refillRate)
		require.NoError(t, err)
		assert.True(t, result.Allowed)
	}

	// 4th request should be denied
	result, err := limiter.TokenBucketRateLimit(ctx, "user1", capacity, refillRate)
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Greater(t, result.RetryAfter, time.Duration(0))
}

func TestTokenBucketRateLimit_Refill(t *testing.T) {
	mr, client := setupRateLimitTestRedis(t)
	defer mr.Close()

	limiter := NewRateLimiter(client)
	ctx := context.Background()

	// Bucket capacity of 2 tokens, refill rate of 10 tokens per second
	capacity := int64(2)
	refillRate := 10.0

	// Make 2 requests - all should be allowed
	for i := 0; i < 2; i++ {
		result, err := limiter.TokenBucketRateLimit(ctx, "user1", capacity, refillRate)
		require.NoError(t, err)
		assert.True(t, result.Allowed)
	}

	// 3rd request should be denied
	result, err := limiter.TokenBucketRateLimit(ctx, "user1", capacity, refillRate)
	require.NoError(t, err)
	assert.False(t, result.Allowed)

	// Wait for refill
	time.Sleep(200 * time.Millisecond) // Should refill ~2 tokens

	// Should be able to make requests again
	result, err = limiter.TokenBucketRateLimit(ctx, "user1", capacity, refillRate)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestMultiRateLimit_AllWithinAllLimits(t *testing.T) {
	mr, client := setupRateLimitTestRedis(t)
	defer mr.Close()

	limiter := NewRateLimiter(client)
	ctx := context.Background()

	// Define multiple limits: 5 per minute, 10 per hour
	limits := []struct {
		Limit  int64
		Window time.Duration
	}{
		{5, time.Minute},
		{10, time.Hour},
	}

	// Make 5 requests - all should be allowed
	for i := 0; i < 5; i++ {
		result, err := limiter.MultiRateLimit(ctx, "user1", limits)
		require.NoError(t, err)
		assert.True(t, result.Allowed, "Request %d should be allowed", i)
	}
}

func TestMultiRateLimit_ExceedOneLimit(t *testing.T) {
	mr, client := setupRateLimitTestRedis(t)
	defer mr.Close()

	limiter := NewRateLimiter(client)
	ctx := context.Background()

	// Define multiple limits: 2 per minute, 10 per hour
	limits := []struct {
		Limit  int64
		Window time.Duration
	}{
		{2, time.Minute},
		{10, time.Hour},
	}

	// Make 2 requests - all should be allowed
	for i := 0; i < 2; i++ {
		result, err := limiter.MultiRateLimit(ctx, "user1", limits)
		require.NoError(t, err)
		assert.True(t, result.Allowed)
	}

	// 3rd request should be denied (exceeds minute limit)
	result, err := limiter.MultiRateLimit(ctx, "user1", limits)
	require.NoError(t, err)
	assert.False(t, result.Allowed)
}

func TestGetUserRateLimit_NoConsume(t *testing.T) {
	mr, client := setupRateLimitTestRedis(t)
	defer mr.Close()

	limiter := NewRateLimiter(client)
	ctx := context.Background()

	// Allow 3 requests per minute
	limit := int64(3)
	window := time.Minute

	// Check initial status
	result, err := limiter.GetUserRateLimit(ctx, "user1", limit, window)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.Equal(t, limit, result.Remaining)

	// Make a request
	_, err = limiter.SlidingWindowRateLimit(ctx, "user1", limit, window)
	require.NoError(t, err)

	// Check status without consuming
	result, err = limiter.GetUserRateLimit(ctx, "user1", limit, window)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.Equal(t, limit-1, result.Remaining)
}

func TestRateLimitResult_Fields(t *testing.T) {
	mr, client := setupRateLimitTestRedis(t)
	defer mr.Close()

	limiter := NewRateLimiter(client)
	ctx := context.Background()

	limit := int64(5)
	window := time.Minute

	result, err := limiter.SlidingWindowRateLimit(ctx, "user1", limit, window)
	require.NoError(t, err)

	assert.Equal(t, limit, result.Limit)
	assert.Equal(t, window, result.Window)
	assert.True(t, result.ResetTime.After(time.Now()))
	assert.GreaterOrEqual(t, result.Remaining, int64(0))
}
