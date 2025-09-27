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

	_, session, stop, err := mcpserver.Start(ctx, &mcpserver.ServerOptions{
		EnableCache: true,
		Cache: &mcpserver.CacheConfig{
			TTL:               2 * time.Minute,  // Short TTL for demo
			MaxEntries:        100,
			MaxMemoryBytes:    10 * 1024 * 1024, // 10MB
			MaxEntrySizeBytes: 1 * 1024 * 1024,  // 1MB per entry
			CleanupInterval:   10 * time.Second, // Frequent cleanup
			EnableMetrics:     true,
			EnableCompression: true,
		},
	})
	if err != nil {
		log.Fatalf("Failed to start MCP server: %v", err)
	}
	defer stop()

	projectKey := "PROJ"
	repoSlug := "my-repo"
	prID := 123

	fmt.Println("=== Cache Invalidation Demo ===")

	// 1. Get PR details (will be cached)
	fmt.Println("\n1. Getting PR details (cache miss expected)...")
	start := time.Now()
	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "bitbucket.data-center.pr.get",
		Arguments: map[string]any{
			"projectKey": projectKey,
			"repoSlug":   repoSlug,
			"id":         prID,
		},
	})
	elapsed1 := time.Since(start)
	
	if err != nil {
		log.Printf("First PR get failed: %v", err)
	} else {
		fmt.Printf("First call completed in %v\n", elapsed1)
	}

	// 2. Get PR details again (should hit cache)
	fmt.Println("\n2. Getting PR details again (cache hit expected)...")
	start = time.Now()
	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "bitbucket.data-center.pr.get",
		Arguments: map[string]any{
			"projectKey": projectKey,
			"repoSlug":   repoSlug,
			"id":         prID,
		},
	})
	elapsed2 := time.Since(start)
	
	if err != nil {
		log.Printf("Second PR get failed: %v", err)
	} else {
		fmt.Printf("Second call completed in %v (should be much faster)\n", elapsed2)
	}

	// 3. Update PR (this should invalidate the cache)
	fmt.Println("\n3. Updating PR (this will invalidate cache)...")
	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "bitbucket.data-center.pr.update",
		Arguments: map[string]any{
			"projectKey":  projectKey,
			"repoSlug":    repoSlug,
			"id":          prID,
			"version":     1, // Assumes PR version is 1
			"title":       "Updated Title for Cache Demo",
			"description": "This update should invalidate the cache",
		},
	})
	
	if err != nil {
		log.Printf("PR update failed (expected if PR doesn't exist): %v", err)
		fmt.Println("This is expected in the demo - we're just showing the invalidation pattern")
	} else {
		fmt.Println("PR updated successfully - cache invalidated")
	}

	// 4. Get PR details again (should be cache miss due to invalidation)
	fmt.Println("\n4. Getting PR details after update (cache miss expected due to invalidation)...")
	start = time.Now()
	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "bitbucket.data-center.pr.get",
		Arguments: map[string]any{
			"projectKey": projectKey,
			"repoSlug":   repoSlug,
			"id":         prID,
		},
	})
	elapsed3 := time.Since(start)
	
	if err != nil {
		log.Printf("Third PR get failed: %v", err)
	} else {
		fmt.Printf("Third call completed in %v (should be slower again due to cache invalidation)\n", elapsed3)
	}

	// 5. Demonstrate diff caching
	fmt.Println("\n5. Getting PR diff (cache miss expected)...")
	start = time.Now()
	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "bitbucket.data-center.pr.diff.raw",
		Arguments: map[string]any{
			"projectKey":   projectKey,
			"repoSlug":     repoSlug,
			"id":           prID,
			"contextLines": 3,
			"whitespace":   "",
		},
	})
	elapsed4 := time.Since(start)
	
	if err != nil {
		log.Printf("Diff request failed (expected if PR doesn't exist): %v", err)
	} else {
		fmt.Printf("Diff call completed in %v\n", elapsed4)
	}

	// 6. Get same diff again (should hit cache)
	fmt.Println("\n6. Getting same PR diff again (cache hit expected)...")
	start = time.Now()
	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "bitbucket.data-center.pr.diff.raw",
		Arguments: map[string]any{
			"projectKey":   projectKey,
			"repoSlug":     repoSlug,
			"id":           prID,
			"contextLines": 3,
			"whitespace":   "",
		},
	})
	elapsed5 := time.Since(start)
	
	if err != nil {
		log.Printf("Second diff request failed: %v", err)
	} else {
		fmt.Printf("Second diff call completed in %v (should be much faster)\n", elapsed5)
	}

	// 7. Get diff with different parameters (should be cache miss)
	fmt.Println("\n7. Getting PR diff with different context lines (cache miss expected)...")
	start = time.Now()
	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "bitbucket.data-center.pr.diff.raw",
		Arguments: map[string]any{
			"projectKey":   projectKey,
			"repoSlug":     repoSlug,
			"id":           prID,
			"contextLines": 5, // Different context lines
			"whitespace":   "",
		},
	})
	elapsed6 := time.Since(start)
	
	if err != nil {
		log.Printf("Different diff request failed: %v", err)
	} else {
		fmt.Printf("Different diff call completed in %v (cache miss due to different parameters)\n", elapsed6)
	}

	fmt.Println("\n=== Performance Comparison ===")
	fmt.Printf("First PR get:     %v (cache miss)\n", elapsed1)
	fmt.Printf("Second PR get:    %v (cache hit)\n", elapsed2)
	fmt.Printf("Third PR get:     %v (cache miss after invalidation)\n", elapsed3)
	fmt.Printf("First diff:       %v (cache miss)\n", elapsed4)
	fmt.Printf("Second diff:      %v (cache hit)\n", elapsed5)
	fmt.Printf("Different diff:   %v (cache miss - different params)\n", elapsed6)

	fmt.Println("\n=== Demo Complete ===")
	fmt.Println("The cache system provides:")
	fmt.Println("- Automatic caching of read operations (get, list, diff, comments, activities)")
	fmt.Println("- Smart invalidation on write operations (create, update, merge, decline, reopen)")
	fmt.Println("- Parameter-aware caching (different parameters = different cache entries)")
	fmt.Println("- Memory and entry limits to prevent resource exhaustion")
	fmt.Println("- Configurable TTL and cleanup intervals")
}
