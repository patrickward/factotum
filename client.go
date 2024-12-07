package faktotum

import faktory "github.com/contribsys/faktory/client"

// Client defines the interface for job operations. It is a
// wrapper around the Faktory client, so that we can mock it later.
type Client interface {
	// Push enqueues a job
	Push(job *faktory.Job) error

	// Close closes the client
	Close()

	// Cleanup cleans up the client
	Cleanup()
}

// ClientFactory defines a function that creates a new Client and a cleanup function
type ClientFactory func() (Client, error)

// faktoryClient wraps the real Faktory client for job operations
type faktoryClient struct {
	client *faktory.Client
	pool   *faktory.Pool
}

// Push enqueues a job
func (fc *faktoryClient) Push(job *faktory.Job) error {
	return fc.client.Push(job)
}

// Close closes the client
func (fc *faktoryClient) Close() {
	fc.pool.Put(fc.client)
}

// Cleanup cleans up the client
func (fc *faktoryClient) Cleanup() {
	fc.pool.Put(fc.client)
}

// defaultClientFactory creates a new ClientFactory using the real Faktory client
func defaultClientFactory(pool *faktory.Pool) ClientFactory {
	return func() (Client, error) {
		client, err := pool.Get()
		if err != nil {
			return nil, err
		}
		return &faktoryClient{client: client, pool: pool}, nil
	}
}
