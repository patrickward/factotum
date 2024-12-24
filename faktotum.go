package faktotum

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	faktory "github.com/contribsys/faktory/client"
	worker "github.com/contribsys/faktory_worker_go"
)

// Config holds configuration for the Faktory worker module
type Config struct {
	// Logger is the logger to use
	Logger *slog.Logger
	// WorkerCount is the number of concurrent workers
	WorkerCount int
	// Queues are the queues to process, in order of precedence
	Queues []string
	// QueueWeights maps queue names to their weights (optional). If provided, the worker will process queues in weighted order.
	// If not provided, the worker will process queues in strict order.
	QueueWeights map[string]int
	// Labels to attach to this worker pool
	Labels []string
	// ShutdownTimeout is how long to wait for jobs to finish during shutdown
	ShutdownTimeout time.Duration
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		WorkerCount:     20,
		Queues:          []string{"default"},
		ShutdownTimeout: 30 * time.Second,
	}
}

// Option represents a functional option for configuring the Faktotum
type Option func(module *Faktotum)

// Faktotum implements the Module interfaces for Faktory worker integration
type Faktotum struct {
	config        *Config
	logger        *slog.Logger
	mgr           *worker.Manager
	pool          *faktory.Pool
	hasPool       bool
	hookRegistry  *HookRegistry
	cancel        context.CancelFunc
	clientFactory ClientFactory
	scheduler     *Scheduler
	mu            sync.RWMutex
	schedulerMu   sync.RWMutex
}

// PanicError is returned when a panic occurs during job execution
type PanicError struct {
	Value   any
	Stack   []byte
	Context string
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("panic: %v", e.Value)
}

// WithClientFactory allows injection of a custom client creation function
func WithClientFactory(factory ClientFactory) Option {
	return func(f *Faktotum) {
		f.clientFactory = factory
	}
}

// WithScheduler adds a scheduler to the Faktotum
func WithScheduler(config SchedulerConfig) Option {
	return func(f *Faktotum) {
		f.schedulerMu.Lock()
		defer f.schedulerMu.Unlock()

		scheduler, err := NewScheduler(f, &config)
		if err != nil {
			f.logger.Info("failed to create scheduler", systemError(err)...)
			return
		}
		f.scheduler = scheduler
	}
}

