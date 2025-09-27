package bitbucketdatacenter

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// HealthChecker monitors the health of connections and implements circuit breaker pattern
type HealthChecker struct {
	// Health check configuration
	endpoint string        // "/status" or lightweight endpoint for health checks
	interval time.Duration // How often to check
	timeout  time.Duration // Timeout for each check

	// Circuit breaker logic
	consecutiveFailures int32         // Current consecutive failures (atomic)
	maxFailures         int           // Circuit breaker threshold
	circuitOpenTime     time.Time     // When circuit was opened
	cooldownPeriod      time.Duration // Time before allowing retry

	// Custom health validation
	customHealthFunc func(*Client) error // Allow custom health checks
	validateTLS      bool                 // Validate TLS certificates

	// Backoff strategy for failed checks
	backoffStrategy BackoffStrategy // Exponential backoff configuration

	// Metrics
	totalChecks       int64         // Total health checks performed (atomic)
	failedChecks      int64         // Total failed health checks (atomic)
	lastCheckTime     time.Time     // Last health check timestamp
	lastCheckDuration time.Duration // Duration of last health check

	// Concurrency
	mu sync.RWMutex

	// Connection pool reference
	pool   *ConnectionPool
	client *Client // Test client for health checks
}

// BackoffStrategy defines exponential backoff configuration
type BackoffStrategy struct {
	InitialDelay time.Duration `default:"1s"`
	MaxDelay     time.Duration `default:"60s"`
	Multiplier   float64       `default:"2.0"`
	Jitter       float64       `default:"0.1"` // Add randomness to prevent thundering herd
}

// NewHealthChecker creates a new health checker with the given configuration
func NewHealthChecker(config PoolConfig) *HealthChecker {
	backoffStrategy := BackoffStrategy{
		InitialDelay: 1 * time.Second,
		MaxDelay:     60 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.1,
	}

	hc := &HealthChecker{
		endpoint:        "/status", // Default health check endpoint
		interval:        config.HealthCheckInterval,
		timeout:         config.HealthCheckTimeout,
		maxFailures:     config.MaxConsecutiveFailures,
		cooldownPeriod:  config.CircuitBreakerCooldown,
		validateTLS:     true,
		backoffStrategy: backoffStrategy,
	}

	return hc
}

// Start begins the health checking routine
func (hc *HealthChecker) Start(stopChan <-chan bool) {
	// Create a test client for health checks
	if client, err := NewClient(); err == nil {
		hc.client = client
	}

	ticker := time.NewTicker(hc.interval)
	defer ticker.Stop()

	// Perform initial health check
	hc.performHealthCheck()

	for {
		select {
		case <-ticker.C:
			hc.performHealthCheck()
		case <-stopChan:
			return
		}
	}
}

// IsHealthy returns whether the service is currently healthy
func (hc *HealthChecker) IsHealthy() bool {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	// If we have a custom health function, use it
	if hc.customHealthFunc != nil && hc.client != nil {
		return hc.customHealthFunc(hc.client) == nil
	}

	// Check consecutive failures
	failures := atomic.LoadInt32(&hc.consecutiveFailures)
	return int(failures) < hc.maxFailures
}

// GetCircuitState returns the current circuit breaker state
func (hc *HealthChecker) GetCircuitState() CircuitState {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	failures := atomic.LoadInt32(&hc.consecutiveFailures)
	
	if int(failures) >= hc.maxFailures {
		// Check if cooldown period has passed
		if time.Since(hc.circuitOpenTime) >= hc.cooldownPeriod {
			return CircuitHalfOpen
		}
		return CircuitOpen
	}
	
	return CircuitClosed
}

// RecordSuccess records a successful operation
func (hc *HealthChecker) RecordSuccess() {
	atomic.StoreInt32(&hc.consecutiveFailures, 0)
	
	// Update pool circuit state if available
	if hc.pool != nil {
		atomic.StoreInt32(&hc.pool.circuitState, int32(CircuitClosed))
	}
}

