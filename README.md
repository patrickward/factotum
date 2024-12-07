# Faktotum

A Go package that wraps the Faktory job processing system. Provides type-safe handlers, hook support, and integrates with modular Go applications.

## Features

- Type-safe job handlers using Go generics
- Global and per-job hooks for job lifecycle management
- Connection pooling for job operations
- Structured logging with slog
- Parallel job enqueueing

## Installation

```bash
go get github.com/patrickward/faktotum
```

## Basic Usage

```go
package main

import (
    "context"
    "log/slog"
    "time"
    
    "github.com/patrickward/faktotum"
)

// Define a job payload
type EmailJob struct {
    To      string
    Subject string
    Body    string
}

func main() {
    // Create and initialize Faktotum
    f := faktotum.New(faktotum.DefaultConfig())
    if err := f.Init(); err != nil {
        panic(err)
    }
    
    // Create a type-safe handler
    handler := faktotum.NewTypedHandler(func(ctx context.Context, job EmailJob) error {
        return sendEmail(job)
    })
    
    // Register the handler
    f.RegisterJob("send_email", handler.Perform)
    
    // Start processing
    ctx := context.Background()
    if err := f.Start(ctx); err != nil {
        panic(err)
    }
    
    // Create and enqueue a job
    job := faktotum.NewJob("send_email", EmailJob{
        To:      "user@example.com",
        Subject: "Hello",
        Body:    "World",
    }).
        Queue("critical").
        Retry(3).
        Build()
    
    if err := f.EnqueueJob(ctx, job); err != nil {
        slog.Error("Failed to enqueue job", "error", err)
    }
}
```

## Configuration

```go
config := &faktotum.Config{
    WorkerCount:     20,                            // Concurrent workers
    Queues:          []string{"default", "high"},   // Queues to process
    QueueWeights:    map[string]int{                // Optional weights
        "default": 1,
        "high":    10,
    },
    Labels:          []string{"api"},
    ServerURL:       "localhost:7419",
    ShutdownTimeout: 30 * time.Second,
    Logger:          slog.Default(),
}
```

## Creating Jobs

Use the job builder to configure jobs:

```go
job := faktotum.NewJob("job_type", payload).
    Queue("high").              // Queue name
    Retry(3).                   // Retry attempts
    Schedule(time.Now().Add(1 * time.Hour)). // Future execution
    ReserveFor(5 * time.Minute).    // Reservation timeout
    Backtrace(20).              // Backtrace line count
    Custom(map[string]interface{}{
        "customer_id": "12345",
    }).
    Build()
```

## Hooks

Hooks can be registered globally or per job type:

```go
type MetricsHook struct {
    successCount uint64
    failureCount uint64
}

func (h *MetricsHook) BeforeJob(ctx context.Context, job *faktory.Job) error {
    // Pre-job processing
    return nil
}

func (h *MetricsHook) AfterJob(ctx context.Context, job *faktory.Job, err error) {
    // Post-job processing
}

// Register globally - runs for all jobs
f.RegisterGlobalHook("metrics", &MetricsHook{})

// Register for specific job type - runs only for "email" jobs
f.RegisterJobHook("email", "metrics", &MetricsHook{})
```

Hooks execute in this order:
1. Global hooks in registration order
2. Job-specific hooks in registration order

## Bulk Job Enqueueing

Enqueue multiple jobs in parallel:

```go
jobs := []*faktory.Job{
    faktotum.NewJob("email", email1).Queue("high").Build(),
    faktotum.NewJob("email", email2).Queue("low").Build(),
}

results := f.BulkEnqueue(ctx, jobs)
for _, result := range results {
    if result.Error != nil {
        slog.Error("Job enqueue failed", 
            "job_id", result.JobID,
            "error", result.Error)
    }
}
```

## Module System Integration

Faktotum implements these interfaces:

```go
type Module interface {
    ID() string
    Init() error
}

type StartupModule interface {
    Module
    Start(ctx context.Context) error
}

type ShutdownModule interface {
    Module
    Stop(ctx context.Context) error
}
```

Usage with a module system:

```go
app.RegisterModule(faktotum.New(config))
```

## Testing

Mock the client for testing:

```go
// Create mock client
mockClient := &MockClient{}
mockClient.On("Push", mock.AnythingOfType("*client.Job")).Return(nil)

// Use mock client in Faktotum
f := faktotum.New(config,
    faktotum.WithClientFactory(func() (faktotum.Client, error) {
        return mockClient, nil
    }),
)
```

