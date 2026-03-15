package lock

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupLockTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	return mr, client
}

func TestDistributedLock_AcquireAndRelease(t *testing.T) {
	mr, client := setupLockTestRedis(t)
	defer mr.Close()

	lock := NewDistributedLock(client, "test_lock")
	ctx := context.Background()

	opts := DefaultLockOptions()

	// Acquire lock
	handle, err := lock.Acquire(ctx, opts)
	require.NoError(t, err)
	assert.NotNil(t, handle)

	// Verify lock is held
	isLocked, err := lock.IsLocked(ctx)
	require.NoError(t, err)
	assert.True(t, isLocked)

	// Release lock
	err = handle.Release(ctx)
	require.NoError(t, err)

	// Verify lock is released
	isLocked, err = lock.IsLocked(ctx)
	require.NoError(t, err)
	assert.False(t, isLocked)
}

func TestDistributedLock_AcquireFailsWhenLocked(t *testing.T) {
	mr, client := setupLockTestRedis(t)
	defer mr.Close()

	lock1 := NewDistributedLock(client, "test_lock")
	lock2 := NewDistributedLock(client, "test_lock")
	ctx := context.Background()

	opts := DefaultLockOptions()

	// First lock should succeed
	handle1, err := lock1.Acquire(ctx, opts)
	require.NoError(t, err)
	assert.NotNil(t, handle1)

	// Second lock should fail
	handle2, err := lock2.Acquire(ctx, opts)
	assert.Error(t, err)
	assert.Nil(t, handle2)

	// Cleanup
	err = handle1.Release(ctx)
	require.NoError(t, err)
}

func TestDistributedLock_WithRetry(t *testing.T) {
	mr, client := setupLockTestRedis(t)
	defer mr.Close()

	lock1 := NewDistributedLock(client, "test_lock")
	lock2 := NewDistributedLock(client, "test_lock")
	ctx := context.Background()

	opts := DefaultLockOptions()
	opts.RetryCount = 5
	opts.RetryDelay = 10 * time.Millisecond

	// First lock should succeed
	handle1, err := lock1.Acquire(ctx, opts)
	require.NoError(t, err)
	assert.NotNil(t, handle1)

	// Start second lock acquisition in background
	done := make(chan error, 1)
	go func() {
		handle2, err := lock2.Acquire(ctx, opts)
		if err != nil {
			done <- err
			return
		}
		defer handle2.Release(ctx)
		done <- nil
	}()

	// Wait a bit then release first lock
	time.Sleep(50 * time.Millisecond)
	err = handle1.Release(ctx)
	require.NoError(t, err)

	// Second lock should eventually succeed
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(1 * time.Second):
		t.Fatal("Second lock acquisition timed out")
	}
}

func TestDistributedLock_Extend(t *testing.T) {
	mr, client := setupLockTestRedis(t)
	defer mr.Close()

	lock := NewDistributedLock(client, "test_lock")
	ctx := context.Background()

	opts := DefaultLockOptions()
	opts.TTL = 100 * time.Millisecond

	// Acquire lock
	handle, err := lock.Acquire(ctx, opts)
	require.NoError(t, err)

	// Check TTL
	ttl, err := handle.GetTTL(ctx)
	require.NoError(t, err)
	assert.Greater(t, ttl, time.Duration(0))
	assert.LessOrEqual(t, ttl, opts.TTL)

	// Extend lock
	newTTL := 200 * time.Millisecond
	err = handle.Extend(ctx, newTTL)
	require.NoError(t, err)

	// Check new TTL
	ttl, err = handle.GetTTL(ctx)
	require.NoError(t, err)
	assert.Greater(t, ttl, 100*time.Millisecond)
	assert.LessOrEqual(t, ttl, newTTL)

	// Release lock
	err = handle.Release(ctx)
	require.NoError(t, err)
}

func TestDistributedLock_AutoExtend(t *testing.T) {
	mr, client := setupLockTestRedis(t)
	defer mr.Close()

	lock := NewDistributedLock(client, "test_lock")
	ctx := context.Background()

	opts := DefaultLockOptions()
	opts.TTL = 100 * time.Millisecond
	opts.AutoExtend = true
	opts.ExtendRatio = 0.5 // Extend when 50% of TTL remains

	// Acquire lock with auto-extend
	handle, err := lock.Acquire(ctx, opts)
	require.NoError(t, err)

	// Wait longer than original TTL
	time.Sleep(150 * time.Millisecond)

	// Lock should still be held due to auto-extend
	isLocked, err := lock.IsLocked(ctx)
	require.NoError(t, err)
	assert.True(t, isLocked)

	// Release lock
	err = handle.Release(ctx)
	require.NoError(t, err)
}

func TestDistributedLock_ReleaseOnlyIfOwned(t *testing.T) {
	mr, client := setupLockTestRedis(t)
	defer mr.Close()

	lock1 := NewDistributedLock(client, "test_lock")
	lock2 := NewDistributedLock(client, "test_lock")
	ctx := context.Background()

	opts := DefaultLockOptions()

	// First lock should succeed
	handle1, err := lock1.Acquire(ctx, opts)
	require.NoError(t, err)

	// Second lock should fail
	_, err = lock2.Acquire(ctx, opts)
	assert.Error(t, err)

	// Try to release with wrong owner (should fail)
	err = lock2.Release(ctx, "wrong_value")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not owned")

	// Release with correct owner
	err = handle1.Release(ctx)
	require.NoError(t, err)
}

