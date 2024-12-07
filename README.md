# README.md
# Factotum

Factotum is a Go module that provides a modular, extensible approach to working with [Faktory](https://contribsys.com/faktory/) background job processing. It offers type-safe job definitions, middleware support, and easy integration with existing applications.

## Features

- 🔒 Type-safe job definitions and handlers
- 🔌 Modular design for easy integration
- 🧰 Middleware support for cross-cutting concerns
- 📊 Built-in metrics and logging
- 🔄 Automatic retry handling
- 📦 Batch job processing
- ⚡ Efficient connection pooling
- 🎛️ Configurable worker pools

## Installation

```bash
go get github.com/yourusername/factotum
```

## Quick Start

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/yourusername/factotum"
)

// Define your job payload
type EmailPayload struct {
    To      string   `json:"to"`
    Subject string   `json:"subject"`
    Body    string   `json:"body"`
    CC      []string `json:"cc,omitempty"`
}

func main() {
    // Create and configure the worker module
    mod := factotum.New(factotum.Config{
        Concurrency: 10,
        Queues:     []string{"critical", "default", "bulk"},
    })

    // Register a typed job handler
    emailJob, err := mod.RegisterTypedJob("SendEmail", func(ctx context.Context, payload EmailPayload) error {
        log.Printf("Sending email to %s: %s", payload.To, payload.Subject)
        // Implement email sending logic
        return nil
    })
    if err != nil {
        log.Fatal(err)
    }

    // Initialize the module
    if err := mod.Init(); err != nil {
        log.Fatal(err)
    }

    // Start processing jobs
    if err := mod.Start(context.Background()); err != nil {
        log.Fatal(err)
    }

    // Enqueue a job
    jid, err := emailJob.Enqueue(context.Background(), EmailPayload{
        To:      "user@example.com",
        Subject: "Hello!",
        Body:    "This is a test email",
    }, factotum.DefaultJobOptions().
        WithQueue("critical").
        WithRetry(3))

    if err != nil {
        log.Printf("Failed to enqueue job: %v", err)
    } else {
        log.Printf("Enqueued job with ID: %s", jid)
    }
}
```

## Module Integration

Factotum implements common module interfaces for easy integration with existing applications:

```go
// Register with your application's module system
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

## Middleware Support

Add cross-cutting concerns with middleware:

```go
// Add logging middleware
mod.RegisterHook(factotum.NewLoggingHook(log.Default()))

// Add metrics collection
metrics := factotum.NewMetricsHook()
mod.RegisterHook(metrics)

// Add retry handling
mod.RegisterHook(factotum.NewRetryHook(5))
```

## Configuration

Customize worker behavior with the Config struct:

```go
config := factotum.Config{
    Concurrency:     20,              // Number of concurrent workers
    Queues:         []string{"default", "critical"},  // Queue priority order
    ShutdownTimeout: 30 * time.Second, // Graceful shutdown period
    PoolSize:       10,               // Connection pool size
    Labels:         []string{"app:myapp"}, // Worker labels
}
```

## Job Options

Control job execution with options:

```go
opts := factotum.DefaultJobOptions().
    WithQueue("critical").           // Set queue
    WithPriority(9).                // Set priority (0-9)
    WithRetry(3).                   // Set retry count
    WithSchedule(time.Now().Add(5 * time.Minute)). // Delay execution
    WithLabels([]string{"customer:premium"})        // Add labels
```

## Batch Processing

Efficiently process multiple jobs:

```go
jobs := []EmailPayload{
    {To: "user1@example.com", Subject: "Hello 1"},
    {To: "user2@example.com", Subject: "Hello 2"},
}

jids, err := emailJob.EnqueueBatch(ctx, jobs, factotum.DefaultJobOptions().
    WithQueue("bulk"))
```

## Contributing

Contributions are welcome! Please read our [Contributing Guide](CONTRIBUTING.md) for details on our code of conduct and the process for submitting pull requests.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

