package pool

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

// ErrPoolStopped возвращается при попытке отправить задачу в остановленный пул
var ErrPoolStopped = errors.New("worker pool is stopped")

type Task interface {
	Execute(ctx context.Context) error
}

type FuncTask struct {
	fn func(ctx context.Context) error
}

func (t *FuncTask) Execute(ctx context.Context) error {
	return t.fn(ctx)
}

func NewFuncTask(fn func(ctx context.Context) error) *FuncTask {
	return &FuncTask{fn: fn}
}

type WorkerPool struct {
	workers    int
	taskQueue  chan Task
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	taskCount  int64
	errorCount int64
	activeJobs int32
	mu         sync.RWMutex
	stopped    int32
	stopOnce   sync.Once // гарантирует однократное закрытие канала
}

func NewWorkerPool(ctx context.Context, workers, queueSize int) *WorkerPool {
	poolCtx, cancel := context.WithCancel(ctx)

	return &WorkerPool{
		workers:   workers,
		taskQueue: make(chan Task, queueSize),
		ctx:       poolCtx,
		cancel:    cancel,
	}
}

func (p *WorkerPool) Start() {
	p.wg.Add(p.workers)

	for i := 0; i < p.workers; i++ {
		go p.worker(i)
	}
}

func (p *WorkerPool) worker(id int) {
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return

		case task, ok := <-p.taskQueue:
			if !ok {
				return
			}

			atomic.AddInt32(&p.activeJobs, 1)

			if err := task.Execute(p.ctx); err != nil {
				atomic.AddInt64(&p.errorCount, 1)
			}

			atomic.AddInt32(&p.activeJobs, -1)
			atomic.AddInt64(&p.taskCount, 1)
		}
	}
}

// Submit добавляет задачу в очередь
func (p *WorkerPool) Submit(task Task) error {
	if atomic.LoadInt32(&p.stopped) != 0 {
		return ErrPoolStopped
	}

	select {
	case <-p.ctx.Done():
		return p.ctx.Err()
	case p.taskQueue <- task:
		return nil
	}
}

func (p *WorkerPool) SubmitFunc(fn func(ctx context.Context) error) error {
	return p.Submit(NewFuncTask(fn))
}

// Stop останавливает пул воркеров
func (p *WorkerPool) Stop() {
	p.stopOnce.Do(func() {
		atomic.StoreInt32(&p.stopped, 1)
		p.cancel()
		p.wg.Wait()
		close(p.taskQueue)
	})
}

func (p *WorkerPool) Stats() PoolStats {
	return PoolStats{
		Workers:    p.workers,
		QueueSize:  len(p.taskQueue),
		TaskCount:  atomic.LoadInt64(&p.taskCount),
		ErrorCount: atomic.LoadInt64(&p.errorCount),
		ActiveJobs: atomic.LoadInt32(&p.activeJobs),
	}
}

type PoolStats struct {
	Workers    int   `json:"workers"`
	QueueSize  int   `json:"queue_size"`
	TaskCount  int64 `json:"task_count"`
	ErrorCount int64 `json:"error_count"`
	ActiveJobs int32 `json:"active_jobs"`
}

