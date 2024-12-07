package factotum

import (
	"time"

	faktory "github.com/contribsys/faktory/client"
)

// JobOption configures a job
type JobOption func(*faktory.Job)

// WithQueue sets the job queue
func WithQueue(queue string) JobOption {
	return func(j *faktory.Job) {
		j.Queue = queue
	}
}

// WithRetry sets the retry count
func WithRetry(retry int) JobOption {
	return func(j *faktory.Job) {
		j.Retry = &retry
	}
}

// WithSchedule schedules the job for future execution
func WithSchedule(at time.Time) JobOption {
	return func(j *faktory.Job) {
		j.At = at.Format(time.RFC3339)
	}
}

// WithCustom adds custom data to the job
func WithCustom(custom map[string]interface{}) JobOption {
	return func(j *faktory.Job) {
		j.Custom = custom
	}
}

// WithReserveFor sets the job reservation time
func WithReserveFor(duration time.Duration) JobOption {
	return func(j *faktory.Job) {
		j.ReserveFor = int(duration.Seconds())
	}
}
