package faktotum

import (
	"context"
	"encoding/json"
	"fmt"
)

// NilPayloadError represents an error when a nil payload is received
type NilPayloadError struct{}

func (e *NilPayloadError) Error() string {
	return "nil payload received, but typed handler requires a valid payload"
}

// TypedHandler helps create type-safe job handlers
type TypedHandler[T any] struct {
	handlerFunc func(context.Context, T) error
}

// NewTypedHandler creates a new typed handler
func NewTypedHandler[T any](handler func(context.Context, T) error) *TypedHandler[T] {
	if handler == nil {
		panic("handler function cannot be nil")
	}

	return &TypedHandler[T]{
		handlerFunc: handler,
	}
}

// Perform implements the faktory_worker.Perform type.
func (th *TypedHandler[T]) Perform(ctx context.Context, args ...any) error {
	if len(args) != 1 {
		return fmt.Errorf("expected 1 argument, got %d", len(args))
	}

	if args[0] == nil {
		return &NilPayloadError{}
	}

	var payload T
	data, err := json.Marshal(args[0])
	if err != nil {
		return fmt.Errorf("failed to marshal job payload: %w", err)
	}

	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal job payload: %w", err)
	}

	return th.handlerFunc(ctx, payload)
}
