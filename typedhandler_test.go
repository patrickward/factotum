package faktotum_test

import (
	"context"
	"testing"
	"time"

	faktory "github.com/contribsys/faktory/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/patrickward/faktotum"
)

type TestPayload struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Count     int       `json:"count"`
	CreatedAt time.Time `json:"created_at"`
}

func TestTypedHandler(t *testing.T) {
	tests := []struct {
		name          string
		payload       interface{}
		setupHandler  func() *faktotum.TypedHandler[TestPayload]
		expectError   bool
		errorContains string
	}{
		{
			name: "successful handling of valid payload",
			payload: TestPayload{
				ID:        "123",
				Name:      "test",
				Count:     42,
				CreatedAt: time.Now(),
			},
			setupHandler: func() *faktotum.TypedHandler[TestPayload] {
				return faktotum.NewTypedHandler(func(ctx context.Context, p TestPayload) error {
					assert.Equal(t, "123", p.ID)
					assert.Equal(t, "test", p.Name)
					assert.Equal(t, 42, p.Count)
					return nil
				})
			},
			expectError: false,
		},
		{
			name: "handles invalid payload structure",
			payload: map[string]interface{}{
				"id":   123, // wrong type, should be string
				"name": "test",
			},
			setupHandler: func() *faktotum.TypedHandler[TestPayload] {
				return faktotum.NewTypedHandler(func(ctx context.Context, p TestPayload) error {
					return nil
				})
			},
			expectError:   true,
			errorContains: "failed to unmarshal job payload",
		},
		{
			name:    "handles nil payload",
			payload: nil,
			setupHandler: func() *faktotum.TypedHandler[TestPayload] {
				return faktotum.NewTypedHandler(func(ctx context.Context, p TestPayload) error {
					return nil
				})
			},
			expectError:   true,
			errorContains: "nil payload received, but typed handler requires a valid payload",
		},
		{
			name: "handles handler error",
			payload: TestPayload{
				ID:   "123",
				Name: "test",
			},
			setupHandler: func() *faktotum.TypedHandler[TestPayload] {
				return faktotum.NewTypedHandler(func(ctx context.Context, p TestPayload) error {
					return assert.AnError
				})
			},
			expectError:   true,
			errorContains: "assert.AnError general error for testing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := tt.setupHandler()
			err := handler.Perform(context.Background(), tt.payload)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestTypedHandlerIntegration(t *testing.T) {
	// Create a mock client
	mockClient := new(mockClient)
	mockClient.On("Push", mock.AnythingOfType("*client.Job")).Return(nil)
	mockClient.On("Cleanup").Return()

	// Initialize Faktotum with the mock client
	module := setupNewFactotum(mockClient, &faktotum.Config{
		WorkerCount: 1,
		Queues:      []string{"test"},
	})
	require.NoError(t, module.Init())

	// Create a channel to signal job completion
	jobDone := make(chan struct{})

	// Create and register a typed handler
	handler := faktotum.NewTypedHandler(func(ctx context.Context, p TestPayload) error {
		assert.Equal(t, "test-id", p.ID)
		assert.Equal(t, "Test Job", p.Name)
		assert.Equal(t, 42, p.Count)
		close(jobDone)
		return nil
	})

	// Register the job
	module.RegisterJob("test.typed.job", handler.Perform)

	// Create and enqueue a job
	payload := TestPayload{
		ID:        "test-id",
		Name:      "Test Job",
		Count:     42,
		CreatedAt: time.Now(),
	}

	job := faktory.NewJob("test.typed.job", payload)
	err := module.EnqueueJob(context.Background(), job)
	require.NoError(t, err)

	// Verify mock expectations
	mockClient.AssertExpectations(t)
}
