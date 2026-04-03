package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"

	"github.com/availhealth/archon/internal/config"
	"github.com/availhealth/archon/internal/githuboauth"
)

func runAuthCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: archon auth <login|status|logout> [github]")
	}
	subject := "github"
	if len(args) > 1 {
		subject = args[1]
	}
	if subject != "github" {
		return fmt.Errorf("only github auth is supported right now")
	}
	switch args[0] {
	case "login":
		return runGitHubAuthLogin()
	case "status":
		return runGitHubAuthStatus()
	case "logout":
		return runGitHubAuthLogout()
	default:
		return fmt.Errorf("unknown auth subcommand %q", strings.Join(args, " "))
	}
}

func runGitHubAuthLogin() error {
	cfg, err := config.LoadUserOnly()
	if err != nil {
		return err
	}
	if cfg.GitHub.AuthMethod != "oauth_browser" {
		return fmt.Errorf("github.auth_method must be oauth_browser to use `archon auth login github`")
	}
	builtInClient := config.EffectiveGitHubOAuthBrowserClientID(cfg.GitHub) == config.BuiltInGitHubOAuthBrowserClientID && strings.TrimSpace(cfg.GitHub.OAuthBrowser.ClientID) == ""
	flow := githuboauth.NewDeviceFlow(cfg)
	store := githuboauth.NewStore(cfg.GitHub.OAuthBrowser.TokenStorePath)

	bold := color.New(color.FgCyan, color.Bold)
	accent := color.New(color.FgGreen)
	warn := color.New(color.FgYellow)

	bold.Println("GitHub Browser Login")
	if builtInClient {
		accent.Println("Using Archon's built-in GitHub OAuth client.")
	}
	setupCtx, setupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	device, opened, err := flow.Start(setupCtx)
	setupCancel()
	if err != nil {
		return err
	}
	if opened {
		accent.Println("Opened your browser for GitHub authorization.")
	} else {
		warn.Println("Could not open a browser automatically.")
	}
	fmt.Printf("Visit: %s\n", device.VerificationURI)
	fmt.Printf("Enter code: %s\n", device.UserCode)
	accent.Println("Waiting for GitHub authorization...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	token, err := flow.WaitForToken(ctx, device)
	if err != nil {
		return err
	}
	if err := store.Save(token); err != nil {
		return err
	}
	accent.Printf("Stored GitHub OAuth token at %s\n", store.Path())
	return nil
}

func runGitHubAuthStatus() error {
	cfg, err := config.LoadUserOnly()
	if err != nil {
		return err
	}
	if cfg.GitHub.AuthMethod != "oauth_browser" {
		fmt.Printf("github auth method: %s\n", cfg.GitHub.AuthMethod)
		return nil
	}
	store := githuboauth.NewStore(cfg.GitHub.OAuthBrowser.TokenStorePath)
	token, err := store.Load()
	if err != nil {
		if err == githuboauth.ErrTokenNotFound {
			fmt.Printf("github auth method: oauth_browser\nstatus: not logged in\nclient: %s\ntoken_store: %s\n", describeGitHubClient(cfg), store.Path())
			return nil
		}
		return err
	}
	fmt.Printf("github auth method: oauth_browser\nstatus: logged in\nclient: %s\ntoken_store: %s\nscopes: %s\ncreated_at: %s\n", describeGitHubClient(cfg), store.Path(), token.Scope, token.CreatedAt.Format(time.RFC3339))
	return nil
}

func runGitHubAuthLogout() error {
	cfg, err := config.LoadUserOnly()
	if err != nil {
		return err
	}
	store := githuboauth.NewStore(cfg.GitHub.OAuthBrowser.TokenStorePath)
	if err := store.Delete(); err != nil {
		return err
	}
	fmt.Printf("Deleted GitHub OAuth token store at %s\n", store.Path())
	return nil
}

func describeGitHubClient(cfg config.Config) string {
	if strings.TrimSpace(cfg.GitHub.OAuthBrowser.ClientID) == "" {
		return "built-in"
	}
	return "custom"
}
