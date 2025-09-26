package bitbucketdatacenter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// PagedResponse is Bitbucket’s standard paging shape.
type PagedResponse[T any] struct {
	Size          int  `json:"size"`
	Limit         int  `json:"limit"`
	IsLastPage    bool `json:"isLastPage"`
	Values        []T  `json:"values"`
	Start         int  `json:"start"`
	NextPageStart int  `json:"nextPageStart"`
}

// PullRequest minimal fields (extend as needed).
type PullRequest struct {
	ID          int            `json:"id"`
	Version     int            `json:"version"`
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	State       string         `json:"state"` // OPEN, MERGED, DECLINED
	Open        bool           `json:"open"`
	Closed      bool           `json:"closed"`
	FromRef     PRRef          `json:"fromRef"`
	ToRef       PRRef          `json:"toRef"`
	Links       map[string]any `json:"links,omitempty"`
	Properties  map[string]any `json:"properties,omitempty"`
	Attrs       map[string]any `json:"attributes,omitempty"`
	Raw         map[string]any `json:"-"`
}

type PRRef struct {
	ID         string     `json:"id"` // e.g., refs/heads/feature
	DisplayID  string     `json:"displayId,omitempty"`
	LatestHash string     `json:"latestCommit,omitempty"`
	Repo       Repository `json:"repository"`
}

type Repository struct {
	Slug    string  `json:"slug"`
	Project Project `json:"project"`
}

type Project struct {
	Key string `json:"key"`
}

type CreatePRRequest struct {
	Title       string       `json:"title"`
	Description string       `json:"description,omitempty"`
	FromRef     PRRef        `json:"fromRef"`
	ToRef       PRRef        `json:"toRef"`
	Reviewers   []PRReviewer `json:"reviewers,omitempty"`
	Draft       bool         `json:"draft,omitempty"` // DC supports draft PRs
}

type PRReviewer struct {
	User PRUser `json:"user"`
}

type PRUser struct {
	Slug string `json:"slug,omitempty"` // Bitbucket DC commonly uses "slug"
	Name string `json:"name,omitempty"`
	ID   int    `json:"id,omitempty"`
}