// New creates a new Faktotum with the given configuration
func New(cfg *Config, opts ...Option) *Faktotum {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	m := &Faktotum{
		config:       cfg,
		logger:       cfg.Logger.WithGroup("faktotum"),
		hookRegistry: NewHookRegistry(),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// ID implements Module interface
func (f *Faktotum) ID() string {
	return "factotum"
}

// Scheduler returns the scheduler, if one is configured. If no scheduler is configured, this will return nil.
//
// This method is safe for concurrent access.
//
// The scheduler is not started automatically. It must be started by the caller and added via the WithScheduler option.
//
// If no scheduler is configured, this will return nil. The caller should check for nil before using the scheduler.
//
// Example:
//
//	if s := m.Scheduler(); s != nil {
//		s.ScheduleDaily("my-job", "0 0 * * *", myJobHandler)
//	}
func (f *Faktotum) Scheduler() *Scheduler {
	f.schedulerMu.RLock()
	defer f.schedulerMu.RUnlock()

	return f.scheduler
}

// Init implements Module interface
func (f *Faktotum) Init() error {
	// Create the manager
	mgr := worker.NewManager()
	mgr.Logger = NewFaktoryLogger(f.logger)

	if f.config.WorkerCount < 1 {
		return fmt.Errorf("worker count must be at least 1")
	}

	// Configure the manager
	mgr.Concurrency = f.config.WorkerCount
	if len(f.config.Labels) > 0 {
		mgr.Labels = f.config.Labels
	}
	mgr.ShutdownTimeout = f.config.ShutdownTimeout

	// Set up queue processing
	if len(f.config.QueueWeights) > 0 {
		mgr.ProcessWeightedPriorityQueues(f.config.QueueWeights)
	} else {
		mgr.ProcessStrictPriorityQueues(f.config.Queues...)
	}

	// Create the client pool, if not using a custom factory
	if f.clientFactory == nil {
		poolSize := f.config.WorkerCount
		if poolSize < 1 {
			poolSize = 1
		}

		pool, err := faktory.NewPool(poolSize)
		if err != nil {
			return fmt.Errorf("failed to create client pool: %w", err)
		}

		f.pool = pool
		f.hasPool = true
		f.clientFactory = defaultClientFactory(pool)
	}

	f.mgr = mgr
	return nil
}

// Start implements StartupModule interface and starts the Faktory worker
func (f *Faktotum) Start(ctx context.Context) error {
	// Start the manager in a goroutine
	go func() {
		if err := f.mgr.RunWithContext(ctx); err != nil {
			f.logger.Error("faktory worker error", systemError(err)...)
		}
	}()

	// If a scheduler is configured, start it
	if f.scheduler != nil {
		f.schedulerMu.RLock()
		defer f.schedulerMu.RUnlock()

		if err := f.scheduler.Start(ctx); err != nil {
			return fmt.Errorf("failed to start scheduler: %w", err)
		}
	}

	return nil
}

// Stop implements ShutdownModule interface and stops the Faktory worker
func (f *Faktotum) Stop(_ context.Context) error {
	// Stop the scheduler, if configured
	if f.scheduler != nil {
		f.schedulerMu.RLock()
		defer f.schedulerMu.RUnlock()

		_ = f.scheduler.Stop(context.Background())
	}

	f.mu.Lock()
	if f.cancel != nil {
		f.cancel()
		f.cancel = nil // Prevent multiple cancel calls
	}
	mgr := f.mgr // Capture under lock
	hasPool := f.hasPool
	f.mu.Unlock()

	if mgr == nil {
		return nil
	}

	done := make(chan struct{})

	go func() {
		if hasPool {
			mgr.Terminate(false)
		}
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(f.config.ShutdownTimeout):
		return fmt.Errorf("shutdown timed out after %v", f.config.ShutdownTimeout)
	}
}

// RegisterGlobalHook adds a new hook
func (f *Faktotum) RegisterGlobalHook(name string, hook Hook) error {
	return f.hookRegistry.RegisterGlobalHook(name, hook)
}

// RegisterJobHook adds a new hook for a specific job type
func (f *Faktotum) RegisterJobHook(jobType, name string, hook Hook) error {
	return f.hookRegistry.RegisterJobHook(jobType, name, hook)
}

// UnregisterGlobalHook removes a global hook
func (f *Faktotum) UnregisterGlobalHook(name string) error {
	return f.hookRegistry.UnregisterGlobalHook(name)
}

// UnregisterJobHook removes a job-specific hook
func (f *Faktotum) UnregisterJobHook(jobType string, name string) error {
	return f.hookRegistry.UnregisterJobHook(jobType, name)
}

// GetGlobalHooks returns all global hooks
func (f *Faktotum) GetGlobalHooks() map[string]Hook {
	return f.hookRegistry.GetGlobalHooks()
}

// GetJobTypeHooks returns all hooks for a specific job type
func (f *Faktotum) GetJobTypeHooks(jobType string) map[string]Hook {
	return f.hookRegistry.GetJobTypeHooks(jobType)
}

// RegisterJob registers a job handler with hooks and panic recovery
func (f *Faktotum) RegisterJob(jobType string, handler worker.Perform) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Log registration
	f.logger.Info("registering job handler", slog.String("job_type", jobType))

	// First wrap with panic recovery
	safeHandler := f.WrapWithPanicRecovery(jobType, handler)

	// Then wrap with hooks and logging
	wrappedHandler := f.WrapWithHooks(jobType, safeHandler)

	f.mgr.Register(jobType, wrappedHandler)
}

// EnqueueJob pushes a job to Faktory
func (f *Faktotum) EnqueueJob(_ context.Context, job *faktory.Job) error {
	client, err := f.clientFactory()
	if err != nil {
		return fmt.Errorf("failed to get client from pool: %w", err)
	}
	defer client.Cleanup()

	if err := client.Push(job); err != nil {
		return fmt.Errorf("failed to push job: %w", err)
	}

	return nil
}

// BulkEnqueueResult represents the result of a bulk enqueue operation
type BulkEnqueueResult struct {
	JobID string
	Error error
}

// BulkEnqueue enqueues multiple jobs in parallel
func (f *Faktotum) BulkEnqueue(_ context.Context, jobs []*faktory.Job) []BulkEnqueueResult {
	results := make([]BulkEnqueueResult, len(jobs))
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, f.config.WorkerCount)

	for i, job := range jobs {
		wg.Add(1)
		go func(index int, j *faktory.Job) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					stack := debug.Stack()
					panicErr := &PanicError{
						Value:   r,
						Stack:   stack,
						Context: fmt.Sprintf("bulk enqueue job %s (%s)", j.Jid, j.Type),
					}

					// Log the panic with structured data
					f.logger.Error("bulk enqueue panic detected",
						slog.String("job_id", j.Jid),
						slog.String("job_type", j.Type),
						slog.Any("panic_value", r),
						slog.String("stack_trace", string(stack)),
						slog.String("queue", j.Queue),
						slog.Int("bulk_index", index),
					)

					results[index] = BulkEnqueueResult{
						JobID: j.Jid,
						Error: panicErr,
					}
				}
			}()

			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			client, err := f.clientFactory()
			if err != nil {
				f.logger.Error("failed to get client from pool", jobError(j.Jid, j.Type, err)...)
				results[index] = BulkEnqueueResult{
					Error: fmt.Errorf("failed to get client from pool: %w", err),
				}
				return
			}

			defer client.Cleanup()

			err = client.Push(j)
			if err != nil {
				f.logger.Error("failed to push job", jobError(j.Jid, j.Type, err)...)
			} else {
				f.logger.Debug("job enqueued successfully", jobInfo(j.Jid, j.Type, slog.String("queue", j.Queue))...)
			}

			results[index] = BulkEnqueueResult{
				JobID: j.Jid,
				Error: err,
			}
		}(i, job)
	}

	wg.Wait()
	return results
}

