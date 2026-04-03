package jira

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/availhealth/archon/internal/config"
)

type Client struct {
	baseURL    *url.URL
	authMethod string
	email      string
	apiToken   string
	oauthToken string
	httpClient *http.Client
}

type SearchResult struct {
	StartAt    int               `json:"startAt"`
	MaxResults int               `json:"maxResults"`
	Total      int               `json:"total"`
	Issues     []SearchIssueItem `json:"issues"`
}

type SearchIssueItem struct {
	ID     string            `json:"id"`
	Key    string            `json:"key"`
	Fields SearchIssueFields `json:"fields"`
}

type SearchIssueFields struct {
	Summary string `json:"summary"`
	Updated string `json:"updated"`
}

type Issue struct {
	ID     string      `json:"id"`
	Key    string      `json:"key"`
	Fields IssueFields `json:"fields"`
}

type IssueFields struct {
	Summary     string           `json:"summary"`
	Description any              `json:"description"`
	IssueType   NamedField       `json:"issuetype"`
	Priority    NamedField       `json:"priority"`
	Status      NamedField       `json:"status"`
	Labels      []string         `json:"labels"`
	Components  []NamedField     `json:"components"`
	Parent      *IssueParent     `json:"parent"`
	Comment     CommentContainer `json:"comment"`
	IssueLinks  []IssueLink      `json:"issuelinks"`
	Updated     string           `json:"updated"`
	Created     string           `json:"created"`
	Unknown     map[string]any   `json:"-"`
}

type NamedField struct {
	Name string `json:"name"`
}

type IssueParent struct {
	Key    string           `json:"key"`
	Fields IssueParentField `json:"fields"`
}

type IssueParentField struct {
	Summary string `json:"summary"`
}

type CommentContainer struct {
	Comments []IssueComment `json:"comments"`
}

type IssueComment struct {
	ID      string        `json:"id"`
	Body    any           `json:"body"`
	Author  CommentAuthor `json:"author"`
	Created string        `json:"created"`
	Updated string        `json:"updated"`
}

type CommentAuthor struct {
	DisplayName string `json:"displayName"`
	Email       string `json:"emailAddress"`
}

type IssueLink struct {
	OutwardIssue *LinkedIssue `json:"outwardIssue"`
	InwardIssue  *LinkedIssue `json:"inwardIssue"`
}

type LinkedIssue struct {
	Key    string           `json:"key"`
	Fields IssueParentField `json:"fields"`
}

type commentResponse struct {
	ID string `json:"id"`
}

