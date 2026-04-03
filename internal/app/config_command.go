package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"

	"github.com/availhealth/archon/internal/config"
)

func runConfigCommand(args []string) error {
	if len(args) == 0 {
		return runConfigInit(nil)
	}
	if args[0] == "init" {
		return runConfigInit(args[1:])
	}
	if args[0] == "validate" {
		return runConfigValidate()
	}
	if args[0] == "path" {
		return runConfigPath()
	}
	if args[0] == "print" {
		return runConfigPrint()
	}
	return fmt.Errorf("unknown config subcommand %q", strings.Join(args, " "))
}

func runConfigInit(args []string) error {
	_ = args
	bold := color.New(color.FgCyan, color.Bold)
	accent := color.New(color.FgGreen)
	warn := color.New(color.FgYellow)

	bold.Println("Archon Configuration Wizard")
	accent.Println("This will create a user config in ~/.archon/archon.yaml and a repo-safe config in ./archon.yaml.")

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}
	globalConfigPath, err := config.ExpandPath(config.GlobalConfigRelativePath)
	if err != nil {
		return fmt.Errorf("resolve global config path: %w", err)
	}
	repoConfigPath := filepath.Join(wd, config.RepoConfigFileName)

	cfg := config.Default()
	cfg.Jira.Projects = []config.JiraProject{{
		AutoTransition: true,
		Transitions: config.Transitions{
			InProgress: "In Progress",
			InReview:   "In Review",
			Done:       "Done",
		},
	}}
	mode := cfg.Mode
	uiPort := strconv.Itoa(cfg.UI.Port)
	enableAuth := cfg.UI.Auth.Enabled
	authUsername := cfg.UI.Auth.Username
	jiraBaseURL := cfg.Jira.BaseURL
	jiraEmail := cfg.Jira.Email
	projectKey := ""
	useDefaultWatchFilter := true
	transitionInProgress := cfg.Jira.Projects[0].Transitions.InProgress
	transitionInReview := cfg.Jira.Projects[0].Transitions.InReview
	transitionDone := cfg.Jira.Projects[0].Transitions.Done
	githubAuthMethod := cfg.GitHub.AuthMethod
	githubRepo := cfg.GitHub.Repo
	githubBaseBranch := cfg.GitHub.BaseBranch
	opencodeBinary := cfg.Opencode.BinaryPath
	sandboxImage := cfg.Sandbox.Image
	stateStorePath := filepath.Join(home, ".archon", "archon.db")

	answers := struct {
		Mode                  string
		UIPort                string
		EnableAuth            bool
		AuthUsername          string
		AuthPassword          string
		JiraBaseURL           string
		JiraEmail             string
		JiraAPIToken          string
		ProjectKey            string
		UseDefaultWatchFilter bool
		WatchFilter           string
		TransitionInProgress  string
		TransitionInReview    string
		TransitionDone        string
		GitHubAuthMethod      string
		GitHubToken           string
		GitHubRepo            string
		GitHubBaseBranch      string
		OpencodeBinary        string
		SandboxImage          string
		StateStorePath        string
	}{}

	if err := survey.AskOne(&survey.Select{Message: "Mode:", Options: []string{"sandbox", "approval"}, Default: mode}, &answers.Mode); err != nil {
		return err
	}
	if err := survey.AskOne(&survey.Input{Message: "UI port:", Default: uiPort}, &answers.UIPort, survey.WithValidator(validatePort)); err != nil {
		return err
	}
	if err := survey.AskOne(&survey.Confirm{Message: "Enable basic auth for the UI?", Default: enableAuth}, &answers.EnableAuth); err != nil {
		return err
	}
	if answers.EnableAuth {
		if err := survey.AskOne(&survey.Input{Message: "UI auth username:", Default: authUsername}, &answers.AuthUsername, survey.WithValidator(survey.Required)); err != nil {
			return err
		}
		if err := survey.AskOne(&survey.Password{Message: "UI auth password:"}, &answers.AuthPassword); err != nil {
			return err
		}
	}
	if err := survey.AskOne(&survey.Input{Message: "Jira base URL:", Default: jiraBaseURL}, &answers.JiraBaseURL, survey.WithValidator(survey.Required)); err != nil {
		return err
	}
	if err := survey.AskOne(&survey.Input{Message: "Jira email:", Default: jiraEmail}, &answers.JiraEmail, survey.WithValidator(survey.Required)); err != nil {
		return err
	}
	if err := survey.AskOne(&survey.Password{Message: "Jira API token:"}, &answers.JiraAPIToken, survey.WithValidator(survey.Required)); err != nil {
		return err
	}
	if err := survey.AskOne(&survey.Input{Message: "Jira project key:", Default: projectKey}, &answers.ProjectKey, survey.WithValidator(survey.Required)); err != nil {
		return err
	}
	if err := survey.AskOne(&survey.Confirm{Message: "Use the default Jira watch filter?", Default: useDefaultWatchFilter}, &answers.UseDefaultWatchFilter); err != nil {
		return err
	}
	if !answers.UseDefaultWatchFilter {
		if err := survey.AskOne(&survey.Multiline{Message: "Custom JQL watch filter:"}, &answers.WatchFilter, survey.WithValidator(survey.Required)); err != nil {
			return err
		}
	}
	if err := survey.AskOne(&survey.Input{Message: "Jira transition for implementation start:", Default: transitionInProgress}, &answers.TransitionInProgress, survey.WithValidator(survey.Required)); err != nil {
		return err
	}
	if err := survey.AskOne(&survey.Input{Message: "Jira transition for PR open:", Default: transitionInReview}, &answers.TransitionInReview, survey.WithValidator(survey.Required)); err != nil {
		return err
	}
	if err := survey.AskOne(&survey.Input{Message: "Jira transition for PR merged:", Default: transitionDone}, &answers.TransitionDone, survey.WithValidator(survey.Required)); err != nil {
		return err
	}
	if err := survey.AskOne(&survey.Select{Message: "GitHub auth method:", Options: []string{"token", "oauth_browser"}, Default: githubAuthMethod}, &answers.GitHubAuthMethod); err != nil {
		return err
	}
	if answers.GitHubAuthMethod == "token" {
		if err := survey.AskOne(&survey.Password{Message: "GitHub token:"}, &answers.GitHubToken, survey.WithValidator(survey.Required)); err != nil {
			return err
		}
	}
	if err := survey.AskOne(&survey.Input{Message: "GitHub repo (owner/repo):", Default: githubRepo}, &answers.GitHubRepo, survey.WithValidator(survey.Required)); err != nil {
		return err
	}
	if err := survey.AskOne(&survey.Input{Message: "GitHub base branch:", Default: githubBaseBranch}, &answers.GitHubBaseBranch, survey.WithValidator(survey.Required)); err != nil {
		return err
	}
	if err := survey.AskOne(&survey.Input{Message: "opencode binary path:", Default: opencodeBinary}, &answers.OpencodeBinary, survey.WithValidator(survey.Required)); err != nil {
		return err
	}
	if err := survey.AskOne(&survey.Input{Message: "Sandbox Docker image:", Default: sandboxImage}, &answers.SandboxImage, survey.WithValidator(survey.Required)); err != nil {
		return err
	}
	if err := survey.AskOne(&survey.Input{Message: "State store path:", Default: stateStorePath}, &answers.StateStorePath, survey.WithValidator(survey.Required)); err != nil {
		return err
	}

	port, err := strconv.Atoi(strings.TrimSpace(answers.UIPort))
	if err != nil {
		return fmt.Errorf("parse ui port: %w", err)
	}
	watchFilterValue := strings.TrimSpace(answers.WatchFilter)
	if answers.UseDefaultWatchFilter {
		watchFilterValue = config.DefaultWatchFilter(strings.TrimSpace(answers.ProjectKey))
	}

	cfg.Mode = strings.TrimSpace(answers.Mode)
	cfg.UI.Port = port
	cfg.UI.Auth.Enabled = answers.EnableAuth
	cfg.UI.Auth.Username = strings.TrimSpace(answers.AuthUsername)
	cfg.UI.Auth.Password = answers.AuthPassword
	cfg.Jira.BaseURL = strings.TrimSpace(answers.JiraBaseURL)
	cfg.Jira.AuthMethod = "api_token"
	cfg.Jira.Email = strings.TrimSpace(answers.JiraEmail)
	cfg.Jira.APIToken = answers.JiraAPIToken
	cfg.Jira.OAuthToken = ""
	cfg.Jira.Projects[0].Key = strings.TrimSpace(answers.ProjectKey)
	cfg.Jira.Projects[0].WatchFilter = watchFilterValue
	cfg.Jira.Projects[0].Transitions.InProgress = strings.TrimSpace(answers.TransitionInProgress)
	cfg.Jira.Projects[0].Transitions.InReview = strings.TrimSpace(answers.TransitionInReview)
	cfg.Jira.Projects[0].Transitions.Done = strings.TrimSpace(answers.TransitionDone)
	cfg.GitHub.AuthMethod = strings.TrimSpace(answers.GitHubAuthMethod)
	cfg.GitHub.Token = ""
	if cfg.GitHub.AuthMethod == "token" {
		cfg.GitHub.Token = answers.GitHubToken
	}
	cfg.GitHub.Repo = strings.TrimSpace(answers.GitHubRepo)
	cfg.GitHub.BaseBranch = strings.TrimSpace(answers.GitHubBaseBranch)
	cfg.Repo.Path = "."
	cfg.Opencode.BinaryPath = strings.TrimSpace(answers.OpencodeBinary)
	cfg.Sandbox.Image = strings.TrimSpace(answers.SandboxImage)
	cfg.StateStore.Path = strings.TrimSpace(answers.StateStorePath)

	if err := config.Validate(cfg); err != nil {
		return err
	}

	globalRendered, err := config.RenderUserConfig(cfg)
	if err != nil {
		return fmt.Errorf("render user config yaml: %w", err)
	}
	repoRendered, err := config.RenderRepoConfig(cfg)
	if err != nil {
		return fmt.Errorf("render repo config yaml: %w", err)
	}

	if existing, err := os.Stat(globalConfigPath); err == nil && !existing.IsDir() {
		overwrite := false
		if err := survey.AskOne(&survey.Confirm{Message: fmt.Sprintf("%s already exists. Overwrite it?", globalConfigPath), Default: false}, &overwrite); err != nil {
			return err
		}
		if !overwrite {
			warn.Println("Aborted without writing the user config.")
			return nil
		}
	}
	if existing, err := os.Stat(repoConfigPath); err == nil && !existing.IsDir() {
		overwrite := false
		if err := survey.AskOne(&survey.Confirm{Message: fmt.Sprintf("%s already exists. Overwrite it?", repoConfigPath), Default: false}, &overwrite); err != nil {
			return err
		}
		if !overwrite {
			warn.Println("Aborted without writing the repo config.")
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(globalConfigPath), 0o755); err != nil {
		return fmt.Errorf("create user config directory: %w", err)
	}
	if err := os.WriteFile(globalConfigPath, globalRendered, 0o600); err != nil {
		return fmt.Errorf("write user config file: %w", err)
	}
	if err := os.WriteFile(repoConfigPath, repoRendered, 0o644); err != nil {
		return fmt.Errorf("write repo config file: %w", err)
	}

	accent.Printf("Wrote user config %s\n", globalConfigPath)
	accent.Printf("Wrote repo config %s\n", repoConfigPath)
	bold.Println("Next steps")
	fmt.Println("1. Review ~/.archon/archon.yaml and ./archon.yaml")
	if cfg.GitHub.AuthMethod == "oauth_browser" {
		fmt.Println("2. Run `archon auth login github`")
		fmt.Println("3. Run `archon config validate`")
		fmt.Println("4. Run `archon start`")
	} else {
		fmt.Println("2. Run `archon config validate`")
		fmt.Println("3. Run `archon start`")
	}
	return nil
}

func validatePort(value any) error {
	text := strings.TrimSpace(fmt.Sprint(value))
	port, err := strconv.Atoi(text)
	if err != nil {
		return fmt.Errorf("enter a valid port number")
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	return nil
}

func runConfigPath() error {
	globalPath, err := config.ResolveConfigPath()
	if err != nil {
		return err
	}
	repoPath, err := config.ResolveRepoConfigPath()
	if err != nil {
		return err
	}
	fmt.Printf("user: %s\n", globalPath)
	if strings.TrimSpace(repoPath) == "" {
		fmt.Println("repo: not found")
	} else {
		fmt.Printf("repo: %s\n", repoPath)
	}
	return nil
}

func runConfigPrint() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	rendered, err := config.RenderYAML(config.Redacted(cfg))
	if err != nil {
		return err
	}
	_, err = fmt.Print(string(rendered))
	return err
}
