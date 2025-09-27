package mcpserver

import (
	"context"
	"fmt"

	"github.com/a2y-d5l/bitbucket-datacenter-mcp-go/bitbucketdatacenter"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type (
	ListPRsInput struct {
		ProjectKey string `json:"projectKey" jsonschema:"Project key (e.g., PROJ)"`
		RepoSlug   string `json:"repoSlug"   jsonschema:"Repository slug"`
		State      string `json:"state,omitempty" jsonschema:"optional; one of OPEN,MERGED,DECLINED,ALL"`
		Start      int    `json:"start,omitempty" jsonschema:"optional paging start index"`
		Limit      int    `json:"limit,omitempty" jsonschema:"optional paging size"`
	}

	GetPRInput struct {
		ProjectKey string `json:"projectKey"`
		RepoSlug   string `json:"repoSlug"`
		ID         int    `json:"id"`
	}

	CreatePRInput struct {
		ProjectKey  string   `json:"projectKey"`
		RepoSlug    string   `json:"repoSlug"`
		Title       string   `json:"title"`
		Description string   `json:"description,omitempty"`
		FromRefID   string   `json:"fromRefId" jsonschema:"refs/heads/feature"`
		ToRefID     string   `json:"toRefId"   jsonschema:"refs/heads/main"`
		Reviewers   []string `json:"reviewers,omitempty" jsonschema:"list of reviewer slugs"`
		Draft       bool     `json:"draft,omitempty"`
	}

	UpdatePRInput struct {
		ProjectKey  string `json:"projectKey"`
		RepoSlug    string `json:"repoSlug"`
		ID          int    `json:"id"`
		Version     int    `json:"version" jsonschema:"current PR version to avoid conflicts"`
		Title       string `json:"title,omitempty"`
		Description string `json:"description,omitempty"`
	}

	MergePRInput struct {
		ProjectKey string `json:"projectKey"`
		RepoSlug   string `json:"repoSlug"`
		ID         int    `json:"id"`
		Version    int    `json:"version" jsonschema:"required current PR version"`
		Message    string `json:"message,omitempty"`
		StrategyID string `json:"strategyId,omitempty" jsonschema:"e.g., no-ff, squash, merge-commit (varies by server settings)"`
	}

	DeclinePRInput struct {
		ProjectKey string `json:"projectKey"`
		RepoSlug   string `json:"repoSlug"`
		ID         int    `json:"id"`
		Version    int    `json:"version"`
	}

	ReopenPRInput struct {
		ProjectKey string `json:"projectKey"`
		RepoSlug   string `json:"repoSlug"`
		ID         int    `json:"id"`
		Version    int    `json:"version"`
	}

	ActivitiesInput struct {
		ProjectKey string `json:"projectKey"`
		RepoSlug   string `json:"repoSlug"`
		ID         int    `json:"id"`
		Start      int    `json:"start,omitempty"`
		Limit      int    `json:"limit,omitempty"`
	}

	ListCommentsInput struct {
		ProjectKey string `json:"projectKey"`
		RepoSlug   string `json:"repoSlug"`
		ID         int    `json:"id"`
		Start      int    `json:"start,omitempty"`
		Limit      int    `json:"limit,omitempty"`
	}

	CreateCommentInput struct {
		ProjectKey string `json:"projectKey"`
		RepoSlug   string `json:"repoSlug"`
		ID         int    `json:"id"`
		Text       string `json:"text"`
	}

	DiffPRInput struct {
		ProjectKey   string `json:"projectKey"`
		RepoSlug     string `json:"repoSlug"`
		ID           int    `json:"id"`
		ContextLines int    `json:"contextLines,omitempty"` // optional
		Whitespace   string `json:"whitespace,omitempty"`   // optional; e.g., "ignore-all"
	}

	// Tool result wrapper for diff text to keep responses structured JSON.
	DiffPRResult struct {
		// Unified diff text for the requested PR.
		Diff string `json:"diff"`
	}
)

// NewListPRsTool creates a tool and handler for listing PRs with optional state and paging.
func NewListPRsTool(bbAPI *bitbucketdatacenter.Client) (*mcp.Tool, mcp.ToolHandlerFor[ListPRsInput, *bitbucketdatacenter.PagedResponse[bitbucketdatacenter.PullRequest]]) {
	return &mcp.Tool{
			Name:        "bitbucket.data-center.pr.list",
			Description: "List pull requests (state: OPEN|MERGED|DECLINED|ALL; supports paging)",
		}, mcp.ToolHandlerFor[ListPRsInput, *bitbucketdatacenter.PagedResponse[bitbucketdatacenter.PullRequest]](func(ctx context.Context, req *mcp.CallToolRequest, in ListPRsInput) (*mcp.CallToolResult, *bitbucketdatacenter.PagedResponse[bitbucketdatacenter.PullRequest], error) {
			if in.ProjectKey == "" || in.RepoSlug == "" {
				return nil, nil, fmt.Errorf("projectKey and repoSlug are required")
			}
			page, err := bbAPI.ListPullRequests(ctx, in.ProjectKey, in.RepoSlug, in.State, in.Start, in.Limit)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to list pull requests: %w", err)
			}
			return nil, page, nil
		})
}

