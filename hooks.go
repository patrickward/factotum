package factotum

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	faktory "github.com/contribsys/faktory/client"
)

// Hook defines the interface for job processing hooks
type Hook interface {
	BeforeJob(ctx context.Context, job *faktory.Job) error
	AfterJob(ctx context.Context, job *faktory.Job, err error)
}

// LoggingHook implements Hook to provide logging of job execution
type LoggingHook struct {
	logger *log.Logger
}

func NewLoggingHook(logger *log.Logger) *LoggingHook {
	if logger == nil {
		logger = log.Default()
	}
	return &LoggingHook{logger: logger}
}

func (h *LoggingHook) BeforeJob(ctx context.Context, job *faktory.Job) error {
	h.logger.Printf("Starting job %s of type %s", job.Jid, job.Type)
	return nil
}

func (h *LoggingHook) AfterJob(ctx context.Context, job *faktory.Job, err error) {
	if err != nil {
		h.logger.Printf("Job %s failed: %v", job.Jid, err)
	} else {
		h.logger.Printf("Job %s completed successfully", job.Jid)
	}
}

// MetricsHook implements Hook to collect metrics about job execution
type MetricsHook struct {
	successCount uint64
	failureCount uint64
	durations    []time.Duration
	mu           sync.RWMutex
}

func (h *MetricsHook) BeforeJob(ctx context.Context, job *faktory.Job) error {
	if job.Custom == nil {
		job.Custom = make(map[string]interface{})
	}
	job.Custom["start_time"] = time.Now()
	return nil
}

func (h *MetricsHook) AfterJob(ctx context.Context, job *faktory.Job, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if startTime, ok := job.Custom["start_time"].(time.Time); ok {
		h.durations = append(h.durations, time.Since(startTime))
	}

	if err != nil {
		atomic.AddUint64(&h.failureCount, 1)
	} else {
		atomic.AddUint64(&h.successCount, 1)
	}
}

func (h *MetricsHook) GetStats() (success uint64, failure uint64, avgDuration time.Duration) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	success = atomic.LoadUint64(&h.successCount)
	failure = atomic.LoadUint64(&h.failureCount)

	if len(h.durations) > 0 {
		var total time.Duration
		for _, d := range h.durations {
			total += d
		}
		avgDuration = total / time.Duration(len(h.durations))
	}

	return
}
