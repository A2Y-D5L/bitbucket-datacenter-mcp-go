package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/a2y-d5l/bitbucket-datacenter-mcp-go/mcpserver"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	ctx := context.Background()

	_, session, stop, err := mcpserver.Start(ctx, nil)
	if err != nil {
		log.Fatalf("start MCP server: %v", err)
	}
	defer stop()

	// If RUN_DEMO is set, show a small end-to-end call using environment-driven params.
	if os.Getenv("RUN_DEMO") == "1" {
		project := os.Getenv("DEMO_PROJECT")
		repo := os.Getenv("DEMO_REPO")
		if project == "" || repo == "" {
			log.Println("Set DEMO_PROJECT and DEMO_REPO to run the demo (skipping).")
			return
		}
		// Example: list OPEN PRs (first page).
		params := mcpserver.NewListPRsToolCallParams(project, repo, "OPEN", 0, 5)
		res, err := session.CallTool(ctx, params)
		if err != nil {
			log.Fatalf("CallTool(%s) failed: %v", params.Name, err)
		}
		if res.IsError {
			// Print MCP-side error text.
			var buf strings.Builder
			for _, c := range res.Content {
				if tc, ok := c.(*mcp.TextContent); ok {
					buf.WriteString(tc.Text)
					buf.WriteByte('\n')
				}
			}
			log.Fatalf("MCP tool error: %s", buf.String())
		}
		// Pretty-print result JSON
		var out bytes.Buffer
		enc := json.NewEncoder(&out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(res); err != nil {
			log.Printf("encode result: %v", err)
		}
		fmt.Printf("List PRs Result:\n%s\n", out.String())

		if prIDStr := os.Getenv("DEMO_PR_ID"); prIDStr != "" && os.Getenv("DEMO_COMMENT") != "" {
			prID, _ := strconv.Atoi(prIDStr)
			// Call the create comment tool.
			params := mcpserver.NewCreatePRCommentToolCallParams(project, repo, prID, os.Getenv("DEMO_COMMENT"))
			result, err := session.CallTool(ctx, params)
			if err != nil {
				log.Fatalf("CallTool(%s) failed: %v", params.Name, err)
			}
			if result.IsError {
				var buf strings.Builder
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						buf.WriteString(tc.Text)
						buf.WriteByte('\n')
					}
				}
				log.Fatalf("CallTool(%s) failed: %v", params.Name, err)
			}
			if result.IsError {
				var buf strings.Builder
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						buf.WriteString(tc.Text)
						buf.WriteByte('\n')
					}
				}
				log.Fatalf("MCP tool error: %s", buf.String())
			}
			fmt.Printf("CallTool(%s) succeeded.\n", params.Name)
		}
	}

}
