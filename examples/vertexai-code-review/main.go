package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/a2y-d5l/bitbucket-datacenter-mcp-go/mcpserver"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/genai"
)

// Config holds all inputs (env-driven only).
type Config struct {
	Model          string
	Project        string
	Location       string
	UseVertex      bool
	ProjectKey     string
	RepoSlug       string
	PRID           int
	ContextLines   int
	MaxPromptChars int
	PostComments   bool
}

// loadConfig reads from env and validates.
func loadConfig() (*Config, error) {
	get := func(k string) string { return strings.TrimSpace(os.Getenv(k)) }

	cfg := &Config{
		Model:          fallback(get("GENAI_MODEL"), "gemini-2.0-flash"),
		Project:        get("GOOGLE_CLOUD_PROJECT"),
		Location:       fallback(get("GOOGLE_CLOUD_LOCATION"), "global"),
		UseVertex:      strings.EqualFold(get("GOOGLE_GENAI_USE_VERTEXAI"), "true"),
		ProjectKey:     fallback(get("REVIEW_PROJECT"), os.Getenv("BITBUCKET_DEFAULT_PROJECT")),
		RepoSlug:       get("REVIEW_REPO"),
		PostComments:   strings.EqualFold(get("REVIEW_POST_COMMENTS"), "true"),
		MaxPromptChars: atoiDefault(get("REVIEW_MAX_PROMPT_CHARS"), 180000),
		ContextLines:   atoiDefault(get("REVIEW_CONTEXT_LINES"), 10),
	}

	// Required for PR targeting
	prStr := get("REVIEW_PR_ID")
	if prStr == "" {
		return nil, errors.New("REVIEW_PR_ID is required")
	}
	prID, err := strconv.Atoi(prStr)
	if err != nil {
		return nil, fmt.Errorf("invalid REVIEW_PR_ID: %w", err)
	}
	cfg.PRID = prID

	if cfg.RepoSlug == "" {
		return nil, errors.New("REVIEW_REPO is required")
	}
	if cfg.ProjectKey == "" {
		return nil, errors.New("REVIEW_PROJECT or BITBUCKET_DEFAULT_PROJECT is required")
	}

	// Strongly encourage Vertex AI usage here.
	if !cfg.UseVertex {
		return nil, errors.New("GOOGLE_GENAI_USE_VERTEXAI must be 'true' (this agent targets Vertex AI)")
	}
	// Expect ADC or ambient credentials when calling Vertex AI.
	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") == "" {
		// Not strictly required on GCP runtimes, but warn locally.
		log.Printf("[warn] GOOGLE_APPLICATION_CREDENTIALS not set; relying on ambient ADC if available")
	}

	return cfg, nil
}

func fallback(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// ReviewFinding is the structured shape we ask the model to return.
type ReviewFinding struct {
	Title    string   `json:"title"`
	Severity string   `json:"severity"` // "info"|"nit"|"suggestion"|"warning"|"critical"
	Files    []string `json:"files,omitempty"`
	// A concise comment for the PR thread (Markdown allowed)
	Comment string `json:"comment"`
}

type ReviewOutput struct {
	Summary   string          `json:"summary"`
	Risks     []string        `json:"risks,omitempty"`
	Checklist []string        `json:"checklist,omitempty"`
	Findings  []ReviewFinding `json:"findings"`
}

// connectMCP spawns the Bitbucket MCP server as a subprocess and returns a live session.
func connectMCP(ctx context.Context, cfg *Config) (*mcp.ClientSession, func(), error) {
	_, session, stop, err := mcpserver.Start(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("start MCP server: %w", err)
	}
	return session, stop, nil
}

// callTool is a thin helper to call MCP tools with arguments.
func callTool(ctx context.Context, s *mcp.ClientSession, name string, args map[string]any) (string, error) {
	res, err := s.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return "", err
	}
	if res.IsError {
		return "", fmt.Errorf("tool %s reported error", name)
	}
	var b strings.Builder
	for _, c := range res.Content {
		if t, ok := c.(*mcp.TextContent); ok {
			b.WriteString(t.Text)
			b.WriteString("\n")
		}
	}
	return b.String(), nil
}

// fetchPRContext pulls PR metadata + diff via MCP.
func fetchPRContext(ctx context.Context, s *mcp.ClientSession, cfg *Config) (prJSON string, diff string, comments string, err error) {
	args := map[string]any{
		"projectKey": cfg.ProjectKey,
		"repoSlug":   cfg.RepoSlug,
		"id":         cfg.PRID,
	}
	prJSON, err = callTool(ctx, s, "bitbucket.data-center.pr.get", args) // Updated tool name
	if err != nil {
		return
	}
	diff, err = callTool(ctx, s, "bitbucket.data-center.pr.diff.raw", map[string]any{
		"projectKey":   cfg.ProjectKey,
		"repoSlug":     cfg.RepoSlug,
		"id":           cfg.PRID,
		"contextLines": cfg.ContextLines,
	})
	if err != nil {
		return
	}
	comments, err = callTool(ctx, s, "bitbucket.data-center.pr.comments.list", args) // Updated tool name
	return
}

