# Faktotum

Faktotum is a Go module that provides a robust, type-safe wrapper around [Faktory](https://github.com/contribsys/faktory) job processing system. 

## Features

- 🔒 Type-safe job handlers using generics
- 🪝 Extensible hook system for job lifecycle management
- 📊 Built-in metrics collection
- 🔄 Graceful shutdown handling
- 🏊‍♂️ Efficient client connection pooling
- 📝 Structured logging with `slog`
- ⚡ Parallel job enqueueing with configurable concurrency
- 🧪 Comprehensive test coverage with mocking support

## Installation

```bash
go get github.com/yourusername/faktotum
```

## Quick Start

```go
package main

import (
    "context"
    "log/slog"
    "time"
    
    "github.com/yourusername/faktotum"
)

// Define your job payload
type EmailJob struct {
    To      string
    Subject string
    Body    string
}

func main() {
    // Create a new Faktotum instance with default configuration
    f := faktotum.New(faktotum.DefaultConfig())
    
    // Initialize the module
    if err := f.Init(); err != nil {
        panic(err)
    }
    
    // Register a typed job handler
    handler := faktotum.NewTypedHandler(func(ctx context.Context, job EmailJob) error {
        // Process the email job
        return sendEmail(job)
    })
    
    f.RegisterJob("send_email", handler.Perform)
    
    // Start the worker
    ctx := context.Background()
    if err := f.Start(ctx); err != nil {
        panic(err)
    }
    
    // Create and enqueue a job using the job builder
    job := faktotum.NewJob("send_email", EmailJob{
        To:      "user@example.com",
        Subject: "Hello",
        Body:    "World",
    }).
        Queue("critical").
        Retry(3).
        Schedule(time.Now().Add(1 * time.Hour)).
        Build()
    
    if err := f.EnqueueJob(ctx, job); err != nil {
        slog.Error("Failed to enqueue job", "error", err)
    }
}
```

## Configuration

Faktotum can be configured using the `Config` struct:

```go
config := &faktotum.Config{
    WorkerCount:     20,              // Number of concurrent workers
    Queues:          []string{"default", "critical"}, // Queues to process
    QueueWeights:    map[string]int{   // Optional queue weights
        "default":  1,
        "critical": 10,
    },
    Labels:          []string{"api"},  // Worker labels
    ServerURL:       "localhost:7419", // Faktory server URL
    ShutdownTimeout: 30 * time.Second, // Graceful shutdown timeout
    Logger:          slog.Default(),   // Custom logger
}
```

## Job Building

Faktotum provides a fluent job builder interface for creating jobs:

```go
job := faktotum.NewJob("email", emailPayload).
    Queue("critical").                    // Set queue name
    Retry(3).                            // Set retry count
    Schedule(time.Now().Add(1 * time.Hour)). // Schedule for later
    ReserveFor(5 * time.Minute).         // Set reservation timeout
    Backtrace(20).                       // Set backtrace lines count
    Custom(map[string]interface{}{       // Add custom metadata
        "customer_id": "12345",
    }).
    Build()                              // Create the job

// Enqueue the job
f.EnqueueJob(ctx, job)
```

## Hooks

Hooks allow you to execute code before and after job processing:

```go
type MetricsHook struct {
    // ... metrics fields
}

func (h *MetricsHook) BeforeJob(ctx context.Context, job *faktory.Job) error {
    // Record job start time
    return nil
}

func (h *MetricsHook) AfterJob(ctx context.Context, job *faktory.Job, err error) {
    // Record job completion metrics
}

// Register the hook
f.RegisterHook(&MetricsHook{})
```

## Bulk Job Enqueueing

Faktotum supports efficient parallel job enqueueing:

```go
jobs := []*faktory.Job{
    faktotum.NewJob("email", emailJob1).
        Queue("critical").
        Retry(3).
        Build(),
    faktotum.NewJob("email", emailJob2).
        Queue("default").
        Build(),
    // ... more jobs
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

## Integration with Module Systems

Faktotum implements common module interfaces for easy integration:

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

This makes it easy to use Faktotum with modular applications:

```go
app.RegisterModule(faktotum.New(config))
```

## Testing

Faktotum provides mocking support for testing:

```go
// Create a mock client
mockClient := &MockClient{}
mockClient.On("Push", mock.AnythingOfType("*client.Job")).Return(nil)

// Create Faktotum with mock client
f := faktotum.New(config,
    faktotum.WithClientFactory(func() (faktotum.Client, error) {
        return mockClient, nil
    }),
)
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
