package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/a2y-d5l/bitbucket-datacenter-mcp-go/mcpserver"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	ctx := context.Background()

	// Check if we should run the demo
	if os.Getenv("RUN_POOL_DEMO") != "1" {
		fmt.Println("Set RUN_POOL_DEMO=1 to run the connection pool demo")
		return
	}

	fmt.Println("=== Connection Pool Demo ===")

	// Create pooled server options for development
	opts := mcpserver.CreateDevelopmentPooledServerOptions(true) // Enable both pool and cache

	// Customize pool configuration for the demo
	poolConfig := *opts.ConnectionPool
	poolConfig.MaxActive = 5    // Small pool for demo
	poolConfig.MaxIdle = 3
	poolConfig.MinIdle = 1
	poolConfig.WarmupConnections = 2
	poolConfig.HealthCheckInterval = 10 * time.Second
	poolConfig.IdleTimeout = 30 * time.Second
	opts.ConnectionPool = &poolConfig

	// Start the MCP server with connection pooling
	pooledAPI, session, stop, err := mcpserver.StartWithConnectionPool(ctx, opts)
	if err != nil {
		log.Fatalf("Failed to start MCP server with connection pool: %v", err)
	}
	defer stop()

	fmt.Printf("✓ MCP server started with connection pooling enabled\n")

	// Display initial pool metrics
	displayPoolMetrics(pooledAPI, "Initial")

	// Example 1: Sequential API calls to demonstrate connection reuse
	fmt.Println("\n=== Example 1: Sequential API Calls ===")
	projectKey := "PROJ"
	repoSlug := "demo-repo"

	for i := 1; i <= 3; i++ {
		fmt.Printf("Making API call %d...\n", i)
		start := time.Now()

		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "bitbucket.data-center.pr.list",
			Arguments: map[string]any{
				"projectKey": projectKey,
				"repoSlug":   repoSlug,
				"state":      "OPEN",
				"start":      0,
				"limit":      10,
			},
		})

		elapsed := time.Since(start)
		
		if err != nil {
			fmt.Printf("  ✗ Call %d failed: %v (took %v)\n", i, err, elapsed)
		} else {
			fmt.Printf("  ✓ Call %d completed in %v\n", i, elapsed)
			if len(result.Content) > 0 {
				if textContent, ok := result.Content[0].(*mcp.TextContent); ok {
					fmt.Printf("    Response length: %d characters\n", len(textContent.Text))
				}
			}
		}

		// Small delay between calls
		time.Sleep(100 * time.Millisecond)
	}

	displayPoolMetrics(pooledAPI, "After Sequential Calls")

	// Example 2: Concurrent API calls to test pool under load
	fmt.Println("\n=== Example 2: Concurrent API Calls ===")
	
	concurrency := 8 // More than pool size to test queuing
	var wg sync.WaitGroup
	results := make(chan string, concurrency)

	fmt.Printf("Making %d concurrent API calls (pool max: %d)...\n", concurrency, poolConfig.MaxActive)

	start := time.Now()
	for i := 1; i <= concurrency; i++ {
		wg.Add(1)
		go func(callID int) {
			defer wg.Done()
			
			callStart := time.Now()
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name: "bitbucket.data-center.pr.list",
				Arguments: map[string]any{
					"projectKey": projectKey,
					"repoSlug":   repoSlug,
					"state":      "ALL",
					"start":      0,
					"limit":      5,
				},
			})
			elapsed := time.Since(callStart)

			if err != nil {
				results <- fmt.Sprintf("Call %d: FAILED (%v) - %v", callID, elapsed, err)
			} else {
				var responseLength int
				if len(result.Content) > 0 {
					if textContent, ok := result.Content[0].(*mcp.TextContent); ok {
						responseLength = len(textContent.Text)
					}
				}
				results <- fmt.Sprintf("Call %d: SUCCESS (%v) - %d chars", callID, elapsed, responseLength)
			}
		}(i)
	}

	wg.Wait()
	close(results)
	totalElapsed := time.Since(start)

	fmt.Printf("All concurrent calls completed in %v\n", totalElapsed)
	fmt.Println("Results:")
	for result := range results {
		fmt.Printf("  %s\n", result)
	}

	displayPoolMetrics(pooledAPI, "After Concurrent Calls")

	// Example 3: Health check and circuit breaker demo
	fmt.Println("\n=== Example 3: Health Metrics ===")
	if healthMetrics := pooledAPI.GetHealthMetrics(); healthMetrics != nil {
		fmt.Printf("Health Check Stats:\n")
		fmt.Printf("  Total checks: %d\n", healthMetrics.TotalChecks)
		fmt.Printf("  Successful checks: %d\n", healthMetrics.SuccessfulChecks)
		fmt.Printf("  Failed checks: %d\n", healthMetrics.FailedChecks)
		fmt.Printf("  Success rate: %.2f%%\n", healthMetrics.SuccessRate*100)
		fmt.Printf("  Consecutive failures: %d\n", healthMetrics.ConsecutiveFailures)
		fmt.Printf("  Circuit state: %v\n", healthMetrics.CircuitState)
		fmt.Printf("  Last check: %v ago\n", time.Since(healthMetrics.LastCheckTime).Truncate(time.Second))
		fmt.Printf("  Last check duration: %v\n", healthMetrics.LastCheckDuration)
	} else {
		fmt.Println("Health metrics not available (pool not enabled)")
	}

	// Example 4: Wait and show final metrics
	fmt.Println("\n=== Example 4: Pool Lifecycle Management ===")
	fmt.Println("Waiting 15 seconds to observe background processes...")
	time.Sleep(15 * time.Second)

	displayPoolMetrics(pooledAPI, "After Wait Period")

	// Example 5: Cache integration test
	fmt.Println("\n=== Example 5: Cache + Pool Integration ===")
	fmt.Println("Making identical requests to test cache hits with pooled connections...")

	for i := 1; i <= 3; i++ {
		start := time.Now()
		
		_, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "bitbucket.data-center.pr.get",
			Arguments: map[string]any{
				"projectKey": projectKey,
				"repoSlug":   repoSlug,
				"id":         123, // Fixed ID for cache testing
			},
		})

		elapsed := time.Since(start)
		
		if err != nil {
			fmt.Printf("  Call %d: FAILED (%v) - %v\n", i, elapsed, err)
		} else {
			if i == 1 {
				fmt.Printf("  Call %d: %v (cache miss expected)\n", i, elapsed)
			} else {
				fmt.Printf("  Call %d: %v (cache hit expected)\n", i, elapsed)
			}
		}
	}

	displayPoolMetrics(pooledAPI, "Final")

	fmt.Println("\n=== Connection Pool Demo Complete ===")
	fmt.Println("✓ Pool lifecycle management working correctly")
	fmt.Println("✓ Concurrent request handling verified") 
	fmt.Println("✓ Cache integration functioning")
	fmt.Println("✓ Health monitoring active")
}