// NewListPRsToolCallParams creates the call parameters for the list PRs tool.
func NewListPRsToolCallParams(
	projectKey string,
	repoSlug string,
	state string,
	start int,
	limit int,
) *mcp.CallToolParams {
	return &mcp.CallToolParams{
		Name: "bitbucket.data-center.pr.list",
		Arguments: map[string]any{
			"projectKey": projectKey,
			"repoSlug":   repoSlug,
			"state":      state,
			"start":      start,
			"limit":      limit,
		},
	}
}

// NewGetPRTool creates a tool and handler for getting a PR by ID.
func NewGetPRTool(bbAPI *bitbucketdatacenter.Client) (*mcp.Tool, mcp.ToolHandlerFor[GetPRInput, *bitbucketdatacenter.PullRequest]) {
	return &mcp.Tool{
			Name:        "bitbucket.data-center.pr.get",
			Description: "Get a pull request by ID",
		}, mcp.ToolHandlerFor[GetPRInput, *bitbucketdatacenter.PullRequest](func(ctx context.Context, req *mcp.CallToolRequest, in GetPRInput) (*mcp.CallToolResult, *bitbucketdatacenter.PullRequest, error) {
			if in.ProjectKey == "" || in.RepoSlug == "" || in.ID == 0 {
				return nil, nil, fmt.Errorf("projectKey and repoSlug are required")
			}
			pr, err := bbAPI.GetPullRequest(ctx, in.ProjectKey, in.RepoSlug, in.ID)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to get pull request: %w", err)
			}
			return nil, pr, nil
		})
}

// NewGetPRToolCallParams creates the call parameters for the get PR tool.
func NewGetPRToolCallParams(
	projectKey string,
	repoSlug string,
	id int,
) *mcp.CallToolParams {
	return &mcp.CallToolParams{
		Name: "bitbucket.data-center.pr.get",
		Arguments: map[string]any{
			"projectKey": projectKey,
			"repoSlug":   repoSlug,
			"id":         id,
		},
	}
}

// NewCreatePRTool creates a tool and handler for creating a new PR.
func NewCreatePRTool(bbAPI *bitbucketdatacenter.Client) (*mcp.Tool, mcp.ToolHandlerFor[CreatePRInput, *bitbucketdatacenter.PullRequest]) {
	return &mcp.Tool{
			Name:        "bitbucket.data-center.pr.create",
			Description: "Create a new pull request",
		}, mcp.ToolHandlerFor[CreatePRInput, *bitbucketdatacenter.PullRequest](func(ctx context.Context, req *mcp.CallToolRequest, in CreatePRInput) (*mcp.CallToolResult, *bitbucketdatacenter.PullRequest, error) {
			if in.ProjectKey == "" || in.RepoSlug == "" || in.Title == "" || in.FromRefID == "" || in.ToRefID == "" {
				return nil, nil, fmt.Errorf("projectKey, repoSlug, title, fromRefId and toRefId are required")
			}
			cr := bitbucketdatacenter.CreatePRRequest{
				Title:       in.Title,
				Description: in.Description,
				FromRef:     bitbucketdatacenter.PRRef{ID: in.FromRefID},
				ToRef:       bitbucketdatacenter.PRRef{ID: in.ToRefID},
				Draft:       in.Draft,
			}
			for _, slug := range in.Reviewers {
				cr.Reviewers = append(cr.Reviewers, bitbucketdatacenter.PRReviewer{User: bitbucketdatacenter.PRUser{Slug: slug}})
			}
			pr, err := bbAPI.CreatePullRequest(ctx, in.ProjectKey, in.RepoSlug, cr)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create pull request: %w", err)
			}
			return nil, pr, nil
		})
}

