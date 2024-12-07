package faktotum

import (
	"context"
	"fmt"
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

// HookRegistry manages both global and job-specific hooks
type HookRegistry struct {
	globalHooks map[string]Hook
	jobHooks    map[string]map[string]Hook
	mu          sync.RWMutex
}

// NewHookRegistry creates a new hook registry
func NewHookRegistry() *HookRegistry {
	return &HookRegistry{
		globalHooks: make(map[string]Hook),
		jobHooks:    make(map[string]map[string]Hook),
	}
}

// RegisterGlobalHook adds a hook that runs for all jobs
func (r *HookRegistry) RegisterGlobalHook(name string, hook Hook) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.globalHooks[name]; exists {
		return fmt.Errorf("global hook already registered: %s", name)
	}
	r.globalHooks[name] = hook
	return nil
}

// RegisterJobHook adds a hook for a specific job type
func (r *HookRegistry) RegisterJobHook(jobType string, name string, hook Hook) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.jobHooks[jobType] == nil {
		r.jobHooks[jobType] = make(map[string]Hook)
	}

	if _, exists := r.jobHooks[jobType][name]; exists {
		return fmt.Errorf("job hook already registered for job type %s: %s", jobType, name)
	}
	r.jobHooks[jobType][name] = hook
	return nil
}

// GetHooks returns all hooks that should run for a job
func (r *HookRegistry) GetHooks(jobType string) []Hook {
	r.mu.RLock()
	defer r.mu.RUnlock()

	totalHooks := len(r.globalHooks) + len(r.jobHooks[jobType])
	hooks := make([]Hook, 0, totalHooks)

	// Add global hooks first
	for _, hook := range r.globalHooks {
		hooks = append(hooks, hook)
	}

	// Add job-specific hooks
	for _, hook := range r.jobHooks[jobType] {
		hooks = append(hooks, hook)
	}

	return hooks
}

// UnregisterGlobalHook removes a global hook by name
func (r *HookRegistry) UnregisterGlobalHook(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.globalHooks[name]; !exists {
		return fmt.Errorf("global hook not found: %s", name)
	}
	delete(r.globalHooks, name)
	return nil
}

// UnregisterJobHook removes a job-specific hook
func (r *HookRegistry) UnregisterJobHook(jobType string, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.jobHooks[jobType] == nil {
		return fmt.Errorf("no hooks registered for job type: %s", jobType)
	}

	if _, exists := r.jobHooks[jobType][name]; !exists {
		return fmt.Errorf("job hook not found: %s for job type: %s", name, jobType)
	}
	delete(r.jobHooks[jobType], name)

	// Clean up empty job type map
	if len(r.jobHooks[jobType]) == 0 {
		delete(r.jobHooks, jobType)
	}
	return nil
}

// GetGlobalHooks returns a copy of the global hooks map
func (r *HookRegistry) GetGlobalHooks() map[string]Hook {
	r.mu.RLock()
	defer r.mu.RUnlock()

	hooks := make(map[string]Hook, len(r.globalHooks))
	for name, hook := range r.globalHooks {
		hooks[name] = hook
	}
	return hooks
}

// GetJobTypeHooks returns a copy of the hooks map for a specific job type
func (r *HookRegistry) GetJobTypeHooks(jobType string) map[string]Hook {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.jobHooks[jobType] == nil {
		return make(map[string]Hook)
	}

	hooks := make(map[string]Hook, len(r.jobHooks[jobType]))
	for name, hook := range r.jobHooks[jobType] {
		hooks[name] = hook
	}
	return hooks
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
