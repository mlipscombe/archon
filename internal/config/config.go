package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const BuiltInGitHubOAuthBrowserClientID = "Ov23liFD6I8n6HmpA4gY"
const GlobalConfigRelativePath = "~/.archon/archon.yaml"
const RepoConfigFileName = "archon.yaml"
const DefaultSandboxImage = "ghcr.io/mlipscombe/archon-opencode-sandbox:latest"

type Config struct {
	Mode                string     `yaml:"mode"`
	ConfidenceThreshold float64    `yaml:"confidence_threshold"`
	UI                  UI         `yaml:"ui"`
	Jira                Jira       `yaml:"jira"`
	GitHub              GitHub     `yaml:"github"`
	Repo                Repo       `yaml:"repo"`
	Opencode            Opencode   `yaml:"opencode"`
	Sandbox             Sandbox    `yaml:"sandbox"`
	StateStore          StateStore `yaml:"state_store"`
	Log                 Log        `yaml:"log"`

	ConfigFile       string `yaml:"-"`
	GlobalConfigFile string `yaml:"-"`
	RepoConfigFile   string `yaml:"-"`
}

type UI struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	Auth UIAuth `yaml:"auth"`
}

type UIAuth struct {
	Enabled  bool   `yaml:"enabled"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type Jira struct {
	BaseURL                       string        `yaml:"base_url"`
	AuthMethod                    string        `yaml:"auth_method"`
	Email                         string        `yaml:"email"`
	APIToken                      string        `yaml:"api_token"`
	OAuthToken                    string        `yaml:"oauth_token"`
	PollIntervalSeconds           int           `yaml:"poll_interval_seconds"`
	DebounceSeconds               int           `yaml:"debounce_seconds"`
	EscalationTimeoutBusinessDays int           `yaml:"escalation_timeout_business_days"`
	Projects                      []JiraProject `yaml:"projects"`
}

type JiraProject struct {
	Key            string      `yaml:"key"`
	WatchFilter    string      `yaml:"watch_filter"`
	AutoTransition bool        `yaml:"auto_transition"`
	Transitions    Transitions `yaml:"transitions"`
}

type Transitions struct {
	InProgress string `yaml:"in_progress"`
	InReview   string `yaml:"in_review"`
	Done       string `yaml:"done"`
}

type GitHub struct {
	AuthMethod       string             `yaml:"auth_method"`
	Token            string             `yaml:"token"`
	Repo             string             `yaml:"repo"`
	BaseBranch       string             `yaml:"base_branch"`
	DraftPRs         bool               `yaml:"draft_prs"`
	Labels           []string           `yaml:"labels"`
	DefaultReviewers []string           `yaml:"default_reviewers"`
	OAuthBrowser     GitHubOAuthBrowser `yaml:"oauth_browser"`
}

type GitHubOAuthBrowser struct {
	ClientID       string   `yaml:"client_id,omitempty"`
	Scopes         []string `yaml:"scopes"`
	TokenStorePath string   `yaml:"token_store_path"`
}

type Repo struct {
	Path            string `yaml:"path"`
	PrimaryLanguage string `yaml:"primary_language"`
}

type Opencode struct {
	BinaryPath               string `yaml:"binary_path"`
	TimeoutMinutes           int    `yaml:"timeout_minutes"`
	MaxConcurrentSessions    int    `yaml:"max_concurrent_sessions"`
	MaxConcurrentEvaluations int    `yaml:"max_concurrent_evaluations"`
	MaxRevisionCycles        int    `yaml:"max_revision_cycles"`
}

type Sandbox struct {
	Enabled     bool   `yaml:"enabled"`
	Image       string `yaml:"image"`
	CPULimit    string `yaml:"cpu_limit"`
	MemoryLimit string `yaml:"memory_limit"`
	Network     string `yaml:"network"`
}

type StateStore struct {
	Path string `yaml:"path"`
}

type Log struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type UserConfig struct {
	UI         UI         `yaml:"ui,omitempty"`
	Jira       UserJira   `yaml:"jira,omitempty"`
	GitHub     UserGitHub `yaml:"github,omitempty"`
	Opencode   Opencode   `yaml:"opencode,omitempty"`
	Sandbox    Sandbox    `yaml:"sandbox,omitempty"`
	StateStore StateStore `yaml:"state_store,omitempty"`
	Log        Log        `yaml:"log,omitempty"`
}

type UserJira struct {
	BaseURL                       string `yaml:"base_url,omitempty"`
	AuthMethod                    string `yaml:"auth_method,omitempty"`
	Email                         string `yaml:"email,omitempty"`
	APIToken                      string `yaml:"api_token,omitempty"`
	OAuthToken                    string `yaml:"oauth_token,omitempty"`
	PollIntervalSeconds           int    `yaml:"poll_interval_seconds,omitempty"`
	DebounceSeconds               int    `yaml:"debounce_seconds,omitempty"`
	EscalationTimeoutBusinessDays int    `yaml:"escalation_timeout_business_days,omitempty"`
}

type UserGitHub struct {
	AuthMethod   string             `yaml:"auth_method,omitempty"`
	Token        string             `yaml:"token,omitempty"`
	OAuthBrowser GitHubOAuthBrowser `yaml:"oauth_browser,omitempty"`
}

type RepoScopedConfig struct {
	Mode   string     `yaml:"mode,omitempty"`
	Jira   RepoJira   `yaml:"jira,omitempty"`
	GitHub RepoGitHub `yaml:"github,omitempty"`
	Repo   Repo       `yaml:"repo,omitempty"`
}

type RepoJira struct {
	Projects []JiraProject `yaml:"projects,omitempty"`
}

type RepoGitHub struct {
	Repo             string   `yaml:"repo,omitempty"`
	BaseBranch       string   `yaml:"base_branch,omitempty"`
	DraftPRs         bool     `yaml:"draft_prs,omitempty"`
	Labels           []string `yaml:"labels,omitempty"`
	DefaultReviewers []string `yaml:"default_reviewers,omitempty"`
}

func Default() Config {
	return Config{
		Mode:                "sandbox",
		ConfidenceThreshold: 0.7,
		UI: UI{
			Host: "0.0.0.0",
			Port: 8080,
			Auth: UIAuth{Username: "archon"},
		},
		Jira: Jira{
			AuthMethod:                    "api_token",
			PollIntervalSeconds:           60,
			DebounceSeconds:               30,
			EscalationTimeoutBusinessDays: 5,
		},
		GitHub: GitHub{
			AuthMethod: "oauth_browser",
			BaseBranch: "main",
			Labels:     []string{"archon-generated"},
			OAuthBrowser: GitHubOAuthBrowser{
				Scopes:         []string{"repo", "read:user", "read:packages", "read:org"},
				TokenStorePath: "~/.archon/github-oauth.json",
			},
		},
		Opencode: Opencode{
			BinaryPath:               "opencode",
			TimeoutMinutes:           30,
			MaxConcurrentSessions:    3,
			MaxConcurrentEvaluations: 2,
			MaxRevisionCycles:        3,
		},
		Sandbox: Sandbox{
			Enabled:     true,
			Image:       DefaultSandboxImage,
			CPULimit:    "4",
			MemoryLimit: "8g",
			Network:     "bridge",
		},
		StateStore: StateStore{Path: "~/.archon/archon.db"},
		Log:        Log{Level: "info", Format: "pretty"},
	}
}

func Load() (Config, error) {
	cfg, err := loadConfig(true)
	if err != nil {
		return Config{}, err
	}
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func LoadUserOnly() (Config, error) {
	return loadConfig(false)
}

func loadConfig(includeRepo bool) (Config, error) {
	cfg := Default()
	explicitGlobalConfig := strings.TrimSpace(os.Getenv("ARCHON_CONFIG")) != ""

	globalConfigPath, err := ResolveConfigPath()
	if err != nil {
		return Config{}, err
	}
	if globalConfigPath != "" {
		if _, err := os.Stat(globalConfigPath); err == nil {
			data, err := os.ReadFile(globalConfigPath)
			if err != nil {
				return Config{}, fmt.Errorf("read config: %w", err)
			}
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				return Config{}, fmt.Errorf("parse config: %w", err)
			}
			cfg.GlobalConfigFile = globalConfigPath
		} else if explicitGlobalConfig || !errors.Is(err, os.ErrNotExist) {
			return Config{}, fmt.Errorf("read config: %w", err)
		}
	}

	if includeRepo {
		repoConfigPath, err := ResolveRepoConfigPath()
		if err != nil {
			return Config{}, err
		}
		if repoConfigPath != "" {
			data, err := os.ReadFile(repoConfigPath)
			if err != nil {
				return Config{}, fmt.Errorf("read repo config: %w", err)
			}
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				return Config{}, fmt.Errorf("parse repo config: %w", err)
			}
			cfg.RepoConfigFile = repoConfigPath
		}
	}

	applyEnvOverrides(&cfg)
	applyDerivedDefaults(&cfg)
	if cfg.RepoConfigFile != "" && strings.TrimSpace(cfg.Repo.Path) == "" {
		cfg.Repo.Path = filepath.Dir(cfg.RepoConfigFile)
	}

	repoBase := ""
	if cfg.RepoConfigFile != "" {
		repoBase = filepath.Dir(cfg.RepoConfigFile)
	}
	globalBase := ""
	if cfg.GlobalConfigFile != "" {
		globalBase = filepath.Dir(cfg.GlobalConfigFile)
	}

	cfg.Repo.Path, err = ExpandPathFromBase(cfg.Repo.Path, repoBase)
	if err != nil {
		return Config{}, fmt.Errorf("expand repo path: %w", err)
	}
	cfg.StateStore.Path, err = ExpandPathFromBase(cfg.StateStore.Path, globalBase)
	if err != nil {
		return Config{}, fmt.Errorf("expand state store path: %w", err)
	}
	cfg.GitHub.OAuthBrowser.TokenStorePath, err = ExpandPathFromBase(cfg.GitHub.OAuthBrowser.TokenStorePath, globalBase)
	if err != nil {
		return Config{}, fmt.Errorf("expand github oauth browser token store path: %w", err)
	}

	if cfg.GlobalConfigFile != "" {
		cfg.GlobalConfigFile, err = ExpandPath(cfg.GlobalConfigFile)
		if err != nil {
			return Config{}, fmt.Errorf("expand global config path: %w", err)
		}
	}
	if cfg.RepoConfigFile != "" {
		cfg.RepoConfigFile, err = ExpandPath(cfg.RepoConfigFile)
		if err != nil {
			return Config{}, fmt.Errorf("expand repo config path: %w", err)
		}
	}
	cfg.ConfigFile = joinConfigSources(cfg.GlobalConfigFile, cfg.RepoConfigFile)

	return cfg, nil
}

func discoverConfigPath() (string, error) {
	if path := strings.TrimSpace(os.Getenv("ARCHON_CONFIG")); path != "" {
		return ExpandPath(path)
	}
	return ExpandPath(GlobalConfigRelativePath)
}

func ResolveConfigPath() (string, error) {
	return discoverConfigPath()
}

func ResolveRepoConfigPath() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	for {
		candidate := filepath.Join(wd, RepoConfigFileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			break
		}
		wd = parent
	}
	return "", nil
}

func applyDerivedDefaults(cfg *Config) {
	if len(cfg.Jira.Projects) == 1 && strings.TrimSpace(cfg.Jira.Projects[0].WatchFilter) == "" && strings.TrimSpace(cfg.Jira.Projects[0].Key) != "" {
		cfg.Jira.Projects[0].WatchFilter = DefaultWatchFilter(cfg.Jira.Projects[0].Key)
	}
	if len(cfg.Jira.Projects) == 1 {
		cfg.Jira.Projects[0].AutoTransition = true
		if strings.TrimSpace(cfg.Jira.Projects[0].Transitions.InProgress) == "" {
			cfg.Jira.Projects[0].Transitions.InProgress = "In Progress"
		}
		if strings.TrimSpace(cfg.Jira.Projects[0].Transitions.InReview) == "" {
			cfg.Jira.Projects[0].Transitions.InReview = "In Review"
		}
		if strings.TrimSpace(cfg.Jira.Projects[0].Transitions.Done) == "" {
			cfg.Jira.Projects[0].Transitions.Done = "Done"
		}
	}
	if len(cfg.GitHub.Labels) == 0 {
		cfg.GitHub.Labels = []string{"archon-generated"}
	}
	if strings.TrimSpace(cfg.Jira.AuthMethod) == "" {
		cfg.Jira.AuthMethod = "api_token"
	}
	if strings.TrimSpace(cfg.GitHub.AuthMethod) == "" {
		cfg.GitHub.AuthMethod = "oauth_browser"
	}
	if len(cfg.GitHub.OAuthBrowser.Scopes) == 0 {
		cfg.GitHub.OAuthBrowser.Scopes = []string{"repo", "read:user", "read:packages", "read:org"}
	}
	if strings.TrimSpace(cfg.GitHub.OAuthBrowser.TokenStorePath) == "" {
		cfg.GitHub.OAuthBrowser.TokenStorePath = "~/.archon/github-oauth.json"
	}
	if strings.TrimSpace(cfg.Sandbox.Image) == "" {
		cfg.Sandbox.Image = DefaultSandboxImage
	}
	if strings.TrimSpace(cfg.GitHub.BaseBranch) == "" {
		cfg.GitHub.BaseBranch = "main"
	}
	if strings.TrimSpace(cfg.Opencode.BinaryPath) == "" {
		cfg.Opencode.BinaryPath = "opencode"
	}
}

func applyEnvOverrides(cfg *Config) {
	applyString := func(env string, target *string) {
		if value := strings.TrimSpace(os.Getenv(env)); value != "" {
			*target = value
		}
	}
	applyInt := func(env string, target *int) {
		if value := strings.TrimSpace(os.Getenv(env)); value != "" {
			if parsed, err := strconv.Atoi(value); err == nil {
				*target = parsed
			}
		}
	}
	applyBool := func(env string, target *bool) {
		if value := strings.TrimSpace(os.Getenv(env)); value != "" {
			if parsed, err := strconv.ParseBool(value); err == nil {
				*target = parsed
			}
		}
	}
	applyFloat := func(env string, target *float64) {
		if value := strings.TrimSpace(os.Getenv(env)); value != "" {
			if parsed, err := strconv.ParseFloat(value, 64); err == nil {
				*target = parsed
			}
		}
	}

	applyString("ARCHON_MODE", &cfg.Mode)
	applyFloat("ARCHON_CONFIDENCE_THRESHOLD", &cfg.ConfidenceThreshold)
	applyString("ARCHON_UI_HOST", &cfg.UI.Host)
	applyInt("ARCHON_UI_PORT", &cfg.UI.Port)
	applyBool("ARCHON_UI_AUTH_ENABLED", &cfg.UI.Auth.Enabled)
	applyString("ARCHON_UI_AUTH_USERNAME", &cfg.UI.Auth.Username)
	applyString("ARCHON_UI_AUTH_PASSWORD", &cfg.UI.Auth.Password)

	applyString("JIRA_BASE_URL", &cfg.Jira.BaseURL)
	applyString("JIRA_AUTH_METHOD", &cfg.Jira.AuthMethod)
	applyString("JIRA_EMAIL", &cfg.Jira.Email)
	applyString("JIRA_API_TOKEN", &cfg.Jira.APIToken)
	applyString("JIRA_OAUTH_TOKEN", &cfg.Jira.OAuthToken)
	applyInt("ARCHON_JIRA_POLL_INTERVAL_SECONDS", &cfg.Jira.PollIntervalSeconds)
	applyInt("ARCHON_JIRA_DEBOUNCE_SECONDS", &cfg.Jira.DebounceSeconds)
	applyInt("ARCHON_JIRA_ESCALATION_TIMEOUT_BUSINESS_DAYS", &cfg.Jira.EscalationTimeoutBusinessDays)

	projectKey := strings.TrimSpace(os.Getenv("JIRA_PROJECT_KEY"))
	if projectKey != "" {
		if len(cfg.Jira.Projects) == 0 {
			cfg.Jira.Projects = []JiraProject{{}}
		}
		cfg.Jira.Projects[0].Key = projectKey
	}
	if len(cfg.Jira.Projects) > 0 {
		applyString("ARCHON_JIRA_PROJECT_WATCH_FILTER", &cfg.Jira.Projects[0].WatchFilter)
		applyBool("ARCHON_JIRA_PROJECT_AUTO_TRANSITION", &cfg.Jira.Projects[0].AutoTransition)
		applyString("ARCHON_JIRA_PROJECT_TRANSITIONS_IN_PROGRESS", &cfg.Jira.Projects[0].Transitions.InProgress)
		applyString("ARCHON_JIRA_PROJECT_TRANSITIONS_IN_REVIEW", &cfg.Jira.Projects[0].Transitions.InReview)
		applyString("ARCHON_JIRA_PROJECT_TRANSITIONS_DONE", &cfg.Jira.Projects[0].Transitions.Done)
	}

	applyString("GITHUB_AUTH_METHOD", &cfg.GitHub.AuthMethod)
	applyString("GITHUB_TOKEN", &cfg.GitHub.Token)
	applyString("ARCHON_GITHUB_OAUTH_BROWSER_CLIENT_ID", &cfg.GitHub.OAuthBrowser.ClientID)
	applyString("ARCHON_GITHUB_OAUTH_BROWSER_TOKEN_STORE_PATH", &cfg.GitHub.OAuthBrowser.TokenStorePath)
	if scopes := strings.TrimSpace(os.Getenv("ARCHON_GITHUB_OAUTH_BROWSER_SCOPES")); scopes != "" {
		cfg.GitHub.OAuthBrowser.Scopes = SplitScopes(scopes)
	}
	applyString("GITHUB_REPO", &cfg.GitHub.Repo)
	applyString("ARCHON_GITHUB_BASE_BRANCH", &cfg.GitHub.BaseBranch)
	applyBool("ARCHON_GITHUB_DRAFT_PRS", &cfg.GitHub.DraftPRs)

	applyString("REPO_PATH", &cfg.Repo.Path)
	applyString("ARCHON_REPO_PRIMARY_LANGUAGE", &cfg.Repo.PrimaryLanguage)

	applyString("ARCHON_OPENCODE_BINARY_PATH", &cfg.Opencode.BinaryPath)
	applyInt("ARCHON_OPENCODE_TIMEOUT_MINUTES", &cfg.Opencode.TimeoutMinutes)
	applyInt("ARCHON_OPENCODE_MAX_CONCURRENT_SESSIONS", &cfg.Opencode.MaxConcurrentSessions)
	applyInt("ARCHON_OPENCODE_MAX_CONCURRENT_EVALUATIONS", &cfg.Opencode.MaxConcurrentEvaluations)
	applyInt("ARCHON_OPENCODE_MAX_REVISION_CYCLES", &cfg.Opencode.MaxRevisionCycles)

	applyBool("ARCHON_SANDBOX_ENABLED", &cfg.Sandbox.Enabled)
	applyString("ARCHON_SANDBOX_IMAGE", &cfg.Sandbox.Image)
	applyString("ARCHON_SANDBOX_CPU_LIMIT", &cfg.Sandbox.CPULimit)
	applyString("ARCHON_SANDBOX_MEMORY_LIMIT", &cfg.Sandbox.MemoryLimit)
	applyString("ARCHON_SANDBOX_NETWORK", &cfg.Sandbox.Network)

	applyString("ARCHON_STATE_STORE_PATH", &cfg.StateStore.Path)
	applyString("ARCHON_LOG_LEVEL", &cfg.Log.Level)
	applyString("ARCHON_LOG_FORMAT", &cfg.Log.Format)
}

func DefaultWatchFilter(projectKey string) string {
	return fmt.Sprintf("project = %s\nAND issuetype in (Story, Task, Bug)\nAND status = \"Ready for Dev\"\nAND assignee is EMPTY", projectKey)
}

func RenderYAML(cfg Config) ([]byte, error) {
	clone := cfg
	clone.ConfigFile = ""
	clone.GlobalConfigFile = ""
	clone.RepoConfigFile = ""
	return yaml.Marshal(clone)
}

func RenderUserConfig(cfg Config) ([]byte, error) {
	user := UserConfig{
		UI: cfg.UI,
		Jira: UserJira{
			BaseURL:                       cfg.Jira.BaseURL,
			AuthMethod:                    cfg.Jira.AuthMethod,
			Email:                         cfg.Jira.Email,
			APIToken:                      cfg.Jira.APIToken,
			OAuthToken:                    cfg.Jira.OAuthToken,
			PollIntervalSeconds:           cfg.Jira.PollIntervalSeconds,
			DebounceSeconds:               cfg.Jira.DebounceSeconds,
			EscalationTimeoutBusinessDays: cfg.Jira.EscalationTimeoutBusinessDays,
		},
		GitHub: UserGitHub{
			AuthMethod:   cfg.GitHub.AuthMethod,
			Token:        cfg.GitHub.Token,
			OAuthBrowser: cfg.GitHub.OAuthBrowser,
		},
		Opencode:   cfg.Opencode,
		Sandbox:    cfg.Sandbox,
		StateStore: cfg.StateStore,
		Log:        cfg.Log,
	}
	return yaml.Marshal(user)
}

func RenderRepoConfig(cfg Config) ([]byte, error) {
	repoCfg := RepoScopedConfig{
		Mode: cfg.Mode,
		Jira: RepoJira{Projects: cfg.Jira.Projects},
		GitHub: RepoGitHub{
			Repo:             cfg.GitHub.Repo,
			BaseBranch:       cfg.GitHub.BaseBranch,
			DraftPRs:         cfg.GitHub.DraftPRs,
			Labels:           cfg.GitHub.Labels,
			DefaultReviewers: cfg.GitHub.DefaultReviewers,
		},
		Repo: Repo{Path: cfg.Repo.Path},
	}
	return yaml.Marshal(repoCfg)
}

func Redacted(cfg Config) Config {
	clone := cfg
	clone.ConfigFile = ""
	if strings.TrimSpace(clone.Jira.APIToken) != "" {
		clone.Jira.APIToken = "***REDACTED***"
	}
	if strings.TrimSpace(clone.Jira.OAuthToken) != "" {
		clone.Jira.OAuthToken = "***REDACTED***"
	}
	if strings.TrimSpace(clone.GitHub.Token) != "" {
		clone.GitHub.Token = "***REDACTED***"
	}
	if strings.TrimSpace(clone.UI.Auth.Password) != "" {
		clone.UI.Auth.Password = "***REDACTED***"
	}
	return clone
}

func EffectiveGitHubOAuthBrowserClientID(cfg GitHub) string {
	if strings.TrimSpace(cfg.OAuthBrowser.ClientID) != "" {
		return strings.TrimSpace(cfg.OAuthBrowser.ClientID)
	}
	return BuiltInGitHubOAuthBrowserClientID
}

func SplitScopes(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t'
	})
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func joinConfigSources(sources ...string) string {
	parts := make([]string, 0, len(sources))
	for _, source := range sources {
		trimmed := strings.TrimSpace(source)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, ", ")
}

func ExpandPath(path string) (string, error) {
	return ExpandPathFromBase(path, "")
}

func ExpandPathFromBase(path string, baseDir string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	if !filepath.IsAbs(path) && strings.TrimSpace(baseDir) != "" {
		path = filepath.Join(baseDir, path)
	}
	return filepath.Abs(path)
}
