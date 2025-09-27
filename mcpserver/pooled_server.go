package mcpserver

import (
	"context"
	"log"
	"time"

	"github.com/a2y-d5l/bitbucket-datacenter-mcp-go/bitbucketdatacenter"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// PooledServerOptions extends ServerOptions to include connection pool configuration
type PooledServerOptions struct {
	*ServerOptions
	ConnectionPool *bitbucketdatacenter.PoolConfig
	EnablePool     bool
}

// PooledBitbucketAPI wraps either a regular client or a pooled client
type PooledBitbucketAPI struct {
	pooledClient *bitbucketdatacenter.PooledBitbucketClient
	regularClient *bitbucketdatacenter.Client
	usePool      bool
}

// NewPooledBitbucketAPI creates a new pooled Bitbucket API wrapper
func NewPooledBitbucketAPI(opts *PooledServerOptions) (*PooledBitbucketAPI, error) {
	if opts != nil && opts.EnablePool && opts.ConnectionPool != nil {
		// Create pooled client
		pooledClient, err := bitbucketdatacenter.NewPooledBitbucketClient(*opts.ConnectionPool)
		if err != nil {
			return nil, err
		}

		return &PooledBitbucketAPI{
			pooledClient: pooledClient,
			usePool:      true,
		}, nil
	}

	// Create regular client
	regularClient, err := bitbucketdatacenter.NewClient()
	if err != nil {
		return nil, err
	}

	return &PooledBitbucketAPI{
		regularClient: regularClient,
		usePool:       false,
	}, nil
}

// ListPullRequests lists pull requests using appropriate client
func (p *PooledBitbucketAPI) ListPullRequests(ctx context.Context, projectKey, repoSlug string, state string, start, limit int) (*bitbucketdatacenter.PagedResponse[bitbucketdatacenter.PullRequest], error) {
	if p.usePool {
		return p.pooledClient.ListPullRequests(ctx, projectKey, repoSlug, state, start, limit)
	}
	return p.regularClient.ListPullRequests(ctx, projectKey, repoSlug, state, start, limit)
}

// GetPullRequest gets a pull request using appropriate client
func (p *PooledBitbucketAPI) GetPullRequest(ctx context.Context, projectKey, repoSlug string, prID int) (*bitbucketdatacenter.PullRequest, error) {
	if p.usePool {
		return p.pooledClient.GetPullRequest(ctx, projectKey, repoSlug, prID)
	}
	return p.regularClient.GetPullRequest(ctx, projectKey, repoSlug, prID)
}

// CreatePullRequest creates a pull request using appropriate client
func (p *PooledBitbucketAPI) CreatePullRequest(ctx context.Context, projectKey, repoSlug string, req bitbucketdatacenter.CreatePRRequest) (*bitbucketdatacenter.PullRequest, error) {
	if p.usePool {
		return p.pooledClient.CreatePullRequest(ctx, projectKey, repoSlug, req)
	}
	return p.regularClient.CreatePullRequest(ctx, projectKey, repoSlug, req)
}

// UpdatePullRequest updates a pull request using appropriate client
func (p *PooledBitbucketAPI) UpdatePullRequest(ctx context.Context, projectKey, repoSlug string, prID int, req bitbucketdatacenter.UpdatePRRequest) (*bitbucketdatacenter.PullRequest, error) {
	if p.usePool {
		return p.pooledClient.UpdatePullRequest(ctx, projectKey, repoSlug, prID, req)
	}
	return p.regularClient.UpdatePullRequest(ctx, projectKey, repoSlug, prID, req)
}

// MergePullRequest merges a pull request using appropriate client
func (p *PooledBitbucketAPI) MergePullRequest(ctx context.Context, projectKey, repoSlug string, prID int, req bitbucketdatacenter.MergeRequest) (*bitbucketdatacenter.PullRequest, error) {
	if p.usePool {
		return p.pooledClient.MergePullRequest(ctx, projectKey, repoSlug, prID, req)
	}
	return p.regularClient.MergePullRequest(ctx, projectKey, repoSlug, prID, req)
}

// DeclinePullRequest declines a pull request using appropriate client
func (p *PooledBitbucketAPI) DeclinePullRequest(ctx context.Context, projectKey, repoSlug string, prID int, version int) (*bitbucketdatacenter.PullRequest, error) {
	if p.usePool {
		return p.pooledClient.DeclinePullRequest(ctx, projectKey, repoSlug, prID, version)
	}
	return p.regularClient.DeclinePullRequest(ctx, projectKey, repoSlug, prID, version)
}

// ReopenPullRequest reopens a pull request using appropriate client
func (p *PooledBitbucketAPI) ReopenPullRequest(ctx context.Context, projectKey, repoSlug string, prID int, version int) (*bitbucketdatacenter.PullRequest, error) {
	if p.usePool {
		return p.pooledClient.ReopenPullRequest(ctx, projectKey, repoSlug, prID, version)
	}
	return p.regularClient.ReopenPullRequest(ctx, projectKey, repoSlug, prID, version)
}

// ListActivities lists PR activities using appropriate client
func (p *PooledBitbucketAPI) ListActivities(ctx context.Context, projectKey, repoSlug string, prID int, start, limit int) (*bitbucketdatacenter.PagedResponse[bitbucketdatacenter.PRActivity], error) {
	if p.usePool {
		return p.pooledClient.ListActivities(ctx, projectKey, repoSlug, prID, start, limit)
	}
	return p.regularClient.ListActivities(ctx, projectKey, repoSlug, prID, start, limit)
}

// ListComments lists PR comments using appropriate client
func (p *PooledBitbucketAPI) ListComments(ctx context.Context, projectKey, repoSlug string, prID int, start, limit int) (*bitbucketdatacenter.PagedResponse[bitbucketdatacenter.PRComment], error) {
	if p.usePool {
		return p.pooledClient.ListComments(ctx, projectKey, repoSlug, prID, start, limit)
	}
	return p.regularClient.ListComments(ctx, projectKey, repoSlug, prID, start, limit)
}

// CreateComment creates a PR comment using appropriate client
func (p *PooledBitbucketAPI) CreateComment(ctx context.Context, projectKey, repoSlug string, prID int, text string) (*bitbucketdatacenter.PRComment, error) {
	if p.usePool {
		return p.pooledClient.CreateComment(ctx, projectKey, repoSlug, prID, text)
	}
	return p.regularClient.CreateComment(ctx, projectKey, repoSlug, prID, text)
}

// GetPullRequestRawDiff gets PR diff using appropriate client
func (p *PooledBitbucketAPI) GetPullRequestRawDiff(ctx context.Context, projectKey, repoSlug string, prID int, opts bitbucketdatacenter.DiffOptions) (string, error) {
	if p.usePool {
		return p.pooledClient.GetPullRequestRawDiff(ctx, projectKey, repoSlug, prID, opts)
	}
	return p.regularClient.GetPullRequestRawDiff(ctx, projectKey, repoSlug, prID, opts)
}

// GetPoolMetrics returns connection pool metrics (if using pool)
func (p *PooledBitbucketAPI) GetPoolMetrics() *bitbucketdatacenter.PoolMetrics {
	if p.usePool && p.pooledClient != nil {
		metrics := p.pooledClient.GetMetrics()
		return &metrics
	}
	return nil
}

// GetHealthMetrics returns health check metrics (if using pool)
func (p *PooledBitbucketAPI) GetHealthMetrics() *bitbucketdatacenter.HealthMetrics {
	if p.usePool && p.pooledClient != nil {
		metrics := p.pooledClient.GetHealthMetrics()
		return &metrics
	}
	return nil
}

// WarmUp warms up the connection pool (if using pool)
func (p *PooledBitbucketAPI) WarmUp(ctx context.Context) error {
	if p.usePool && p.pooledClient != nil {
		return p.pooledClient.WarmUp(ctx)
	}
	return nil
}

// Close closes the API wrapper
func (p *PooledBitbucketAPI) Close() error {
	if p.usePool && p.pooledClient != nil {
		return p.pooledClient.Close()
	}
	return nil
}

// StartWithConnectionPool starts the MCP server with connection pooling enabled
func StartWithConnectionPool(ctx context.Context, opts *PooledServerOptions) (*PooledBitbucketAPI, *mcp.ClientSession, func(), error) {
	// Create pooled API
	pooledAPI, err := NewPooledBitbucketAPI(opts)
	if err != nil {
		return nil, nil, nil, err
	}

	// Create regular server options
	serverOpts := opts.ServerOptions
	if serverOpts == nil {
		serverOpts = &ServerOptions{}
	}

	// Start the MCP server (will use regular tools but with pooled client)
	_, session, stopServer, err := Start(ctx, serverOpts)
	if err != nil {
		pooledAPI.Close()
		return nil, nil, nil, err
	}

	// Warm up the pool if enabled
	if opts != nil && opts.EnablePool {
		if err := pooledAPI.WarmUp(ctx); err != nil {
			log.Printf("Warning: Failed to warm up connection pool: %v", err)
		} else {
			log.Printf("Connection pool warmed up successfully")
		}
	}

	// Create combined stop function
	stop := func() {
		stopServer()
		pooledAPI.Close()
	}

	return pooledAPI, session, stop, nil
}

// CreateProductionPooledServerOptions creates production-ready pooled server options
func CreateProductionPooledServerOptions(enableCache bool) *PooledServerOptions {
	return &PooledServerOptions{
		ServerOptions: &ServerOptions{
			EnableCache: enableCache,
			Cache: &CacheConfig{
				TTL:               10 * time.Minute,
				MaxEntries:        2000,
				MaxMemoryBytes:    200 * 1024 * 1024, // 200MB
				MaxEntrySizeBytes: 20 * 1024 * 1024,  // 20MB
				CleanupInterval:   2 * time.Minute,
				EnableMetrics:     true,
				EnableCompression: true,
			},
		},
		ConnectionPool: &bitbucketdatacenter.ProductionPoolConfig,
		EnablePool:     true,
	}
}

// CreateDevelopmentPooledServerOptions creates development-friendly pooled server options
func CreateDevelopmentPooledServerOptions(enableCache bool) *PooledServerOptions {
	devPoolConfig := bitbucketdatacenter.DefaultPoolConfig
	// Reduce pool size for development
	devPoolConfig.MaxActive = 20
	devPoolConfig.MaxIdle = 5
	devPoolConfig.MinIdle = 1
	devPoolConfig.WarmupConnections = 1

	return &PooledServerOptions{
		ServerOptions: &ServerOptions{
			EnableCache: enableCache,
			Cache: &CacheConfig{
				TTL:               5 * time.Minute,
				MaxEntries:        500,
				MaxMemoryBytes:    50 * 1024 * 1024, // 50MB
				MaxEntrySizeBytes: 10 * 1024 * 1024, // 10MB
				CleanupInterval:   1 * time.Minute,
				EnableMetrics:     true,
				EnableCompression: false,
			},
		},
		ConnectionPool: &devPoolConfig,
		EnablePool:     true,
	}
}
