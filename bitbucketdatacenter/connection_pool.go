package bitbucketdatacenter

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ConnectionPool manages a pool of Bitbucket Data Center client connections
// for a single Bitbucket instance to improve resource utilization and lifecycle management.
type ConnectionPool struct {
	// Connection management (single Bitbucket instance)
	idleConns    chan *PooledConnection      // Available idle connections
	activeConns  map[*PooledConnection]bool  // Currently active connections
	waitingQueue chan chan *PooledConnection // Queue for waiting requests

	// Configuration and integration
	config  PoolConfig
	metrics *PoolMetrics
	factory ConnectionFactory

	// Health checking and circuit breaker
	health       *HealthChecker
	circuitState int32 // CircuitState using atomic operations

	// Statistics (atomic counters)
	active         int32 // Current active connections
	totalCreated   int64 // Total connections ever created
	totalClosed    int64 // Total connections closed
	totalRequests  int64 // Total connection requests

	// Concurrency control
	mu     sync.RWMutex
	closed bool

	// Background goroutines management
	stopCleanup chan bool
	stopWarmup  chan bool
	wg          sync.WaitGroup // To wait for background goroutines to finish
}

// PooledConnection wraps a client with lifecycle metadata
type PooledConnection struct {
	*Client
	createdAt   time.Time
	lastUsedAt  time.Time
	usageCount  int64
	poolRef     *ConnectionPool // Reference to parent pool
	id          string          // Unique connection identifier
}

// Circuit breaker states for health management
type CircuitState int32

const (
	CircuitClosed   CircuitState = iota // Normal operation
	CircuitOpen                         // Failing, reject requests
	CircuitHalfOpen                     // Testing if recovery is possible
)

// PoolConfig holds all configuration for the connection pool
type PoolConfig struct {
	// Connection pool sizing
	MaxIdle           int `default:"10"`
	MinIdle           int `default:"2"`    // Minimum idle connections to maintain
	MaxActive         int `default:"100"`
	WarmupConnections int `default:"1"`    // Pre-create connections on startup

	// Connection lifecycle
	IdleTimeout         time.Duration `default:"30m"`
	SoftMaxLifetime     time.Duration `default:"30m"` // Preferred max lifetime
	HardMaxLifetime     time.Duration `default:"60m"` // Absolute max lifetime
	ConnectTimeout      time.Duration `default:"30s"`

	// Request handling
	MaxWait                  time.Duration `default:"30s"`  // Max wait time for connection
	ConnectionRequestTimeout time.Duration `default:"10s"`  // Timeout waiting for connection
	QueueRequestsWhenFull    bool          `default:"true"` // Queue vs reject when full
	MaxQueueSize             int           `default:"100"`  // Max queued requests

	// Health checking
	HealthCheckInterval    time.Duration `default:"60s"`
	HealthCheckTimeout     time.Duration `default:"10s"`
	MaxConsecutiveFailures int           `default:"3"`    // Circuit breaker threshold
	CircuitBreakerCooldown time.Duration `default:"60s"` // Time before retry after circuit opens

	// Connection validation
	ValidateOnGet    bool `default:"true"`  // Validate connection before use
	ValidateOnReturn bool `default:"false"` // Validate when returning to pool
	PingOnValidate   bool `default:"true"`  // Use ping for validation

	// Fallback behavior
	FallbackToDirectConnection bool `default:"true"`  // Create direct connection if pool exhausted
	AllowPoolBypass            bool `default:"false"` // Allow bypassing pool entirely
}

