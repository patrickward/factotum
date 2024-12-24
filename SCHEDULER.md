# Faktotum Scheduler

The Faktotum Scheduler provides recurring job scheduling capabilities built on top of [go-co-op/gocron](https://github.com/go-co-op/gocron). It can be enabled as an optional component of your Faktotum instance.

## Table of Contents

- [Configuration](#configuration)
- [Enabling the Scheduler](#enabling-the-scheduler)
- [Scheduling Patterns](#scheduling-patterns)
- [Job Options](#job-options)
- [Error Handling](#error-handling)
- [Monitoring and Management](#monitoring-and-management)
- [Integration Patterns](#integration-patterns)
- [Migration Guide](#migration-guide)
- [Troubleshooting](#troubleshooting)

## Configuration

The scheduler is configured using `SchedulerConfig`:

```go
type SchedulerConfig struct {
    // Name is the identifier for this scheduler instance
    Name string

    // Location is the timezone for the scheduler
    Location *time.Location
}
```

### Default Configuration

If no config is provided, these defaults are used:
- Name: "scheduler"
- Location: time.Local

### Time Zone Considerations

- The scheduler's time zone is set through SchedulerConfig.Location
- All scheduled jobs run according to this configured time zone
- Use time.UTC for systems spanning multiple time zones
- Use time.Local (default) for jobs that should run in system local time

## Enabling the Scheduler

Enable scheduling when creating your Faktotum instance:

```go
f := faktotum.New(
    faktotumConfig,
    faktotum.WithScheduler(&faktotum.SchedulerConfig{
        Name:     "app-scheduler",
        Location: time.UTC,
    }),
)

// Check for scheduler availability
if scheduler := f.Scheduler(); scheduler != nil {
    // Use scheduler
}
```

### Or, create a new scheduler instance:

```go
scheduler := faktotum.NewScheduler(&faktotum.SchedulerConfig{
    Name:     "app-scheduler",
    Location: time.UTC,
})
``` 

## Scheduling Patterns

All scheduling methods accept optional gocron.JobOption parameters for customizing job behavior. These options allow you to control job execution, add tags, limit runs, and more.

### Basic Scheduling with Cron Expressions

The `Schedule` method provides the most flexible scheduling using cron expressions:

```go
scheduler.Schedule(faktotum.ScheduleOptions{
    Name: "backup-db",
    Job: faktotum.NewJob("backup.database", nil).Build(),
    Schedule: "0 0 * * *",      // Daily at midnight
    WithSeconds: false,         // Don't use seconds field
    JobOptions: []gocron.JobOption{
        gocron.WithSingletonMode(true),
        gocron.WithStartAt(time.Now().Add(time.Hour)),
    },
})

// With seconds precision
scheduler.Schedule(faktotum.ScheduleOptions{
    Name: "health-check",
    Job: faktotum.NewJob("monitor.health", nil).Build(),
    Schedule: "*/30 * * * * *", // Every 30 seconds
    WithSeconds: true,          // Use seconds field
    JobOptions: []gocron.JobOption{
        gocron.WithTags("monitoring", "health"),
    },
})
```

### Duration-based Scheduling

The `ScheduleEvery` method provides simple interval-based scheduling:

```go
// Every 5 minutes
scheduler.ScheduleEvery(
    "process-queue",
    faktotum.NewJob("queue.process", nil).Build(),
    5*time.Minute,
    gocron.WithSingletonMode(true),
    gocron.WithTags("queue", "processing"),
)

// Every hour
scheduler.ScheduleEvery(
    "hourly-task",
    faktotum.NewJob("task.hourly", nil).Build(),
    time.Hour,
    gocron.WithStartAt(time.Now().Add(time.Minute)),
)
```

### Daily Jobs

The `ScheduleDaily` method schedules jobs to run at specific times. The interval parameter determines how many days between runs:
- 1: Every day
- 2: Every other day
- 3: Every third day
- etc.

```go
// Every day at 9 AM and 5 PM
scheduler.ScheduleDaily(
    "daily-report",
    faktotum.NewJob("reports.generate", nil).Build(),
    1, // run every day
    gocron.NewAtTimes(
        gocron.NewAtTime(9, 0, 0),  // 9 AM
        gocron.NewAtTime(17, 0, 0), // 5 PM
    ),
    gocron.WithSingletonMode(true),
    gocron.WithTags("reports", "daily"),
)

// Every other day at 3 AM
scheduler.ScheduleDaily(
    "bi-daily-cleanup",
    faktotum.NewJob("cleanup", nil).Build(),
    2, // run every 2 days
    gocron.NewAtTimes(
        gocron.NewAtTime(3, 0, 0),
    ),
    gocron.WithStartAt(time.Now().Add(time.Hour)),
)
```

### Weekly Jobs

The `ScheduleWeekly` method schedules jobs to run on specific days of the week. The interval parameter determines how many weeks between runs:
- 1: Every week
- 2: Every other week (bi-weekly)
- 3: Every third week
- etc.

```go
// Every week on Saturday and Sunday at 1 AM
scheduler.ScheduleWeekly(
    "weekly-cleanup",
    faktotum.NewJob("maintenance.cleanup", nil).Build(),
    1, // run every week
    gocron.NewWeekdays(time.Saturday, time.Sunday),
    gocron.NewAtTimes(gocron.NewAtTime(1, 0, 0)),
    gocron.WithSingletonMode(true),
)

// Every other week on Monday at 9 AM
scheduler.ScheduleWeekly(
    "bi-weekly-report",
    faktotum.NewJob("reports.generate", nil).Build(),
    2, // run every 2 weeks
    gocron.NewWeekdays(time.Monday),
    gocron.NewAtTimes(gocron.NewAtTime(9, 0, 0)),
    gocron.WithTags("reports", "bi-weekly"),
)
```

### Monthly Jobs

The `ScheduleMonthly` method schedules jobs to run on specific days of the month. The interval parameter determines how many months between runs:
- 1: Every month
- 2: Every other month
- 3: Every quarter
- etc.

Days can be specified as 1-31 or negative numbers (-1 to -31) counting from the end of the month.

```go
// Every month on the 1st and 15th at midnight
scheduler.ScheduleMonthly(
    "monthly-billing",
    faktotum.NewJob("billing.process", nil).Build(),
    1, // run every month
    gocron.NewDaysOfTheMonth(1, 15), // 1st and 15th
    gocron.NewAtTimes(gocron.NewAtTime(0, 0, 0)),
    gocron.WithSingletonMode(true),
)

// Every quarter (3 months) on the first at 9 AM
scheduler.ScheduleMonthly(
    "quarterly-report",
    faktotum.NewJob("reports.quarterly", nil).Build(),
    3, // run every 3 months
    gocron.NewDaysOfTheMonth(1),    // First of the month
    gocron.NewAtTimes(gocron.NewAtTime(9, 0, 0)),
    gocron.WithTags("reports", "quarterly"),
)

// Last day of every month at 11 PM
scheduler.ScheduleMonthly(
    "end-of-month",
    faktotum.NewJob("month.close", nil).Build(),
    1, // run every month
    gocron.NewDaysOfTheMonth(-1),   // Last day of month
    gocron.NewAtTimes(gocron.NewAtTime(23, 0, 0)),
)
```

## Job Options

The scheduler supports all gocron job options. Common options include:

### Execution Control
```go
// Start time
gocron.WithStartAt(time.Now().Add(time.Hour))

// Limited runs
gocron.WithLimitedRuns(100)

// Prevent overlapping
gocron.WithSingletonMode(true)

// Custom tags
gocron.WithTags("billing", "critical")
```

### Using Multiple Options
```go
scheduler.Schedule(faktotum.ScheduleOptions{
    Name: "important-task",
    Job: faktotum.NewJob("task.important", nil).Build(),
    Schedule: "*/5 * * * *",
    JobOptions: []gocron.JobOption{
        gocron.WithStartAt(time.Now().Add(time.Hour)),
        gocron.WithSingletonMode(true),
        gocron.WithLimitedRuns(100),
    },
})
```

## Error Handling

### Initialization Errors
```go
f := faktotum.New(
    faktotumConfig,
    faktotum.WithScheduler(schedulerConfig),
)
if err := f.Init(); err != nil {
    // Handle scheduler initialization error
}
```

### Scheduling Errors
```go
if err := scheduler.Schedule(opts); err != nil {
    switch {
    case errors.Is(err, gocron.ErrJobAlreadyExists):
        // Handle duplicate job
    case errors.Is(err, gocron.ErrInvalidSchedule):
        // Handle invalid schedule
    default:
        // Handle other errors
    }
}
```

### Job Execution Errors
The scheduler logs job execution errors but doesn't stop the job from running again. Handle errors in your job implementation:

```go
func myJob(ctx context.Context, args ...interface{}) error {
    if err := doWork(); err != nil {
        // Log error, cleanup, etc.
        return err // Job will be marked as failed
    }
    return nil
}
```

## Monitoring and Management

### Listing Jobs
```go
jobs := scheduler.ListJobs()
for _, name := range jobs {
    fmt.Printf("Scheduled job: %s\n", name)
}
```

### Removing Jobs
```go
if err := scheduler.Unschedule("job-name"); err != nil {
    // Handle error
}
```

### Logging
The scheduler uses Faktotum's logger and includes structured fields:
- job_name: The name of the scheduled job
- job_type: The Faktory job type
- error: Any error message (when applicable)

## Integration Patterns

### Using with TypedHandler

```go
// Define your job type
type CleanupJob struct {
    OlderThan time.Duration `json:"older_than"`
    Type      string        `json:"type"`
}

// Create and register handler
handler := faktotum.NewTypedHandler(func(ctx context.Context, job CleanupJob) error {
    // Implementation
    return nil
})
f.RegisterJob("cleanup.data", handler.Perform)

// Schedule with type-safe parameters
scheduler.Schedule(faktotum.ScheduleOptions{
    Name: "typed-cleanup",
    Job: faktotum.NewJob("cleanup.data", CleanupJob{
        OlderThan: 24 * time.Hour,
        Type:      "temp-files",
    }).Build(),
    Schedule: "0 0 * * *",
})
```

### Service Integration

```go
type CleanupService struct {
    faktotum *faktotum.Faktotum
}

func NewCleanupService(f *faktotum.Faktotum) *CleanupService {
    return &CleanupService{faktotum: f}
}

func (s *CleanupService) Init() error {
    // Register job handlers
    s.faktotum.RegisterJob("cleanup.data", s.cleanupHandler)

    // Setup scheduling if enabled
    scheduler := s.faktotum.Scheduler()
    if scheduler == nil {
        return fmt.Errorf("scheduler required but not enabled")
    }

    return scheduler.ScheduleDaily(
        "daily-cleanup",
        faktotum.NewJob("cleanup.data", nil).Build(),
        1,
        gocron.NewAtTimes(gocron.NewAtTime(3, 0, 0)),
    )
}
```

## Troubleshooting

### Common Issues

1. **Jobs not running at expected times**
   - Check time zone configuration
   - Verify cron expression syntax
   - Look for overlapping job prevention (singleton mode)

2. **Job name conflicts**
   - Use unique, descriptive names
   - Consider namespacing with service names
   - Check for existing jobs before scheduling

3. **Memory usage**
   - Monitor number of scheduled jobs
   - Use appropriate intervals (avoid too frequent scheduling)
   - Clean up unused schedules

### Known Limitations

1. No persistent schedules (lost on restart)
2. No distributed coordination
3. All schedules must be re-created on startup

### Debugging Tips

1. Enable debug logging in Faktotum
2. Use job options to test timing:
   ```go
   gocron.WithStartAt(time.Now().Add(time.Minute))
   ```
3. Monitor job execution through Faktory UI
