package lock

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type DistributedLock struct {
	rdb   *redis.Client
	key   string
	value string
	ttl   time.Duration
}

type LockOptions struct {
	TTL         time.Duration
	RetryCount  int
	RetryDelay  time.Duration
	AutoExtend  bool
	ExtendRatio float64 // Extend lock when TTL * ExtendRatio time remains
}

func DefaultLockOptions() LockOptions {
	return LockOptions{
		TTL:         30 * time.Second,
		RetryCount:  3,
		RetryDelay:  100 * time.Millisecond,
		AutoExtend:  false,
		ExtendRatio: 0.7,
	}
}

func NewDistributedLock(rdb *redis.Client, key string) *DistributedLock {
	return &DistributedLock{
		rdb: rdb,
		key: fmt.Sprintf("lock:%s", key),
	}
}

func (dl *DistributedLock) generateValue() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random value: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// Acquire obtains a distributed lock with retry logic
func (dl *DistributedLock) Acquire(ctx context.Context, opts LockOptions) (*LockHandle, error) {
	value, err := dl.generateValue()
	if err != nil {
		return nil, fmt.Errorf("failed to generate lock value: %w", err)
	}

	var lastErr error

	for attempt := 0; attempt <= opts.RetryCount; attempt++ {
		if attempt > 0 {
			// Exponential backoff with jitter
			delay := opts.RetryDelay * time.Duration(1<<(attempt-1))
			if delay > 5*time.Second {
				delay = 5 * time.Second
			}

			// Add jitter to prevent thundering herd
			jitter := time.Duration(float64(delay) * 0.1 * (2*randFloat64() - 1))
			delay += jitter

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		acquired, err := dl.tryAcquire(ctx, value, opts.TTL)
		if err != nil {
			lastErr = err
			continue
		}

		if acquired {
			handle := &LockHandle{
				lock:        dl,
				value:       value,
				ttl:         opts.TTL,
				autoExtend:  opts.AutoExtend,
				extendRatio: opts.ExtendRatio,
			}

			if opts.AutoExtend {
				handle.startAutoExtend(ctx)
			}

			return handle, nil
		}

		lastErr = fmt.Errorf("lock already held")
	}

	return nil, fmt.Errorf("failed to acquire lock after %d attempts: %w", opts.RetryCount+1, lastErr)
}

func (dl *DistributedLock) tryAcquire(ctx context.Context, value string, ttl time.Duration) (bool, error) {
	// Lua script for atomic lock acquisition
	script := `
		local key = KEYS[1]
		local value = ARGV[1]
		local ttl = tonumber(ARGV[2])
		
		local result = redis.call('SET', key, value, 'NX', 'PX', ttl)
		if result then
			return 1
		else
			return 0
		end
	`

	result, err := dl.rdb.Eval(ctx, script, []string{dl.key}, value, ttl.Milliseconds()).Result()
	if err != nil {
		return false, fmt.Errorf("failed to execute lock script: %w", err)
	}

	return result.(int64) == 1, nil
}

// Release safely releases the lock if we still own it
func (dl *DistributedLock) Release(ctx context.Context, value string) error {
	// Lua script for atomic lock release
	script := `
		local key = KEYS[1]
		local value = ARGV[1]
		
		local current = redis.call('GET', key)
		if current == value then
			return redis.call('DEL', key)
		else
			return 0
		end
	`

	result, err := dl.rdb.Eval(ctx, script, []string{dl.key}, value).Result()
	if err != nil {
		return fmt.Errorf("failed to execute release script: %w", err)
	}

	if result.(int64) == 0 {
		return fmt.Errorf("lock not owned by this instance or already expired")
	}

	return nil
}

// Extend extends the lock TTL if we still own it
func (dl *DistributedLock) Extend(ctx context.Context, value string, ttl time.Duration) error {
	// Lua script for atomic lock extension
	script := `
		local key = KEYS[1]
		local value = ARGV[1]
		local ttl = tonumber(ARGV[2])
		
		local current = redis.call('GET', key)
		if current == value then
			redis.call('PEXPIRE', key, ttl)
			return 1
		else
			return 0
		end
	`

	result, err := dl.rdb.Eval(ctx, script, []string{dl.key}, value, ttl.Milliseconds()).Result()
	if err != nil {
		return fmt.Errorf("failed to execute extend script: %w", err)
	}

	if result.(int64) == 0 {
		return fmt.Errorf("lock not owned by this instance or already expired")
	}

	return nil
}

// IsLocked checks if the lock is currently held
func (dl *DistributedLock) IsLocked(ctx context.Context) (bool, error) {
	exists, err := dl.rdb.Exists(ctx, dl.key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check lock existence: %w", err)
	}
	return exists > 0, nil
}

// GetTTL returns the remaining TTL of the lock
func (dl *DistributedLock) GetTTL(ctx context.Context) (time.Duration, error) {
	ttl, err := dl.rdb.TTL(ctx, dl.key).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get lock TTL: %w", err)
	}
	return ttl, nil
}

