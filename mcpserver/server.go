package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/a2y-d5l/bitbucket-datacenter-mcp-go/bitbucketdatacenter"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

//
// Bitbucket Data Center v8.19 — Pull Requests MCP Server
//
// This program exposes a Model Context Protocol (MCP) server that wraps the
// Bitbucket Data Center/Server REST API (v8.19) Pull Requests endpoints.
// It is designed to run in-process alongside a local AI agent, communicating
// over in-memory transports (no sockets/stdio).
//
// Key points:
//
//   • Uses the official MCP Go SDK (github.com/modelcontextprotocol/go-sdk)
//     with in-memory transports (mcp.NewInMemoryTransports) so you can spawn
//     the server as a goroutine and connect a local MCP client without IPC.
//   • Covers core Pull Requests operations: list, get, create, update title/desc,
//     merge, decline, reopen, activities, comments (list/post minimal).
//   • Auth: Personal Access Token (recommended) via BITBUCKET_TOKEN, or Basic
//     via BITBUCKET_USERNAME / BITBUCKET_PASSWORD.
//   • API base path defaults to /rest/api/1.0 (override with BITBUCKET_API_BASE)
//     and server base URL is BITBUCKET_BASE_URL (e.g. https://bitbucket.example.com).
//
// References (see Atlassian docs for v8.19 and the MCP SDK docs):
// - Bitbucket DC/Server REST API v8.19: https://developer.atlassian.com/server/bitbucket/rest/v819/intro/#about
// - v8.19 OpenAPI (Swagger): https://dac-static.atlassian.com/server/bitbucket/8.19.swagger.v3.json?v=1.637.23
// - MCP Go SDK README & package docs: https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk
//
// Environment Variables:
//   BITBUCKET_BASE_URL   (required) e.g. https://bitbucket.example.com
//   BITBUCKET_TOKEN      (preferred) Personal Access Token (PAT) for Bearer auth
//   BITBUCKET_USERNAME   (optional) for Basic auth
//   BITBUCKET_PASSWORD   (optional) for Basic auth
//   BITBUCKET_API_BASE   (optional) default: /rest/api/1.0 (set to /rest/api/latest if you prefer)
//   BITBUCKET_TIMEOUT    (optional) HTTP timeout (e.g., 15s). Default: 30s
//
// Demo:
//   Set BITBUCKET* env vars and RUN_DEMO=1 to see sample client calls.
//   Otherwise, this binary starts the MCP server in-memory and connects a client
//   session you can reuse in your process.
//

// ========================
// MCP Server Construction
// ========================

type ServerOptions struct {
	// Cache configuration for intelligent caching layer
	Cache       *CacheConfig
	EnableCache bool
}

type ToolRegistrationFunc func(server *mcp.Server, bbAPI *bitbucketdatacenter.Client)

func RegisterTools(server *mcp.Server, bbAPI *bitbucketdatacenter.Client, regFns ...ToolRegistrationFunc) {
	for _, fn := range regFns {
		fn(server, bbAPI)
	}
}

// StartBitbucketPRServer starts an MCP server for Bitbucket PRs over an in-memory
// transport and returns a connected MCP client session for local use,
// plus a shutdown function to stop the server.
//
// You can embed the returned session into your agent. Tools are registered under
// names like "bb.pr.list", "bb.pr.get", etc.
func Start(ctx context.Context, opts *ServerOptions) (serverClient *mcp.Client, clientSession *mcp.ClientSession, stop func(), err error) {
	bbAPI, err := bitbucketdatacenter.NewClient()
	if err != nil {
		return nil, nil, nil, err
	}

	impl := &mcp.Implementation{Name: "bitbucket-prs", Version: "v8.19"}
	server := mcp.NewServer(impl, nil)
	
	// Initialize cache if enabled
	var cache Cache
	var cacheManager *CacheManager
	if opts != nil && opts.EnableCache {
		cacheConfig := CacheConfig{
			TTL:               10 * time.Minute,
			MaxEntries:        1000,
			MaxMemoryBytes:    100 * 1024 * 1024, // 100MB
			MaxEntrySizeBytes: 10 * 1024 * 1024,  // 10MB
			CleanupInterval:   1 * time.Minute,
			EnableMetrics:     true,
			EnableCompression: true,
		}
		
		// Override with user-provided config if available
		if opts.Cache != nil {
			if opts.Cache.TTL > 0 {
				cacheConfig.TTL = opts.Cache.TTL
			}
			if opts.Cache.MaxEntries > 0 {
				cacheConfig.MaxEntries = opts.Cache.MaxEntries
			}
			if opts.Cache.MaxMemoryBytes > 0 {
				cacheConfig.MaxMemoryBytes = opts.Cache.MaxMemoryBytes
			}
			if opts.Cache.MaxEntrySizeBytes > 0 {
				cacheConfig.MaxEntrySizeBytes = opts.Cache.MaxEntrySizeBytes
			}
			if opts.Cache.CleanupInterval > 0 {
				cacheConfig.CleanupInterval = opts.Cache.CleanupInterval
			}
			cacheConfig.EnableMetrics = opts.Cache.EnableMetrics
			cacheConfig.EnableCompression = opts.Cache.EnableCompression
		}
		
		cacheManager = NewCacheManager(cacheConfig)
		cache = cacheManager
		log.Printf("Cache enabled with TTL=%v, MaxEntries=%d, MaxMemory=%dMB", 
			cacheConfig.TTL, cacheConfig.MaxEntries, cacheConfig.MaxMemoryBytes/(1024*1024))
	}
	
	// Register PR tools (cached or regular based on configuration)
	if opts != nil && opts.EnableCache && cache != nil {
		RegisterAllCachedPullRequestTools(server, bbAPI, cache)
		log.Printf("Registered cached PR tools")
	} else {
		RegisterTools(server, bbAPI, RegisterAllPullRequestTools)
		log.Printf("Registered standard PR tools")
	}

	// Create a pair of in-memory transports: one for the server, one for the client.
	serverT, clientT := mcp.NewInMemoryTransports()

	// Run the MCP server in the background.
	serverCtx, cancel := context.WithCancel(ctx)
	go func() {
		if err := server.Run(serverCtx, serverT); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("MCP server stopped with error: %v", err)
		}
	}()

	// Build a local MCP client and connect over the paired transport.
	client := mcp.NewClient(&mcp.Implementation{Name: "local-agent", Version: "v1.0.0"}, nil)
	session, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		cancel()
		return nil, nil, nil, fmt.Errorf("connect client to in-memory server: %w", err)
	}

	stop = func() {
		_ = session.Close()
		if cacheManager != nil {
			_ = cacheManager.Close()
		}
		cancel()
	}
	return client, session, stop, nil

}
