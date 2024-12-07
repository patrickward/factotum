package factotum

import (
	"time"

	faktory "github.com/contribsys/faktory/client"
)

// JobBuilder provides a fluent interface for building Faktory jobs
type JobBuilder struct {
	job *faktory.Job
}

// NewJob creates a new JobBuilder with the given job type and arguments
func NewJob(jobType string, args ...interface{}) *JobBuilder {
	return &JobBuilder{
		job: faktory.NewJob(jobType, args...),
	}
}

// Queue sets the job queue
func (b *JobBuilder) Queue(queue string) *JobBuilder {
	b.job.Queue = queue
	return b
}

// Retry sets the number of times to retry the job
// Use 0 for no retries, -1 for infinite retries
func (b *JobBuilder) Retry(count int) *JobBuilder {
	b.job.Retry = &count
	return b
}

// Schedule sets when the job should be executed
func (b *JobBuilder) Schedule(at time.Time) *JobBuilder {
	b.job.At = at.Format(time.RFC3339)
	return b
}

// ReserveFor sets how long the job should be reserved for (in seconds)
func (b *JobBuilder) ReserveFor(duration time.Duration) *JobBuilder {
	b.job.ReserveFor = int(duration.Seconds())
	return b
}

// Custom sets custom metadata for the job
func (b *JobBuilder) Custom(data map[string]interface{}) *JobBuilder {
	b.job.Custom = data
	return b
}

// Backtrace sets the number of backtrace lines to retain on errors
func (b *JobBuilder) Backtrace(lines int) *JobBuilder {
	b.job.Backtrace = lines
	return b
}

// Build returns the configured Faktory job
func (b *JobBuilder) Build() *faktory.Job {
	return b.job
}