type LockHandle struct {
	lock        *DistributedLock
	value       string
	ttl         time.Duration
	autoExtend  bool
	extendRatio float64
	stopExtend  chan struct{}
}

func (lh *LockHandle) startAutoExtend(ctx context.Context) {
	lh.stopExtend = make(chan struct{})

	go func() {
		ticker := time.NewTicker(time.Duration(float64(lh.ttl) * lh.extendRatio))
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-lh.stopExtend:
				return
			case <-ticker.C:
				if err := lh.lock.Extend(ctx, lh.value, lh.ttl); err != nil {
					// Log error but continue trying
					fmt.Printf("failed to extend lock: %v\n", err)
				}
			}
		}
	}()
}

func (lh *LockHandle) Release(ctx context.Context) error {
	if lh.autoExtend && lh.stopExtend != nil {
		close(lh.stopExtend)
		lh.stopExtend = nil
	}

	return lh.lock.Release(ctx, lh.value)
}

func (lh *LockHandle) Extend(ctx context.Context, ttl time.Duration) error {
	if err := lh.lock.Extend(ctx, lh.value, ttl); err != nil {
		return err
	}

	lh.ttl = ttl
	return nil
}

func (lh *LockHandle) GetTTL(ctx context.Context) (time.Duration, error) {
	return lh.lock.GetTTL(ctx)
}

// SyncJobLock is a specialized lock for sync jobs
type SyncJobLock struct {
	lock *DistributedLock
}

func NewSyncJobLock(rdb *redis.Client, jobID string) *SyncJobLock {
	return &SyncJobLock{
		lock: NewDistributedLock(rdb, fmt.Sprintf("sync_job:%s", jobID)),
	}
}

func (sjl *SyncJobLock) Execute(ctx context.Context, fn func() error) error {
	opts := DefaultLockOptions()
	opts.TTL = 10 * time.Minute // Longer TTL for sync jobs
	opts.AutoExtend = true
	opts.ExtendRatio = 0.8

	handle, err := sjl.lock.Acquire(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to acquire sync job lock: %w", err)
	}
	defer handle.Release(ctx)

	return fn()
}

// WithLock is a helper function that executes a function within a distributed lock
func WithLock(ctx context.Context, rdb *redis.Client, lockKey string, fn func() error) error {
	lock := NewDistributedLock(rdb, lockKey)
	opts := DefaultLockOptions()

	handle, err := lock.Acquire(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to acquire lock '%s': %w", lockKey, err)
	}
	defer handle.Release(ctx)

	return fn()
}

// WithLockOptions is a helper function with custom lock options
func WithLockOptions(ctx context.Context, rdb *redis.Client, lockKey string, opts LockOptions, fn func() error) error {
	lock := NewDistributedLock(rdb, lockKey)

	handle, err := lock.Acquire(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to acquire lock '%s': %w", lockKey, err)
	}
	defer handle.Release(ctx)

	return fn()
}

// Redlock implements the Redlock algorithm for higher availability
type Redlock struct {
	clients []*redis.Client
	quorum  int
}

func NewRedlock(clients []*redis.Client) *Redlock {
	return &Redlock{
		clients: clients,
		quorum:  len(clients)/2 + 1, // Majority quorum
	}
}

func (rl *Redlock) Acquire(ctx context.Context, key string, value string, ttl time.Duration) (*RedlockHandle, error) {
	startTime := time.Now()
	successCount := 0

	for _, client := range rl.clients {
		lock := NewDistributedLock(client, key)
		acquired, err := lock.tryAcquire(ctx, value, ttl)
		if err != nil {
			continue // Try other nodes
		}
		if acquired {
			successCount++
		}
	}

	elapsed := time.Since(startTime)
	if elapsed > ttl {
		// Took too long to acquire lock
		rl.Release(ctx, key, value)
		return nil, fmt.Errorf("lock acquisition took too long: %v", elapsed)
	}

	if successCount >= rl.quorum {
		return &RedlockHandle{
			redlock: rl,
			key:     key,
			value:   value,
			ttl:     ttl,
		}, nil
	}

	// Failed to acquire quorum, cleanup
	rl.Release(ctx, key, value)
	return nil, fmt.Errorf("failed to acquire quorum: got %d, need %d", successCount, rl.quorum)
}

func (rl *Redlock) Release(ctx context.Context, key string, value string) error {
	var errors []error
	successCount := 0

	for _, client := range rl.clients {
		lock := NewDistributedLock(client, key)
		if err := lock.Release(ctx, value); err != nil {
			errors = append(errors, err)
		} else {
			successCount++
		}
	}

	if successCount >= rl.quorum {
		return nil
	}

	return fmt.Errorf("failed to release quorum: got %d, need %d, errors: %v", successCount, rl.quorum, errors)
}

type RedlockHandle struct {
	redlock *Redlock
	key     string
	value   string
	ttl     time.Duration
}

func (rlh *RedlockHandle) Release(ctx context.Context) error {
	return rlh.redlock.Release(ctx, rlh.key, rlh.value)
}

func randFloat64() float64 {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return float64(bytes[0]) / 255.0
}