// NewCreatePRToolCallParams creates the call parameters for the create PR tool.
func NewCreatePRToolCallParams(
	projectKey string,
	repoSlug string,
	title string,
	description string,
	fromRefID string,
	toRefID string,
	reviewers []string,
	draft bool,
) *mcp.CallToolParams {
	return &mcp.CallToolParams{
		Name: "bitbucket.data-center.pr.create",
		Arguments: map[string]any{
			"projectKey":  projectKey,
			"repoSlug":    repoSlug,
			"title":       title,
			"description": description,
			"fromRefId":   fromRefID,
			"toRefId":     toRefID,
			"reviewers":   reviewers,
			"draft":       draft,
		},
	}
}

// NewUpdatePRTool creates a tool and handler for updating a PR's title/description.
func NewUpdatePRTool(bbAPI *bitbucketdatacenter.Client) (*mcp.Tool, mcp.ToolHandlerFor[UpdatePRInput, *bitbucketdatacenter.PullRequest]) {
	return &mcp.Tool{
			Name:        "bitbucket.data-center.pr.update",
			Description: "Update PR title/description (requires current version)",
		}, mcp.ToolHandlerFor[UpdatePRInput, *bitbucketdatacenter.PullRequest](func(ctx context.Context, req *mcp.CallToolRequest, in UpdatePRInput) (*mcp.CallToolResult, *bitbucketdatacenter.PullRequest, error) {
			if in.ProjectKey == "" || in.RepoSlug == "" || in.ID == 0 || in.Version == 0 {
				return nil, nil, fmt.Errorf("projectKey, repoSlug, id, version are required")
			}
			u := bitbucketdatacenter.UpdatePRRequest{Version: in.Version, Title: in.Title, Description: in.Description}
			pr, err := bbAPI.UpdatePullRequest(ctx, in.ProjectKey, in.RepoSlug, in.ID, u)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to update pull request: %w", err)
			}
			return nil, pr, nil
		})
}

// NewUpdatePRToolCallParams creates the call parameters for the update PR tool.
func NewUpdatePRToolCallParams(
	projectKey string,
	repoSlug string,
	id int,
	version int,
	title string,
	description string,
) *mcp.CallToolParams {
	return &mcp.CallToolParams{
		Name: "bitbucket.data-center.pr.update",
		Arguments: map[string]any{
			"projectKey":  projectKey,
			"repoSlug":    repoSlug,
			"id":          id,
			"version":     version,
			"title":       title,
			"description": description,
		},
	}
}

// NewMergePRTool creates a tool and handler for merging a PR.
func NewMergePRTool(bbAPI *bitbucketdatacenter.Client) (*mcp.Tool, mcp.ToolHandlerFor[MergePRInput, *bitbucketdatacenter.PullRequest]) {
	return &mcp.Tool{
			Name:        "bitbucket.data-center.pr.merge",
			Description: "Merge a pull request (requires current version)",
		}, mcp.ToolHandlerFor[MergePRInput, *bitbucketdatacenter.PullRequest](func(ctx context.Context, req *mcp.CallToolRequest, in MergePRInput) (*mcp.CallToolResult, *bitbucketdatacenter.PullRequest, error) {
			if in.ProjectKey == "" || in.RepoSlug == "" || in.ID == 0 || in.Version == 0 {
				return nil, nil, fmt.Errorf("projectKey, repoSlug, id, version are required")
			}
			m := bitbucketdatacenter.MergeRequest{Version: in.Version, Message: in.Message, StrategyID: in.StrategyID}
			pr, err := bbAPI.MergePullRequest(ctx, in.ProjectKey, in.RepoSlug, in.ID, m)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to merge pull request: %w", err)
			}
			return nil, pr, nil
		})
}