// PoolMetrics provides observability into pool performance
type PoolMetrics struct {
	// Connection statistics
	ActiveConnections  int64 `json:"activeConnections"`
	IdleConnections    int64 `json:"idleConnections"`
	TotalConnections   int64 `json:"totalConnections"`
	CreatedConnections int64 `json:"createdConnections"`
	ClosedConnections  int64 `json:"closedConnections"`

	// Request statistics
	TotalRequests          int64 `json:"totalRequests"`
	QueuedRequests         int64 `json:"queuedRequests"`
	RejectedRequests       int64 `json:"rejectedRequests"`
	TimeoutRequests        int64 `json:"timeoutRequests"`
	IdleConnectionHits     int64 `json:"idleConnectionHits"`     // Requests served by idle connections
	NewConnectionCreations int64 `json:"newConnectionCreations"` // Requests requiring new connections

	// Performance metrics
	AverageWaitTime     time.Duration `json:"averageWaitTime"`
	FastestResponseTime time.Duration `json:"fastestResponseTime"`
	SlowestResponseTime time.Duration `json:"slowestResponseTime"`
	AverageResponseTime time.Duration `json:"averageResponseTime"`

	// Health and reliability
	HealthCheckSuccesses int64 `json:"healthCheckSuccesses"`
	HealthCheckFailures  int64 `json:"healthCheckFailures"`
	CircuitBreakerTrips  int64 `json:"circuitBreakerTrips"`
	ConnectionErrors     int64 `json:"connectionErrors"`
	ValidationFailures   int64 `json:"validationFailures"`

	// Resource utilization
	PoolExhaustionEvents   int64   `json:"poolExhaustionEvents"`
	FallbackConnections    int64   `json:"fallbackConnections"`
	MaxConcurrentRequests  int64   `json:"maxConcurrentRequests"`
	AveragePoolUtilization float64 `json:"averagePoolUtilization"`

	// Connection lifecycle
	AverageConnectionAge time.Duration `json:"averageConnectionAge"`
	ExpiredConnections   int64         `json:"expiredConnections"`
	ReplacedConnections  int64         `json:"replacedConnections"`

	// Timing stats (for calculating averages)
	totalWaitTime    time.Duration
	totalRequestTime time.Duration
	requestCount     int64
}

// ConnectionFactory creates new connections for the pool
type ConnectionFactory interface {
	CreateConnection(ctx context.Context) (*Client, error)
	ValidateConnection(ctx context.Context, conn *Client) error
	CloseConnection(conn *Client) error
}

// DefaultConnectionFactory implements ConnectionFactory using environment variables
type DefaultConnectionFactory struct{}

// CreateConnection creates a new Bitbucket client connection
func (f *DefaultConnectionFactory) CreateConnection(ctx context.Context) (*Client, error) {
	return NewClient()
}

// ValidateConnection validates that a connection is still healthy
func (f *DefaultConnectionFactory) ValidateConnection(ctx context.Context, conn *Client) error {
	// Simple validation - could be enhanced with actual API ping
	if conn == nil {
		return fmt.Errorf("connection is nil")
	}
	if conn.HTTP == nil {
		return fmt.Errorf("HTTP client is nil")
	}
	return nil
}

// CloseConnection closes a connection (no-op for HTTP clients)
func (f *DefaultConnectionFactory) CloseConnection(conn *Client) error {
	// HTTP clients don't need explicit closing, but we could close idle connections here
	if conn != nil && conn.HTTP != nil {
		conn.HTTP.CloseIdleConnections()
	}
	return nil
}

// NewConnectionPool creates a new connection pool with the given configuration
func NewConnectionPool(config PoolConfig) (*ConnectionPool, error) {
	// Set default values (only when not explicitly set)
	if config.MaxIdle == 0 {
		config.MaxIdle = 10
	}
	// Only set MinIdle default if MaxIdle was also defaulted (indicating user didn't configure pool sizing)
	if config.MinIdle == 0 && config.MaxActive == 0 {
		config.MinIdle = 2
	}
	if config.MaxActive == 0 {
		config.MaxActive = 100
	}
	if config.WarmupConnections == 0 {
		config.WarmupConnections = 1
	}
	if config.IdleTimeout == 0 {
		config.IdleTimeout = 30 * time.Minute
	}
	if config.SoftMaxLifetime == 0 {
		config.SoftMaxLifetime = 30 * time.Minute
	}
	if config.HardMaxLifetime == 0 {
		config.HardMaxLifetime = 60 * time.Minute
	}
	if config.ConnectTimeout == 0 {
		config.ConnectTimeout = 30 * time.Second
	}
	if config.MaxWait == 0 {
		config.MaxWait = 30 * time.Second
	}
	if config.ConnectionRequestTimeout == 0 {
		config.ConnectionRequestTimeout = 10 * time.Second
	}
	if config.HealthCheckInterval == 0 {
		config.HealthCheckInterval = 60 * time.Second
	}
	if config.HealthCheckTimeout == 0 {
		config.HealthCheckTimeout = 10 * time.Second
	}
	if config.MaxConsecutiveFailures == 0 {
		config.MaxConsecutiveFailures = 3
	}
	if config.CircuitBreakerCooldown == 0 {
		config.CircuitBreakerCooldown = 60 * time.Second
	}
	if config.MaxQueueSize == 0 {
		config.MaxQueueSize = 100
	}

	// Validate configuration
	if config.MinIdle > config.MaxIdle {
		return nil, fmt.Errorf("MinIdle (%d) cannot be greater than MaxIdle (%d)", config.MinIdle, config.MaxIdle)
	}
	if config.MaxIdle > config.MaxActive {
		return nil, fmt.Errorf("MaxIdle (%d) cannot be greater than MaxActive (%d)", config.MaxIdle, config.MaxActive)
	}
	if config.SoftMaxLifetime > config.HardMaxLifetime {
		return nil, fmt.Errorf("SoftMaxLifetime (%v) cannot be greater than HardMaxLifetime (%v)", config.SoftMaxLifetime, config.HardMaxLifetime)
	}

	healthChecker := NewHealthChecker(config)
	
	pool := &ConnectionPool{
		idleConns:    make(chan *PooledConnection, config.MaxIdle),
		activeConns:  make(map[*PooledConnection]bool),
		waitingQueue: make(chan chan *PooledConnection, config.MaxQueueSize),
		config:       config,
		metrics:      &PoolMetrics{},
		factory:      &DefaultConnectionFactory{},
		health:       healthChecker,
		circuitState: int32(CircuitClosed),
		stopCleanup:  make(chan bool),
		stopWarmup:   make(chan bool),
	}

	// Set pool reference in health checker for circuit breaker integration
	healthChecker.SetPool(pool)

	// Start background processes
	pool.startBackgroundProcesses()

	return pool, nil
}

