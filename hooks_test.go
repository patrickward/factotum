package faktotum_test

import (
	"context"
	"errors"
	"testing"

	faktory "github.com/contribsys/faktory/client"
	worker "github.com/contribsys/faktory_worker_go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/patrickward/faktotum"
)

func TestHookRegistry(t *testing.T) {
	type hookExecution struct {
		name      string
		jobType   string
		beforeJob bool
	}

	tests := []struct {
		name            string
		setupHooks      func(*faktotum.Faktotum)
		job             *faktory.Job
		expectedOrder   []hookExecution
		expectError     bool
		errorHookName   string
		errorOnAfterJob bool
	}{
		{
			name: "global and job specific hooks execute in order",
			setupHooks: func(m *faktotum.Faktotum) {
				var executed []hookExecution

				// Global hook
				globalHook := &testHook{
					beforeJob: func(ctx context.Context, job *faktory.Job) error {
						executed = append(executed, hookExecution{"global", job.Type, true})
						return nil
					},
					afterJob: func(ctx context.Context, job *faktory.Job, err error) {
						executed = append(executed, hookExecution{"global", job.Type, false})
					},
				}
				require.NoError(t, m.RegisterGlobalHook("global-hook", globalHook))

				// Job-specific hook
				jobHook := &testHook{
					beforeJob: func(ctx context.Context, job *faktory.Job) error {
						executed = append(executed, hookExecution{"job-specific", job.Type, true})
						return nil
					},
					afterJob: func(ctx context.Context, job *faktory.Job, err error) {
						executed = append(executed, hookExecution{"job-specific", job.Type, false})
					},
				}
				require.NoError(t, m.RegisterJobHook("test-job", "job-hook", jobHook))
			},
			job: faktory.NewJob("test-job", "arg1"),
			expectedOrder: []hookExecution{
				{"global", "test-job", true},
				{"job-specific", "test-job", true},
				{"job-specific", "test-job", false},
				{"global", "test-job", false},
			},
		},
		{
			name: "duplicate global hook registration fails",
			setupHooks: func(m *faktotum.Faktotum) {
				hook1 := &testHook{
					beforeJob: func(ctx context.Context, job *faktory.Job) error {
						return nil
					},
					afterJob: func(ctx context.Context, job *faktory.Job, err error) {},
				}
				hook2 := &testHook{
					beforeJob: func(ctx context.Context, job *faktory.Job) error {
						return nil
					},
					afterJob: func(ctx context.Context, job *faktory.Job, err error) {},
				}
				require.NoError(t, m.RegisterGlobalHook("my-hook", hook1))
				require.Error(t, m.RegisterGlobalHook("my-hook", hook2))
			},
			job: faktory.NewJob("test-job", "arg1"),
		},
		{
			name: "duplicate job hook registration fails",
			setupHooks: func(m *faktotum.Faktotum) {
				hook1 := &testHook{
					beforeJob: func(ctx context.Context, job *faktory.Job) error {
						return nil
					},
					afterJob: func(ctx context.Context, job *faktory.Job, err error) {},
				}
				hook2 := &testHook{
					beforeJob: func(ctx context.Context, job *faktory.Job) error {
						return nil
					},
					afterJob: func(ctx context.Context, job *faktory.Job, err error) {},
				}
				require.NoError(t, m.RegisterJobHook("test-job", "my-hook", hook1))
				require.Error(t, m.RegisterJobHook("test-job", "my-hook", hook2))
			},
			job: faktory.NewJob("test-job", "arg1"),
		},
		{
			name: "before hook error stops execution",
			setupHooks: func(m *faktotum.Faktotum) {
				errorHook := &testHook{
					beforeJob: func(ctx context.Context, job *faktory.Job) error {
						return errors.New("hook error")
					},
					afterJob: func(ctx context.Context, job *faktory.Job, err error) {},
				}
				require.NoError(t, m.RegisterJobHook("test-job", "error-hook", errorHook))
			},
			job:           faktory.NewJob("test-job", "arg1"),
			expectError:   true,
			errorHookName: "error-hook",
		},
		{
			name: "after hook errors are logged but don't fail job",
			setupHooks: func(m *faktotum.Faktotum) {
				hook := &testHook{
					beforeJob: func(ctx context.Context, job *faktory.Job) error {
						return nil
					},
					afterJob: func(ctx context.Context, job *faktory.Job, err error) {
						panic("after hook panic")
					},
				}
				require.NoError(t, m.RegisterJobHook("test-job", "panic-hook", hook))
			},
			job:             faktory.NewJob("test-job", "arg1"),
			errorOnAfterJob: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(mockClient)
			module := setupNewFactotum(mockClient, &faktotum.Config{
				WorkerCount: 1,
				Queues:      []string{"test"},
			})
			require.NoError(t, module.Init())

			// Set up hooks
			tt.setupHooks(module)

			// Create and register handler
			executed := false
			handler := worker.Perform(func(ctx context.Context, args ...interface{}) error {
				executed = true
				return nil
			})

			// Wrap handler with hooks
			wrappedHandler := module.WrapWithHooks(tt.job.Type, handler)

			// Execute
			err := wrappedHandler(context.Background(), tt.job.Args...)

			if tt.expectError {
				assert.Error(t, err)
				assert.False(t, executed, "handler should not execute when hook fails")
			} else {
				assert.NoError(t, err)
				assert.True(t, executed, "handler should execute")
			}
		})
	}
}

