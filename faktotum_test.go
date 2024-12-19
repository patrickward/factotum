package faktotum_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	worker "github.com/contribsys/faktory_worker_go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	faktory "github.com/contribsys/faktory/client"

	"github.com/patrickward/faktotum"
)

type mockClient struct {
	mock.Mock
}

func (m *mockClient) Push(job *faktory.Job) error {
	args := m.Called(job)
	return args.Error(0)
}

func (m *mockClient) Close() {
	m.Called()
}

func (m *mockClient) Cleanup() {
	m.Called()
}

func setupNewFactotum(c *mockClient, config *faktotum.Config) *faktotum.Faktotum {
	return faktotum.New(config, faktotum.WithClientFactory(func() (faktotum.Client, error) {
		return c, nil
	}))
}

// TestConfig tests configuration initialization and validation
func TestConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    *faktotum.Config
		expectErr bool
	}{
		{
			name:      "nil config uses defaults",
			config:    nil,
			expectErr: false,
		},
		{
			name: "valid custom config",
			config: &faktotum.Config{
				WorkerCount: 10,
				Queues:      []string{"high", "default", "low"},
				Labels:      []string{"api", "worker"},
			},
			expectErr: false,
		},
		{
			name: "invalid worker count",
			config: &faktotum.Config{
				WorkerCount: -1,
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			module := setupNewFactotum(new(mockClient), tt.config)
			err := module.Init()

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestJobRegistration tests job registration and execution
func TestJobRegistration(t *testing.T) {
	type testJob struct {
		Name  string
		Count int
	}

	tests := []struct {
		name        string
		jobType     string
		payload     testJob
		setupMock   func(*mockClient)
		expectError bool
	}{
		{
			name:    "successful job registration and execution",
			jobType: "test_job",
			payload: testJob{Name: "test", Count: 1},
			setupMock: func(m *mockClient) {
				m.On("Push", mock.AnythingOfType("*client.Job")).Return(nil)
				m.On("Cleanup").Return()
			},
			expectError: false,
		},
		{
			name:    "failed job execution",
			jobType: "failing_job",
			payload: testJob{Name: "fail", Count: 0},
			setupMock: func(m *mockClient) {
				m.On("Push", mock.AnythingOfType("*client.Job")).Return(errors.New("mock error"))
				m.On("Cleanup").Return()
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock client
			mockClient := new(mockClient)
			tt.setupMock(mockClient)
			module := setupNewFactotum(mockClient, &faktotum.Config{
				WorkerCount: 1,
				Queues:      []string{"test"},
			})
			require.NoError(t, module.Init())

			// Create and register typed handler
			handler := faktotum.NewTypedHandler(func(ctx context.Context, job testJob) error {
				if job.Count == 0 {
					return errors.New("count cannot be zero")
				}
				return nil
			})

			module.RegisterJob(tt.jobType, handler.Perform)

			// Test job execution
			ctx := context.Background()
			err := module.EnqueueJob(ctx, &faktory.Job{
				Type: tt.jobType,
				Args: []interface{}{tt.payload},
			})

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

// TestPanicRecovery tests the panic recovery middleware
func TestPanicRecovery(t *testing.T) {
	module := setupNewFactotum(nil, &faktotum.Config{
		WorkerCount: 1,
		Queues:      []string{"test"},
	})
	require.NoError(t, module.Init())

	// Create a handler that panics
	baseHandler := worker.Perform(func(ctx context.Context, args ...interface{}) error {
		panic("test panic")
	})

	// Wrap it with panic recovery
	handler := module.WrapWithPanicRecovery("test_job", baseHandler)

	// Execute the wrapped handler
	ctx := context.Background()
	err := handler(ctx, "test")

	require.Error(t, err)
	var panicErr *faktotum.PanicError
	require.ErrorAs(t, err, &panicErr)
	assert.Contains(t, panicErr.Value, "test panic")
}

// TestHookExecution tests hook execution order and error handling
func TestHookExecution(t *testing.T) {
	var hookOrder []string

	testHook := &testHook{
		beforeJob: func(ctx context.Context, job *faktory.Job) error {
			hookOrder = append(hookOrder, "before")
			return nil
		},
		afterJob: func(ctx context.Context, job *faktory.Job, err error) {
			hookOrder = append(hookOrder, "after")
		},
	}

	module := setupNewFactotum(nil, &faktotum.Config{
		WorkerCount: 1,
		Queues:      []string{"test"},
	})
	require.NoError(t, module.Init())
	err := module.RegisterGlobalHook("test", testHook)
	require.NoError(t, err)

	// Create a test handler
	baseHandler := worker.Perform(func(ctx context.Context, args ...interface{}) error {
		hookOrder = append(hookOrder, "execute")
		return nil
	})

	// Wrap it with hooks
	handler := module.WrapWithHooks("test_job", baseHandler)

	// Execute the wrapped handler
	ctx := context.Background()
	err = handler(ctx, "test")

	require.NoError(t, err)
	assert.Equal(t, []string{"before", "execute", "after"}, hookOrder)
}

// testHook implementation remains the same
type testHook struct {
	beforeJob func(context.Context, *faktory.Job) error
	afterJob  func(context.Context, *faktory.Job, error)
}

func (h *testHook) BeforeJob(ctx context.Context, job *faktory.Job) error {
	return h.beforeJob(ctx, job)
}

func (h *testHook) AfterJob(ctx context.Context, job *faktory.Job, err error) {
	h.afterJob(ctx, job, err)
}

// Keep just the basic shutdown test
func TestGracefulShutdown(t *testing.T) {
	mockClient := new(mockClient)
	module := setupNewFactotum(mockClient, &faktotum.Config{
		WorkerCount:     1,
		Queues:          []string{"test"},
		ShutdownTimeout: 100 * time.Millisecond,
	})
	require.NoError(t, module.Init())

	ctx := context.Background()
	require.NoError(t, module.Start(ctx))

	err := module.Stop(ctx)
	assert.NoError(t, err)
}

func TestBulkEnqueue(t *testing.T) {
	tests := []struct {
		name      string
		jobs      []*faktory.Job
		setupMock func(*mockClient)
		verify    func(*testing.T, []faktotum.BulkEnqueueResult)
	}{
		{
			name: "successful bulk enqueue",
			jobs: []*faktory.Job{
				faktory.NewJob("job1", "arg1"),
				faktory.NewJob("job2", "arg2"),
				faktory.NewJob("job3", "arg3"),
			},
			setupMock: func(m *mockClient) {
				m.On("Push", mock.AnythingOfType("*client.Job")).Return(nil).Times(3)
				m.On("Cleanup").Return().Times(3)
			},
			verify: func(t *testing.T, results []faktotum.BulkEnqueueResult) {
				require.Len(t, results, 3)
				for _, result := range results {
					assert.NotEmpty(t, result.JobID)
					assert.NoError(t, result.Error)
				}
			},
		},
		{
			name: "partial failures",
			jobs: []*faktory.Job{
				faktory.NewJob("job1", "arg1"),
				faktory.NewJob("job2", "arg2"),
				faktory.NewJob("job3", "arg3"),
			},
			setupMock: func(m *mockClient) {
				m.On("Push", mock.MatchedBy(func(job *faktory.Job) bool {
					return job.Type == "job1"
				})).Return(nil).Once()
				m.On("Push", mock.MatchedBy(func(job *faktory.Job) bool {
					return job.Type == "job2"
				})).Return(errors.New("push error")).Once()
				m.On("Push", mock.MatchedBy(func(job *faktory.Job) bool {
					return job.Type == "job3"
				})).Return(nil).Once()
				m.On("Cleanup").Return().Times(3)
			},
			verify: func(t *testing.T, results []faktotum.BulkEnqueueResult) {
				require.Len(t, results, 3)
				assert.NoError(t, results[0].Error)
				assert.Error(t, results[1].Error)
				assert.NoError(t, results[2].Error)
			},
		},
		{
			name: "handles client creation failure",
			jobs: []*faktory.Job{
				faktory.NewJob("job1", "arg1"),
			},
			setupMock: func(m *mockClient) {
				// We'll create a module with a failing client factory instead
			},
			verify: func(t *testing.T, results []faktotum.BulkEnqueueResult) {
				require.Len(t, results, 1)
				assert.Error(t, results[0].Error)
				assert.Contains(t, results[0].Error.Error(), "failed to get client")
			},
		},
		{
			name: "handles panics",
			jobs: []*faktory.Job{
				faktory.NewJob("panic", "arg1"),
			},
			setupMock: func(m *mockClient) {
				m.On("Push", mock.AnythingOfType("*client.Job")).Run(func(args mock.Arguments) {
					panic("test panic")
				}).Once()
				m.On("Cleanup").Return().Once()
			},
			verify: func(t *testing.T, results []faktotum.BulkEnqueueResult) {
				require.Len(t, results, 1)
				assert.Error(t, results[0].Error)
				var panicErr *faktotum.PanicError
				assert.ErrorAs(t, results[0].Error, &panicErr)
				assert.Contains(t, panicErr.Value, "test panic")
			},
		},
		{
			name: "respects concurrency limit",
			jobs: func() []*faktory.Job {
				jobs := make([]*faktory.Job, 10)
				for i := range jobs {
					jobs[i] = faktory.NewJob(fmt.Sprintf("job%d", i), "arg")
				}
				return jobs
			}(),
			setupMock: func(m *mockClient) {
				m.On("Push", mock.AnythingOfType("*client.Job")).Run(func(args mock.Arguments) {
					// Add a small delay to test concurrency
					time.Sleep(50 * time.Millisecond)
				}).Return(nil).Times(10)
				m.On("Cleanup").Return().Times(10)
			},
			verify: func(t *testing.T, results []faktotum.BulkEnqueueResult) {
				require.Len(t, results, 10)
				for _, result := range results {
					assert.NoError(t, result.Error)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "handles client creation failure" {
				// Special case: use failing client factory
				module := faktotum.New(&faktotum.Config{WorkerCount: 2}, faktotum.WithClientFactory(func() (faktotum.Client, error) {
					return nil, errors.New("failed to get client")
				}))
				require.NoError(t, module.Init())
				results := module.BulkEnqueue(context.Background(), tt.jobs)
				tt.verify(t, results)
				return
			}

			mockClient := new(mockClient)
			tt.setupMock(mockClient)

			module := setupNewFactotum(mockClient, &faktotum.Config{
				WorkerCount: 2, // Small number to test concurrency
				Queues:      []string{"test"},
			})
			require.NoError(t, module.Init())

			start := time.Now()
			results := module.BulkEnqueue(context.Background(), tt.jobs)
			duration := time.Since(start)

			tt.verify(t, results)
			mockClient.AssertExpectations(t)

			// For the concurrency test, verify the duration
			if tt.name == "respects concurrency limit" {
				// With 10 jobs, 50ms sleep, and concurrency of 2,
				// it should take at least 250ms (5 batches * 50ms)
				assert.GreaterOrEqual(t, duration.Milliseconds(), int64(250))
			}
		})
	}
}