// GetConnection retrieves a connection from the pool
func (p *ConnectionPool) GetConnection(ctx context.Context) (*PooledConnection, error) {
	start := time.Now()
	atomic.AddInt64(&p.totalRequests, 1)
	atomic.AddInt64(&p.metrics.TotalRequests, 1)

	// Check circuit breaker state using health checker
	circuitState := p.health.GetCircuitState()
	if circuitState == CircuitOpen {
		atomic.AddInt64(&p.metrics.RejectedRequests, 1)
		return nil, fmt.Errorf("circuit breaker open: connections rejected")
	}
	
	// Update pool's circuit state to match health checker
	atomic.StoreInt32(&p.circuitState, int32(circuitState))

	// Try to get an idle connection first
	select {
	case conn := <-p.idleConns:
		if p.isConnectionValid(ctx, conn) {
			p.activateConnection(conn)
			p.updateMetrics(start, true)
			return conn, nil
		}
		// Connection was invalid, close it and continue
		p.closeConnection(conn)
	default:
		// No idle connections available
	}

	// Check if we can create a new connection
	currentActive := atomic.LoadInt32(&p.active)
	if int(currentActive) < p.config.MaxActive {
		if conn, err := p.createNewConnection(ctx); err == nil {
			p.activateConnection(conn)
			p.updateMetrics(start, false)
			return conn, nil
		}
		// Failed to create connection, continue to queuing logic
	}

	// Pool is at capacity, decide whether to queue or fallback
	if !p.config.QueueRequestsWhenFull {
		if p.config.FallbackToDirectConnection {
			return p.createDirectConnection(ctx)
		}
		atomic.AddInt64(&p.metrics.RejectedRequests, 1)
		return nil, fmt.Errorf("connection pool exhausted and queuing disabled")
	}

	// Queue the request
	return p.queueConnectionRequest(ctx, start)
}

