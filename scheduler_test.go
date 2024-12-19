package faktotum_test

import (
	"context"
	"testing"
	"time"

	faktory "github.com/contribsys/faktory/client"
	"github.com/go-co-op/gocron/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/patrickward/faktotum"
)

func TestNewPeriodicScheduler(t *testing.T) {
	tests := []struct {
		name      string
		config    *faktotum.SchedulerConfig
		expectErr bool
	}{
		{
			name:      "creates scheduler with nil config",
			config:    nil,
			expectErr: false,
		},
		{
			name: "creates scheduler with custom location",
			config: &faktotum.SchedulerConfig{
				Location: time.UTC,
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(mockClient)
			f := setupNewFactotum(mockClient, &faktotum.Config{})

			scheduler, err := faktotum.NewScheduler(f, tt.config)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, scheduler)
			}
		})
	}
}

func TestScheduling(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*faktotum.Scheduler) error
		expectErr   bool
		errContains string
		validate    func(*testing.T, *faktotum.Scheduler)
	}{
		{
			name: "schedule cron job",
			setup: func(s *faktotum.Scheduler) error {
				return s.Schedule(faktotum.ScheduleOptions{
					Name:     "test-cron",
					Job:      faktory.NewJob("test.job", "arg1"),
					Schedule: "*/5 * * * *",
				})
			},
			expectErr: false,
			validate: func(t *testing.T, s *faktotum.Scheduler) {
				jobs := s.ListJobs()
				assert.Contains(t, jobs, "test-cron")
			},
		},
		{
			name: "schedule cron job with seconds",
			setup: func(s *faktotum.Scheduler) error {
				return s.Schedule(faktotum.ScheduleOptions{
					Name:        "test-cron-seconds",
					Job:         faktory.NewJob("test.job", "arg1"),
					Schedule:    "*/30 * * * * *",
					WithSeconds: true,
				})
			},
			expectErr: false,
			validate: func(t *testing.T, s *faktotum.Scheduler) {
				jobs := s.ListJobs()
				assert.Contains(t, jobs, "test-cron-seconds")
			},
		},
		{
			name: "schedule daily job",
			setup: func(s *faktotum.Scheduler) error {
				return s.ScheduleDaily(
					"test-daily",
					faktory.NewJob("test.job", "arg1"),
					1,
					gocron.NewAtTimes(gocron.NewAtTime(9, 0, 0)),
				)
			},
			expectErr: false,
			validate: func(t *testing.T, s *faktotum.Scheduler) {
				jobs := s.ListJobs()
				assert.Contains(t, jobs, "test-daily")
			},
		},
		{
			name: "schedule weekly job",
			setup: func(s *faktotum.Scheduler) error {
				return s.ScheduleWeekly(
					"test-weekly",
					faktory.NewJob("test.job", "arg1"),
					1,
					gocron.NewWeekdays(time.Monday, time.Wednesday),
					gocron.NewAtTimes(gocron.NewAtTime(10, 0, 0)),
				)
			},
			expectErr: false,
			validate: func(t *testing.T, s *faktotum.Scheduler) {
				jobs := s.ListJobs()
				assert.Contains(t, jobs, "test-weekly")
			},
		},
		{
			name: "schedule monthly job",
			setup: func(s *faktotum.Scheduler) error {
				return s.ScheduleMonthly(
					"test-monthly",
					faktory.NewJob("test.job", "arg1"),
					1,
					gocron.NewDaysOfTheMonth(1, 15),
					gocron.NewAtTimes(gocron.NewAtTime(12, 0, 0)),
				)
			},
			expectErr: false,
			validate: func(t *testing.T, s *faktotum.Scheduler) {
				jobs := s.ListJobs()
				assert.Contains(t, jobs, "test-monthly")
			},
		},
		{
			name: "schedule every duration",
			setup: func(s *faktotum.Scheduler) error {
				return s.ScheduleEvery(
					"test-duration",
					faktory.NewJob("test.job", "arg1"),
					5*time.Minute,
				)
			},
			expectErr: false,
			validate: func(t *testing.T, s *faktotum.Scheduler) {
				jobs := s.ListJobs()
				assert.Contains(t, jobs, "test-duration")
			},
		},
		{
			name: "fails with empty name",
			setup: func(s *faktotum.Scheduler) error {
				return s.Schedule(faktotum.ScheduleOptions{
					Name:     "",
					Job:      faktory.NewJob("test.job", "arg1"),
					Schedule: "*/5 * * * *",
				})
			},
			expectErr:   true,
			errContains: "job name is required",
		},
		{
			name: "fails with nil job",
			setup: func(s *faktotum.Scheduler) error {
				return s.Schedule(faktotum.ScheduleOptions{
					Name:     "test-job",
					Job:      nil,
					Schedule: "*/5 * * * *",
				})
			},
			expectErr:   true,
			errContains: "job configuration is required",
		},
		{
			name: "fails with empty schedule",
			setup: func(s *faktotum.Scheduler) error {
				return s.Schedule(faktotum.ScheduleOptions{
					Name:     "test-job",
					Job:      faktory.NewJob("test.job", "arg1"),
					Schedule: "",
				})
			},
			expectErr:   true,
			errContains: "schedule is required",
		},
		{
			name: "fails with duplicate job name",
			setup: func(s *faktotum.Scheduler) error {
				if err := s.Schedule(faktotum.ScheduleOptions{
					Name:     "test-job",
					Job:      faktory.NewJob("test.job", "arg1"),
					Schedule: "*/5 * * * *",
				}); err != nil {
					return err
				}
				return s.Schedule(faktotum.ScheduleOptions{
					Name:     "test-job",
					Job:      faktory.NewJob("test.job", "arg1"),
					Schedule: "*/10 * * * *",
				})
			},
			expectErr:   true,
			errContains: "already scheduled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(mockClient)
			mockClient.On("Push", mock.AnythingOfType("*client.Job")).Return(nil)
			mockClient.On("Cleanup").Return()

			f := setupNewFactotum(mockClient, &faktotum.Config{})
			scheduler, err := faktotum.NewScheduler(f, nil)
			require.NoError(t, err)

			err = scheduler.Start(context.Background())
			require.NoError(t, err)
			defer scheduler.Stop(context.Background())

			err = tt.setup(scheduler)
			if tt.expectErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, scheduler)
				}
			}
		})
	}
}

