package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/availhealth/archon/internal/config"
	"github.com/availhealth/archon/internal/githuboauth"
)

type Client struct {
	repo       string
	authMethod string
	token      string
	store      *githuboauth.Store
	httpClient *http.Client
}

type CreatePRInput struct {
	Title string `json:"title"`
	Head  string `json:"head"`
	Base  string `json:"base"`
	Body  string `json:"body"`
	Draft bool   `json:"draft"`
}

type PullRequest struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
}

type ReviewComment struct {
	Body string `json:"body"`
}

type Label struct {
	Name string `json:"name"`
}

func NewClient(cfg config.Config) *Client {
	return &Client{
		repo:       cfg.GitHub.Repo,
		authMethod: cfg.GitHub.AuthMethod,
		token:      cfg.GitHub.Token,
		store:      githuboauth.NewStore(cfg.GitHub.OAuthBrowser.TokenStorePath),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) CreatePullRequest(ctx context.Context, input CreatePRInput) (PullRequest, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return PullRequest{}, fmt.Errorf("marshal pull request payload: %w", err)
	}

	var pr PullRequest
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("https://api.github.com/repos/%s/pulls", c.repo), bytes.NewReader(data), &pr); err != nil {
		return PullRequest{}, err
	}
	return pr, nil
}

func (c *Client) AddLabels(ctx context.Context, number int, labels []string) error {
	if len(labels) == 0 {
		return nil
	}
	data, err := json.Marshal(map[string]any{"labels": labels})
	if err != nil {
		return fmt.Errorf("marshal labels payload: %w", err)
	}
	return c.doJSON(ctx, http.MethodPost, fmt.Sprintf("https://api.github.com/repos/%s/issues/%d/labels", c.repo, number), bytes.NewReader(data), &map[string]any{})
}

func (c *Client) ListLabels(ctx context.Context, number int) ([]Label, error) {
	var labels []Label
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("https://api.github.com/repos/%s/issues/%d/labels", c.repo, number), nil, &labels); err != nil {
		return nil, err
	}
	return labels, nil
}

func (c *Client) ListReviewComments(ctx context.Context, number int) ([]ReviewComment, error) {
	var comments []ReviewComment
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("https://api.github.com/repos/%s/pulls/%d/comments", c.repo, number), nil, &comments); err != nil {
		return nil, err
	}
	return comments, nil
}

func (c *Client) HasMatchingBranch(ctx context.Context, prefix string) (bool, error) {
	var refs []struct {
		Ref string `json:"ref"`
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/git/matching-refs/heads/%s", c.repo, prefix)
	if err := c.doJSON(ctx, http.MethodGet, url, nil, &refs); err != nil {
		if strings.Contains(err.Error(), "404") {
			return false, nil
		}
		return false, err
	}
	return len(refs) > 0, nil
}

func (c *Client) Check(ctx context.Context) error {
	var payload map[string]any
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("https://api.github.com/repos/%s", c.repo), nil, &payload); err != nil {
		return fmt.Errorf("github health check failed: %w", err)
	}
	return nil
}

func (c *Client) doJSON(ctx context.Context, method, url string, body io.Reader, target any) error {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return fmt.Errorf("build github request: %w", err)
	}
	token, err := c.activeToken()
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("github request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("github request failed: %s: %s", resp.Status, strings.TrimSpace(string(bodyBytes)))
	}
	if target == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil && err != io.EOF {
		return fmt.Errorf("decode github response: %w", err)
	}
	return nil
}

func (c *Client) activeToken() (string, error) {
	if strings.EqualFold(strings.TrimSpace(c.authMethod), "oauth_browser") {
		token, err := c.store.Load()
		if err != nil {
			if errors.Is(err, githuboauth.ErrTokenNotFound) {
				return "", fmt.Errorf("github browser oauth is configured but no token is stored; run `archon auth login github`")
			}
			return "", err
		}
		return token.AccessToken, nil
	}
	if strings.TrimSpace(c.token) == "" {
		return "", fmt.Errorf("github token is not configured")
	}
	return c.token, nil
}