// ReturnConnection returns a connection to the pool
func (p *ConnectionPool) ReturnConnection(conn *PooledConnection) error {
	if conn == nil {
		return fmt.Errorf("cannot return nil connection")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Remove from active connections
	if !p.activeConns[conn] {
		// Connection was not in active set, might be a direct connection
		return p.factory.CloseConnection(conn.Client)
	}
	delete(p.activeConns, conn)
	atomic.AddInt32(&p.active, -1)

	// Update usage statistics
	conn.lastUsedAt = time.Now()
	conn.usageCount++

	// Check if connection should be retired
	if p.shouldRetireConnection(conn) {
		return p.closeConnection(conn)
	}

	// Validate connection if required
	if p.config.ValidateOnReturn {
		if err := p.factory.ValidateConnection(context.Background(), conn.Client); err != nil {
			atomic.AddInt64(&p.metrics.ValidationFailures, 1)
			return p.closeConnection(conn)
		}
	}

	// Try to return to idle pool or serve waiting request
	select {
	case p.idleConns <- conn:
		// Successfully returned to idle pool
		return nil
	default:
		// Idle pool is full, try to serve waiting request
		select {
		case waitingChan := <-p.waitingQueue:
			// Serve waiting request (lock already held)
			p.activateConnectionLocked(conn)
			waitingChan <- conn
			return nil
		default:
			// No waiting requests, close the connection
			return p.closeConnection(conn)
		}
	}
}

// Close shuts down the connection pool and closes all connections
func (p *ConnectionPool) Close() error {
	// Set closed flag first
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()

	// Stop background processes first
	close(p.stopCleanup)
	close(p.stopWarmup)

	// Wait for all background goroutines to finish (without holding locks)
	p.wg.Wait()

	// Now it's safe to close channels since no goroutines are running
	p.mu.Lock()
	defer p.mu.Unlock()

	// Close all idle connections
	close(p.idleConns)
	for conn := range p.idleConns {
		p.factory.CloseConnection(conn.Client)
		atomic.AddInt64(&p.totalClosed, 1)
		atomic.AddInt64(&p.metrics.ClosedConnections, 1)
	}

	// Close all active connections
	for conn := range p.activeConns {
		p.factory.CloseConnection(conn.Client)
		atomic.AddInt64(&p.totalClosed, 1)
		atomic.AddInt64(&p.metrics.ClosedConnections, 1)
	}

	// Close waiting queue
	close(p.waitingQueue)
	for waitingChan := range p.waitingQueue {
		close(waitingChan)
	}

	return nil
}

// GetMetrics returns current pool metrics
func (p *ConnectionPool) GetMetrics() PoolMetrics {
	p.mu.RLock()
	defer p.mu.RUnlock()

	metrics := *p.metrics
	metrics.ActiveConnections = int64(atomic.LoadInt32(&p.active))
	metrics.IdleConnections = int64(len(p.idleConns))
	metrics.TotalConnections = metrics.ActiveConnections + metrics.IdleConnections
	metrics.CreatedConnections = atomic.LoadInt64(&p.totalCreated)
	metrics.ClosedConnections = atomic.LoadInt64(&p.totalClosed)
	
	// Read atomically updated fields to avoid race conditions
	metrics.HealthCheckSuccesses = atomic.LoadInt64(&p.metrics.HealthCheckSuccesses)
	metrics.HealthCheckFailures = atomic.LoadInt64(&p.metrics.HealthCheckFailures)
	metrics.CircuitBreakerTrips = atomic.LoadInt64(&p.metrics.CircuitBreakerTrips)
	metrics.ValidationFailures = atomic.LoadInt64(&p.metrics.ValidationFailures)

	// Calculate averages
	if metrics.requestCount > 0 {
		metrics.AverageWaitTime = metrics.totalWaitTime / time.Duration(metrics.requestCount)
		metrics.AverageResponseTime = metrics.totalRequestTime / time.Duration(metrics.requestCount)
	}

	// Calculate pool utilization
	if p.config.MaxActive > 0 {
		metrics.AveragePoolUtilization = float64(metrics.ActiveConnections) / float64(p.config.MaxActive)
	}

	// Calculate connection age
	now := time.Now()
	var totalAge time.Duration
	var connectionCount int64

	for conn := range p.activeConns {
		totalAge += now.Sub(conn.createdAt)
		connectionCount++
	}

	// Include idle connections in age calculation
	idleConnsCopy := make([]*PooledConnection, 0, len(p.idleConns))
drainLoop:
	for i := 0; i < len(p.idleConns); i++ {
		select {
		case conn := <-p.idleConns:
			totalAge += now.Sub(conn.createdAt)
			connectionCount++
			idleConnsCopy = append(idleConnsCopy, conn)
		default:
			// No more connections available, exit the loop
			break drainLoop
		}
	}

	// Put idle connections back
	for _, conn := range idleConnsCopy {
		select {
		case p.idleConns <- conn:
		default:
			// Should not happen, but close if we can't return it
			p.factory.CloseConnection(conn.Client)
		}
	}

	if connectionCount > 0 {
		metrics.AverageConnectionAge = totalAge / time.Duration(connectionCount)
	}

	return metrics
}

// Private helper methods

func (p *ConnectionPool) activateConnection(conn *PooledConnection) {
	p.mu.Lock()
	p.activateConnectionLocked(conn)
	p.mu.Unlock()
}

// activateConnectionLocked activates a connection with the lock already held
func (p *ConnectionPool) activateConnectionLocked(conn *PooledConnection) {
	p.activeConns[conn] = true
	atomic.AddInt32(&p.active, 1)
}

func (p *ConnectionPool) createNewConnection(ctx context.Context) (*PooledConnection, error) {
	createCtx, cancel := context.WithTimeout(ctx, p.config.ConnectTimeout)
	defer cancel()

	// Access factory with proper synchronization
	p.mu.RLock()
	factory := p.factory
	p.mu.RUnlock()

	client, err := factory.CreateConnection(createCtx)
	if err != nil {
		atomic.AddInt64(&p.metrics.ConnectionErrors, 1)
		return nil, fmt.Errorf("failed to create connection: %w", err)
	}

	conn := &PooledConnection{
		Client:     client,
		createdAt:  time.Now(),
		lastUsedAt: time.Now(),
		usageCount: 0,
		poolRef:    p,
		id:         fmt.Sprintf("conn-%d", atomic.AddInt64(&p.totalCreated, 1)),
	}

	atomic.AddInt64(&p.metrics.CreatedConnections, 1)
	return conn, nil
}

func (p *ConnectionPool) createDirectConnection(ctx context.Context) (*PooledConnection, error) {
	// Access factory with proper synchronization
	p.mu.RLock()
	factory := p.factory
	p.mu.RUnlock()

	client, err := factory.CreateConnection(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create direct connection: %w", err)
	}

	atomic.AddInt64(&p.metrics.FallbackConnections, 1)
	return &PooledConnection{
		Client:     client,
		createdAt:  time.Now(),
		lastUsedAt: time.Now(),
		usageCount: 0,
		poolRef:    nil, // Not managed by pool
		id:         fmt.Sprintf("direct-conn-%d", time.Now().UnixNano()),
	}, nil
}

func (p *ConnectionPool) queueConnectionRequest(ctx context.Context, startTime time.Time) (*PooledConnection, error) {
	requestChan := make(chan *PooledConnection, 1)
	
	select {
	case p.waitingQueue <- requestChan:
		atomic.AddInt64(&p.metrics.QueuedRequests, 1)
	default:
		// Queue is full
		if p.config.FallbackToDirectConnection {
			return p.createDirectConnection(ctx)
		}
		atomic.AddInt64(&p.metrics.RejectedRequests, 1)
		return nil, fmt.Errorf("connection queue full and fallback disabled")
	}

	// Wait for connection or timeout
	timeout := p.config.ConnectionRequestTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining < timeout {
			timeout = remaining
		}
	}

	select {
	case conn := <-requestChan:
		p.updateMetrics(startTime, false)
		return conn, nil
	case <-time.After(timeout):
		atomic.AddInt64(&p.metrics.TimeoutRequests, 1)
		return nil, fmt.Errorf("timeout waiting for connection after %v", timeout)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (p *ConnectionPool) isConnectionValid(ctx context.Context, conn *PooledConnection) bool {
	if conn == nil {
		return false
	}

	// Check age limits
	now := time.Now()
	if now.Sub(conn.createdAt) > p.config.HardMaxLifetime {
		atomic.AddInt64(&p.metrics.ExpiredConnections, 1)
		return false
	}

	// Check idle timeout
	if now.Sub(conn.lastUsedAt) > p.config.IdleTimeout {
		atomic.AddInt64(&p.metrics.ExpiredConnections, 1)
		return false
	}

	// Validate connection if required
	if p.config.ValidateOnGet {
		if err := p.factory.ValidateConnection(ctx, conn.Client); err != nil {
			atomic.AddInt64(&p.metrics.ValidationFailures, 1)
			return false
		}
	}

	return true
}

func (p *ConnectionPool) shouldRetireConnection(conn *PooledConnection) bool {
	now := time.Now()
	age := now.Sub(conn.createdAt)

	// Hard limit - always retire
	if age > p.config.HardMaxLifetime {
		atomic.AddInt64(&p.metrics.ExpiredConnections, 1)
		return true
	}

	// Soft limit - retire if we have enough connections
	if age > p.config.SoftMaxLifetime {
		idleCount := len(p.idleConns)
		if idleCount >= p.config.MinIdle {
			atomic.AddInt64(&p.metrics.ReplacedConnections, 1)
			return true
		}
	}

	return false
}

func (p *ConnectionPool) closeConnection(conn *PooledConnection) error {
	err := p.factory.CloseConnection(conn.Client)
	atomic.AddInt64(&p.totalClosed, 1)
	atomic.AddInt64(&p.metrics.ClosedConnections, 1)
	return err
}

func (p *ConnectionPool) updateMetrics(startTime time.Time, wasIdle bool) {
	elapsed := time.Since(startTime)
	
	// Update timing metrics with proper synchronization
	p.mu.Lock()
	p.metrics.requestCount++
	p.metrics.totalWaitTime += elapsed
	p.metrics.totalRequestTime += elapsed

	// Track idle vs new connection performance
	if wasIdle {
		p.metrics.IdleConnectionHits++
	} else {
		p.metrics.NewConnectionCreations++
	}

	if elapsed < p.metrics.FastestResponseTime || p.metrics.FastestResponseTime == 0 {
		p.metrics.FastestResponseTime = elapsed
	}
	if elapsed > p.metrics.SlowestResponseTime {
		p.metrics.SlowestResponseTime = elapsed
	}

	// Update concurrent request tracking while holding the lock
	currentActive := atomic.LoadInt32(&p.active)
	if int64(currentActive) > p.metrics.MaxConcurrentRequests {
		p.metrics.MaxConcurrentRequests = int64(currentActive)
	}
	p.mu.Unlock()
}

func (p *ConnectionPool) startBackgroundProcesses() {
	// Start connection cleanup routine
	p.wg.Add(1)
	go p.cleanupRoutine()

	// Start warmup routine
	p.wg.Add(1)
	go p.warmupRoutine()

	// Start health check routine
	p.wg.Add(1)
	go p.healthRoutine()
}

func (p *ConnectionPool) cleanupRoutine() {
	defer p.wg.Done()
	ticker := time.NewTicker(p.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.performCleanup()
		case <-p.stopCleanup:
			return
		}
	}
}

func (p *ConnectionPool) healthRoutine() {
	defer p.wg.Done()
	p.health.Start(p.stopCleanup)
}

func (p *ConnectionPool) warmupRoutine() {
	defer p.wg.Done()
	
	// Create warmup connections
	ctx := context.Background()
	for i := 0; i < p.config.WarmupConnections; i++ {
		// Check if we should stop before creating connections
		p.mu.RLock()
		closed := p.closed
		p.mu.RUnlock()
		if closed {
			return
		}

		if conn, err := p.createNewConnection(ctx); err == nil {
			// Check again before sending to channel
			p.mu.RLock()
			closed := p.closed
			p.mu.RUnlock()
			if closed {
				p.factory.CloseConnection(conn.Client)
				return
			}

			select {
			case p.idleConns <- conn:
				// Successfully added to idle pool
			default:
				// Idle pool is full, close the connection
				p.factory.CloseConnection(conn.Client)
			}
		}
	}

	// Maintain minimum idle connections
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.maintainMinimumIdle()
		case <-p.stopWarmup:
			return
		}
	}
}

