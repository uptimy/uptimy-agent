package checks

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// scheduledCheck pairs a check with its execution interval.
type scheduledCheck struct {
	check    Check
	interval time.Duration
}

// Scheduler runs registered checks at their configured intervals
// and delivers results to a channel.
type Scheduler struct {
	registry *Registry
	results  chan CheckResult
	logger   *zap.SugaredLogger
	workers  int

	checks []scheduledCheck

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewScheduler creates a Scheduler wired to the given registry and result channel.
func NewScheduler(registry *Registry, results chan CheckResult, workers int, logger *zap.SugaredLogger) *Scheduler {
	return &Scheduler{
		registry: registry,
		results:  results,
		logger:   logger,
		workers:  workers,
	}
}

// AddCheck schedules a check for periodic execution.
func (s *Scheduler) AddCheck(c Check, interval time.Duration) {
	s.checks = append(s.checks, scheduledCheck{check: c, interval: interval})
}

// Start begins executing all scheduled checks. It blocks until ctx is canceled.
// Calling Start more than once without Stop will stop the previous run first.
func (s *Scheduler) Start(ctx context.Context) {
	// Ensure idempotency: stop any previous run before starting a new one.
	if s.cancel != nil {
		s.cancel()
		s.wg.Wait()
	}
	ctx, s.cancel = context.WithCancel(ctx)

	// Worker pool that executes check jobs.
	jobs := make(chan scheduledJob, len(s.checks))

	for i := 0; i < s.workers; i++ {
		s.wg.Add(1)
		go s.worker(ctx, jobs)
	}

	// Launch a ticker goroutine per check that feeds jobs to the worker pool.
	for _, sc := range s.checks {
		sc := sc
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			ticker := time.NewTicker(sc.interval)
			defer ticker.Stop()

			// Run immediately on start, then on tick.
			select {
			case jobs <- scheduledJob{check: sc.check}:
			case <-ctx.Done():
				return
			}

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					select {
					case jobs <- scheduledJob{check: sc.check}:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}
}

// Stop signals all goroutines to stop and waits for them to finish.
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

type scheduledJob struct {
	check Check
}

func (s *Scheduler) worker(ctx context.Context, jobs <-chan scheduledJob) {
	defer s.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-jobs:
			result := job.check.Run(ctx)
			select {
			case s.results <- result:
			case <-ctx.Done():
				return
			}
			if result.Status != StatusHealthy {
				s.logger.Warnw("check unhealthy",
					"check", result.Name,
					"status", result.Status,
					"error", result.Error,
					"duration", result.Duration,
				)
			} else {
				s.logger.Debugw("check healthy",
					"check", result.Name,
					"duration", result.Duration,
				)
			}
		}
	}
}