type transitionResponse struct {
	Transitions []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"transitions"`
}

func NewClient(cfg config.Config) (*Client, error) {
	baseURL, err := url.Parse(strings.TrimRight(cfg.Jira.BaseURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("parse jira base url: %w", err)
	}

	return &Client{
		baseURL:    baseURL,
		authMethod: strings.TrimSpace(cfg.Jira.AuthMethod),
		email:      cfg.Jira.Email,
		apiToken:   cfg.Jira.APIToken,
		oauthToken: cfg.Jira.OAuthToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func (c *Client) SearchIssues(ctx context.Context, jql string, startAt, maxResults int) (SearchResult, error) {
	rel := &url.URL{Path: path.Join(c.baseURL.Path, "/rest/api/3/search")}
	query := rel.Query()
	query.Set("jql", jql)
	query.Set("startAt", strconv.Itoa(startAt))
	query.Set("maxResults", strconv.Itoa(maxResults))
	query.Set("fields", "summary,updated")
	rel.RawQuery = query.Encode()

	var result SearchResult
	if err := c.doJSON(ctx, http.MethodGet, rel, &result); err != nil {
		return SearchResult{}, err
	}
	return result, nil
}

func (c *Client) GetIssue(ctx context.Context, issueKey string) (Issue, error) {
	rel := &url.URL{Path: path.Join(c.baseURL.Path, "/rest/api/3/issue/", issueKey)}
	query := rel.Query()
	query.Set("fields", strings.Join([]string{
		"summary",
		"description",
		"issuetype",
		"priority",
		"status",
		"labels",
		"components",
		"parent",
		"comment",
		"issuelinks",
		"updated",
		"created",
	}, ","))
	rel.RawQuery = query.Encode()

	var issue Issue
	if err := c.doJSON(ctx, http.MethodGet, rel, &issue); err != nil {
		return Issue{}, err
	}
	return issue, nil
}

func (c *Client) PostComment(ctx context.Context, issueKey, body string) (string, error) {
	payload := map[string]any{
		"body": commentADF(body),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal jira comment: %w", err)
	}

	rel := &url.URL{Path: path.Join(c.baseURL.Path, "/rest/api/3/issue/", issueKey, "comment")}
	endpoint := c.baseURL.ResolveReference(rel)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("build jira comment request: %w", err)
	}
	c.applyAuth(req)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("jira comment request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("jira comment request failed: %s: %s", resp.Status, strings.TrimSpace(string(bodyBytes)))
	}

	var result commentResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode jira comment response: %w", err)
	}
	return result.ID, nil
}

func (c *Client) TransitionIssueByName(ctx context.Context, issueKey, transitionName string) error {
	transitionName = strings.TrimSpace(transitionName)
	if transitionName == "" {
		return fmt.Errorf("transition name is required")
	}
	rel := &url.URL{Path: path.Join(c.baseURL.Path, "/rest/api/3/issue/", issueKey, "transitions")}
	var transitions transitionResponse
	if err := c.doJSON(ctx, http.MethodGet, rel, &transitions); err != nil {
		return err
	}
	var transitionID string
	for _, transition := range transitions.Transitions {
		if strings.EqualFold(strings.TrimSpace(transition.Name), transitionName) {
			transitionID = transition.ID
			break
		}
	}
	if transitionID == "" {
		return fmt.Errorf("transition %q is not available for %s", transitionName, issueKey)
	}
	payload := map[string]any{"transition": map[string]string{"id": transitionID}}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal transition payload: %w", err)
	}
	endpoint := c.baseURL.ResolveReference(rel)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("build jira transition request: %w", err)
	}
	c.applyAuth(req)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("jira transition request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("jira transition request failed: %s: %s", resp.Status, strings.TrimSpace(string(bodyBytes)))
	}
	return nil
}

func (c *Client) Check(ctx context.Context) error {
	rel := &url.URL{Path: path.Join(c.baseURL.Path, "/rest/api/3/myself")}
	var payload map[string]any
	if err := c.doJSON(ctx, http.MethodGet, rel, &payload); err != nil {
		return fmt.Errorf("jira health check failed: %w", err)
	}
	return nil
}

func (c *Client) doJSON(ctx context.Context, method string, rel *url.URL, target any) error {
	endpoint := c.baseURL.ResolveReference(rel)
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("build jira request: %w", err)
	}
	c.applyAuth(req)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("jira request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("jira request failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode jira response: %w", err)
	}
	return nil
}

func (c *Client) applyAuth(req *http.Request) {
	if strings.EqualFold(c.authMethod, "oauth") {
		req.Header.Set("Authorization", "Bearer "+c.oauthToken)
		return
	}
	req.SetBasicAuth(c.email, c.apiToken)
}

func ParseTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty jira time")
	}
	layouts := []string{
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05.999-0700",
		time.RFC3339Nano,
		time.RFC3339,
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported jira time %q", value)
}

func commentADF(body string) map[string]any {
	paragraphs := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n\n")
	content := make([]map[string]any, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}
		lines := strings.Split(paragraph, "\n")
		inline := make([]map[string]any, 0, len(lines)*2)
		for idx, line := range lines {
			if line != "" {
				inline = append(inline, map[string]any{"type": "text", "text": line})
			}
			if idx < len(lines)-1 {
				inline = append(inline, map[string]any{"type": "hardBreak"})
			}
		}
		if len(inline) == 0 {
			inline = append(inline, map[string]any{"type": "text", "text": " "})
		}
		content = append(content, map[string]any{
			"type":    "paragraph",
			"content": inline,
		})
	}
	if len(content) == 0 {
		content = append(content, map[string]any{
			"type":    "paragraph",
			"content": []map[string]any{{"type": "text", "text": " "}},
		})
	}
	return map[string]any{
		"type":    "doc",
		"version": 1,
		"content": content,
	}
}
