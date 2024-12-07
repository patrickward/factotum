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
	// QueueWeights maps queue names to their weights (optional)
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
	mu            sync.RWMutex
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
	return func(m *Faktotum) {
		m.clientFactory = factory
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
		logger:       cfg.Logger,
		hookRegistry: NewHookRegistry(),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// ID implements Module interface
func (m *Faktotum) ID() string {
	return "factotum"
}

// Init implements Module interface
func (m *Faktotum) Init() error {
	// Create the manager
	mgr := worker.NewManager()

	if m.config.WorkerCount < 1 {
		return fmt.Errorf("worker count must be at least 1")
	}

	// Configure the manager
	mgr.Concurrency = m.config.WorkerCount
	if len(m.config.Labels) > 0 {
		mgr.Labels = m.config.Labels
	}
	mgr.ShutdownTimeout = m.config.ShutdownTimeout

	// Set up queue processing
	if len(m.config.QueueWeights) > 0 {
		mgr.ProcessWeightedPriorityQueues(m.config.QueueWeights)
	} else {
		mgr.ProcessStrictPriorityQueues(m.config.Queues...)
	}

	// Create the client pool, if not using a custom factory
	if m.clientFactory == nil {
		poolSize := m.config.WorkerCount
		if poolSize < 1 {
			poolSize = 1
		}

		pool, err := faktory.NewPool(poolSize)
		if err != nil {
			return fmt.Errorf("failed to create client pool: %w", err)
		}

		m.pool = pool
		m.hasPool = true
		m.clientFactory = defaultClientFactory(pool)
	}

	m.mgr = mgr
	return nil
}

// Start implements StartupModule interface and starts the Faktory worker
func (m *Faktotum) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel

	// Start the manager in a goroutine
	go func() {
		if err := m.mgr.RunWithContext(ctx); err != nil {
			m.logger.Error("Faktory worker error: %v", slog.String("err", err.Error()))
		}
	}()

	return nil
}

// Stop implements ShutdownModule interface and stops the Faktory worker
func (m *Faktotum) Stop(_ context.Context) error {
	if m.cancel != nil {
		m.cancel()
	}

	// Set up a channel to track termination completion
	done := make(chan struct{})

	// Run Terminate in a goroutine since it blocks
	go func() {
		if m.hasPool {
			m.mgr.Terminate(false)
		}
		close(done)
	}()

	// Wait for either termination to complete or timeout
	select {
	case <-done:
		return nil
	case <-time.After(m.config.ShutdownTimeout):
		return fmt.Errorf("shutdown timed out after %v", m.config.ShutdownTimeout)
	}
}

// RegisterGlobalHook adds a new hook
func (m *Faktotum) RegisterGlobalHook(name string, hook Hook) error {
	return m.hookRegistry.RegisterGlobalHook(name, hook)
}

// RegisterJobHook adds a new hook for a specific job type
func (m *Faktotum) RegisterJobHook(jobType, name string, hook Hook) error {
	return m.hookRegistry.RegisterJobHook(jobType, name, hook)
}

// UnregisterGlobalHook removes a global hook
func (m *Faktotum) UnregisterGlobalHook(name string) error {
	return m.hookRegistry.UnregisterGlobalHook(name)
}

// UnregisterJobHook removes a job-specific hook
func (m *Faktotum) UnregisterJobHook(jobType string, name string) error {
	return m.hookRegistry.UnregisterJobHook(jobType, name)
}

// GetGlobalHooks returns all global hooks
func (m *Faktotum) GetGlobalHooks() map[string]Hook {
	return m.hookRegistry.GetGlobalHooks()
}

// GetJobTypeHooks returns all hooks for a specific job type
func (m *Faktotum) GetJobTypeHooks(jobType string) map[string]Hook {
	return m.hookRegistry.GetJobTypeHooks(jobType)
}

// RegisterJob registers a job handler with hooks and panic recovery
func (m *Faktotum) RegisterJob(jobType string, handler worker.Perform) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Log registration
	m.logger.Info("registering job handler",
		slog.String("job_type", jobType),
	)

	// First wrap with panic recovery
	safeHandler := m.WrapWithPanicRecovery(jobType, handler)

	// Then wrap with hooks and logging
	wrappedHandler := m.WrapWithHooks(jobType, safeHandler)

	m.mgr.Register(jobType, wrappedHandler)
}

// EnqueueJob pushes a job to Faktory
func (m *Faktotum) EnqueueJob(_ context.Context, job *faktory.Job) error {
	client, err := m.clientFactory()
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
func (m *Faktotum) BulkEnqueue(_ context.Context, jobs []*faktory.Job) []BulkEnqueueResult {
	results := make([]BulkEnqueueResult, len(jobs))
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, m.config.WorkerCount)

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
					m.logger.Error("bulk enqueue panic detected",
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

			client, err := m.clientFactory()
			if err != nil {
				m.logger.Error("failed to get client from pool",
					slog.String("job_id", j.Jid),
					slog.String("job_type", j.Type),
					slog.String("error", err.Error()),
				)
				results[index] = BulkEnqueueResult{
					Error: fmt.Errorf("failed to get client from pool: %w", err),
				}
				return
			}

			defer client.Cleanup()

			err = client.Push(j)
			if err != nil {
				m.logger.Error("failed to push job",
					slog.String("job_id", j.Jid),
					slog.String("job_type", j.Type),
					slog.String("error", err.Error()),
				)
			} else {
				m.logger.Debug("job enqueued successfully",
					slog.String("job_id", j.Jid),
					slog.String("job_type", j.Type),
					slog.String("queue", j.Queue),
				)
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
func (m *Faktotum) WrapWithHooks(jobType string, handler worker.Perform) worker.Perform {
	return func(ctx context.Context, args ...interface{}) error {
		// Create a job instance for hooks and logging
		job := faktory.NewJob(jobType, args...)

		// Log job start
		m.logger.Debug("starting job processing",
			slog.String("job_id", job.Jid),
			slog.String("job_type", job.Type),
			slog.Any("args", args),
		)

		hooks := m.hookRegistry.GetHooks(jobType)

		// Run pre-job hooks
		for _, hook := range hooks {
			if err := func() error {
				defer func() {
					if r := recover(); r != nil {
						stack := debug.Stack()
						m.logger.Error("panic in BeforeJob hook",
							slog.String("job_id", job.Jid),
							slog.String("job_type", job.Type),
							slog.Any("panic_value", r),
							slog.String("stack_trace", string(stack)),
						)
					}
				}()
				return hook.BeforeJob(ctx, job)
			}(); err != nil {
				m.logger.Error("BeforeJob hook failed",
					slog.String("job_id", job.Jid),
					slog.String("job_type", job.Type),
					slog.String("error", err.Error()),
				)
				return fmt.Errorf("hook failed: %w", err)
			}
		}

		// Execute the handler
		err := handler(ctx, args...)

		// Log completion or error
		if err != nil {
			m.logger.Error("job failed",
				slog.String("job_id", job.Jid),
				slog.String("job_type", job.Type),
				slog.String("error", err.Error()),
			)
		} else {
			m.logger.Debug("job completed successfully",
				slog.String("job_id", job.Jid),
				slog.String("job_type", job.Type),
			)
		}

		// Run post-job hooks
		for _, hook := range hooks {
			//hook.AfterJob(ctx, job, err)
			func() {
				defer func() {
					if r := recover(); r != nil {
						stack := debug.Stack()
						m.logger.Error("panic in AfterJob hook",
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
func (m *Faktotum) WrapWithPanicRecovery(jobType string, handler worker.Perform) worker.Perform {
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
				m.logger.Error("job panic detected",
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