// RecordFailure records a failed operation
func (hc *HealthChecker) RecordFailure() {
	failures := atomic.AddInt32(&hc.consecutiveFailures, 1)
	
	if int(failures) >= hc.maxFailures {
		hc.mu.Lock()
		hc.circuitOpenTime = time.Now()
		hc.mu.Unlock()
		
		// Update pool circuit state if available
		if hc.pool != nil {
			atomic.StoreInt32(&hc.pool.circuitState, int32(CircuitOpen))
			atomic.AddInt64(&hc.pool.metrics.CircuitBreakerTrips, 1)
		}
	}
}

// GetHealthMetrics returns current health check metrics
func (hc *HealthChecker) GetHealthMetrics() HealthMetrics {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	totalChecks := atomic.LoadInt64(&hc.totalChecks)
	failedChecks := atomic.LoadInt64(&hc.failedChecks)
	
	var successRate float64
	if totalChecks > 0 {
		successRate = float64(totalChecks-failedChecks) / float64(totalChecks)
	}

	return HealthMetrics{
		TotalChecks:         totalChecks,
		SuccessfulChecks:    totalChecks - failedChecks,
		FailedChecks:        failedChecks,
		ConsecutiveFailures: int(atomic.LoadInt32(&hc.consecutiveFailures)),
		SuccessRate:         successRate,
		LastCheckTime:       hc.lastCheckTime,
		LastCheckDuration:   hc.lastCheckDuration,
		CircuitState:        hc.GetCircuitState(),
	}
}

// HealthMetrics provides health check statistics
type HealthMetrics struct {
	TotalChecks         int64         `json:"totalChecks"`
	SuccessfulChecks    int64         `json:"successfulChecks"`
	FailedChecks        int64         `json:"failedChecks"`
	ConsecutiveFailures int           `json:"consecutiveFailures"`
	SuccessRate         float64       `json:"successRate"`
	LastCheckTime       time.Time     `json:"lastCheckTime"`
	LastCheckDuration   time.Duration `json:"lastCheckDuration"`
	CircuitState        CircuitState  `json:"circuitState"`
}

// SetPool sets the connection pool reference for circuit breaker integration
func (hc *HealthChecker) SetPool(pool *ConnectionPool) {
	hc.pool = pool
}

// SetCustomHealthFunc sets a custom health check function
func (hc *HealthChecker) SetCustomHealthFunc(fn func(*Client) error) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.customHealthFunc = fn
}

// performHealthCheck executes a health check
func (hc *HealthChecker) performHealthCheck() {
	start := time.Now()
	atomic.AddInt64(&hc.totalChecks, 1)

	ctx, cancel := context.WithTimeout(context.Background(), hc.timeout)
	defer cancel()

	var err error
	if hc.customHealthFunc != nil && hc.client != nil {
		err = hc.customHealthFunc(hc.client)
	} else {
		err = hc.defaultHealthCheck(ctx)
	}

	duration := time.Since(start)

	hc.mu.Lock()
	hc.lastCheckTime = start
	hc.lastCheckDuration = duration
	hc.mu.Unlock()

	if err != nil {
		atomic.AddInt64(&hc.failedChecks, 1)
		hc.RecordFailure()
		
		// Apply backoff for next check if configured
		hc.applyBackoff()
	} else {
		hc.RecordSuccess()
	}

	// Update pool metrics if available
	if hc.pool != nil {
		if err != nil {
			atomic.AddInt64(&hc.pool.metrics.HealthCheckFailures, 1)
		} else {
			atomic.AddInt64(&hc.pool.metrics.HealthCheckSuccesses, 1)
		}
	}
}

