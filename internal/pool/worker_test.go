package pool

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestWorkerPool_BasicExecution verifies tasks are executed.
func TestWorkerPool_BasicExecution(t *testing.T) {
	ctx := context.Background()
	pool := NewWorkerPool(ctx, 4, 100)
	pool.Start()

	var count int64
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		err := pool.Submit(NewFuncTask(func(ctx context.Context) error {
			atomic.AddInt64(&count, 1)
			wg.Done()
			return nil
		}))
		if err != nil {
			t.Fatalf("Submit failed: %v", err)
		}
	}

	wg.Wait()
	pool.Stop()

	if atomic.LoadInt64(&count) != 10 {
		t.Errorf("expected 10 tasks executed, got %d", count)
	}
}

// TestWorkerPool_Stop_NoPanic verifies Bug 1.6 fix:
// concurrent Submit + Stop must not panic with "send on closed channel".
func TestWorkerPool_Stop_NoPanic(t *testing.T) {
	for round := 0; round < 50; round++ {
		ctx := context.Background()
		pool := NewWorkerPool(ctx, 2, 64)
		pool.Start()

		var wg sync.WaitGroup

		// Goroutine A: submit tasks rapidly
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_ = pool.Submit(NewFuncTask(func(ctx context.Context) error {
					time.Sleep(time.Microsecond)
					return nil
				}))
			}
		}()

		// Goroutine B: stop the pool concurrently
		wg.Add(1)
		go func() {
			defer wg.Done()
			time.Sleep(time.Microsecond * 10)
			pool.Stop()
		}()

		wg.Wait()
	}
	// If we reach here without panic, Bug 1.6 is fixed.
}

// TestWorkerPool_Stop_RejectsNewTasks verifies that Submit returns ErrPoolStopped after Stop.
func TestWorkerPool_Stop_RejectsNewTasks(t *testing.T) {
	ctx := context.Background()
	pool := NewWorkerPool(ctx, 2, 10)
	pool.Start()
	pool.Stop()

	err := pool.Submit(NewFuncTask(func(ctx context.Context) error {
		return nil
	}))

	if err != ErrPoolStopped {
		t.Errorf("expected ErrPoolStopped after Stop(), got %v", err)
	}
}

// TestWorkerPool_Stop_Idempotent verifies calling Stop() twice does not panic.
func TestWorkerPool_Stop_Idempotent(t *testing.T) {
	ctx := context.Background()
	pool := NewWorkerPool(ctx, 2, 10)
	pool.Start()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("double Stop() panicked: %v", r)
		}
	}()

	pool.Stop()
	pool.Stop() // should be a no-op
}

// TestWorkerPool_Stats verifies Stats returns reasonable values.
func TestWorkerPool_Stats(t *testing.T) {
	ctx := context.Background()
	pool := NewWorkerPool(ctx, 3, 50)
	pool.Start()

	done := make(chan struct{})
	_ = pool.Submit(NewFuncTask(func(ctx context.Context) error {
		close(done)
		return nil
	}))

	<-done
	pool.Stop()

	stats := pool.Stats()
	if stats.Workers != 3 {
		t.Errorf("expected 3 workers, got %d", stats.Workers)
	}
	if stats.TaskCount < 1 {
		t.Errorf("expected at least 1 completed task, got %d", stats.TaskCount)
	}
}