// WrapWithHooks wraps a job handler with hook processing and logging
func (f *Faktotum) WrapWithHooks(jobType string, handler worker.Perform) worker.Perform {
	return func(ctx context.Context, args ...interface{}) error {
		// Create a job instance for hooks and logging
		job := faktory.NewJob(jobType, args...)

		// Log job start
		f.logger.Debug("starting job processing", jobInfo(job.Jid, job.Type, slog.Any("args", args))...)

		hooks := f.hookRegistry.GetHooks(jobType)

		// Run pre-job hooks
		for _, hook := range hooks {
			if err := func() error {
				defer func() {
					if r := recover(); r != nil {
						stack := debug.Stack()
						f.logger.Error("panic in BeforeJob hook",
							slog.String("job_id", job.Jid),
							slog.String("job_type", job.Type),
							slog.Any("panic_value", r),
							slog.String("stack_trace", string(stack)),
						)
					}
				}()
				return hook.BeforeJob(ctx, job)
			}(); err != nil {
				f.logger.Error("BeforeJob hook failed", jobError(job.Jid, job.Type, err)...)
				return fmt.Errorf("hook failed: %w", err)
			}
		}

		// Execute the handler
		err := handler(ctx, args...)

		// Log completion or error
		if err != nil {
			f.logger.Error("job failed", jobError(job.Jid, job.Type, err)...)
		} else {
			f.logger.Debug("job completed successfully", jobInfo(job.Jid, job.Type)...)
		}

		// Run post-job hooks
		for _, hook := range hooks {
			//hook.AfterJob(ctx, job, err)
			func() {
				defer func() {
					if r := recover(); r != nil {
						stack := debug.Stack()
						f.logger.Error("panic in AfterJob hook",
							slog.String("job_id", job.Jid),
							slog.String("job_type", job.Type),
							slog.Any("panic_value", r),
							slog.String("stack_trace", string(stack)),
						)
					}
				}()
				hook.AfterJob(ctx, job, err)
			}()
		}

		return err
	}
}

// WrapWithPanicRecovery wraps a job handler with panic recovery
func (f *Faktotum) WrapWithPanicRecovery(jobType string, handler worker.Perform) worker.Perform {
	return func(ctx context.Context, args ...interface{}) (err error) {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				panicErr := &PanicError{
					Value:   r,
					Stack:   stack,
					Context: fmt.Sprintf("job type: %s", jobType),
				}

				// Create a job instance for logging context
				job := faktory.NewJob(jobType, args...)

				// Log the panic with structured data
				f.logger.Error("job panic detected",
					slog.String("job_id", job.Jid),
					slog.String("job_type", job.Type),
					slog.Any("panic_value", r),
					slog.String("stack_trace", string(stack)),
					slog.String("queue", job.Queue),
					slog.Any("args", args),
				)

				// Set the error to be returned
				err = panicErr
			}
		}()

		return handler(ctx, args...)
	}
}