func TestDistributedLock_ContextCancellation(t *testing.T) {
	mr, client := setupLockTestRedis(t)
	defer mr.Close()

	lock1 := NewDistributedLock(client, "test_lock")
	lock2 := NewDistributedLock(client, "test_lock")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	opts := DefaultLockOptions()
	opts.RetryCount = 10
	opts.RetryDelay = 10 * time.Millisecond

	// First lock should succeed
	handle1, err := lock1.Acquire(context.Background(), opts)
	require.NoError(t, err)
	defer handle1.Release(context.Background())

	// Second lock should fail due to context cancellation
	_, err = lock2.Acquire(ctx, opts)
	assert.Error(t, err)
	assert.Equal(t, context.DeadlineExceeded, err)
}

func TestSyncJobLock_Execute(t *testing.T) {
	mr, client := setupLockTestRedis(t)
	defer mr.Close()

	syncLock := NewSyncJobLock(client, "job1")
	ctx := context.Background()

	executed := false
	err := syncLock.Execute(ctx, func() error {
		executed = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, executed)
}

func TestSyncJobLock_ConcurrentExecution(t *testing.T) {
	mr, client := setupLockTestRedis(t)
	defer mr.Close()

	syncLock := NewSyncJobLock(client, "job1")
	ctx := context.Background()

	executionCount := 0
	errors := make(chan error, 2)

	// Start two concurrent executions
	for i := 0; i < 2; i++ {
		go func() {
			err := syncLock.Execute(ctx, func() error {
				executionCount++
				time.Sleep(100 * time.Millisecond) // Simulate work
				return nil
			})
			errors <- err
		}()
	}

	// Wait for both to complete
	for i := 0; i < 2; i++ {
		err := <-errors
		assert.NoError(t, err)
	}

	// Only one should have executed
	assert.Equal(t, 1, executionCount)
}

func TestWithLock_Helper(t *testing.T) {
	mr, client := setupLockTestRedis(t)
	defer mr.Close()

	ctx := context.Background()
	executed := false

	err := WithLock(ctx, client, "test_helper", func() error {
		executed = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, executed)
}

func TestRedlock_BasicFunctionality(t *testing.T) {
	// Setup multiple Redis instances
	mr1, client1 := setupLockTestRedis(t)
	defer mr1.Close()

	mr2, client2 := setupLockTestRedis(t)
	defer mr2.Close()

	mr3, client3 := setupLockTestRedis(t)
	defer mr3.Close()

	clients := []*redis.Client{client1, client2, client3}
	redlock := NewRedlock(clients)
	ctx := context.Background()

	key := "redlock_test"
	value := "test_value"
	ttl := time.Second

	// Acquire lock
	handle, err := redlock.Acquire(ctx, key, value, ttl)
	require.NoError(t, err)
	assert.NotNil(t, handle)

	// Try to acquire again (should fail)
	_, err = redlock.Acquire(ctx, key, "other_value", ttl)
	assert.Error(t, err)

	// Release lock
	err = handle.Release(ctx)
	require.NoError(t, err)

	// Should be able to acquire again
	handle2, err := redlock.Acquire(ctx, key, value, ttl)
	require.NoError(t, err)
	assert.NotNil(t, handle2)

	err = handle2.Release(ctx)
	require.NoError(t, err)
}

func TestRedlock_FailsWithoutQuorum(t *testing.T) {
	// Setup 3 Redis instances but we'll make one fail
	mr1, client1 := setupLockTestRedis(t)
	defer mr1.Close()

	mr2, client2 := setupLockTestRedis(t)
	defer mr2.Close()

	mr3, client3 := setupLockTestRedis(t)
	defer mr3.Close()

	// Stop one Redis instance to simulate failure
	mr3.Close()

	clients := []*redis.Client{client1, client2, client3}
	redlock := NewRedlock(clients)
	ctx := context.Background()

	key := "redlock_quorum_test"
	value := "test_value"
	ttl := time.Second

	// Should fail to acquire quorum (need 2 out of 3, but only 2 are available)
	_, err := redlock.Acquire(ctx, key, value, ttl)
	assert.Error(t, err)
}

func TestDistributedLock_TTLExpiration(t *testing.T) {
	mr, client := setupLockTestRedis(t)
	defer mr.Close()

	lock := NewDistributedLock(client, "test_lock")
	ctx := context.Background()

	opts := DefaultLockOptions()
	opts.TTL = 50 * time.Millisecond

	// Acquire lock
	_, err := lock.Acquire(ctx, opts)
	require.NoError(t, err)

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Lock should be expired
	isLocked, err := lock.IsLocked(ctx)
	require.NoError(t, err)
	assert.False(t, isLocked)

	// Should be able to acquire again
	handle2, err := lock.Acquire(ctx, opts)
	require.NoError(t, err)
	assert.NotNil(t, handle2)

	err = handle2.Release(ctx)
	require.NoError(t, err)
}