// NewMergePRToolCallParams creates the call parameters for the merge PR tool.
func NewMergePRToolCallParams(
	projectKey string,
	repoSlug string,
	id int,
	version int,
	message string,
	strategyID string,
) *mcp.CallToolParams {
	return &mcp.CallToolParams{
		Name: "bitbucket.data-center.pr.merge",
		Arguments: map[string]any{
			"projectKey": projectKey,
			"repoSlug":   repoSlug,
			"id":         id,
			"version":    version,
			"message":    message,
			"strategyId": strategyID,
		},
	}
}

// NewDeclinePRTool creates a tool and handler for declining a PR.
func NewDeclinePRTool(bbAPI *bitbucketdatacenter.Client) (*mcp.Tool, mcp.ToolHandlerFor[DeclinePRInput, *bitbucketdatacenter.PullRequest]) {
	return &mcp.Tool{
			Name:        "bitbucket.data-center.pr.decline",
			Description: "Decline a pull request (requires current version)",
		}, mcp.ToolHandlerFor[DeclinePRInput, *bitbucketdatacenter.PullRequest](func(ctx context.Context, req *mcp.CallToolRequest, in DeclinePRInput) (*mcp.CallToolResult, *bitbucketdatacenter.PullRequest, error) {
			if in.ProjectKey == "" || in.RepoSlug == "" || in.ID == 0 || in.Version == 0 {
				return nil, nil, fmt.Errorf("projectKey, repoSlug, id, version are required")
			}
			pr, err := bbAPI.DeclinePullRequest(ctx, in.ProjectKey, in.RepoSlug, in.ID, in.Version)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to decline pull request: %w", err)
			}
			return nil, pr, nil
		})
}

// NewDeclinePRToolCallParams creates the call parameters for the decline PR tool.
func NewDeclinePRToolCallParams(
	projectKey string,
	repoSlug string,
	id int,
	version int,
) *mcp.CallToolParams {
	return &mcp.CallToolParams{
		Name: "bitbucket.data-center.pr.decline",
		Arguments: map[string]any{
			"projectKey": projectKey,
			"repoSlug":   repoSlug,
			"id":         id,
			"version":    version,
		},
	}
}

// NewReopenPRTool creates a tool and handler for reopening a declined PR.
func NewReopenPRTool(bbAPI *bitbucketdatacenter.Client) (*mcp.Tool, mcp.ToolHandlerFor[ReopenPRInput, *bitbucketdatacenter.PullRequest]) {
	return &mcp.Tool{
			Name:        "bitbucket.data-center.pr.reopen",
			Description: "Reopen a declined pull request (requires current version)",
		}, mcp.ToolHandlerFor[ReopenPRInput, *bitbucketdatacenter.PullRequest](func(ctx context.Context, req *mcp.CallToolRequest, in ReopenPRInput) (*mcp.CallToolResult, *bitbucketdatacenter.PullRequest, error) {
			if in.ProjectKey == "" || in.RepoSlug == "" || in.ID == 0 || in.Version == 0 {
				return nil, nil, fmt.Errorf("projectKey, repoSlug, id, version are required")
			}
			pr, err := bbAPI.ReopenPullRequest(ctx, in.ProjectKey, in.RepoSlug, in.ID, in.Version)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to reopen pull request: %w", err)
			}
			return nil, pr, nil
		})
}

// NewReopenPRToolCallParams creates the call parameters for the reopen PR tool.
func NewReopenPRToolCallParams(
	projectKey string,
	repoSlug string,
	id int,
	version int,
) *mcp.CallToolParams {
	return &mcp.CallToolParams{
		Name: "bitbucket.data-center.pr.reopen",
		Arguments: map[string]any{
			"projectKey": projectKey,
			"repoSlug":   repoSlug,
			"id":         id,
			"version":    version,
		},
	}
}

