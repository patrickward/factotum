package faktotum

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	faktory "github.com/contribsys/faktory/client"
	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
)

// SchedulerConfig holds configuration for the scheduler
type SchedulerConfig struct {
	Name     string
	Location *time.Location
}

// DefaultSchedulerConfig returns the default scheduler configuration
func DefaultSchedulerConfig() *SchedulerConfig {
	return &SchedulerConfig{
		Name:     "faktotum-scheduler",
		Location: time.Local,
	}
}

// Scheduler manages periodic job scheduling for Faktotum
type Scheduler struct {
	scheduler gocron.Scheduler
	faktotum  *Faktotum
	config    *SchedulerConfig
	logger    *slog.Logger
	mu        sync.RWMutex
	jobs      map[string]uuid.UUID
}

// ScheduleOptions holds options for scheduling a job
type ScheduleOptions struct {
	Name        string
	Job         *faktory.Job
	Schedule    string // Cron expression
	WithSeconds bool   // Whether the cron expression includes seconds
	JobOptions  []gocron.JobOption
}

// NewScheduler creates a new Scheduler
func NewScheduler(f *Faktotum, cfg *SchedulerConfig) (*Scheduler, error) {
	if cfg == nil {
		cfg = DefaultSchedulerConfig()
	}

	scheduler, err := gocron.NewScheduler(
		gocron.WithLocation(cfg.Location),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create scheduler: %w", err)
	}

	return &Scheduler{
		scheduler: scheduler,
		faktotum:  f,
		config:    cfg,
		logger:    f.logger,
		jobs:      make(map[string]uuid.UUID),
	}, nil
}

// ID returns the scheduler ID and implements the module interface for hop.
func (s *Scheduler) ID() string {
	return s.config.Name
}

// Init initializes the scheduler. This implements the module interface for hop. It's a no-op for the scheduler.
func (s *Scheduler) Init() error {
	return nil
}

// Start starts the scheduler
func (s *Scheduler) Start(ctx context.Context) error {
	s.logger.Info("starting scheduler")
	s.scheduler.Start()

	go func() {
		<-ctx.Done()
		if err := s.scheduler.Shutdown(); err != nil {
			s.logger.Error("error shutting down scheduler",
				slog.String("error", err.Error()),
			)
		}
		s.logger.Info("scheduler stopped")
	}()

	return nil
}

// Stop stops the scheduler
func (s *Scheduler) Stop(_ context.Context) error {
	s.logger.Info("stopping scheduler")
	return s.scheduler.Shutdown()
}

// createJob is a helper function to create and register a new job
func (s *Scheduler) createJob(name string, faktoryJob *faktory.Job, jobDef gocron.JobDefinition, options []gocron.JobOption) error {
	if name == "" {
		return fmt.Errorf("job name is required")
	}
	if faktoryJob == nil {
		return fmt.Errorf("job configuration is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.jobs[name]; exists {
		return fmt.Errorf("job %s already scheduled", name)
	}

	task := func() {
		if err := s.faktotum.EnqueueJob(context.Background(), faktoryJob); err != nil {
			s.logger.Error("scheduled job execution failed",
				slog.String("job_name", name),
				slog.String("job_type", faktoryJob.Type),
				slog.String("error", err.Error()),
			)
		}
	}

	// Combine default options with user-provided options
	allOptions := append([]gocron.JobOption{
		gocron.WithName(name),
		gocron.WithTags(faktoryJob.Type),
	}, options...)

	job, err := s.scheduler.NewJob(
		jobDef,
		gocron.NewTask(task),
		allOptions...,
	)
	if err != nil {
		return fmt.Errorf("failed to create job: %w", err)
	}

	s.jobs[name] = job.ID()

	s.logger.Info("job scheduled successfully",
		slog.String("job_name", name),
		slog.String("job_type", faktoryJob.Type),
	)

	return nil
}

// Schedule schedules a job using a cron expression
func (s *Scheduler) Schedule(opts ScheduleOptions) error {
	if opts.Schedule == "" {
		return fmt.Errorf("schedule is required")
	}
	return s.createJob(opts.Name, opts.Job, gocron.CronJob(opts.Schedule, opts.WithSeconds), opts.JobOptions)
}

// ScheduleEvery schedules a job to run at a fixed duration
func (s *Scheduler) ScheduleEvery(name string, job *faktory.Job, duration time.Duration, options ...gocron.JobOption) error {
	return s.createJob(name, job, gocron.DurationJob(duration), options)
}

// ScheduleDaily schedules a job to run daily at specific times
func (s *Scheduler) ScheduleDaily(name string, job *faktory.Job, interval uint, atTimes gocron.AtTimes, options ...gocron.JobOption) error {
	return s.createJob(name, job, gocron.DailyJob(interval, atTimes), options)
}

// ScheduleWeekly schedules a job to run weekly on specific days and times
func (s *Scheduler) ScheduleWeekly(name string, job *faktory.Job, interval uint, daysOfTheWeek gocron.Weekdays, atTimes gocron.AtTimes, options ...gocron.JobOption) error {
	return s.createJob(name, job, gocron.WeeklyJob(interval, daysOfTheWeek, atTimes), options)
}

// ScheduleMonthly schedules a job to run monthly on specific days and times
func (s *Scheduler) ScheduleMonthly(name string, job *faktory.Job, interval uint, daysOfTheMonth gocron.DaysOfTheMonth, atTimes gocron.AtTimes, options ...gocron.JobOption) error {
	return s.createJob(name, job, gocron.MonthlyJob(interval, daysOfTheMonth, atTimes), options)
}

func (s *Scheduler) Unschedule(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobID, exists := s.jobs[name]
	if !exists {
		return fmt.Errorf("job %s not found", name)
	}

	if err := s.scheduler.RemoveJob(jobID); err != nil {
		// If RemoveJob fails, the job probably doesn't exist in the scheduler anymore
		s.logger.Warn("job not found in scheduler, cleaning up local reference",
			slog.String("job_name", name),
		)
	}

	delete(s.jobs, name)

	s.logger.Info("job unscheduled",
		slog.String("job_name", name),
	)

	return nil
}

// ListJobs returns all scheduled job names, validating against the scheduler
func (s *Scheduler) ListJobs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get all jobs from the scheduler
	schedulerJobs := s.scheduler.Jobs()

	// Create a map of valid job IDs
	validJobs := make(map[uuid.UUID]struct{})
	for _, job := range schedulerJobs {
		validJobs[job.ID()] = struct{}{}
	}

	// Build list of valid job names and clean up our map
	validNames := make([]string, 0, len(s.jobs))
	for name, id := range s.jobs {
		if _, exists := validJobs[id]; exists {
			validNames = append(validNames, name)
		} else {
			// Clean up jobs that no longer exist in the scheduler
			delete(s.jobs, name)
		}
	}

	return validNames
}
