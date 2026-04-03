package config

import (
	"fmt"
	"os"
	"strings"
)

func Validate(cfg Config) error {
	if cfg.Mode != "approval" && cfg.Mode != "sandbox" {
		return fmt.Errorf("mode must be 'approval' or 'sandbox'")
	}
	if cfg.UI.Port <= 0 || cfg.UI.Port > 65535 {
		return fmt.Errorf("ui.port must be between 1 and 65535")
	}
	if strings.TrimSpace(cfg.Jira.BaseURL) == "" {
		return fmt.Errorf("jira.base_url is required")
	}
	switch strings.TrimSpace(cfg.Jira.AuthMethod) {
	case "api_token":
		if strings.TrimSpace(cfg.Jira.Email) == "" {
			return fmt.Errorf("jira.email is required when jira.auth_method=api_token")
		}
		if strings.TrimSpace(cfg.Jira.APIToken) == "" {
			return fmt.Errorf("jira.api_token is required when jira.auth_method=api_token")
		}
	case "oauth":
		if strings.TrimSpace(cfg.Jira.OAuthToken) == "" {
			return fmt.Errorf("jira.oauth_token is required when jira.auth_method=oauth")
		}
	default:
		return fmt.Errorf("jira.auth_method must be 'api_token' or 'oauth'")
	}
	if len(cfg.Jira.Projects) != 1 {
		return fmt.Errorf("mvp requires exactly one jira project")
	}
	project := cfg.Jira.Projects[0]
	if strings.TrimSpace(project.Key) == "" {
		return fmt.Errorf("jira.projects[0].key is required")
	}
	if !project.AutoTransition {
		return fmt.Errorf("jira.projects[0].auto_transition must be true in mvp")
	}
	if strings.TrimSpace(project.Transitions.InProgress) == "" || strings.TrimSpace(project.Transitions.InReview) == "" || strings.TrimSpace(project.Transitions.Done) == "" {
		return fmt.Errorf("jira.projects[0].transitions.in_progress, in_review, and done are required")
	}
	switch strings.TrimSpace(cfg.GitHub.AuthMethod) {
	case "token":
		if strings.TrimSpace(cfg.GitHub.Token) == "" {
			return fmt.Errorf("github.token is required when github.auth_method=token")
		}
	case "oauth_browser":
		if strings.TrimSpace(EffectiveGitHubOAuthBrowserClientID(cfg.GitHub)) == "" {
			return fmt.Errorf("github oauth browser client id is unavailable")
		}
		if strings.TrimSpace(cfg.GitHub.OAuthBrowser.TokenStorePath) == "" {
			return fmt.Errorf("github.oauth_browser.token_store_path is required when github.auth_method=oauth_browser")
		}
		if len(cfg.GitHub.OAuthBrowser.Scopes) == 0 {
			return fmt.Errorf("github.oauth_browser.scopes must contain at least one scope")
		}
	default:
		return fmt.Errorf("github.auth_method must be 'token' or 'oauth_browser'")
	}
	if strings.TrimSpace(cfg.GitHub.Repo) == "" {
		return fmt.Errorf("github.repo is required")
	}
	if strings.TrimSpace(cfg.Repo.Path) == "" {
		return fmt.Errorf("repo.path is required")
	}
	stat, err := os.Stat(cfg.Repo.Path)
	if err != nil {
		return fmt.Errorf("repo.path: %w", err)
	}
	if !stat.IsDir() {
		return fmt.Errorf("repo.path must be a directory")
	}
	if !cfg.Sandbox.Enabled {
		return fmt.Errorf("sandbox.enabled must be true in mvp")
	}
	if strings.TrimSpace(cfg.Sandbox.Image) == "" {
		return fmt.Errorf("sandbox.image is required")
	}
	if strings.TrimSpace(cfg.StateStore.Path) == "" {
		return fmt.Errorf("state_store.path is required")
	}
	if cfg.ConfidenceThreshold < 0 || cfg.ConfidenceThreshold > 1 {
		return fmt.Errorf("confidence_threshold must be between 0 and 1")
	}
	if cfg.Opencode.TimeoutMinutes <= 0 {
		return fmt.Errorf("opencode.timeout_minutes must be greater than 0")
	}
	if cfg.Opencode.MaxConcurrentSessions <= 0 || cfg.Opencode.MaxConcurrentEvaluations <= 0 || cfg.Opencode.MaxRevisionCycles <= 0 {
		return fmt.Errorf("opencode concurrency and revision limits must be greater than 0")
	}
	if cfg.Log.Level == "" {
		return fmt.Errorf("log.level is required")
	}
	if cfg.Log.Format != "pretty" && cfg.Log.Format != "json" {
		return fmt.Errorf("log.format must be 'pretty' or 'json'")
	}
	return nil
}
