package mcpserver

import (
	"context"
	"fmt"

	"github.com/a2y-d5l/bitbucket-datacenter-mcp-go/bitbucketdatacenter"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// CachedPRToolRegistry holds cached versions of all PR tools
type CachedPRToolRegistry struct {
	cache            Cache
	invalidator      *CacheInvalidationHelper
	bbAPI            *bitbucketdatacenter.Client
	cacheEnabled     bool
}

// NewCachedPRToolRegistry creates a new registry of cached PR tools
func NewCachedPRToolRegistry(cache Cache, bbAPI *bitbucketdatacenter.Client) *CachedPRToolRegistry {
	return &CachedPRToolRegistry{
		cache:        cache,
		invalidator:  NewCacheInvalidationHelper(cache),
		bbAPI:        bbAPI,
		cacheEnabled: true,
	}
}

// SetCacheEnabled enables or disables caching for all tools
func (r *CachedPRToolRegistry) SetCacheEnabled(enabled bool) {
	r.cacheEnabled = enabled
}

// NewCachedListPRsTool creates a cached version of the list PRs tool
func (r *CachedPRToolRegistry) NewCachedListPRsTool() (*mcp.Tool, mcp.ToolHandlerFor[ListPRsInput, *bitbucketdatacenter.PagedResponse[bitbucketdatacenter.PullRequest]]) {
	tool, originalHandler := NewListPRsTool(r.bbAPI)
	
	if !r.cacheEnabled {
		return tool, originalHandler
	}

	keyFunc := func(input ListPRsInput) CacheKey {
		return CreatePRKeyFunc("pr-list")(input)
	}

	tagsFunc := func(input ListPRsInput) []string {
		return CreatePRTagsFunc(input)
	}

	cachedHandler := NewCachedMCPHandler(r.cache, originalHandler, keyFunc, tagsFunc)
	return tool, cachedHandler
}

// NewCachedGetPRTool creates a cached version of the get PR tool
func (r *CachedPRToolRegistry) NewCachedGetPRTool() (*mcp.Tool, mcp.ToolHandlerFor[GetPRInput, *bitbucketdatacenter.PullRequest]) {
	tool, originalHandler := NewGetPRTool(r.bbAPI)
	
	if !r.cacheEnabled {
		return tool, originalHandler
	}

	keyFunc := func(input GetPRInput) CacheKey {
		return CreatePRKeyFunc("pr-get")(input)
	}

	tagsFunc := func(input GetPRInput) []string {
		return CreatePRTagsFunc(input)
	}

	cachedHandler := NewCachedMCPHandler(r.cache, originalHandler, keyFunc, tagsFunc)
	return tool, cachedHandler
}

// NewCachedDiffPRTool creates a cached version of the diff PR tool
func (r *CachedPRToolRegistry) NewCachedDiffPRTool(server *mcp.Server) (*mcp.Tool, mcp.ToolHandlerFor[DiffPRInput, *DiffPRResult]) {
	tool, originalHandler := NewDiffPRTool(server, r.bbAPI)
	
	if !r.cacheEnabled {
		return tool, originalHandler
	}

	keyFunc := func(input DiffPRInput) CacheKey {
		return CreatePRKeyFunc("diff")(input)
	}

	tagsFunc := func(input DiffPRInput) []string {
		return CreatePRTagsFunc(input)
	}

	cachedHandler := NewCachedMCPHandler(r.cache, originalHandler, keyFunc, tagsFunc)
	return tool, cachedHandler
}

// NewCachedListPRCommentsTool creates a cached version of the list PR comments tool
func (r *CachedPRToolRegistry) NewCachedListPRCommentsTool() (*mcp.Tool, mcp.ToolHandlerFor[ListCommentsInput, *bitbucketdatacenter.PagedResponse[bitbucketdatacenter.PRComment]]) {
	tool, originalHandler := NewListPRCommentsTool(r.bbAPI)
	
	if !r.cacheEnabled {
		return tool, originalHandler
	}

	keyFunc := func(input ListCommentsInput) CacheKey {
		return CreatePRKeyFunc("comments")(input)
	}

	tagsFunc := func(input ListCommentsInput) []string {
		return CreatePRTagsFunc(input)
	}

	cachedHandler := NewCachedMCPHandler(r.cache, originalHandler, keyFunc, tagsFunc)
	return tool, cachedHandler
}

// NewCachedListPRActivitiesTool creates a cached version of the list PR activities tool
func (r *CachedPRToolRegistry) NewCachedListPRActivitiesTool() (*mcp.Tool, mcp.ToolHandlerFor[ActivitiesInput, *bitbucketdatacenter.PagedResponse[bitbucketdatacenter.PRActivity]]) {
	tool, originalHandler := NewListPRActivitiesTool(r.bbAPI)
	
	if !r.cacheEnabled {
		return tool, originalHandler
	}

	keyFunc := func(input ActivitiesInput) CacheKey {
		return CreatePRKeyFunc("activities")(input)
	}

	tagsFunc := func(input ActivitiesInput) []string {
		return CreatePRTagsFunc(input)
	}

	cachedHandler := NewCachedMCPHandler(r.cache, originalHandler, keyFunc, tagsFunc)
	return tool, cachedHandler
}

// Mutating operations that should invalidate cache

// NewCachedCreatePRTool creates a cached version of the create PR tool with cache invalidation
func (r *CachedPRToolRegistry) NewCachedCreatePRTool() (*mcp.Tool, mcp.ToolHandlerFor[CreatePRInput, *bitbucketdatacenter.PullRequest]) {
	tool, originalHandler := NewCreatePRTool(r.bbAPI)
	
	// Wrap with invalidation logic
	wrappedHandler := func(ctx context.Context, req *mcp.CallToolRequest, input CreatePRInput) (*mcp.CallToolResult, *bitbucketdatacenter.PullRequest, error) {
		result, pr, err := originalHandler(ctx, req, input)
		if err == nil && r.cacheEnabled {
			// Invalidate repository-level caches when new PR is created
			if invalidateErr := r.invalidator.InvalidateRepo(ctx, input.ProjectKey, input.RepoSlug); invalidateErr != nil {
				// Log but don't fail the operation
				fmt.Printf("Failed to invalidate cache after PR creation: %v\n", invalidateErr)
			}
		}
		return result, pr, err
	}
	
	return tool, wrappedHandler
}

// NewCachedUpdatePRTool creates a cached version of the update PR tool with cache invalidation
func (r *CachedPRToolRegistry) NewCachedUpdatePRTool() (*mcp.Tool, mcp.ToolHandlerFor[UpdatePRInput, *bitbucketdatacenter.PullRequest]) {
	tool, originalHandler := NewUpdatePRTool(r.bbAPI)
	
	wrappedHandler := func(ctx context.Context, req *mcp.CallToolRequest, input UpdatePRInput) (*mcp.CallToolResult, *bitbucketdatacenter.PullRequest, error) {
		result, pr, err := originalHandler(ctx, req, input)
		if err == nil && r.cacheEnabled {
			if invalidateErr := r.invalidator.InvalidatePR(ctx, input.ProjectKey, input.RepoSlug, input.ID); invalidateErr != nil {
				fmt.Printf("Failed to invalidate cache after PR update: %v\n", invalidateErr)
			}
		}
		return result, pr, err
	}
	
	return tool, wrappedHandler
}

// NewCachedMergePRTool creates a cached version of the merge PR tool with cache invalidation
func (r *CachedPRToolRegistry) NewCachedMergePRTool() (*mcp.Tool, mcp.ToolHandlerFor[MergePRInput, *bitbucketdatacenter.PullRequest]) {
	tool, originalHandler := NewMergePRTool(r.bbAPI)
	
	wrappedHandler := func(ctx context.Context, req *mcp.CallToolRequest, input MergePRInput) (*mcp.CallToolResult, *bitbucketdatacenter.PullRequest, error) {
		result, pr, err := originalHandler(ctx, req, input)
		if err == nil && r.cacheEnabled {
			if invalidateErr := r.invalidator.InvalidatePR(ctx, input.ProjectKey, input.RepoSlug, input.ID); invalidateErr != nil {
				fmt.Printf("Failed to invalidate cache after PR merge: %v\n", invalidateErr)
			}
		}
		return result, pr, err
	}
	
	return tool, wrappedHandler
}

// NewCachedDeclinePRTool creates a cached version of the decline PR tool with cache invalidation
func (r *CachedPRToolRegistry) NewCachedDeclinePRTool() (*mcp.Tool, mcp.ToolHandlerFor[DeclinePRInput, *bitbucketdatacenter.PullRequest]) {
	tool, originalHandler := NewDeclinePRTool(r.bbAPI)
	
	wrappedHandler := func(ctx context.Context, req *mcp.CallToolRequest, input DeclinePRInput) (*mcp.CallToolResult, *bitbucketdatacenter.PullRequest, error) {
		result, pr, err := originalHandler(ctx, req, input)
		if err == nil && r.cacheEnabled {
			if invalidateErr := r.invalidator.InvalidatePR(ctx, input.ProjectKey, input.RepoSlug, input.ID); invalidateErr != nil {
				fmt.Printf("Failed to invalidate cache after PR decline: %v\n", invalidateErr)
			}
		}
		return result, pr, err
	}
	
	return tool, wrappedHandler
}

// NewCachedReopenPRTool creates a cached version of the reopen PR tool with cache invalidation
func (r *CachedPRToolRegistry) NewCachedReopenPRTool() (*mcp.Tool, mcp.ToolHandlerFor[ReopenPRInput, *bitbucketdatacenter.PullRequest]) {
	tool, originalHandler := NewReopenPRTool(r.bbAPI)
	
	wrappedHandler := func(ctx context.Context, req *mcp.CallToolRequest, input ReopenPRInput) (*mcp.CallToolResult, *bitbucketdatacenter.PullRequest, error) {
		result, pr, err := originalHandler(ctx, req, input)
		if err == nil && r.cacheEnabled {
			if invalidateErr := r.invalidator.InvalidatePR(ctx, input.ProjectKey, input.RepoSlug, input.ID); invalidateErr != nil {
				fmt.Printf("Failed to invalidate cache after PR reopen: %v\n", invalidateErr)
			}
		}
		return result, pr, err
	}
	
	return tool, wrappedHandler
}

// NewCachedCreatePRCommentTool creates a cached version of the create PR comment tool with cache invalidation
func (r *CachedPRToolRegistry) NewCachedCreatePRCommentTool() (*mcp.Tool, mcp.ToolHandlerFor[CreateCommentInput, *bitbucketdatacenter.PRComment]) {
	tool, originalHandler := NewCreatePRCommentTool(r.bbAPI)
	
	wrappedHandler := func(ctx context.Context, req *mcp.CallToolRequest, input CreateCommentInput) (*mcp.CallToolResult, *bitbucketdatacenter.PRComment, error) {
		result, comment, err := originalHandler(ctx, req, input)
		if err == nil && r.cacheEnabled {
			// Invalidate comment caches for this PR
			tags := []string{
				fmt.Sprintf("project:%s", input.ProjectKey),
				fmt.Sprintf("repo:%s", input.RepoSlug),
				fmt.Sprintf("pr:%d", input.ID),
			}
			if invalidateErr := r.cache.InvalidateByTags(ctx, tags); invalidateErr != nil {
				fmt.Printf("Failed to invalidate cache after comment creation: %v\n", invalidateErr)
			}
		}
		return result, comment, err
	}
	
	return tool, wrappedHandler
}

// RegisterAllCachedPullRequestTools registers all cached PR tools with the MCP server
func RegisterAllCachedPullRequestTools(srv *mcp.Server, bbc *bitbucketdatacenter.Client, cache Cache) {
	registry := NewCachedPRToolRegistry(cache, bbc)
	
	// Read operations (cached)
	listTool, listHandler := registry.NewCachedListPRsTool()
	mcp.AddTool(srv, listTool, listHandler)
	
	getTool, getHandler := registry.NewCachedGetPRTool()
	mcp.AddTool(srv, getTool, getHandler)
	
	diffTool, diffHandler := registry.NewCachedDiffPRTool(srv)
	mcp.AddTool(srv, diffTool, diffHandler)
	
	listCommentsTool, listCommentsHandler := registry.NewCachedListPRCommentsTool()
	mcp.AddTool(srv, listCommentsTool, listCommentsHandler)
	
	listActivitiesTool, listActivitiesHandler := registry.NewCachedListPRActivitiesTool()
	mcp.AddTool(srv, listActivitiesTool, listActivitiesHandler)
	
	// Write operations (with cache invalidation)
	createTool, createHandler := registry.NewCachedCreatePRTool()
	mcp.AddTool(srv, createTool, createHandler)
	
	updateTool, updateHandler := registry.NewCachedUpdatePRTool()
	mcp.AddTool(srv, updateTool, updateHandler)
	
	mergeTool, mergeHandler := registry.NewCachedMergePRTool()
	mcp.AddTool(srv, mergeTool, mergeHandler)
	
	declineTool, declineHandler := registry.NewCachedDeclinePRTool()
	mcp.AddTool(srv, declineTool, declineHandler)
	
	reopenTool, reopenHandler := registry.NewCachedReopenPRTool()
	mcp.AddTool(srv, reopenTool, reopenHandler)
	
	createCommentTool, createCommentHandler := registry.NewCachedCreatePRCommentTool()
	mcp.AddTool(srv, createCommentTool, createCommentHandler)
}