type UpdatePRRequest struct {
	Version     int    `json:"version"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

type MergeRequest struct {
	Version    int    `json:"version"`
	Message    string `json:"message,omitempty"`
	StrategyID string `json:"strategyId,omitempty"`
	// Optional: "autoSubject" bool, "autoMerge" bool, etc. (not included here)
}

type DeclineRequest struct {
	Version int `json:"version"`
}

type ReopenRequest struct {
	Version int `json:"version"`
}

// PRActivity is loosely typed here.
type PRActivity struct {
	ID        int             `json:"id"`
	CreatedAt int64           `json:"createdDate"`
	User      map[string]any  `json:"user"`
	Action    string          `json:"action,omitempty"`
	Raw       json.RawMessage `json:"raw,omitempty"`
}

// PRComment minimal shape (list & create).
type PRComment struct {
	ID      int            `json:"id"`
	Text    string         `json:"text"`
	Version int            `json:"version,omitempty"`
	Author  map[string]any `json:"author,omitempty"`
	Created int64          `json:"createdDate,omitempty"`
}


type Client struct {
	BaseURL    string
	APIBase    string
	Token      string
	Username   string
	Password   string
	HTTP       *http.Client
	UserAgent  string
}

func NewClient() (*Client, error) {
	base := strings.TrimRight(os.Getenv("BITBUCKET_BASE_URL"), "/")
	if base == "" {
		return nil, errors.New("BITBUCKET_BASE_URL is required (e.g., https://bitbucket.example.com)")
	}
	apiBase := os.Getenv("BITBUCKET_API_BASE")
	if apiBase == "" {
		apiBase = "/rest/api/1.0"
	}
	token := os.Getenv("BITBUCKET_TOKEN")
	user := os.Getenv("BITBUCKET_USERNAME")
	pass := os.Getenv("BITBUCKET_PASSWORD")

	timeout := 30 * time.Second
	if s := os.Getenv("BITBUCKET_TIMEOUT"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			timeout = d
		}
	}
	return &Client{
		BaseURL:   base,
		APIBase:   apiBase,
		Token:     token,
		Username:  user,
		Password:  pass,
		HTTP:      &http.Client{Timeout: timeout},
	}, nil

}

func (c *Client) makeURL(path string, q url.Values) string {
	u := c.BaseURL + strings.TrimRight(c.APIBase, "/") + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

func (c *Client) doJSON(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.makeURL(path, query), rdr)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "bitbucket-datacenter-mcp/1.0 (+github.com/modelcontextprotocol/go-sdk)")
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// Prefer PAT (Bearer). Fallback to Basic if username/password provided.
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	} else if c.Username != "" && c.Password != "" {
		cred := base64.StdEncoding.EncodeToString([]byte(c.Username + ":" + c.Password))
		req.Header.Set("Authorization", "Basic "+cred)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 2xx only
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		return fmt.Errorf("bitbucket %s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(b)))
	}
	if out == nil {
		return nil
	}
	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	return dec.Decode(out)
}
func (c *Client) ListPullRequests(ctx context.Context, projectKey, repoSlug string, state string, start, limit int) (*PagedResponse[PullRequest], error) {
	q := url.Values{}
	if state != "" {
		q.Set("state", strings.ToUpper(state)) // OPEN, MERGED, DECLINED, ALL
	}
	if start > 0 {
		q.Set("start", strconv.Itoa(start))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	var page PagedResponse[PullRequest]
	path := fmt.Sprintf("/projects/%s/repos/%s/pull-requests", url.PathEscape(projectKey), url.PathEscape(repoSlug))
	if err := c.doJSON(ctx, http.MethodGet, path, q, nil, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

func (c *Client) GetPullRequest(ctx context.Context, projectKey, repoSlug string, prID int) (*PullRequest, error) {
	var pr PullRequest
	path := fmt.Sprintf("/projects/%s/repos/%s/pull-requests/%d", url.PathEscape(projectKey), url.PathEscape(repoSlug), prID)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, nil, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

func (c *Client) CreatePullRequest(ctx context.Context, projectKey, repoSlug string, req CreatePRRequest) (*PullRequest, error) {
	var pr PullRequest
	// Normalize repository references to the target repository; Bitbucket requires repo+project in fromRef/toRef.
	req.FromRef.Repo = Repository{Slug: repoSlug, Project: Project{Key: projectKey}}
	req.ToRef.Repo = Repository{Slug: repoSlug, Project: Project{Key: projectKey}}
	path := fmt.Sprintf("/projects/%s/repos/%s/pull-requests", url.PathEscape(projectKey), url.PathEscape(repoSlug))
	if err := c.doJSON(ctx, http.MethodPost, path, nil, &req, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

func (c *Client) UpdatePullRequest(ctx context.Context, projectKey, repoSlug string, prID int, req UpdatePRRequest) (*PullRequest, error) {
	var pr PullRequest
	path := fmt.Sprintf("/projects/%s/repos/%s/pull-requests/%d", url.PathEscape(projectKey), url.PathEscape(repoSlug), prID)
	if err := c.doJSON(ctx, http.MethodPut, path, nil, &req, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

func (c *Client) MergePullRequest(ctx context.Context, projectKey, repoSlug string, prID int, req MergeRequest) (*PullRequest, error) {
	var pr PullRequest
	path := fmt.Sprintf("/projects/%s/repos/%s/pull-requests/%d/merge", url.PathEscape(projectKey), url.PathEscape(repoSlug), prID)
	if err := c.doJSON(ctx, http.MethodPost, path, nil, &req, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

func (c *Client) DeclinePullRequest(ctx context.Context, projectKey, repoSlug string, prID int, version int) (*PullRequest, error) {
	var pr PullRequest
	path := fmt.Sprintf("/projects/%s/repos/%s/pull-requests/%d/decline", url.PathEscape(projectKey), url.PathEscape(repoSlug), prID)
	body := DeclineRequest{Version: version}
	if err := c.doJSON(ctx, http.MethodPost, path, nil, &body, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

func (c *Client) ReopenPullRequest(ctx context.Context, projectKey, repoSlug string, prID int, version int) (*PullRequest, error) {
	var pr PullRequest
	path := fmt.Sprintf("/projects/%s/repos/%s/pull-requests/%d/reopen", url.PathEscape(projectKey), url.PathEscape(repoSlug), prID)
	body := ReopenRequest{Version: version}
	if err := c.doJSON(ctx, http.MethodPost, path, nil, &body, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

func (c *Client) ListActivities(ctx context.Context, projectKey, repoSlug string, prID int, start, limit int) (*PagedResponse[PRActivity], error) {
	q := url.Values{}
	if start > 0 {
		q.Set("start", strconv.Itoa(start))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	var page PagedResponse[PRActivity]
	path := fmt.Sprintf("/projects/%s/repos/%s/pull-requests/%d/activities", url.PathEscape(projectKey), url.PathEscape(repoSlug), prID)
	if err := c.doJSON(ctx, http.MethodGet, path, q, nil, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

func (c *Client) ListComments(ctx context.Context, projectKey, repoSlug string, prID int, start, limit int) (*PagedResponse[PRComment], error) {
	q := url.Values{}
	if start > 0 {
		q.Set("start", strconv.Itoa(start))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	var page PagedResponse[PRComment]
	path := fmt.Sprintf("/projects/%s/repos/%s/pull-requests/%d/comments", url.PathEscape(projectKey), url.PathEscape(repoSlug), prID)
	if err := c.doJSON(ctx, http.MethodGet, path, q, nil, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

func (c *Client) CreateComment(ctx context.Context, projectKey, repoSlug string, prID int, text string) (*PRComment, error) {
	body := map[string]any{"text": text}
	var out PRComment
	path := fmt.Sprintf("/projects/%s/repos/%s/pull-requests/%d/comments", url.PathEscape(projectKey), url.PathEscape(repoSlug), prID)
	if err := c.doJSON(ctx, http.MethodPost, path, nil, &body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}