// NewListPRActivitiesTool creates a tool and handler for listing PR activities.
func NewListPRActivitiesTool(bbAPI *bitbucketdatacenter.Client) (*mcp.Tool, mcp.ToolHandlerFor[ActivitiesInput, *bitbucketdatacenter.PagedResponse[bitbucketdatacenter.PRActivity]]) {
	return &mcp.Tool{
			Name:        "bitbucket.data-center.pr.activities",
			Description: "List PR activities (comments, commits, etc.)",
		}, mcp.ToolHandlerFor[ActivitiesInput, *bitbucketdatacenter.PagedResponse[bitbucketdatacenter.PRActivity]](func(ctx context.Context, req *mcp.CallToolRequest, in ActivitiesInput) (*mcp.CallToolResult, *bitbucketdatacenter.PagedResponse[bitbucketdatacenter.PRActivity], error) {
			if in.ProjectKey == "" || in.RepoSlug == "" || in.ID == 0 {
				return nil, nil, fmt.Errorf("projectKey and repoSlug are required")
			}
			act, err := bbAPI.ListActivities(ctx, in.ProjectKey, in.RepoSlug, in.ID, in.Start, in.Limit)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to list pull request activities: %w", err)
			}
			return nil, act, nil
		})
}

// NewListPRActivitiesToolCallParams creates the call parameters for the list PR activities tool.
func NewListPRActivitiesToolCallParams(
	projectKey string,
	repoSlug string,
	id int,
	start int,
	limit int,
) *mcp.CallToolParams {
	return &mcp.CallToolParams{
		Name: "bitbucket.data-center.pr.activities",
		Arguments: map[string]any{
			"projectKey": projectKey,
			"repoSlug":   repoSlug,
			"id":         id,
			"start":      start,
			"limit":      limit,
		},
	}
}

// NewListPRCommentsTool creates a tool and handler for listing PR comments.
func NewListPRCommentsTool(bbAPI *bitbucketdatacenter.Client) (*mcp.Tool, mcp.ToolHandlerFor[ListCommentsInput, *bitbucketdatacenter.PagedResponse[bitbucketdatacenter.PRComment]]) {
	return &mcp.Tool{
			Name:        "bitbucket.data-center.pr.comments.list",
			Description: "List comments on a pull request",
		}, mcp.ToolHandlerFor[ListCommentsInput, *bitbucketdatacenter.PagedResponse[bitbucketdatacenter.PRComment]](func(ctx context.Context, req *mcp.CallToolRequest, in ListCommentsInput) (*mcp.CallToolResult, *bitbucketdatacenter.PagedResponse[bitbucketdatacenter.PRComment], error) {
			if in.ProjectKey == "" || in.RepoSlug == "" || in.ID == 0 {
				return nil, nil, fmt.Errorf("projectKey and repoSlug are required")
			}
			out, err := bbAPI.ListComments(ctx, in.ProjectKey, in.RepoSlug, in.ID, in.Start, in.Limit)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to list pull request comments: %w", err)
			}
			return nil, out, nil
		})
}

// NewListPRCommentsToolCallParams creates the call parameters for the list PR comments tool.
func NewListPRCommentsToolCallParams(
	projectKey string,
	repoSlug string,
	id int,
	start int,
	limit int,
) *mcp.CallToolParams {
	return &mcp.CallToolParams{
		Name: "bitbucket.data-center.pr.comments.list",
		Arguments: map[string]any{
			"projectKey": projectKey,
			"repoSlug":   repoSlug,
			"id":         id,
			"start":      start,
			"limit":      limit,
		},
	}
}

// NewCreatePRCommentTool creates a tool and handler for creating PR comments.
func NewCreatePRCommentTool(bbAPI *bitbucketdatacenter.Client) (*mcp.Tool, mcp.ToolHandlerFor[CreateCommentInput, *bitbucketdatacenter.PRComment]) {
	return &mcp.Tool{
			Name:        "bitbucket.data-center.pr.comments.create",
			Description: "Create a comment on a pull request",
		}, mcp.ToolHandlerFor[CreateCommentInput, *bitbucketdatacenter.PRComment](func(ctx context.Context, req *mcp.CallToolRequest, in CreateCommentInput) (*mcp.CallToolResult, *bitbucketdatacenter.PRComment, error) {
			if in.ProjectKey == "" || in.RepoSlug == "" || in.ID == 0 {
				return nil, nil, fmt.Errorf("projectKey, repoSlug, id are required")
			}
			comment, err := bbAPI.CreateComment(ctx, in.ProjectKey, in.RepoSlug, in.ID, in.Text)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create pull request comment: %w", err)
			}
			return nil, comment, nil
		})
}

