package factotum_test

import (
	"context"
	"errors"
	"testing"
	"time"

	worker "github.com/contribsys/faktory_worker_go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	faktory "github.com/contribsys/faktory/client"

	"github.com/patrickward/factotum"
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

func setupNewFactotum(c *mockClient, config *factotum.Config) *factotum.FaktoryModule {
	return factotum.New(config, factotum.WithClientFactory(func() (factotum.Client, error) {
		return c, nil
	}))
}

// TestConfig tests configuration initialization and validation
func TestConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    *factotum.Config
		expectErr bool
	}{
		{
			name:      "nil config uses defaults",
			config:    nil,
			expectErr: false,
		},
		{
			name: "valid custom config",
			config: &factotum.Config{
				WorkerCount: 10,
				Queues:      []string{"high", "default", "low"},
				Labels:      []string{"api", "worker"},
			},
			expectErr: false,
		},
		{
			name: "invalid worker count",
			config: &factotum.Config{
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
			module := setupNewFactotum(mockClient, &factotum.Config{
				WorkerCount: 1,
				Queues:      []string{"test"},
			})
			require.NoError(t, module.Init())

			// Create and register typed handler
			handler := factotum.NewTypedHandler(func(ctx context.Context, job testJob) error {
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
	module := setupNewFactotum(nil, &factotum.Config{
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
	var panicErr *factotum.PanicError
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

	module := setupNewFactotum(nil, &factotum.Config{
		WorkerCount: 1,
		Queues:      []string{"test"},
	})
	require.NoError(t, module.Init())
	module.RegisterHook(testHook)

	// Create a test handler
	baseHandler := worker.Perform(func(ctx context.Context, args ...interface{}) error {
		hookOrder = append(hookOrder, "execute")
		return nil
	})

	// Wrap it with hooks
	handler := module.WrapWithHooks("test_job", baseHandler)

	// Execute the wrapped handler
	ctx := context.Background()
	err := handler(ctx, "test")

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

// TestGracefulShutdown tests shutdown behavior
func TestGracefulShutdown(t *testing.T) {
	// Test with shorter timeout for faster test
	mockClient := new(mockClient)
	//mockClient.On("Push", mock.AnythingOfType("*client.Job")).Return(nil)
	//mockClient.On("Cleanup").Return()

	module := setupNewFactotum(mockClient, &factotum.Config{
		WorkerCount:     1,
		Queues:          []string{"test"},
		ShutdownTimeout: time.Millisecond, // Very short for test
	})
	require.NoError(t, module.Init())

	// Start the module
	ctx := context.Background()
	require.NoError(t, module.Start(ctx))

	// Test shutdown
	err := module.Stop(ctx)
	assert.NoError(t, err) // Should return quickly since there's no real pool/manager

	mockClient.AssertExpectations(t)
}

func TestGracefulShutdownTimeout(t *testing.T) {
	// Create a mock manager that takes longer than the timeout
	longDelay := 200 * time.Millisecond
	module := factotum.New(&factotum.Config{
		WorkerCount:     1,
		Queues:          []string{"test"},
		ShutdownTimeout: 50 * time.Millisecond, // Shorter than the delay
	})
	require.NoError(t, module.Init())

	// Start the module
	ctx := context.Background()
	require.NoError(t, module.Start(ctx))

	// Sleep in Stop to simulate slow shutdown
	time.Sleep(longDelay)

	// Test shutdown
	err := module.Stop(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "shutdown timed out")
}