// buildPrompt creates the system + user prompts for the LLM.
func buildPrompt(prJSON, diff, comments string, cfg *Config) (system string, user string) {
	// Trim diff if necessary (MCP already supports truncation per file, but guard anyway).
	if len(diff) > cfg.MaxPromptChars {
		diff = diff[:cfg.MaxPromptChars] + "\n\n[truncated]"
	}

	system = `You are a seasoned staff-level code reviewer for Go services.
Focus on: correctness, concurrency/races, resource leaks, API/HTTP edge cases, error handling, security, input validation, logging/observability, performance, test coverage, and idiomatic Go.
Be concise but specific. Prefer actionable suggestions with short code snippets when helpful.
Output strictly in JSON matching this schema:

{
  "summary": "one-paragraph high-level review summary",
  "risks": ["..."],
  "checklist": [
    "tests updated?",
    "errors wrapped with context?",
    "ctx propagation and timeouts?",
    "input validation?",
    "idempotency / retries?",
    "race conditions?",
    "logging levels sane?",
    "security / secrets / auth / ACLs?"
  ],
  "findings": [
    {
      "title": "clear, short",
      "severity": "info|nit|suggestion|warning|critical",
      "files": ["relative/path.go:42", "another/file.go"], 
      "comment": "markdown-ready comment with rationale and if possible a minimal code change or link to a guideline"
    }
  ]
}`

	user = fmt.Sprintf(`PR metadata (JSON):
%s

Existing PR comments (for context):
%s

Unified diff (context=%d):
%s

Return JSON only.`, prJSON, emptyIf(comments, "(none)"), cfg.ContextLines, diff)

	return
}

func emptyIf(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

// genaiClient constructs a Vertex AI client using env configuration.
// The go-genai SDK uses env vars to choose Vertex AI backend and project/location.
func genaiClient(ctx context.Context) (*genai.Client, error) {
	// APIVersion "v1" is the recommended default per current docs.
	return genai.NewClient(ctx, &genai.ClientConfig{
		HTTPOptions: genai.HTTPOptions{APIVersion: "v1"},
	})
}

// runLLM sends the prompt and parses JSON into ReviewOutput.
func runLLM(ctx context.Context, model string, cfg *genai.GenerateContentConfig, user string) (*ReviewOutput, error) {
	client, err := genaiClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("genai client: %w", err)
	}

	resp, err := client.Models.GenerateContent(ctx, model, genai.Text(user), cfg)
	if err != nil {
		return nil, fmt.Errorf("generate content: %w", err)
	}

	txt := resp.Text() // quick accessor for text output
	var out ReviewOutput
	if err := json.Unmarshal([]byte(txt), &out); err != nil {
		// If the model returned extra pre/post text, try to extract JSON block.
		start := strings.Index(txt, "{")
		end := strings.LastIndex(txt, "}")
		if start >= 0 && end > start {
			if err2 := json.Unmarshal([]byte(txt[start:end+1]), &out); err2 == nil {
				return &out, nil
			}
		}
		return nil, fmt.Errorf("failed to parse model JSON: %w\nraw:\n%s", err, txt)
	}
	return &out, nil
}

func postComments(ctx context.Context, s *mcp.ClientSession, findings []ReviewFinding, projKey, repoSlug string, prID int) error {
	if len(findings) == 0 {
		return nil
	}

	for _, f := range findings {
		text := fmt.Sprintf("**%s** _(severity: %s)_\n\nFiles: %s\n\n%s",
			f.Title, f.Severity, strings.Join(f.Files, ", "), f.Comment)

		args := map[string]any{
			"projectKey": projKey,
			"repoSlug":   repoSlug,
			"id":         prID,
			"text":       text,
		}

		if _, err := callTool(ctx, s, "bitbucket.data-center.pr.comments.create", args); err != nil {
			return fmt.Errorf("add_comment failed: %w", err)
		}

		time.Sleep(300 * time.Millisecond)
	}
	return nil
}

func main() {
	log.SetFlags(0)
	ctx := context.Background()

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	// Connect to Bitbucket Data Center MCP Server.
	session, stop, err := connectMCP(ctx, cfg)
	if err != nil {
		log.Fatalf("MCP connection failed: %v", err)
	}
	defer stop() // Stop the MCP server when done

	// Pull PR context + diff.
	prJSON, diff, comments, err := fetchPRContext(ctx, session, cfg)
	if err != nil {
		log.Fatalf("Failed to fetch PR context: %v", err)
	}

	// Build prompt and run the model.
	system, user := buildPrompt(prJSON, diff, comments, cfg)

	var temp float32 = 0.2
	instructions := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{Parts: []*genai.Part{{Text: system}}},
		Temperature:       &temp,
	}

	out, err := runLLM(ctx, cfg.Model, instructions, user)
	if err != nil {
		log.Fatalf("LLM error: %v", err)
	}

	fmt.Println("=== Review Summary ===")
	fmt.Println(out.Summary)
	if len(out.Risks) > 0 {
		fmt.Println("\nRisks:")
		for _, r := range out.Risks {
			fmt.Printf("- %s\n", r)
		}
	}
	if len(out.Checklist) > 0 {
		fmt.Println("\nChecklist:")
		for _, c := range out.Checklist {
			fmt.Printf("- [ ] %s\n", c)
		}
	}
	if len(out.Findings) > 0 {
		fmt.Println("\nFindings:")
		for i, f := range out.Findings {
			fmt.Printf("%d) [%s] %s\n   Files: %s\n   %s\n\n", i+1, f.Severity, f.Title, strings.Join(f.Files, ", "), f.Comment)
		}
	}

	if err := postComments(ctx, session, out.Findings, cfg.ProjectKey, cfg.RepoSlug, cfg.PRID); err != nil {
		log.Fatalf("posting comments failed: %v", err)
	}

	log.Println("Done.")
}