func TestHookRegistryManagement(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*faktotum.Faktotum)
		verify func(*testing.T, *faktotum.Faktotum)
	}{
		{
			name: "unregister global hook",
			setup: func(m *faktotum.Faktotum) {
				hook := &testHook{
					beforeJob: func(ctx context.Context, job *faktory.Job) error { return nil },
					afterJob:  func(ctx context.Context, job *faktory.Job, err error) {},
				}
				require.NoError(t, m.RegisterGlobalHook("test-hook", hook))

				// Verify hook exists
				hooks := m.GetGlobalHooks()
				require.Contains(t, hooks, "test-hook")

				// Unregister
				require.NoError(t, m.UnregisterGlobalHook("test-hook"))
			},
			verify: func(t *testing.T, m *faktotum.Faktotum) {
				hooks := m.GetGlobalHooks()
				assert.Empty(t, hooks)

				// Trying to unregister again should fail
				err := m.UnregisterGlobalHook("test-hook")
				assert.Error(t, err)
			},
		},
		{
			name: "unregister job hook",
			setup: func(m *faktotum.Faktotum) {
				hook := &testHook{
					beforeJob: func(ctx context.Context, job *faktory.Job) error { return nil },
					afterJob:  func(ctx context.Context, job *faktory.Job, err error) {},
				}
				require.NoError(t, m.RegisterJobHook("test-job", "test-hook", hook))

				// Verify hook exists
				hooks := m.GetJobTypeHooks("test-job")
				require.Contains(t, hooks, "test-hook")

				// Unregister
				require.NoError(t, m.UnregisterJobHook("test-job", "test-hook"))
			},
			verify: func(t *testing.T, m *faktotum.Faktotum) {
				hooks := m.GetJobTypeHooks("test-job")
				assert.Empty(t, hooks)

				// Trying to unregister again should fail
				err := m.UnregisterJobHook("test-job", "test-hook")
				assert.Error(t, err)
			},
		},
		{
			name:  "get hooks for non-existent job type",
			setup: func(m *faktotum.Faktotum) {},
			verify: func(t *testing.T, m *faktotum.Faktotum) {
				hooks := m.GetJobTypeHooks("non-existent")
				assert.Empty(t, hooks)
			},
		},
		{
			name: "manage multiple hooks",
			setup: func(m *faktotum.Faktotum) {
				hook1 := &testHook{
					beforeJob: func(ctx context.Context, job *faktory.Job) error { return nil },
					afterJob:  func(ctx context.Context, job *faktory.Job, err error) {},
				}
				hook2 := &testHook{
					beforeJob: func(ctx context.Context, job *faktory.Job) error { return nil },
					afterJob:  func(ctx context.Context, job *faktory.Job, err error) {},
				}

				require.NoError(t, m.RegisterGlobalHook("global-1", hook1))
				require.NoError(t, m.RegisterGlobalHook("global-2", hook2))
				require.NoError(t, m.RegisterJobHook("test-job", "job-1", hook1))
				require.NoError(t, m.RegisterJobHook("test-job", "job-2", hook2))
			},
			verify: func(t *testing.T, m *faktotum.Faktotum) {
				globalHooks := m.GetGlobalHooks()
				jobHooks := m.GetJobTypeHooks("test-job")

				assert.Len(t, globalHooks, 2)
				assert.Len(t, jobHooks, 2)

				require.NoError(t, m.UnregisterGlobalHook("global-1"))
				require.NoError(t, m.UnregisterJobHook("test-job", "job-1"))

				globalHooks = m.GetGlobalHooks()
				jobHooks = m.GetJobTypeHooks("test-job")

				assert.Len(t, globalHooks, 1)
				assert.Len(t, jobHooks, 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(mockClient)
			module := setupNewFactotum(mockClient, &faktotum.Config{
				WorkerCount: 1,
				Queues:      []string{"test"},
			})
			require.NoError(t, module.Init())

			tt.setup(module)
			tt.verify(t, module)
		})
	}
}