func TestUnschedule(t *testing.T) {
	tests := []struct {
		name        string
		setupJobs   func(*faktotum.Scheduler) error
		jobToRemove string
		expectErr   bool
		errContains string
	}{
		{
			name: "removes existing job",
			setupJobs: func(s *faktotum.Scheduler) error {
				return s.Schedule(faktotum.ScheduleOptions{
					Name:     "test-job",
					Job:      faktory.NewJob("test.job", "arg1"),
					Schedule: "*/5 * * * *",
				})
			},
			jobToRemove: "test-job",
			expectErr:   false,
		},
		{
			name:        "fails for non-existent job",
			setupJobs:   func(s *faktotum.Scheduler) error { return nil },
			jobToRemove: "non-existent",
			expectErr:   true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(mockClient)
			mockClient.On("Push", mock.AnythingOfType("*client.Job")).Return(nil)
			mockClient.On("Cleanup").Return()

			f := setupNewFactotum(mockClient, &faktotum.Config{})
			scheduler, err := faktotum.NewScheduler(f, nil)
			require.NoError(t, err)

			err = scheduler.Start(context.Background())
			require.NoError(t, err)
			defer scheduler.Stop(context.Background())

			err = tt.setupJobs(scheduler)
			require.NoError(t, err)

			err = scheduler.Unschedule(tt.jobToRemove)
			if tt.expectErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				jobs := scheduler.ListJobs()
				assert.NotContains(t, jobs, tt.jobToRemove)
			}
		})
	}
}
