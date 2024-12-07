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

## Prerequisites

This package requires a running Faktory server. For server installation and management, see [Faktory Installation](https://github.com/contribsys/faktory/wiki/Installation).

### Faktory Server Configuration

The following are some common server configuration options as defined in the [Faktory wiki](https://github.com/contribsys/faktory/wiki), 
but view the source for the most up-to-date information.

Default server ports:

- `7419`: Main server port for job processing
- `7420`: Web UI interface

Configure the Faktory server connection using environment variables:

```bash
# Server connection
FAKTORY_URL=tcp://localhost:7419      # Server address
FAKTORY_PROVIDER=FAKTORY_URL          # Alternate env var containing server URL
FAKTORY_PASSWORD=your-password        # Server password
```

If you make the web UI available and a password is set, the UI will use Basic Authentication with the FAKTORY_PASSWORD value.

> Only expose the Faktory server and UI to trusted networks.
> 
> See the [Faktory Security Guide](https://github.com/contribsys/faktory/wiki/Security) and any [reported vulnerabilities](https://github.com/contribsys/faktory/security) for more information.
 

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
    WorkerCount:     5,                             // Concurrent workers
    Queues:          []string{"default", "high"},   // Queues to process
    QueueWeights:    map[string]int{                // Optional weights
        "default": 1,
        "high":    10,
    },
    Labels:          []string{"api"},
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

Why does it require ID(), Init(), Start(), and Stop() methods?

This package is designed to be used with my web application framework, which uses a modular 
approach to registering and managing the lifecycle of components/services. However, it can be used
independently of the framework.

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

An example usage with a module system:

```go
app.RegisterModule(faktotum.New(config))
```

The module system would call the appropriate methods when the application starts and stops.

## Testing

You can mock the client factory to test Faktotum without connecting to a Faktory server. 

The following example uses the `github.com/stretchr/testify/mock` package:

```go
// Create mock client
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
```