func (p *ConnectionPool) performCleanup() {
	// Clean up expired idle connections
	var validConns []*PooledConnection
	
	// Drain idle connections and check validity
drainLoop:
	for {
		select {
		case conn := <-p.idleConns:
			if p.isConnectionValid(context.Background(), conn) && !p.shouldRetireConnection(conn) {
				validConns = append(validConns, conn)
			} else {
				p.closeConnection(conn)
			}
		default:
			// No more idle connections
			break drainLoop
		}
	}
	// Put valid connections back
	for _, conn := range validConns {
		select {
		case p.idleConns <- conn:
			// Successfully returned
		default:
			// Pool is full, close excess connections
			p.closeConnection(conn)
		}
	}
}

func (p *ConnectionPool) maintainMinimumIdle() {
	// Check if pool is closed
	p.mu.RLock()
	closed := p.closed
	p.mu.RUnlock()
	if closed {
		return
	}

	currentIdle := len(p.idleConns)
	if currentIdle < p.config.MinIdle {
		needed := p.config.MinIdle - currentIdle
		ctx := context.Background()
		
		for i := 0; i < needed; i++ {
			// Check if closed before creating connection
			p.mu.RLock()
			closed := p.closed
			p.mu.RUnlock()
			if closed {
				return
			}

			if conn, err := p.createNewConnection(ctx); err == nil {
				// Check if closed before sending to channel
				p.mu.RLock()
				closed := p.closed
				p.mu.RUnlock()
				if closed {
					p.factory.CloseConnection(conn.Client)
					return
				}

				select {
				case p.idleConns <- conn:
					// Successfully added
				default:
					// Pool is full, close and stop
					p.factory.CloseConnection(conn.Client)
					return
				}
			} else {
				// Failed to create connection, stop trying
				return
			}
		}
	}
}
