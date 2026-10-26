package sync

import (
	"context"
	"log"
	"time"
)

// schedulerStartDelay — пауза перед первой фоновой синхронизацией после старта,
// чтобы агенты успели зарегистрироваться
const schedulerStartDelay = time.Minute

// RunScheduler периодически синхронизирует все связки.
// Первый проход — через schedulerStartDelay после старта, далее каждые interval.
// Возвращается при отмене ctx.
func RunScheduler(ctx context.Context, pairs *PairManager, interval time.Duration) {
	log.Printf("Scheduler: background sync every %s (first run in %s)", interval, schedulerStartDelay)

	timer := time.NewTimer(schedulerStartDelay)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		pairs.RunAll(ctx)
		timer.Reset(interval)
	}
}
