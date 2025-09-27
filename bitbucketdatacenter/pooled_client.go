package bitbucketdatacenter

import (
	"context"
	"fmt"
	"time"
)

// PooledBitbucketClient provides a high-level interface for using pooled connections
type PooledBitbucketClient struct {
	pool    *ConnectionPool
	timeout time.Duration
}

// NewPooledBitbucketClient creates a new pooled Bitbucket client
func NewPooledBitbucketClient(config PoolConfig) (*PooledBitbucketClient, error) {
	pool, err := NewConnectionPool(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	return &PooledBitbucketClient{
		pool:    pool,
		timeout: 30 * time.Second,
	}, nil
}

// ExecuteWithConnection executes a function with a pooled connection
func (pc *PooledBitbucketClient) ExecuteWithConnection(ctx context.Context, fn func(*Client) error) error {
	conn, err := pc.pool.GetConnection(ctx)
	if err != nil {
		return fmt.Errorf("failed to get connection from pool: %w", err)
	}
	defer pc.pool.ReturnConnection(conn)

	return fn(conn.Client)
}

// ListPullRequests lists pull requests using a pooled connection
func (pc *PooledBitbucketClient) ListPullRequests(ctx context.Context, projectKey, repoSlug string, state string, start, limit int) (*PagedResponse[PullRequest], error) {
	var result *PagedResponse[PullRequest]
	var err error

	executeErr := pc.ExecuteWithConnection(ctx, func(client *Client) error {
		result, err = client.ListPullRequests(ctx, projectKey, repoSlug, state, start, limit)
		return err
	})

	if executeErr != nil {
		return nil, executeErr
	}
	return result, err
}

// GetPullRequest gets a pull request using a pooled connection
func (pc *PooledBitbucketClient) GetPullRequest(ctx context.Context, projectKey, repoSlug string, prID int) (*PullRequest, error) {
	var result *PullRequest
	var err error

	executeErr := pc.ExecuteWithConnection(ctx, func(client *Client) error {
		result, err = client.GetPullRequest(ctx, projectKey, repoSlug, prID)
		return err
	})

	if executeErr != nil {
		return nil, executeErr
	}
	return result, err
}

// CreatePullRequest creates a pull request using a pooled connection
func (pc *PooledBitbucketClient) CreatePullRequest(ctx context.Context, projectKey, repoSlug string, req CreatePRRequest) (*PullRequest, error) {
	var result *PullRequest
	var err error

	executeErr := pc.ExecuteWithConnection(ctx, func(client *Client) error {
		result, err = client.CreatePullRequest(ctx, projectKey, repoSlug, req)
		return err
	})

	if executeErr != nil {
		return nil, executeErr
	}
	return result, err
}

// UpdatePullRequest updates a pull request using a pooled connection
func (pc *PooledBitbucketClient) UpdatePullRequest(ctx context.Context, projectKey, repoSlug string, prID int, req UpdatePRRequest) (*PullRequest, error) {
	var result *PullRequest
	var err error

	executeErr := pc.ExecuteWithConnection(ctx, func(client *Client) error {
		result, err = client.UpdatePullRequest(ctx, projectKey, repoSlug, prID, req)
		return err
	})

	if executeErr != nil {
		return nil, executeErr
	}
	return result, err
}

// MergePullRequest merges a pull request using a pooled connection
func (pc *PooledBitbucketClient) MergePullRequest(ctx context.Context, projectKey, repoSlug string, prID int, req MergeRequest) (*PullRequest, error) {
	var result *PullRequest
	var err error

	executeErr := pc.ExecuteWithConnection(ctx, func(client *Client) error {
		result, err = client.MergePullRequest(ctx, projectKey, repoSlug, prID, req)
		return err
	})

	if executeErr != nil {
		return nil, executeErr
	}
	return result, err
}

// DeclinePullRequest declines a pull request using a pooled connection
func (pc *PooledBitbucketClient) DeclinePullRequest(ctx context.Context, projectKey, repoSlug string, prID int, version int) (*PullRequest, error) {
	var result *PullRequest
	var err error

	executeErr := pc.ExecuteWithConnection(ctx, func(client *Client) error {
		result, err = client.DeclinePullRequest(ctx, projectKey, repoSlug, prID, version)
		return err
	})

	if executeErr != nil {
		return nil, executeErr
	}
	return result, err
}

// ReopenPullRequest reopens a pull request using a pooled connection
func (pc *PooledBitbucketClient) ReopenPullRequest(ctx context.Context, projectKey, repoSlug string, prID int, version int) (*PullRequest, error) {
	var result *PullRequest
	var err error

	executeErr := pc.ExecuteWithConnection(ctx, func(client *Client) error {
		result, err = client.ReopenPullRequest(ctx, projectKey, repoSlug, prID, version)
		return err
	})

	if executeErr != nil {
		return nil, executeErr
	}
	return result, err
}

// ListActivities lists PR activities using a pooled connection
func (pc *PooledBitbucketClient) ListActivities(ctx context.Context, projectKey, repoSlug string, prID int, start, limit int) (*PagedResponse[PRActivity], error) {
	var result *PagedResponse[PRActivity]
	var err error

	executeErr := pc.ExecuteWithConnection(ctx, func(client *Client) error {
		result, err = client.ListActivities(ctx, projectKey, repoSlug, prID, start, limit)
		return err
	})

	if executeErr != nil {
		return nil, executeErr
	}
	return result, err
}

// ListComments lists PR comments using a pooled connection
func (pc *PooledBitbucketClient) ListComments(ctx context.Context, projectKey, repoSlug string, prID int, start, limit int) (*PagedResponse[PRComment], error) {
	var result *PagedResponse[PRComment]
	var err error

	executeErr := pc.ExecuteWithConnection(ctx, func(client *Client) error {
		result, err = client.ListComments(ctx, projectKey, repoSlug, prID, start, limit)
		return err
	})

	if executeErr != nil {
		return nil, executeErr
	}
	return result, err
}

// CreateComment creates a PR comment using a pooled connection
func (pc *PooledBitbucketClient) CreateComment(ctx context.Context, projectKey, repoSlug string, prID int, text string) (*PRComment, error) {
	var result *PRComment
	var err error

	executeErr := pc.ExecuteWithConnection(ctx, func(client *Client) error {
		result, err = client.CreateComment(ctx, projectKey, repoSlug, prID, text)
		return err
	})

	if executeErr != nil {
		return nil, executeErr
	}
	return result, err
}

// GetPullRequestRawDiff gets PR diff using a pooled connection
func (pc *PooledBitbucketClient) GetPullRequestRawDiff(ctx context.Context, projectKey, repoSlug string, prID int, opts DiffOptions) (string, error) {
	var result string
	var err error

	executeErr := pc.ExecuteWithConnection(ctx, func(client *Client) error {
		result, err = client.GetPullRequestRawDiff(ctx, projectKey, repoSlug, prID, opts)
		return err
	})

	if executeErr != nil {
		return "", executeErr
	}
	return result, err
}

// GetMetrics returns connection pool metrics
func (pc *PooledBitbucketClient) GetMetrics() PoolMetrics {
	return pc.pool.GetMetrics()
}

// GetHealthMetrics returns health check metrics
func (pc *PooledBitbucketClient) GetHealthMetrics() HealthMetrics {
	return pc.pool.health.GetHealthMetrics()
}

// Close closes the connection pool
func (pc *PooledBitbucketClient) Close() error {
	return pc.pool.Close()
}

// SetTimeout sets the timeout for operations
func (pc *PooledBitbucketClient) SetTimeout(timeout time.Duration) {
	pc.timeout = timeout
}

// SetCustomHealthCheck sets a custom health check function
func (pc *PooledBitbucketClient) SetCustomHealthCheck(fn func(*Client) error) {
	pc.pool.health.SetCustomHealthFunc(fn)
}

// WarmUp performs connection pool warmup
func (pc *PooledBitbucketClient) WarmUp(ctx context.Context) error {
	// Get and immediately return connections to warm up the pool
	var conns []*PooledConnection
	warmupCount := pc.pool.config.WarmupConnections
	if warmupCount == 0 {
		warmupCount = pc.pool.config.MinIdle
	}

	// Get connections
	for i := 0; i < warmupCount; i++ {
		conn, err := pc.pool.GetConnection(ctx)
		if err != nil {
			// Return any connections we've collected
			for _, c := range conns {
				pc.pool.ReturnConnection(c)
			}
			return fmt.Errorf("failed to warm up connection pool: %w", err)
		}
		conns = append(conns, conn)
	}

	// Return all connections
	for _, conn := range conns {
		if err := pc.pool.ReturnConnection(conn); err != nil {
			return fmt.Errorf("failed to return connection during warmup: %w", err)
		}
	}

	return nil
}

// DefaultPoolConfig provides a production-ready pool configuration
var DefaultPoolConfig = PoolConfig{
	MaxIdle:                    10,
	MinIdle:                    2,
	MaxActive:                  100,
	WarmupConnections:          3,
	IdleTimeout:                30 * time.Minute,
	SoftMaxLifetime:            30 * time.Minute,
	HardMaxLifetime:            60 * time.Minute,
	ConnectTimeout:             30 * time.Second,
	MaxWait:                    30 * time.Second,
	ConnectionRequestTimeout:   10 * time.Second,
	QueueRequestsWhenFull:      true,
	MaxQueueSize:               100,
	HealthCheckInterval:        60 * time.Second,
	HealthCheckTimeout:         10 * time.Second,
	MaxConsecutiveFailures:     3,
	CircuitBreakerCooldown:     60 * time.Second,
	ValidateOnGet:              true,
	ValidateOnReturn:           false,
	PingOnValidate:             true,
	FallbackToDirectConnection: true,
	AllowPoolBypass:            false,
}

// ProductionPoolConfig provides a configuration optimized for production workloads
var ProductionPoolConfig = PoolConfig{
	MaxIdle:                    20,
	MinIdle:                    5,
	MaxActive:                  200,
	WarmupConnections:          10,
	IdleTimeout:                15 * time.Minute,
	SoftMaxLifetime:            20 * time.Minute,
	HardMaxLifetime:            45 * time.Minute,
	ConnectTimeout:             30 * time.Second,
	MaxWait:                    30 * time.Second,
	ConnectionRequestTimeout:   10 * time.Second,
	QueueRequestsWhenFull:      true,
	MaxQueueSize:               200,
	HealthCheckInterval:        30 * time.Second,
	HealthCheckTimeout:         10 * time.Second,
	MaxConsecutiveFailures:     2,
	CircuitBreakerCooldown:     60 * time.Second,
	ValidateOnGet:              true,
	ValidateOnReturn:           false,
	PingOnValidate:             true,
	FallbackToDirectConnection: true,
	AllowPoolBypass:            false,
}