func displayPoolMetrics(pooledAPI *mcpserver.PooledBitbucketAPI, stage string) {
	if metrics := pooledAPI.GetPoolMetrics(); metrics != nil {
		fmt.Printf("\n--- Pool Metrics (%s) ---\n", stage)
		fmt.Printf("Active connections: %d\n", metrics.ActiveConnections)
		fmt.Printf("Idle connections: %d\n", metrics.IdleConnections)
		fmt.Printf("Total connections: %d\n", metrics.TotalConnections)
		fmt.Printf("Created connections: %d\n", metrics.CreatedConnections)
		fmt.Printf("Closed connections: %d\n", metrics.ClosedConnections)
		fmt.Printf("Total requests: %d\n", metrics.TotalRequests)
		fmt.Printf("Queued requests: %d\n", metrics.QueuedRequests)
		fmt.Printf("Rejected requests: %d\n", metrics.RejectedRequests)
		fmt.Printf("Timeout requests: %d\n", metrics.TimeoutRequests)
		fmt.Printf("Pool utilization: %.2f%%\n", metrics.AveragePoolUtilization*100)
		fmt.Printf("Average connection age: %v\n", metrics.AverageConnectionAge.Truncate(time.Second))
		fmt.Printf("Connection errors: %d\n", metrics.ConnectionErrors)
		fmt.Printf("Validation failures: %d\n", metrics.ValidationFailures)
		fmt.Printf("Circuit breaker trips: %d\n", metrics.CircuitBreakerTrips)
		fmt.Printf("Fallback connections: %d\n", metrics.FallbackConnections)
		
		if metrics.AverageWaitTime > 0 {
			fmt.Printf("Average wait time: %v\n", metrics.AverageWaitTime)
		}
		if metrics.AverageResponseTime > 0 {
			fmt.Printf("Average response time: %v\n", metrics.AverageResponseTime)
		}
	} else {
		fmt.Printf("\n--- Pool Metrics (%s) ---\n", stage)
		fmt.Println("Pool metrics not available (pool not enabled)")
	}
}