// defaultHealthCheck performs a basic health check
func (hc *HealthChecker) defaultHealthCheck(ctx context.Context) error {
	if hc.client == nil {
		return fmt.Errorf("no health check client available")
	}

	// Basic validation - ensure HTTP client is available
	if hc.client.HTTP == nil {
		return fmt.Errorf("HTTP client is not available")
	}

	// For Bitbucket Data Center, we could implement a lightweight health check
	// For now, we'll do basic client validation
	if hc.client.BaseURL == "" {
		return fmt.Errorf("base URL not configured")
	}

	// Validate TLS if required
	if hc.validateTLS {
		// Could implement TLS certificate validation here
		// For now, assume valid if we got this far
	}

	return nil
}

// applyBackoff applies exponential backoff with jitter
func (hc *HealthChecker) applyBackoff() {
	failures := atomic.LoadInt32(&hc.consecutiveFailures)
	if failures <= 1 {
		return
	}

	// Calculate delay with exponential backoff
	delay := time.Duration(float64(hc.backoffStrategy.InitialDelay) * math.Pow(hc.backoffStrategy.Multiplier, float64(failures-1)))
	
	// Cap at maximum delay
	if delay > hc.backoffStrategy.MaxDelay {
		delay = hc.backoffStrategy.MaxDelay
	}

	// Add jitter to prevent thundering herd
	if hc.backoffStrategy.Jitter > 0 {
		jitterAmount := float64(delay) * hc.backoffStrategy.Jitter
		jitter := time.Duration(rand.Float64() * jitterAmount * 2 - jitterAmount)
		delay += jitter
	}

	// Ensure minimum delay
	if delay < hc.backoffStrategy.InitialDelay {
		delay = hc.backoffStrategy.InitialDelay
	}

	// Sleep for the calculated delay
	time.Sleep(delay)
}

// ValidatePoolConnection validates a connection from the pool perspective
func (hc *HealthChecker) ValidatePoolConnection(ctx context.Context, conn *PooledConnection) error {
	if conn == nil || conn.Client == nil {
		return fmt.Errorf("invalid connection")
	}

	// Check connection age
	if time.Since(conn.createdAt) > time.Hour*24 {
		return fmt.Errorf("connection too old")
	}

	// If we have a custom health function, use it
	if hc.customHealthFunc != nil {
		return hc.customHealthFunc(conn.Client)
	}

	// Basic validation
	if conn.Client.HTTP == nil {
		return fmt.Errorf("HTTP client is nil")
	}

	if conn.Client.BaseURL == "" {
		return fmt.Errorf("base URL not configured")
	}

	return nil
}

// CreateHealthyConnection attempts to create a healthy connection
func (hc *HealthChecker) CreateHealthyConnection(ctx context.Context, factory ConnectionFactory) (*Client, error) {
	// Check circuit breaker state
	state := hc.GetCircuitState()
	if state == CircuitOpen {
		return nil, fmt.Errorf("circuit breaker open: not creating connections")
	}

	// Create the connection
	client, err := factory.CreateConnection(ctx)
	if err != nil {
		hc.RecordFailure()
		return nil, fmt.Errorf("failed to create connection: %w", err)
	}

	// Validate the new connection
	if hc.customHealthFunc != nil {
		if err := hc.customHealthFunc(client); err != nil {
			hc.RecordFailure()
			factory.CloseConnection(client)
			return nil, fmt.Errorf("new connection failed health check: %w", err)
		}
	}

	hc.RecordSuccess()
	return client, nil
}

// MonitorConnection monitors a connection's health in the background
func (hc *HealthChecker) MonitorConnection(conn *PooledConnection, stopChan <-chan bool) {
	if conn == nil {
		return
	}

	ticker := time.NewTicker(hc.interval * 2) // Less frequent than main health checks
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := hc.ValidatePoolConnection(context.Background(), conn); err != nil {
				// Connection is unhealthy, mark for replacement
				if hc.pool != nil {
					// Could implement connection marking for replacement
					// For now, just record the validation failure
					atomic.AddInt64(&hc.pool.metrics.ValidationFailures, 1)
				}
			}
		case <-stopChan:
			return
		}
	}
}
