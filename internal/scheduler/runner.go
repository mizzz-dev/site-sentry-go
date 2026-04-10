package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"site-sentry-go/internal/service"
)

type Runner struct {
	svc   *service.MonitorService
	tick  time.Duration
	mu    sync.Mutex
	runAt map[int64]time.Time
	run   map[int64]bool
}

func NewRunner(svc *service.MonitorService, tick time.Duration) *Runner {
	return &Runner{svc: svc, tick: tick, runAt: map[int64]time.Time{}, run: map[int64]bool{}}
}

func (r *Runner) Start(ctx context.Context) {
	ticker := time.NewTicker(r.tick)
	defer ticker.Stop()
	r.dispatch(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.dispatch(ctx)
		}
	}
}

func (r *Runner) dispatch(ctx context.Context) {
	monitors, err := r.svc.ListEnabled(ctx)
	if err != nil {
		log.Printf("scheduler list enabled failed: %v", err)
		return
	}
	now := time.Now().UTC()
	for _, m := range monitors {
		r.mu.Lock()
		next, ok := r.runAt[m.ID]
		busy := r.run[m.ID]
		if !ok {
			r.runAt[m.ID] = now
			next = now
		}
		due := !busy && !now.Before(next)
		if due {
			r.run[m.ID] = true
			r.runAt[m.ID] = now.Add(time.Duration(m.IntervalSeconds) * time.Second)
		}
		r.mu.Unlock()

		if due {
			go r.runMonitor(ctx, m.ID)
		}
	}
}

func (r *Runner) runMonitor(ctx context.Context, id int64) {
	defer func() {
		r.mu.Lock()
		delete(r.run, id)
		r.mu.Unlock()
	}()
	result, err := r.svc.RunCheck(ctx, id)
	if err != nil {
		log.Printf("scheduler check failed monitor=%d err=%v", id, err)
		return
	}
	service.LogCheckResult(result)
}