// NewCreatePRCommentToolCallParams creates the call parameters for the create PR comment tool.
func NewCreatePRCommentToolCallParams(
	projectKey string,
	repoSlug string,
	id int,
	text string,
) *mcp.CallToolParams {
	return &mcp.CallToolParams{
		Name: "bitbucket.data-center.pr.comments.create",
		Arguments: map[string]any{
			"projectKey": projectKey,
			"repoSlug":   repoSlug,
			"id":         id,
			"text":       text,
		},
	}
}

func NewDiffPRTool(server *mcp.Server, bb *bitbucketdatacenter.Client) (*mcp.Tool, mcp.ToolHandlerFor[DiffPRInput, *DiffPRResult]) {
	t := mcp.Tool{
		Name:        "bitbucket.data-center.pr.diff.raw",
		Description: "Get the unified diff (text) for a pull request. Optional: contextLines (int), whitespace ('ignore-all').",
	}
	h := mcp.ToolHandlerFor[DiffPRInput, *DiffPRResult](func(ctx context.Context, req *mcp.CallToolRequest, in DiffPRInput) (*mcp.CallToolResult, *DiffPRResult, error) {
		if in.ProjectKey == "" || in.RepoSlug == "" || in.ID == 0 {
			return nil, nil, fmt.Errorf("projectKey, repoSlug, id are required")
		}
		diff, err := bb.GetPullRequestRawDiff(ctx, in.ProjectKey, in.RepoSlug, in.ID, bitbucketdatacenter.DiffOptions{
			ContextLines: in.ContextLines,
			Whitespace:   in.Whitespace,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get pull request diff: %w", err)
		}
		return nil, &DiffPRResult{Diff: diff}, nil
	})

	return &t, h
}

func NewDiffPRToolCallParams(
	projectKey string,
	repoSlug string,
	id int,
	contextLines int,
	whitespace string,
) *mcp.CallToolParams {
	return &mcp.CallToolParams{
		Name: "bitbucket.data-center.pr.diff.raw",
		Arguments: map[string]any{
			"projectKey":   projectKey,
			"repoSlug":     repoSlug,
			"id":           id,
			"contextLines": contextLines,
			"whitespace":   whitespace,
		},
	}
}

// RegisterAllPullRequestTools adds all PR-related tools to the given MCP server.
func RegisterAllPullRequestTools(srv *mcp.Server, bbc *bitbucketdatacenter.Client) {
	listTool, listHandler := NewListPRsTool(bbc)
	mcp.AddTool(srv, listTool, listHandler)
	getTool, getHandler := NewGetPRTool(bbc)
	mcp.AddTool(srv, getTool, getHandler)
	createTool, createHandler := NewCreatePRTool(bbc)
	mcp.AddTool(srv, createTool, createHandler)
	updateTool, updateHandler := NewUpdatePRTool(bbc)
	mcp.AddTool(srv, updateTool, updateHandler)
	mergeTool, mergeHandler := NewMergePRTool(bbc)
	mcp.AddTool(srv, mergeTool, mergeHandler)
	declineTool, declineHandler := NewDeclinePRTool(bbc)
	mcp.AddTool(srv, declineTool, declineHandler)
	reopenTool, reopenHandler := NewReopenPRTool(bbc)
	mcp.AddTool(srv, reopenTool, reopenHandler)
	listActivitiesTool, listActivitiesHandler := NewListPRActivitiesTool(bbc)
	mcp.AddTool(srv, listActivitiesTool, listActivitiesHandler)
	listCommentsTool, listCommentsHandler := NewListPRCommentsTool(bbc)
	mcp.AddTool(srv, listCommentsTool, listCommentsHandler)
	createCommentTool, createCommentHandler := NewCreatePRCommentTool(bbc)
	mcp.AddTool(srv, createCommentTool, createCommentHandler)
	diffTool, diffHandler := NewDiffPRTool(srv, bbc)
	mcp.AddTool(srv, diffTool, diffHandler)
}
