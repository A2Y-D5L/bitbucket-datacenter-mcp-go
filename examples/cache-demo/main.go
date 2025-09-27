package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/a2y-d5l/bitbucket-datacenter-mcp-go/mcpserver"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	ctx := context.Background()

	// Start MCP server with caching enabled
	_, session, stop, err := mcpserver.Start(ctx, &mcpserver.ServerOptions{
		EnableCache: true,
		Cache: &mcpserver.CacheConfig{
			TTL:               5 * time.Minute,  // 5 minute TTL
			MaxEntries:        500,              // Limit to 500 entries
			MaxMemoryBytes:    50 * 1024 * 1024, // 50MB max memory
			MaxEntrySizeBytes: 5 * 1024 * 1024,  // 5MB max per entry
			CleanupInterval:   30 * time.Second, // Clean every 30 seconds
			EnableMetrics:     true,
			EnableCompression: false, // Disable for this example
		},
	})
	if err != nil {
		log.Fatalf("Failed to start MCP server: %v", err)
	}
	defer stop()

	// Example: List PRs multiple times to demonstrate caching
	projectKey := "PROJ"
	repoSlug := "my-repo"

	fmt.Println("=== Cache Demo: Multiple PR List Calls ===")

	for i := range 3 {
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
			log.Printf("Call %d failed: %v", i+1, err)
		} else {
			fmt.Printf("Call %d completed in %v (should be cached after first call)\n",
				i+1, elapsed)

			// Print a summary of the result
			if len(result.Content) > 0 {
				if textContent, ok := result.Content[0].(*mcp.TextContent); ok {
					fmt.Printf("  Result length: %d characters\n", len(textContent.Text))
				}
			}
		}

		// Small delay between calls
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Println("\n=== Cache Demo: Different Parameters (Cache Miss) ===")

	// Call with different parameters - should be cache miss
	start := time.Now()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "bitbucket.data-center.pr.list",
		Arguments: map[string]any{
			"projectKey": projectKey,
			"repoSlug":   repoSlug,
			"state":      "MERGED", // Different state
			"start":      0,
			"limit":      10,
		},
	})
	elapsed := time.Since(start)

	if err != nil {
		log.Printf("Different parameters call failed: %v", err)
	} else {
		fmt.Printf("Different parameters call completed in %v (cache miss expected)\n", elapsed)
		if len(result.Content) > 0 {
			if textContent, ok := result.Content[0].(*mcp.TextContent); ok {
				fmt.Printf("  Result length: %d characters\n", len(textContent.Text))
			}
		}
	}

	fmt.Println("\n=== Cache Statistics ===")
	// Note: In a real implementation, you would expose cache stats through an admin interface
	// For this demo, the cache statistics are logged internally
	fmt.Println("Cache statistics are logged internally by the cache manager.")
	fmt.Println("Check the logs for hit/miss ratios and memory usage.")

	fmt.Println("\n=== Cache Demo Complete ===")
}
